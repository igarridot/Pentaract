import { useState, useEffect, useCallback, useRef } from 'react'
import API from '../../api'
import { createOperationId } from '../../common/operation_id'
import { isTerminalTransferStatus, isActiveDownloadStatus, summarizeTerminalStatuses, resolveBulkTransferStatus } from '../../common/progress'
import { createBulkOperation, getItemPath, buildBulkMoveTargetPath } from './operations'

// clearSelection is called once a bulk delete or move finishes.
export function useBulkOperations(addAlert, storageId, loadTree, clearSelection) {
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false)
  const [bulkMoveOpen, setBulkMoveOpen] = useState(false)
  const [bulkOperation, setBulkOperation] = useState(null)
  const bulkCancelRef = useRef(null)

  const isBulkOperating = bulkOperation?.status === 'running'
  const isBulkUpload = isBulkOperating && bulkOperation?.operation === 'upload'
  const isBulkDownload = isBulkOperating && bulkOperation?.operation === 'download'
  const isBulkDelete = isBulkOperating && bulkOperation?.operation === 'delete'
  const isBulkMove = isBulkOperating && bulkOperation?.operation === 'move'

  // Moves the running bulk card to a terminal status; the effect below
  // removes the card shortly after.
  const finishBulk = useCallback((status) => {
    setBulkOperation((prev) => (prev ? { ...prev, status } : prev))
  }, [])

  const registerBulkTransfer = useCallback((operation, transferId) => {
    setBulkOperation((prev) => {
      if (!prev || prev.operation !== operation) return prev
      if ((prev.transferIds || []).includes(transferId)) return prev
      return { ...prev, transferIds: [...(prev.transferIds || []), transferId] }
    })
  }, [])

  const markBulkTransferTerminal = useCallback((operation, transferId, terminalStatus) => {
    if (!isTerminalTransferStatus(terminalStatus)) return

    setBulkOperation((prev) => {
      if (!prev || prev.operation !== operation) return prev
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

  const finalizeBulkTransferLaunch = useCallback((operation, wasCancelled) => {
    setBulkOperation((prev) => {
      if (!prev || prev.operation !== operation) return prev
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

  // A finished bulk card lingers briefly, longer when it failed.
  useEffect(() => {
    if (!bulkOperation || bulkOperation.status === 'running') return

    const timeoutMs = bulkOperation.status === 'error' ? 3000 : 1500
    const timeout = setTimeout(() => setBulkOperation(null), timeoutMs)
    return () => clearTimeout(timeout)
  }, [bulkOperation])

  // Downloads run one at a time, like bulk uploads: N concurrent downloads
  // would each fetch chunks in parallel and trip the Telegram rate limit.
  const handleBulkDownload = async (selectedFiles, { startDownload, waitForDownload, downloadStatesRef, releaseDownloadTracking }) => {
    const targets = [...selectedFiles]
    const bulkCancelledRef = { current: false }
    setBulkOperation(createBulkOperation('download', targets.length))
    bulkCancelRef.current = async () => {
      bulkCancelledRef.current = true
      const currentIds = downloadStatesRef.current
        .filter((d) => isActiveDownloadStatus(d.status))
        .map((d) => d.id)
      await Promise.all(currentIds.map(async (downloadId) => {
        await API.files.cancelDownload(downloadId).catch(() => {})
        releaseDownloadTracking(downloadId)
        markBulkTransferTerminal('download', downloadId, 'cancelled')
      }))
    }
    try {
      for (const target of targets) {
        if (bulkCancelledRef.current) break
        const downloadId = createOperationId()
        registerBulkTransfer('download', downloadId)
        const startedId = await startDownload(target, downloadId)
        if (!startedId) {
          markBulkTransferTerminal('download', downloadId, 'error')
          continue
        }
        await waitForDownload(startedId)
      }
      finalizeBulkTransferLaunch('download', bulkCancelledRef.current)
    } catch (err) {
      finishBulk('error')
      addAlert(err.message, 'error')
    } finally {
      bulkCancelRef.current = null
      loadTree()
    }
  }

  // Runs step(file) for each selected file in order, one at a time, keeping
  // the bulk card's counter in sync. `label` names the finished action
  // ("Deleted"); onCancel runs in addition to stopping the loop.
  const runSequentialBulk = async (operation, selectedFiles, step, { label, onCancel }) => {
    const targets = [...selectedFiles]
    const bulkCancelledRef = { current: false }
    setBulkOperation(createBulkOperation(operation, targets.length))
    bulkCancelRef.current = () => {
      bulkCancelledRef.current = true
      onCancel?.()
    }
    try {
      let count = 0
      for (const target of targets) {
        if (bulkCancelledRef.current) break
        await step(target)
        count += 1
        setBulkOperation((prev) => (prev ? { ...prev, completed: count } : prev))
      }
      finishBulk(bulkCancelledRef.current ? 'cancelled' : 'done')
      addAlert(`${label} ${count} file(s)`, 'success')
      clearSelection()
    } catch (err) {
      if (err?.name === 'AbortError') {
        finishBulk('cancelled')
        addAlert(`Bulk ${operation} cancelled`, 'info')
      } else {
        finishBulk('error')
        addAlert(err.message, 'error')
      }
    } finally {
      bulkCancelRef.current = null
      loadTree()
    }
  }

  const handleBulkDelete = (selectedFiles) => {
    setBulkDeleteOpen(false)
    return runSequentialBulk('delete', selectedFiles,
      (item) => API.files.delete(storageId, getItemPath(item)),
      { label: 'Deleted' })
  }

  const handleBulkMove = (targetPath, selectedFiles) => {
    setBulkMoveOpen(false)
    const moveAbortController = new AbortController()
    return runSequentialBulk('move', selectedFiles,
      (item) => API.files.move(storageId, item.path, buildBulkMoveTargetPath(targetPath, item.name), { signal: moveAbortController.signal }),
      { label: 'Moved', onCancel: () => moveAbortController.abort() })
  }

  const cleanupBulk = useCallback(() => {
    if (bulkCancelRef.current) bulkCancelRef.current()
  }, [])

  return {
    bulkDeleteOpen,
    setBulkDeleteOpen,
    bulkMoveOpen,
    setBulkMoveOpen,
    bulkOperation,
    setBulkOperation,
    bulkCancelRef,
    isBulkOperating,
    isBulkUpload,
    isBulkDownload,
    isBulkDelete,
    isBulkMove,
    registerBulkTransfer,
    markBulkTransferTerminal,
    finalizeBulkTransferLaunch,
    handleBulkDownload,
    handleBulkDelete,
    handleBulkMove,
    cleanupBulk,
  }
}
