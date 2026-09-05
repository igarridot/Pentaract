import { useState, useCallback, useRef } from 'react'
import API from '../../api'
import { fileNamesOf, normalizeUploadPath } from './upload_conflicts'

function cacheKey(storageId, path) {
  return `${storageId}::${normalizeUploadPath(path)}`
}

// Shared by browser and local uploads: the "file already exists" dialog plus
// a per-directory cache of remote file names used to detect conflicts.
export function useUploadConflicts() {
  const [conflictDialog, setConflictDialog] = useState({
    open: false,
    filename: '',
    targetPath: '',
    applyForAll: false,
  })
  const resolverRef = useRef(null)
  const dirFileNamesCacheRef = useRef(new Map())

  const updateDirCache = useCallback((storageId, path, items) => {
    dirFileNamesCacheRef.current.set(cacheKey(storageId, path), fileNamesOf(items))
  }, [])

  const invalidateDir = useCallback((storageId, path) => {
    dirFileNamesCacheRef.current.delete(cacheKey(storageId, path))
  }, [])

  const getDirFileNames = useCallback(async (storageId, path) => {
    const key = cacheKey(storageId, path)
    if (dirFileNamesCacheRef.current.has(key)) {
      return dirFileNamesCacheRef.current.get(key)
    }
    const fileNames = fileNamesOf(await API.files.tree(storageId, normalizeUploadPath(path)))
    dirFileNamesCacheRef.current.set(key, fileNames)
    return fileNames
  }, [])

  const hasConflict = useCallback(async (storageId, targetPath, filename) => (
    (await getDirFileNames(storageId, targetPath)).has(filename)
  ), [getDirFileNames])

  const askConflictDecision = useCallback((filename, targetPath) => (
    new Promise((resolve) => {
      resolverRef.current = resolve
      setConflictDialog({
        open: true,
        filename,
        targetPath: normalizeUploadPath(targetPath),
        applyForAll: false,
      })
    })
  ), [])

  const handleConflictDecision = useCallback((action, applyForAll) => {
    const resolve = resolverRef.current
    resolverRef.current = null
    setConflictDialog((prev) => ({ ...prev, open: false, applyForAll: false }))
    if (resolve) {
      resolve({ action, applyForAll })
    }
  }, [])

  return {
    conflictDialog,
    setConflictDialog,
    askConflictDecision,
    handleConflictDecision,
    hasConflict,
    updateDirCache,
    invalidateDir,
  }
}
