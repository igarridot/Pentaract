import { useCallback, useEffect, useRef, useState } from 'react'
import API from '../api/index.js'
import { createOperationId } from './operation_id.js'
import {
  isActiveUploadStatus,
  isTerminalTransferStatus,
  summarizeTerminalStatuses,
  resolveBulkTransferStatus,
} from './progress.js'
import {
  applyUploadProgressState,
  createUploadState,
  getSkippedUploadEntries,
} from './upload_manager_helpers.js'
import { normalizeUploadPath, resolveUploadEntries } from '../pages/Files/upload_conflicts.js'
import { createUploadCompletionRegistry } from '../pages/Files/upload_completion.js'
import { createBulkOperation, getBulkOperationMetrics, runUploadPipeline } from '../pages/Files/operations.js'

export function useUploadManager({
  storageId,
  addAlert,
  createRequest,
  successMessage = () => 'File uploaded',
  skippedMessage = (entry) => `Skipped upload for "${entry.filename}"`,
  errorMessage = (entry) => `Upload failed unexpectedly for "${entry.filename}". Please try again.`,
  interruptedMessage = (_, err) => `Upload interrupted: ${err.message}`,
  cancelledMessage = () => 'Upload cancelled',
  successAlertOptions,
  skippedAlertOptions,
  errorAlertOptions = { persistent: true },
  interruptedAlertOptions = { persistent: true },
  cancelledAlertOptions,
  allSkippedMessage = 'All selected files were skipped',
  pipelineRunner = runUploadPipeline,
  onTerminalSettled,
  onRequestSettled,
  onCancelSettled,
  onBatchSettled,
}) {
  const [uploadStates, setUploadStates] = useState([])
  const [bulkOperation, setBulkOperation] = useState(null)
  const [uploadConflictDialog, setUploadConflictDialog] = useState({
    open: false,
    filename: '',
    targetPath: '',
    applyForAll: false,
  })
  const uploadStatesRef = useRef([])
  const uploadProgressCancelsRef = useRef(new Map())
  const uploadAbortControllersRef = useRef(new Map())
  const uploadCompletionRegistryRef = useRef(createUploadCompletionRegistry())
  const uploadConflictResolverRef = useRef(null)
  const dirFileNamesCacheRef = useRef(new Map())
  const bulkCancelRef = useRef(null)

  useEffect(() => {
    uploadStatesRef.current = uploadStates
  }, [uploadStates])

  useEffect(() => {
    return () => {
      bulkCancelRef.current?.()
      uploadProgressCancelsRef.current.forEach((_, uploadId) => {
        API.files.cancelUpload(uploadId).catch(() => {})
      })
      uploadProgressCancelsRef.current.forEach((cancel) => cancel())
      uploadProgressCancelsRef.current.clear()
      uploadAbortControllersRef.current.forEach((controller) => controller.abort())
      uploadAbortControllersRef.current.clear()
      uploadCompletionRegistryRef.current.clear()
    }
  }, [])

  const isUploading = uploadStates.some((upload) => isActiveUploadStatus(upload.status))
  const isBulkUpload = bulkOperation?.status === 'running'

  const updateUploadState = useCallback((id, updater) => {
    setUploadStates((prev) => prev.map((upload) => (upload.id === id ? updater(upload) : upload)))
  }, [])

  const scheduleUploadStateRemoval = useCallback((uploadId, delayMs) => {
    setTimeout(() => {
      setUploadStates((prev) => prev.filter((upload) => upload.id !== uploadId))
    }, delayMs)
  }, [])

  const releaseUploadTracking = useCallback((uploadId, { abort = false } = {}) => {
    uploadProgressCancelsRef.current.get(uploadId)?.()
    uploadProgressCancelsRef.current.delete(uploadId)
    const controller = uploadAbortControllersRef.current.get(uploadId)
    if (abort) controller?.abort()
    uploadAbortControllersRef.current.delete(uploadId)
  }, [])

  const registerBulkTransfer = useCallback((transferId) => {
    setBulkOperation((prev) => {
      if (!prev || prev.operation !== 'upload') return prev
      if ((prev.transferIds || []).includes(transferId)) return prev
      return { ...prev, transferIds: [...(prev.transferIds || []), transferId] }
    })
  }, [])

  const markBulkTransferTerminal = useCallback((transferId, terminalStatus) => {
    if (!isTerminalTransferStatus(terminalStatus)) return

    setBulkOperation((prev) => {
      if (!prev || prev.operation !== 'upload') return prev
      if (!(prev.transferIds || []).includes(transferId)) return prev
      const currentStatus = prev.terminalStatuses?.[transferId]
      if (currentStatus === terminalStatus) return prev

      const terminalStatuses = { ...(prev.terminalStatuses || {}), [transferId]: terminalStatus }
      const summary = summarizeTerminalStatuses(terminalStatuses)
      let nextStatus = prev.status

      if (prev.status === 'running' && prev.startedAll && summary.completed >= prev.total) {
        nextStatus = resolveBulkTransferStatus(summary)
      }

      return {
        ...prev,
        terminalStatuses,
        completed: summary.completed,
        status: nextStatus,
      }
    })
  }, [])

  const finalizeBulkTransferLaunch = useCallback((wasCancelled) => {
    setBulkOperation((prev) => {
      if (!prev || prev.operation !== 'upload') return prev
      const summary = summarizeTerminalStatuses(prev.terminalStatuses || {})
      let nextStatus = prev.status

      if (wasCancelled) {
        nextStatus = 'cancelled'
      } else if (summary.completed >= prev.total) {
        nextStatus = resolveBulkTransferStatus(summary)
      }

      return {
        ...prev,
        startedAll: true,
        completed: summary.completed,
        status: nextStatus,
      }
    })
  }, [])

  useEffect(() => {
    if (!bulkOperation || bulkOperation.operation !== 'upload') return
    if (bulkOperation.status === 'running') return

    const timeoutMs = bulkOperation.status === 'error' ? 3000 : 1500
    const timeout = setTimeout(() => setBulkOperation(null), timeoutMs)
    return () => clearTimeout(timeout)
  }, [bulkOperation])

  const notify = useCallback((message, severity, options) => {
    if (!message) return
    addAlert(message, severity, options)
  }, [addAlert])

  const applyUploadTerminalState = useCallback((uploadId, entry, status) => {
    if (status === 'done') {
      markBulkTransferTerminal(uploadId, 'done')
      releaseUploadTracking(uploadId)
      uploadCompletionRegistryRef.current.settle(uploadId, 'done')
      onTerminalSettled?.(status, entry, uploadId)
      notify(successMessage(entry), 'success', successAlertOptions)
      scheduleUploadStateRemoval(uploadId, 2000)
      return
    }

    if (status === 'error') {
      markBulkTransferTerminal(uploadId, 'error')
      releaseUploadTracking(uploadId)
      uploadCompletionRegistryRef.current.settle(uploadId, 'error')
      onTerminalSettled?.(status, entry, uploadId)
      notify(errorMessage(entry), 'error', errorAlertOptions)
      scheduleUploadStateRemoval(uploadId, 3000)
      return
    }

    if (status === 'skipped') {
      markBulkTransferTerminal(uploadId, 'skipped')
      releaseUploadTracking(uploadId)
      uploadCompletionRegistryRef.current.settle(uploadId, 'skipped')
      onTerminalSettled?.(status, entry, uploadId)
      notify(skippedMessage(entry), 'info', skippedAlertOptions)
      scheduleUploadStateRemoval(uploadId, 1500)
    }
  }, [
    errorAlertOptions,
    errorMessage,
    markBulkTransferTerminal,
    notify,
    onTerminalSettled,
    releaseUploadTracking,
    scheduleUploadStateRemoval,
    skippedAlertOptions,
    skippedMessage,
    successAlertOptions,
    successMessage,
  ])

  const launchUpload = useCallback((entry, onConflict = 'keep_both', providedUploadId = createOperationId()) => {
    const uploadId = providedUploadId
    const completionPromise = uploadCompletionRegistryRef.current.waitFor(uploadId)
    const terminalPromise = completionPromise.then((status) => status)
    setUploadStates((prev) => [...prev, createUploadState(entry, uploadId)])

    const cancel = API.files.subscribeProgress(uploadId, (data) => {
      updateUploadState(uploadId, (prev) => applyUploadProgressState(prev, entry, data))
      applyUploadTerminalState(uploadId, entry, data.status)
    })
    uploadProgressCancelsRef.current.set(uploadId, cancel)

    const controller = new AbortController()
    uploadAbortControllersRef.current.set(uploadId, controller)

    const requestPromise = createRequest(entry, uploadId, controller.signal, onConflict).then(
      () => 'sent',
      (err) => {
        if (err?.name !== 'AbortError') {
          markBulkTransferTerminal(uploadId, 'error')
          updateUploadState(uploadId, (prev) => ({ ...prev, status: 'error' }))
          uploadCompletionRegistryRef.current.settle(uploadId, 'error')
          notify(interruptedMessage(entry, err), 'error', interruptedAlertOptions)
        } else {
          markBulkTransferTerminal(uploadId, 'cancelled')
          uploadCompletionRegistryRef.current.settle(uploadId, 'cancelled')
        }
        releaseUploadTracking(uploadId)
        onRequestSettled?.(entry, uploadId, err)
        scheduleUploadStateRemoval(uploadId, 1500)
        return err?.name === 'AbortError' ? 'cancelled' : 'error'
      },
    )

    return {
      uploadId,
      requestPromise,
      completionPromise: terminalPromise,
    }
  }, [
    applyUploadTerminalState,
    createRequest,
    interruptedAlertOptions,
    interruptedMessage,
    markBulkTransferTerminal,
    notify,
    onRequestSettled,
    releaseUploadTracking,
    scheduleUploadStateRemoval,
    updateUploadState,
  ])

  const askUploadConflictDecision = useCallback((filename, targetPath) => (
    new Promise((resolve) => {
      uploadConflictResolverRef.current = resolve
      setUploadConflictDialog({
        open: true,
        filename,
        targetPath: normalizeUploadPath(targetPath),
        applyForAll: false,
      })
    })
  ), [])

  const handleUploadConflictDecision = useCallback((action, applyForAll) => {
    const resolve = uploadConflictResolverRef.current
    uploadConflictResolverRef.current = null
    setUploadConflictDialog((prev) => ({ ...prev, open: false, applyForAll: false }))
    if (resolve) {
      resolve({ action, applyForAll })
    }
  }, [])

  const primeConflictCache = useCallback((targetPath, fileNames) => {
    dirFileNamesCacheRef.current.set(normalizeUploadPath(targetPath), new Set(fileNames))
  }, [])

  const clearConflictCache = useCallback((targetPath) => {
    if (targetPath === undefined) {
      dirFileNamesCacheRef.current.clear()
      return
    }
    dirFileNamesCacheRef.current.delete(normalizeUploadPath(targetPath))
  }, [])

  const getDirFileNames = useCallback(async (targetPath) => {
    const normalizedPath = normalizeUploadPath(targetPath)
    if (dirFileNamesCacheRef.current.has(normalizedPath)) {
      return dirFileNamesCacheRef.current.get(normalizedPath)
    }
    const data = await API.files.tree(storageId, normalizedPath)
    const fileNames = new Set((data || []).filter((item) => item.is_file).map((item) => item.name))
    dirFileNamesCacheRef.current.set(normalizedPath, fileNames)
    return fileNames
  }, [storageId])

  const hasUploadConflict = useCallback(async (targetPath, filename) => {
    const fileNames = await getDirFileNames(targetPath)
    return fileNames.has(filename)
  }, [getDirFileNames])

  const runUploadBatch = useCallback(async (entries) => {
    const showBulkProgress = entries.length > 1
    const bulkCancelledRef = { current: false }
    let defaultConflictMode = 'keep_both'
    const askConflictDecision = async (filename, targetPath) => {
      const decision = await askUploadConflictDecision(filename, targetPath)
      if (decision.applyForAll && decision.action === 'skip') {
        defaultConflictMode = 'skip'
      }
      return decision
    }

    const entriesToUpload = await resolveUploadEntries(entries, hasUploadConflict, askConflictDecision)
    if (entriesToUpload.length === 0) {
      notify(allSkippedMessage, 'info')
      return
    }

    if (showBulkProgress) {
      setBulkOperation(createBulkOperation('upload', entriesToUpload.length))
      bulkCancelRef.current = async () => {
        bulkCancelledRef.current = true
        const currentIds = uploadStatesRef.current
          .filter((upload) => isActiveUploadStatus(upload.status))
          .map((upload) => upload.id)
        await Promise.all(currentIds.map(async (uploadId) => {
          releaseUploadTracking(uploadId, { abort: true })
          uploadCompletionRegistryRef.current.settle(uploadId, 'cancelled')
          await API.files.cancelUpload(uploadId).catch(() => {})
          markBulkTransferTerminal(uploadId, 'cancelled')
        }))
      }
    }

    getSkippedUploadEntries(entries, entriesToUpload).forEach((entry) => {
      notify(skippedMessage(entry), 'info', skippedAlertOptions)
    })

    try {
      await pipelineRunner(entriesToUpload, async (entry) => {
        if (bulkCancelledRef.current) return null

        const uploadId = createOperationId()
        if (showBulkProgress) registerBulkTransfer(uploadId)

        const transfer = launchUpload(entry, defaultConflictMode, uploadId)
        clearConflictCache(entry.targetPath)
        return transfer
      })

      if (showBulkProgress) {
        finalizeBulkTransferLaunch(bulkCancelledRef.current)
      }
    } catch {
      if (showBulkProgress) {
        setBulkOperation((prev) => (prev ? { ...prev, status: 'error' } : prev))
      }
    } finally {
      onBatchSettled?.(entriesToUpload)
      if (showBulkProgress) bulkCancelRef.current = null
    }
  }, [
    allSkippedMessage,
    askUploadConflictDecision,
    clearConflictCache,
    finalizeBulkTransferLaunch,
    hasUploadConflict,
    launchUpload,
    markBulkTransferTerminal,
    notify,
    onBatchSettled,
    pipelineRunner,
    registerBulkTransfer,
    releaseUploadTracking,
    skippedAlertOptions,
    skippedMessage,
  ])

  const cancelUpload = useCallback(async (uploadId) => {
    if (!uploadId) return
    try {
      releaseUploadTracking(uploadId, { abort: true })
      uploadCompletionRegistryRef.current.settle(uploadId, 'cancelled')
      await API.files.cancelUpload(uploadId)
      markBulkTransferTerminal(uploadId, 'cancelled')
      notify(cancelledMessage(), 'info', cancelledAlertOptions)
    } catch (err) {
      addAlert(err.message, 'error')
    }
    setUploadStates((prev) => prev.filter((upload) => upload.id !== uploadId))
    onCancelSettled?.(uploadId)
  }, [
    addAlert,
    cancelledAlertOptions,
    cancelledMessage,
    markBulkTransferTerminal,
    notify,
    onCancelSettled,
    releaseUploadTracking,
  ])

  return {
    uploadStates,
    bulkOperation,
    bulkMetrics: getBulkOperationMetrics(bulkOperation, uploadStates, []),
    isUploading,
    isBulkUpload,
    runUploadBatch,
    cancelUpload,
    cancelBulkUpload: () => bulkCancelRef.current?.(),
    uploadConflictDialog,
    handleUploadConflictDecision,
    primeConflictCache,
    clearConflictCache,
  }
}
