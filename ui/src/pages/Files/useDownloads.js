import { useState, useEffect, useCallback, useRef } from 'react'
import API from '../../api'
import { createOperationId } from '../../common/operation_id'
import { isActiveDownloadStatus, createDownloadState, applyDownloadProgressUpdate } from '../../common/progress'
import { createCompletionRegistry } from '../../common/completion_registry'

// What each terminal SSE status does to the card: how long it stays visible
// and which alert it raises.
const DOWNLOAD_TERMINAL = {
  done: { delay: 2000 },
  error: {
    delay: 6000,
    alert: (data) => [data.error_message || 'Download failed unexpectedly. Please try again.', 'error', { persistent: true }],
  },
  cancelled: { delay: 1500, alert: () => ['Download cancelled', 'info'] },
}

export function useDownloads(addAlert, storageId, loadTree, {
  markBulkTransferTerminal,
}) {
  const [downloadStates, setDownloadStates] = useState([])
  const downloadStatesRef = useRef([])
  const downloadProgressCancelsRef = useRef(new Map())
  const downloadFramesRef = useRef(new Map())
  const completionRef = useRef(createCompletionRegistry())

  useEffect(() => {
    downloadStatesRef.current = downloadStates
  }, [downloadStates])

  const updateDownloadState = useCallback((id, updater) => {
    setDownloadStates((prev) => prev.map((d) => (d.id === id ? updater(d) : d)))
  }, [])

  const scheduleDownloadStateRemoval = useCallback((downloadId, delayMs) => {
    setTimeout(() => {
      setDownloadStates((prev) => prev.filter((d) => d.id !== downloadId))
    }, delayMs)
  }, [])

  // Stops following a download and resolves whoever awaits its outcome.
  const releaseDownloadTracking = useCallback((downloadId, terminalStatus = 'cancelled') => {
    downloadProgressCancelsRef.current.get(downloadId)?.()
    downloadProgressCancelsRef.current.delete(downloadId)
    downloadFramesRef.current.get(downloadId)?.remove()
    downloadFramesRef.current.delete(downloadId)
    completionRef.current.settle(downloadId, terminalStatus)
  }, [])

  // Resolves with the terminal status once the download ends; bulk downloads
  // use it to run one file at a time.
  const waitForDownload = useCallback((downloadId) => completionRef.current.waitFor(downloadId), [])

  const triggerBrowserDownload = useCallback((downloadId, url) => {
    downloadFramesRef.current.get(downloadId)?.remove()

    const frame = document.createElement('iframe')
    frame.style.display = 'none'
    frame.setAttribute('aria-hidden', 'true')
    frame.src = url

    downloadFramesRef.current.set(downloadId, frame)
    document.body.appendChild(frame)
  }, [])

  const startDownload = async (item, downloadId = createOperationId()) => {
    try {
      const filename = item.is_file ? item.name : `${item.name}.zip`
      setDownloadStates((prev) => [...prev, createDownloadState(downloadId, filename)])

      const cancel = API.files.subscribeDownloadProgress(downloadId, (data) => {
        updateDownloadState(downloadId, (prev) => applyDownloadProgressUpdate(prev, data))

        const terminal = DOWNLOAD_TERMINAL[data.status]
        if (!terminal) return
        markBulkTransferTerminal('download', downloadId, data.status)
        releaseDownloadTracking(downloadId, data.status)
        loadTree()
        if (terminal.alert) addAlert(...terminal.alert(data))
        scheduleDownloadStateRemoval(downloadId, terminal.delay)
      })
      downloadProgressCancelsRef.current.set(downloadId, cancel)

      const url = item.is_file
        ? API.files.downloadFileUrl(storageId, item.path, downloadId)
        : API.files.downloadDirUrl(storageId, item.path, downloadId)

      triggerBrowserDownload(downloadId, url)
      return downloadId
    } catch (err) {
      markBulkTransferTerminal('download', downloadId, 'error')
      completionRef.current.settle(downloadId, 'error')
      addAlert(err.message, 'error')
      return null
    }
  }

  const cancelDownload = async (downloadId) => {
    if (!downloadId) return
    try {
      await API.files.cancelDownload(downloadId)
      releaseDownloadTracking(downloadId)
      updateDownloadState(downloadId, (prev) => (prev ? { ...prev, status: 'cancelled' } : prev))
      markBulkTransferTerminal('download', downloadId, 'cancelled')
      addAlert('Download cancelled', 'info')
      loadTree()
      scheduleDownloadStateRemoval(downloadId, 1500)
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const cleanupDownloads = useCallback(() => {
    downloadProgressCancelsRef.current.forEach((cancel, downloadId) => {
      API.files.cancelDownload(downloadId).catch(() => {})
      cancel()
    })
    downloadProgressCancelsRef.current.clear()
    downloadFramesRef.current.forEach((frame) => frame.remove())
    downloadFramesRef.current.clear()
    completionRef.current.clear()
  }, [])

  const isDownloading = downloadStates.some((d) => isActiveDownloadStatus(d.status))

  return {
    downloadStates,
    downloadStatesRef,
    isDownloading,
    startDownload,
    waitForDownload,
    cancelDownload,
    cleanupDownloads,
    releaseDownloadTracking,
  }
}
