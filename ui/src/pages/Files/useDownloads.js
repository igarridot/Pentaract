import { useState, useEffect, useCallback, useRef } from 'react'
import API from '../../api'
import { createOperationId } from '../../common/operation_id'

export function useDownloads(addAlert, storageId, loadTree, {
  markBulkTransferTerminal,
}) {
  const [downloadStates, setDownloadStates] = useState([])
  const downloadStatesRef = useRef([])
  const downloadProgressCancelsRef = useRef(new Map())
  const downloadFramesRef = useRef(new Map())

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

  const releaseDownloadTracking = useCallback((downloadId) => {
    downloadProgressCancelsRef.current.get(downloadId)?.()
    downloadProgressCancelsRef.current.delete(downloadId)
    downloadFramesRef.current.get(downloadId)?.remove()
    downloadFramesRef.current.delete(downloadId)
  }, [])

  const triggerBrowserDownload = useCallback((downloadId, url) => {
    const existingFrame = downloadFramesRef.current.get(downloadId)
    existingFrame?.remove()

    const frame = document.createElement('iframe')
    frame.style.display = 'none'
    frame.setAttribute('aria-hidden', 'true')
    frame.src = url

    downloadFramesRef.current.set(downloadId, frame)
    document.body.appendChild(frame)
  }, [])

  const startDownload = async (item, providedDownloadId = createOperationId()) => {
    try {
      const downloadId = providedDownloadId
      const filename = item.is_file ? item.name : `${item.name}.zip`

      setDownloadStates((prev) => [...prev, {
        id: downloadId,
        filename,
        totalBytes: 0,
        downloadedBytes: 0,
        totalChunks: 0,
        downloadedChunks: 0,
        status: 'downloading',
        workersStatus: 'active',
        errorMessage: '',
      }])

      const cancel = API.files.subscribeDownloadProgress(downloadId, (data) => {
        updateDownloadState(downloadId, (prev) => ({
          ...prev,
          filename,
          totalBytes: data.total_bytes || prev?.totalBytes || 0,
          downloadedBytes: data.downloaded_bytes || 0,
          totalChunks: data.total || prev?.totalChunks || 0,
          downloadedChunks: data.downloaded || 0,
          status: data.status,
          workersStatus: data.workers_status || prev?.workersStatus || 'active',
          errorMessage: data.status === 'error' ? (data.error_message || prev?.errorMessage || '') : '',
        }))

        if (data.status === 'done') {
          markBulkTransferTerminal('download', downloadId, 'done')
          releaseDownloadTracking(downloadId)
          loadTree()
          scheduleDownloadStateRemoval(downloadId, 2000)
        }
        if (data.status === 'error') {
          markBulkTransferTerminal('download', downloadId, 'error')
          releaseDownloadTracking(downloadId)
          loadTree()
          addAlert(data.error_message || 'Download failed unexpectedly. Please try again.', 'error', { persistent: true })
          scheduleDownloadStateRemoval(downloadId, 6000)
        }
        if (data.status === 'cancelled') {
          markBulkTransferTerminal('download', downloadId, 'cancelled')
          releaseDownloadTracking(downloadId)
          loadTree()
          addAlert('Download cancelled', 'info')
          scheduleDownloadStateRemoval(downloadId, 1500)
        }
      })
      downloadProgressCancelsRef.current.set(downloadId, cancel)

      const url = item.is_file
        ? API.files.downloadFileUrl(storageId, item.path, downloadId)
        : API.files.downloadDirUrl(storageId, item.path, downloadId)

      triggerBrowserDownload(downloadId, url)
      return downloadId
    } catch (err) {
      markBulkTransferTerminal('download', providedDownloadId, 'error')
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
    downloadProgressCancelsRef.current.forEach((_, downloadId) => {
      API.files.cancelDownload(downloadId).catch(() => {})
    })
    downloadProgressCancelsRef.current.forEach((cancel) => cancel())
    downloadProgressCancelsRef.current.clear()
    downloadFramesRef.current.forEach((frame) => frame.remove())
    downloadFramesRef.current.clear()
  }, [])

  const isDownloading = downloadStates.some((d) => d.status === 'downloading')

  return {
    downloadStates,
    downloadStatesRef,
    isDownloading,
    startDownload,
    cancelDownload,
    cleanupDownloads,
    releaseDownloadTracking,
  }
}
