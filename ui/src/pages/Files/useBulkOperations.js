import { useState, useEffect, useCallback, useRef } from 'react'
import API from '../../api'
import { createOperationId } from '../../common/operation_id'
import { isTerminalTransferStatus, summarizeTerminalStatuses, resolveBulkTransferStatus } from '../../common/progress'
import { createBulkOperation, getItemPath, buildBulkMoveTargetPath } from './operations'

export function useBulkOperations(addAlert, storageId, loadTree) {
  const [selectedFilePaths, setSelectedFilePaths] = useState([])
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false)
  const [bulkMoveOpen, setBulkMoveOpen] = useState(false)
  const [bulkOperation, setBulkOperation] = useState(null)
  const bulkCancelRef = useRef(null)

  const isBulkOperating = bulkOperation?.status === 'running'
  const isBulkUpload = isBulkOperating && bulkOperation?.operation === 'upload'
  const isBulkDownload = isBulkOperating && bulkOperation?.operation === 'download'
  const isBulkDelete = isBulkOperating && bulkOperation?.operation === 'delete'
  const isBulkMove = isBulkOperating && bulkOperation?.operation === 'move'

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

  useEffect(() => {
    if (!bulkOperation || !['upload', 'download'].includes(bulkOperation.operation)) return
    if (bulkOperation.status === 'running') return

    const timeoutMs = bulkOperation.status === 'error' ? 3000 : 1500
    const timeout = setTimeout(() => setBulkOperation(null), timeoutMs)
    return () => clearTimeout(timeout)
  }, [bulkOperation])

  const handleBulkDownload = async (selectedFiles, { startDownload, downloadStatesRef, releaseDownloadTracking }) => {
    const targets = [...selectedFiles]
    const bulkCancelledRef = { current: false }
    setBulkOperation(createBulkOperation('download', targets.length))
    bulkCancelRef.current = async () => {
      bulkCancelledRef.current = true
      const currentIds = downloadStatesRef.current
        .filter((d) => d.status === 'downloading')
        .map((d) => d.id)
      await Promise.all(currentIds.map(async (downloadId) => {
        await API.files.cancelDownload(downloadId).catch(() => {})
        releaseDownloadTracking(downloadId)
        markBulkTransferTerminal('download', downloadId, 'cancelled')
      }))
    }
    try {
      for (let i = 0; i < targets.length; i += 1) {
        if (bulkCancelledRef.current) break
        const downloadId = createOperationId()
        registerBulkTransfer('download', downloadId)
        // Keep stable progress state behavior.
        // eslint-disable-next-line no-await-in-loop
        const startedId = await startDownload(targets[i], downloadId)
        if (!startedId) {
          markBulkTransferTerminal('download', downloadId, 'error')
        }
      }
      finalizeBulkTransferLaunch('download', bulkCancelledRef.current)
    } catch (err) {
      setBulkOperation((prev) => (prev ? { ...prev, status: 'error' } : prev))
      addAlert(err.message, 'error')
    } finally {
      bulkCancelRef.current = null
      loadTree()
    }
  }

  const deleteSingleItem = async (item) => {
    await API.files.delete(storageId, getItemPath(item))
  }

  const handleBulkDelete = async (selectedFiles) => {
    setBulkDeleteOpen(false)
    const targets = [...selectedFiles]
    const bulkCancelledRef = { current: false }
    setBulkOperation(createBulkOperation('delete', targets.length))
    bulkCancelRef.current = () => { bulkCancelledRef.current = true }
    try {
      let deletedCount = 0
      for (let i = 0; i < targets.length; i += 1) {
        if (bulkCancelledRef.current) break
        // Keep API calls sequential to avoid overwhelming the backend.
        // eslint-disable-next-line no-await-in-loop
        await deleteSingleItem(targets[i])
        deletedCount += 1
        setBulkOperation((prev) => (prev ? { ...prev, completed: i + 1 } : prev))
      }
      setBulkOperation((prev) => (prev ? { ...prev, status: bulkCancelledRef.current ? 'cancelled' : 'done' } : prev))
      setTimeout(() => setBulkOperation(null), 1500)
      addAlert(`Deleted ${deletedCount} file(s)`, 'success')
      setSelectedFilePaths([])
    } catch (err) {
      setBulkOperation((prev) => (prev ? { ...prev, status: 'error' } : prev))
      setTimeout(() => setBulkOperation(null), 3000)
      addAlert(err.message, 'error')
    } finally {
      bulkCancelRef.current = null
      loadTree()
    }
  }

  const handleBulkMove = async (targetPath, selectedFiles) => {
    setBulkMoveOpen(false)
    const targets = [...selectedFiles]
    const bulkCancelledRef = { current: false }
    const moveAbortController = new AbortController()
    setBulkOperation(createBulkOperation('move', targets.length))
    bulkCancelRef.current = () => {
      bulkCancelledRef.current = true
      moveAbortController.abort()
    }
    try {
      let movedCount = 0
      for (let i = 0; i < targets.length; i += 1) {
        if (bulkCancelledRef.current) break
        const item = targets[i]
        const newPath = buildBulkMoveTargetPath(targetPath, item.name)
        // Keep move operations sequential for predictable ordering and cancellation.
        // eslint-disable-next-line no-await-in-loop
        await API.files.move(storageId, item.path, newPath, { signal: moveAbortController.signal })
        movedCount += 1
        setBulkOperation((prev) => (prev ? { ...prev, completed: i + 1 } : prev))
      }
      setBulkOperation((prev) => (prev ? { ...prev, status: bulkCancelledRef.current ? 'cancelled' : 'done' } : prev))
      setTimeout(() => setBulkOperation(null), 1500)
      addAlert(`Moved ${movedCount} file(s)`, 'success')
      setSelectedFilePaths([])
    } catch (err) {
      if (err?.name === 'AbortError') {
        setBulkOperation((prev) => (prev ? { ...prev, status: 'cancelled' } : prev))
        setTimeout(() => setBulkOperation(null), 1500)
        addAlert('Bulk move cancelled', 'info')
      } else {
        setBulkOperation((prev) => (prev ? { ...prev, status: 'error' } : prev))
        setTimeout(() => setBulkOperation(null), 3000)
        addAlert(err.message, 'error')
      }
    } finally {
      bulkCancelRef.current = null
      loadTree()
    }
  }

  const cleanupBulk = useCallback(() => {
    if (bulkCancelRef.current) bulkCancelRef.current()
  }, [])

  return {
    selectedFilePaths,
    setSelectedFilePaths,
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
