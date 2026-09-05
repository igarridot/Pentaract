import { useState, useEffect, useCallback, useRef } from 'react'
import API from '../../api'
import { createOperationId } from '../../common/operation_id'
import { isActiveUploadStatus, createUploadState, applyUploadProgressUpdate } from '../../common/progress'
import { buildUploadEntries, findSkippedEntries, resolveUploadEntries } from './upload_conflicts'
import { createCompletionRegistry } from '../../common/completion_registry'
import { createBulkOperation, runUploadPipeline } from './operations'
import { useUploadConflicts } from './useUploadConflicts'

export function useUploads(addAlert, storageId, currentPath, loadTree, {
  registerBulkTransfer,
  markBulkTransferTerminal,
  finalizeBulkTransferLaunch,
  setBulkOperation,
  bulkCancelRef,
}) {
  const [uploadStates, setUploadStates] = useState([])
  const uploadStatesRef = useRef([])
  const uploadProgressCancelsRef = useRef(new Map())
  const uploadAbortControllersRef = useRef(new Map())
  const uploadCompletionRegistryRef = useRef(createCompletionRegistry())
  const {
    conflictDialog, setConflictDialog, askConflictDecision, handleConflictDecision,
    hasConflict, updateDirCache: cacheDirListing, invalidateDir,
  } = useUploadConflicts()

  // Called when the tree is loaded, so conflict checks reuse the listing.
  const updateDirCache = useCallback((path, data) => {
    cacheDirListing(storageId, path, data)
  }, [cacheDirListing, storageId])

  useEffect(() => {
    uploadStatesRef.current = uploadStates
  }, [uploadStates])

  const updateUploadState = useCallback((id, updater) => {
    setUploadStates((prev) => prev.map((u) => (u.id === id ? updater(u) : u)))
  }, [])

  const scheduleUploadStateRemoval = useCallback((uploadId, delayMs) => {
    setTimeout(() => {
      setUploadStates((prev) => prev.filter((u) => u.id !== uploadId))
    }, delayMs)
  }, [])

  const releaseUploadTracking = useCallback((uploadId, { abort = false } = {}) => {
    uploadProgressCancelsRef.current.get(uploadId)?.()
    uploadProgressCancelsRef.current.delete(uploadId)
    const controller = uploadAbortControllersRef.current.get(uploadId)
    if (abort) controller?.abort()
    uploadAbortControllersRef.current.delete(uploadId)
  }, [])

  const applyUploadTerminalState = useCallback((uploadId, filename, status) => {
    if (status === 'done') {
      markBulkTransferTerminal('upload', uploadId, 'done')
      releaseUploadTracking(uploadId)
      uploadCompletionRegistryRef.current.settle(uploadId, 'done')
      addAlert('File uploaded', 'success')
      loadTree()
      scheduleUploadStateRemoval(uploadId, 2000)
      return
    }

    if (status === 'error') {
      markBulkTransferTerminal('upload', uploadId, 'error')
      releaseUploadTracking(uploadId)
      uploadCompletionRegistryRef.current.settle(uploadId, 'error')
      loadTree()
      addAlert(`Upload failed unexpectedly for "${filename}". Please try again.`, 'error', { persistent: true })
      scheduleUploadStateRemoval(uploadId, 3000)
      return
    }

    if (status === 'skipped') {
      markBulkTransferTerminal('upload', uploadId, 'skipped')
      releaseUploadTracking(uploadId)
      uploadCompletionRegistryRef.current.settle(uploadId, 'skipped')
      loadTree()
      addAlert(`Skipped upload for "${filename}"`, 'info', { persistent: false })
      scheduleUploadStateRemoval(uploadId, 1500)
    }
  }, [addAlert, loadTree, markBulkTransferTerminal, releaseUploadTracking, scheduleUploadStateRemoval])

  const launchUpload = useCallback((file, targetPath, onConflict = 'keep_both', providedUploadId = createOperationId()) => {
    const filename = file.name
    const uploadId = providedUploadId
    const completionPromise = uploadCompletionRegistryRef.current.waitFor(uploadId)
    setUploadStates((prev) => [...prev, createUploadState(uploadId, filename, file.size)])

    const cancel = API.files.subscribeProgress(uploadId, (data) => {
      updateUploadState(uploadId, (prev) => applyUploadProgressUpdate(prev, data))
      applyUploadTerminalState(uploadId, filename, data.status)
    })
    uploadProgressCancelsRef.current.set(uploadId, cancel)
    const controller = new AbortController()
    uploadAbortControllersRef.current.set(uploadId, controller)

    const requestPromise = API.files.upload(
      storageId,
      targetPath.replace(/\/+$/, ''),
      file,
      uploadId,
      { signal: controller.signal, onConflict },
    ).then(
      () => 'sent',
      (err) => {
        if (err?.name !== 'AbortError') {
          markBulkTransferTerminal('upload', uploadId, 'error')
          updateUploadState(uploadId, (prev) => ({ ...prev, status: 'error' }))
          uploadCompletionRegistryRef.current.settle(uploadId, 'error')
          addAlert(`Upload interrupted: ${err.message}`, 'error', { persistent: true })
        } else {
          markBulkTransferTerminal('upload', uploadId, 'cancelled')
          uploadCompletionRegistryRef.current.settle(uploadId, 'cancelled')
        }
        releaseUploadTracking(uploadId)
        loadTree()
        scheduleUploadStateRemoval(uploadId, 1500)
        return err?.name === 'AbortError' ? 'cancelled' : 'error'
      },
    )

    return {
      uploadId,
      requestPromise,
      completionPromise,
    }
  }, [addAlert, applyUploadTerminalState, loadTree, markBulkTransferTerminal, releaseUploadTracking, scheduleUploadStateRemoval, storageId, updateUploadState])

  const hasUploadConflict = useCallback((targetPath, filename) => (
    hasConflict(storageId, targetPath, filename)
  ), [hasConflict, storageId])

  const runUploadBatch = useCallback(async (entries) => {
    const showBulkProgress = entries.length > 1
    const bulkCancelledRef = { current: false }
    let defaultConflictMode = 'keep_both'
    // A "skip all" answer also becomes the server-side policy for the batch.
    const askAndRememberDecision = async (filename, targetPath) => {
      const decision = await askConflictDecision(filename, targetPath)
      if (decision.applyForAll && decision.action === 'skip') {
        defaultConflictMode = 'skip'
      }
      return decision
    }

    const entriesToUpload = await resolveUploadEntries(entries, hasUploadConflict, askAndRememberDecision)

    if (showBulkProgress) {
      setBulkOperation(createBulkOperation('upload', entriesToUpload.length))
      bulkCancelRef.current = async () => {
        bulkCancelledRef.current = true
        const currentIds = uploadStatesRef.current
          .filter((u) => isActiveUploadStatus(u.status))
          .map((u) => u.id)
        await Promise.all(currentIds.map(async (uploadId) => {
          releaseUploadTracking(uploadId, { abort: true })
          uploadCompletionRegistryRef.current.settle(uploadId, 'cancelled')
          await API.files.cancelUpload(uploadId).catch(() => {})
          markBulkTransferTerminal('upload', uploadId, 'cancelled')
        }))
      }
    }

    findSkippedEntries(entries, entriesToUpload).forEach((entry) => {
      addAlert(`Skipped upload for "${entry.filename}"`, 'info', { persistent: false })
    })

    try {
      await runUploadPipeline(entriesToUpload, async (entry) => {
        if (bulkCancelledRef.current) return null

        const uploadId = createOperationId()
        if (showBulkProgress) registerBulkTransfer('upload', uploadId)

        const transfer = launchUpload(entry.file, entry.targetPath, defaultConflictMode, uploadId)
        invalidateDir(storageId, entry.targetPath)
        return transfer
      })

      if (showBulkProgress) {
        finalizeBulkTransferLaunch('upload', bulkCancelledRef.current)
      }
    } catch (err) {
      if (showBulkProgress) {
        setBulkOperation((prev) => (prev ? { ...prev, status: 'error' } : prev))
      }
      throw err
    } finally {
      loadTree()
      if (showBulkProgress) bulkCancelRef.current = null
    }
  }, [addAlert, askConflictDecision, bulkCancelRef, finalizeBulkTransferLaunch, hasUploadConflict, invalidateDir, launchUpload, loadTree, markBulkTransferTerminal, registerBulkTransfer, releaseUploadTracking, setBulkOperation, storageId])

  const startUpload = async (e) => {
    const files = Array.from(e.target.files || [])
    if (files.length === 0) return
    await runUploadBatch(buildUploadEntries(files, currentPath))
    e.target.value = ''
  }

  const cancelUpload = async (uploadId) => {
    if (!uploadId) return
    try {
      releaseUploadTracking(uploadId, { abort: true })
      uploadCompletionRegistryRef.current.settle(uploadId, 'cancelled')
      await API.files.cancelUpload(uploadId)
      markBulkTransferTerminal('upload', uploadId, 'cancelled')
      addAlert('Upload cancelled', 'info')
    } catch (err) {
      addAlert(err.message, 'error')
    }
    setUploadStates((prev) => prev.filter((u) => u.id !== uploadId))
    loadTree()
  }

  const cleanupUploads = useCallback(() => {
    uploadProgressCancelsRef.current.forEach((_, uploadId) => {
      API.files.cancelUpload(uploadId).catch(() => {})
    })
    uploadProgressCancelsRef.current.forEach((cancel) => cancel())
    uploadProgressCancelsRef.current.clear()
    uploadAbortControllersRef.current.forEach((controller) => controller.abort())
    uploadAbortControllersRef.current.clear()
    uploadCompletionRegistryRef.current.clear()
  }, [])

  const isUploading = uploadStates.some((u) => isActiveUploadStatus(u.status))

  return {
    uploadStates,
    isUploading,
    startUpload,
    cancelUpload,
    cleanupUploads,
    uploadConflictDialog: conflictDialog,
    setUploadConflictDialog: setConflictDialog,
    handleUploadConflictDecision: handleConflictDecision,
    updateDirCache,
  }
}
