import { useState, useEffect, useCallback, useRef } from 'react'
import API from '../../api'
import { createOperationId } from '../../common/operation_id'
import { isActiveUploadStatus, isTerminalTransferStatus } from '../../common/progress'
import { normalizeUploadPath, resolveUploadEntries } from '../Files/upload_conflicts'

export function useLocalUploads(addAlert) {
  const [uploadStates, setUploadStates] = useState([])
  const progressCancelsRef = useRef(new Map())
  const mountedRef = useRef(true)

  // Conflict dialog state
  const [conflictDialog, setConflictDialog] = useState({
    open: false, filename: '', targetPath: '', applyForAll: false,
  })
  const conflictResolverRef = useRef(null)

  // Cache for remote directory listings (targetPath → Set of filenames)
  const dirFileNamesCacheRef = useRef(new Map())

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      progressCancelsRef.current.forEach((cancel) => cancel())
      progressCancelsRef.current.clear()
    }
  }, [])

  const updateUploadState = useCallback((id, updater) => {
    if (!mountedRef.current) return
    setUploadStates((prev) => prev.map((u) => (u.id === id ? updater(u) : u)))
  }, [])

  const scheduleRemoval = useCallback((uploadId, delayMs) => {
    setTimeout(() => {
      if (!mountedRef.current) return
      setUploadStates((prev) => prev.filter((u) => u.id !== uploadId))
    }, delayMs)
  }, [])

  const releaseTracking = useCallback((uploadId) => {
    const cancel = progressCancelsRef.current.get(uploadId)
    if (cancel) cancel()
    progressCancelsRef.current.delete(uploadId)
  }, [])

  const handleTerminalState = useCallback((uploadId, filename, status) => {
    if (status === 'done') {
      releaseTracking(uploadId)
      addAlert(`Uploaded "${filename}"`, 'success')
      scheduleRemoval(uploadId, 2000)
    } else if (status === 'error') {
      releaseTracking(uploadId)
      addAlert(`Upload failed for "${filename}"`, 'error', { persistent: true })
      scheduleRemoval(uploadId, 3000)
    } else if (status === 'skipped') {
      releaseTracking(uploadId)
      addAlert(`Skipped "${filename}"`, 'info')
      scheduleRemoval(uploadId, 1500)
    } else if (status === 'cancelled') {
      releaseTracking(uploadId)
      scheduleRemoval(uploadId, 1500)
    }
  }, [addAlert, releaseTracking, scheduleRemoval])

  const subscribeToUpload = useCallback((uploadId, filename) => {
    const cancel = API.files.subscribeProgress(uploadId, (data) => {
      updateUploadState(uploadId, (prev) => ({
        ...prev,
        totalBytes: data.total_bytes ?? prev.totalBytes ?? 0,
        uploadedBytes: data.uploaded_bytes ?? 0,
        totalChunks: data.total ?? prev.totalChunks ?? 0,
        uploadedChunks: data.uploaded ?? 0,
        verificationTotal: data.verification_total ?? prev.verificationTotal ?? 0,
        verifiedChunks: data.verified ?? 0,
        status: data.status,
        workersStatus: data.workers_status ?? prev.workersStatus ?? 'active',
      }))
      if (isTerminalTransferStatus(data.status)) {
        handleTerminalState(uploadId, filename, data.status)
      }
    })
    progressCancelsRef.current.set(uploadId, cancel)
  }, [updateUploadState, handleTerminalState])

  // -- Conflict resolution (same pattern as useUploads) --

  const getDirFileNames = useCallback(async (storageId, targetPath) => {
    const normalizedPath = normalizeUploadPath(targetPath)
    const cacheKey = `${storageId}::${normalizedPath}`
    if (dirFileNamesCacheRef.current.has(cacheKey)) {
      return dirFileNamesCacheRef.current.get(cacheKey)
    }
    const data = await API.files.tree(storageId, normalizedPath)
    const fileNames = new Set((data || []).filter((item) => item.is_file).map((item) => item.name))
    dirFileNamesCacheRef.current.set(cacheKey, fileNames)
    return fileNames
  }, [])

  const hasConflict = useCallback(async (storageId, targetPath, filename) => {
    const fileNames = await getDirFileNames(storageId, targetPath)
    return fileNames.has(filename)
  }, [getDirFileNames])

  const askConflictDecision = useCallback((filename, targetPath) => (
    new Promise((resolve) => {
      conflictResolverRef.current = resolve
      setConflictDialog({
        open: true,
        filename,
        targetPath: normalizeUploadPath(targetPath),
        applyForAll: false,
      })
    })
  ), [])

  const handleConflictDecision = useCallback((action, applyForAll) => {
    const resolve = conflictResolverRef.current
    conflictResolverRef.current = null
    setConflictDialog((prev) => ({ ...prev, open: false, applyForAll: false }))
    if (resolve) {
      resolve({ action, applyForAll })
    }
  }, [])

  // Resolve conflicts for local upload items.
  // items: [{ local_path, dest_path }]
  // Returns items that should be uploaded, with onConflict set.
  const resolveLocalConflicts = useCallback(async (storageId, items) => {
    // Build entries compatible with resolveUploadEntries
    const entries = items.map((item) => {
      const filename = item.local_path.split('/').pop() || item.local_path
      return { ...item, filename, targetPath: item.dest_path }
    })

    const resolved = await resolveUploadEntries(
      entries,
      (targetPath, filename) => hasConflict(storageId, targetPath, filename),
      askConflictDecision,
    )

    // Report skipped files
    const resolvedKeys = new Set(resolved.map((e) => `${e.targetPath}::${e.filename}`))
    entries.forEach((entry) => {
      const key = `${entry.targetPath}::${entry.filename}`
      if (!resolvedKeys.has(key)) {
        addAlert(`Skipped "${entry.filename}"`, 'info', { persistent: false })
      }
    })

    // Invalidate cache for affected directories
    resolved.forEach((entry) => {
      const normalizedPath = normalizeUploadPath(entry.targetPath)
      dirFileNamesCacheRef.current.delete(`${storageId}::${normalizedPath}`)
    })

    return resolved
  }, [addAlert, askConflictDecision, hasConflict])

  const launchLocalUpload = useCallback(async (storageId, localPath, destPath, onConflict) => {
    const uploadId = createOperationId()
    const filename = localPath.split('/').pop() || localPath

    setUploadStates((prev) => [...prev, {
      id: uploadId,
      filename,
      totalBytes: 0,
      uploadedBytes: 0,
      totalChunks: 0,
      uploadedChunks: 0,
      verificationTotal: 0,
      verifiedChunks: 0,
      status: 'uploading',
      workersStatus: 'active',
    }])

    subscribeToUpload(uploadId, filename)

    try {
      await API.files.uploadLocal(storageId, localPath, destPath, uploadId, onConflict)
    } catch (err) {
      updateUploadState(uploadId, (prev) => ({ ...prev, status: 'error' }))
      releaseTracking(uploadId)
      addAlert(`Upload failed: ${err.message}`, 'error', { persistent: true })
      scheduleRemoval(uploadId, 3000)
    }

    return uploadId
  }, [addAlert, subscribeToUpload, updateUploadState, releaseTracking, scheduleRemoval])

  const launchLocalBatch = useCallback(async (storageId, items, onConflict) => {
    try {
      const result = await API.files.uploadLocalBatch(storageId, items, onConflict)
      // Backend returns { uploads: [{ local_path, upload_id }, ...] }
      const uploads = result?.uploads || []

      if (Array.isArray(uploads)) {
        uploads.forEach((entry, idx) => {
          const uploadId = entry.upload_id || entry
          const item = items[idx] || {}
          const filename = (item.local_path || '').split('/').pop() || `file-${idx}`

          setUploadStates((prev) => [...prev, {
            id: uploadId,
            filename,
            totalBytes: 0,
            uploadedBytes: 0,
            totalChunks: 0,
            uploadedChunks: 0,
            verificationTotal: 0,
            verifiedChunks: 0,
            status: 'uploading',
            workersStatus: 'active',
          }])

          subscribeToUpload(uploadId, filename)
        })
      }

      return uploads.map((e) => e.upload_id || e)
    } catch (err) {
      addAlert(`Batch upload failed: ${err.message}`, 'error', { persistent: true })
      return []
    }
  }, [addAlert, subscribeToUpload])

  const cancelUpload = useCallback(async (uploadId) => {
    if (!uploadId) return
    try {
      releaseTracking(uploadId)
      await API.files.cancelUpload(uploadId)
      addAlert('Upload cancelled', 'info')
    } catch (err) {
      addAlert(err.message, 'error')
    }
    setUploadStates((prev) => prev.filter((u) => u.id !== uploadId))
  }, [addAlert, releaseTracking])

  const isUploading = uploadStates.some((u) => isActiveUploadStatus(u.status))

  return {
    uploadStates,
    isUploading,
    launchLocalUpload,
    launchLocalBatch,
    cancelUpload,
    resolveLocalConflicts,
    conflictDialog,
    setConflictDialog,
    handleConflictDecision,
  }
}
