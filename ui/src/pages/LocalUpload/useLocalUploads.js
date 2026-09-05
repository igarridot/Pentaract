import { useState, useEffect, useCallback, useRef } from 'react'
import API from '../../api'
import { createOperationId } from '../../common/operation_id'
import {
  isActiveUploadStatus, isTerminalTransferStatus, createUploadState, applyUploadProgressUpdate,
} from '../../common/progress'
import { fileNameFromPath, findSkippedEntries, resolveUploadEntries } from '../Files/upload_conflicts'
import { useUploadConflicts } from '../Files/useUploadConflicts'

export function useLocalUploads(addAlert) {
  const [uploadStates, setUploadStates] = useState([])
  const progressCancelsRef = useRef(new Map())
  const mountedRef = useRef(true)
  const conflicts = useUploadConflicts()

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
    releaseTracking(uploadId)
    if (status === 'done') {
      addAlert(`Uploaded "${filename}"`, 'success')
      scheduleRemoval(uploadId, 2000)
    } else if (status === 'error') {
      addAlert(`Upload failed for "${filename}"`, 'error', { persistent: true })
      scheduleRemoval(uploadId, 3000)
    } else if (status === 'skipped') {
      addAlert(`Skipped "${filename}"`, 'info')
      scheduleRemoval(uploadId, 1500)
    } else if (status === 'cancelled') {
      scheduleRemoval(uploadId, 1500)
    }
  }, [addAlert, releaseTracking, scheduleRemoval])

  // Adds the progress card for an upload and follows its SSE stream.
  const trackUpload = useCallback((uploadId, filename) => {
    setUploadStates((prev) => [...prev, createUploadState(uploadId, filename)])
    const cancel = API.files.subscribeProgress(uploadId, (data) => {
      updateUploadState(uploadId, (prev) => applyUploadProgressUpdate(prev, data))
      if (isTerminalTransferStatus(data.status)) {
        handleTerminalState(uploadId, filename, data.status)
      }
    })
    progressCancelsRef.current.set(uploadId, cancel)
  }, [updateUploadState, handleTerminalState])

  // Resolve conflicts for local upload items ([{ local_path, dest_path }]).
  // Returns the items that should be uploaded.
  const resolveLocalConflicts = useCallback(async (storageId, items) => {
    const entries = items.map((item) => ({
      ...item,
      filename: fileNameFromPath(item.local_path),
      targetPath: item.dest_path,
    }))

    const resolved = await resolveUploadEntries(
      entries,
      (targetPath, filename) => conflicts.hasConflict(storageId, targetPath, filename),
      conflicts.askConflictDecision,
    )

    findSkippedEntries(entries, resolved).forEach((entry) => {
      addAlert(`Skipped "${entry.filename}"`, 'info', { persistent: false })
    })
    resolved.forEach((entry) => conflicts.invalidateDir(storageId, entry.targetPath))

    return resolved
  }, [addAlert, conflicts.askConflictDecision, conflicts.hasConflict, conflicts.invalidateDir])

  const launchLocalUpload = useCallback(async (storageId, localPath, destPath, onConflict) => {
    const uploadId = createOperationId()
    const filename = fileNameFromPath(localPath)
    trackUpload(uploadId, filename)

    try {
      await API.files.uploadLocal(storageId, localPath, destPath, uploadId, onConflict)
    } catch (err) {
      updateUploadState(uploadId, (prev) => ({ ...prev, status: 'error' }))
      releaseTracking(uploadId)
      addAlert(`Upload failed: ${err.message}`, 'error', { persistent: true })
      scheduleRemoval(uploadId, 3000)
    }

    return uploadId
  }, [addAlert, trackUpload, updateUploadState, releaseTracking, scheduleRemoval])

  const launchLocalBatch = useCallback(async (storageId, items, onConflict) => {
    try {
      const result = await API.files.uploadLocalBatch(storageId, items, onConflict)
      // Backend returns { uploads: [{ local_path, upload_id }, ...] }
      const uploads = Array.isArray(result?.uploads) ? result.uploads : []

      uploads.forEach((entry, idx) => {
        const uploadId = entry.upload_id || entry
        const filename = fileNameFromPath(items[idx]?.local_path) || `file-${idx}`
        trackUpload(uploadId, filename)
      })

      return uploads.map((e) => e.upload_id || e)
    } catch (err) {
      addAlert(`Batch upload failed: ${err.message}`, 'error', { persistent: true })
      return []
    }
  }, [addAlert, trackUpload])

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
    conflictDialog: conflicts.conflictDialog,
    setConflictDialog: conflicts.setConflictDialog,
    handleConflictDecision: conflicts.handleConflictDecision,
  }
}
