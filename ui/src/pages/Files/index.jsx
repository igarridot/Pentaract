import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate, useLocation, useBlocker } from 'react-router-dom'
import {
  Typography, List, Box, TextField, InputAdornment,
  MenuItem, Divider, Breadcrumbs, Link as MuiLink, Button, FormControlLabel, Checkbox,
} from '@mui/material'
import {
  Search as SearchIcon,
  CreateNewFolder as FolderAddIcon,
  Upload as UploadIcon,
  DriveFolderUpload as FolderUploadIcon,
} from '@mui/icons-material'
import API from '../../api'
import { createOperationId } from '../../common/operation_id'
import { useAlert } from '../../components/AlertStack'
import FSListItem from '../../components/FSListItem'
import FileInfo from '../../components/FileInfo'
import CreateFolderDialog from '../../components/CreateFolderDialog'
import ActionConfirmDialog from '../../components/ActionConfirmDialog'
import NavigationBlockDialog from '../../components/NavigationBlockDialog'
import FloatingMenu from '../../components/Menu'
import UploadProgress from '../../components/UploadProgress'
import DownloadProgress from '../../components/DownloadProgress'
import DeleteProgress from '../../components/DeleteProgress'
import BulkOperationProgress from '../../components/BulkOperationProgress'
import MoveDialog from '../../components/MoveDialog'
import BulkMoveDialog from '../../components/BulkMoveDialog'
import MediaPreviewDialog from '../../components/MediaPreviewDialog'
import RenameFolderDialog from '../../components/RenameFolderDialog'
import UploadConflictDialog from '../../components/UploadConflictDialog'
import { buildUploadEntries, normalizeUploadPath, resolveUploadEntries } from './upload_conflicts'

export default function Files() {
  const { id: storageId } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const addAlert = useAlert()

  const prefix = `/storages/${storageId}/files/`
  const currentPathFromUrl = location.pathname.startsWith(prefix)
    ? location.pathname.slice(prefix.length)
    : ''
  let currentPath = currentPathFromUrl
  try {
    currentPath = decodeURIComponent(currentPathFromUrl)
  } catch {
    currentPath = currentPathFromUrl
  }

  const [items, setItems] = useState([])
  const [search, setSearch] = useState('')
  const [searchResults, setSearchResults] = useState(null)
  const [infoFile, setInfoFile] = useState(null)
  const [previewFile, setPreviewFile] = useState(null)
  const [folderDialogOpen, setFolderDialogOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [forceDelete, setForceDelete] = useState(false)
  const [moveTarget, setMoveTarget] = useState(null)
  const [renameTarget, setRenameTarget] = useState(null)
  const [selectedFilePaths, setSelectedFilePaths] = useState([])
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false)
  const [bulkMoveOpen, setBulkMoveOpen] = useState(false)
  const [bulkOperation, setBulkOperation] = useState(null)
  const [uploadStates, setUploadStates] = useState([])
  const [downloadStates, setDownloadStates] = useState([])
  const [deleteState, setDeleteState] = useState(null)
  const uploadProgressCancelsRef = useRef(new Map())
  const uploadAbortControllersRef = useRef(new Map())
  const uploadConflictResolverRef = useRef(null)
  const dirFileNamesCacheRef = useRef(new Map())
  const downloadProgressCancelsRef = useRef(new Map())
  const bulkCancelRef = useRef(null)
  const cancelDeleteProgressRef = useRef(null)
  const [uploadConflictDialog, setUploadConflictDialog] = useState({
    open: false,
    filename: '',
    targetPath: '',
    applyForAll: false,
  })

  const loadTree = useCallback(async () => {
    try {
      const data = await API.files.tree(storageId, currentPath)
      setItems(data || [])
      const filesInCurrentPath = new Set((data || []).filter((item) => item.is_file).map((item) => item.name))
      dirFileNamesCacheRef.current.set(normalizeUploadPath(currentPath), filesInCurrentPath)
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }, [storageId, currentPath])

  useEffect(() => {
    loadTree()
    setSearchResults(null)
    setSearch('')
    setSelectedFilePaths([])
  }, [loadTree])

  useEffect(() => {
    return () => {
      if (bulkCancelRef.current) bulkCancelRef.current()
      uploadProgressCancelsRef.current.forEach((_, uploadId) => {
        API.files.cancelUpload(uploadId).catch(() => {})
      })
      uploadProgressCancelsRef.current.forEach((cancel) => cancel())
      uploadProgressCancelsRef.current.clear()
      uploadAbortControllersRef.current.forEach((controller) => controller.abort())
      uploadAbortControllersRef.current.clear()
      downloadProgressCancelsRef.current.forEach((_, downloadId) => {
        API.files.cancelDownload(downloadId).catch(() => {})
      })
      downloadProgressCancelsRef.current.forEach((cancel) => cancel())
      downloadProgressCancelsRef.current.clear()
      if (cancelDeleteProgressRef.current) cancelDeleteProgressRef.current()
    }
  }, [])

  const isUploading = uploadStates.some((u) => u.status === 'uploading')
  const isDownloading = downloadStates.some((d) => d.status === 'downloading')
  const isDeleting = deleteState?.status === 'deleting'
  const isBulkOperating = bulkOperation?.status === 'running'
  const isBulkUpload = isBulkOperating && bulkOperation?.operation === 'upload'
  const isBulkDownload = isBulkOperating && bulkOperation?.operation === 'download'
  const isBulkDelete = isBulkOperating && bulkOperation?.operation === 'delete'
  const isBulkMove = isBulkOperating && bulkOperation?.operation === 'move'
  const hasActiveFileOperation = isUploading || isDownloading || isDeleting || isBulkOperating

  useEffect(() => {
    if (!hasActiveFileOperation) return
    const handler = (e) => {
      const isDeleteFlow = isDeleting || isBulkDelete
      const message = isDeleteFlow
        ? 'Operation in progress. Leaving will cancel it. If delete is interrupted, the file will be irrecoverable and orphaned data may remain in storage.'
        : 'Operation in progress. Leaving will cancel it.'
      e.preventDefault()
      e.returnValue = message
      return message
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [hasActiveFileOperation, isDeleting, isBulkDelete])

  const blocker = useBlocker(hasActiveFileOperation)

  const updateUploadState = useCallback((id, updater) => {
    setUploadStates((prev) => prev.map((u) => (u.id === id ? updater(u) : u)))
  }, [])

  const updateDownloadState = useCallback((id, updater) => {
    setDownloadStates((prev) => prev.map((d) => (d.id === id ? updater(d) : d)))
  }, [])

  const handleSearch = async (e) => {
    e.preventDefault()
    if (!search) {
      setSearchResults(null)
      return
    }
    try {
      const data = await API.files.search(storageId, currentPath, search)
      setSearchResults(data || [])
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const handleCreateFolder = async (name) => {
    try {
      await API.files.createFolder(storageId, currentPath, name)
      addAlert('Folder created', 'success')
      loadTree()
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const uploadSingleFile = async (file, targetPath, onConflict = 'keep_both') => {
    const filename = file.name
    const uploadId = createOperationId()
    setUploadStates((prev) => [...prev, {
      id: uploadId,
      filename,
      totalBytes: file.size,
      uploadedBytes: 0,
      totalChunks: 0,
      uploadedChunks: 0,
      status: 'uploading',
      workersStatus: 'active',
    }])

    const cancel = API.files.subscribeProgress(uploadId, (data) => {
      updateUploadState(uploadId, (prev) => ({
        ...prev,
        filename,
        totalBytes: data.total_bytes || prev?.totalBytes || file.size,
        uploadedBytes: data.uploaded_bytes || 0,
        totalChunks: data.total || prev?.totalChunks || 0,
        uploadedChunks: data.uploaded || 0,
        status: data.status,
        workersStatus: data.workers_status || prev?.workersStatus || 'active',
      }))
      if (data.status === 'done') {
        uploadProgressCancelsRef.current.get(uploadId)?.()
        uploadProgressCancelsRef.current.delete(uploadId)
        uploadAbortControllersRef.current.delete(uploadId)
        addAlert('File uploaded', 'success')
        loadTree()
        setTimeout(() => {
          setUploadStates((prev) => prev.filter((u) => u.id !== uploadId))
        }, 2000)
      }
      if (data.status === 'error') {
        uploadProgressCancelsRef.current.get(uploadId)?.()
        uploadProgressCancelsRef.current.delete(uploadId)
        uploadAbortControllersRef.current.delete(uploadId)
        addAlert(`Upload failed unexpectedly for "${filename}". Please try again.`, 'error', { persistent: true })
        setTimeout(() => {
          setUploadStates((prev) => prev.filter((u) => u.id !== uploadId))
        }, 3000)
      }
      if (data.status === 'skipped') {
        uploadProgressCancelsRef.current.get(uploadId)?.()
        uploadProgressCancelsRef.current.delete(uploadId)
        uploadAbortControllersRef.current.delete(uploadId)
        addAlert(`Skipped upload for "${filename}"`, 'info', { persistent: false })
        setTimeout(() => {
          setUploadStates((prev) => prev.filter((u) => u.id !== uploadId))
        }, 1500)
      }
    })
    uploadProgressCancelsRef.current.set(uploadId, cancel)
    const controller = new AbortController()
    uploadAbortControllersRef.current.set(uploadId, controller)

    try {
      await API.files.upload(storageId, targetPath.replace(/\/+$/, ''), file, uploadId, { signal: controller.signal, onConflict })
    } catch (err) {
      if (err?.name !== 'AbortError') {
        updateUploadState(uploadId, (prev) => ({ ...prev, status: 'error' }))
        addAlert(`Upload interrupted: ${err.message}`, 'error', { persistent: true })
      }
      uploadProgressCancelsRef.current.get(uploadId)?.()
      uploadProgressCancelsRef.current.delete(uploadId)
      uploadAbortControllersRef.current.delete(uploadId)
      setTimeout(() => {
        setUploadStates((prev) => prev.filter((u) => u.id !== uploadId))
      }, 1500)
    }
    return uploadId
  }

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

    const entriesToUpload = await resolveUploadEntries(
      entries,
      hasUploadConflict,
      askConflictDecision,
    )

    if (showBulkProgress) {
      setBulkOperation({
        operation: 'upload',
        status: 'running',
        total: entriesToUpload.length,
        completed: 0,
        transferIds: [],
      })
      bulkCancelRef.current = async () => {
        bulkCancelledRef.current = true
        const currentIds = uploadStates
          .filter((u) => u.status === 'uploading')
          .map((u) => u.id)
        await Promise.all(currentIds.map(async (uploadId) => {
          uploadProgressCancelsRef.current.get(uploadId)?.()
          uploadProgressCancelsRef.current.delete(uploadId)
          uploadAbortControllersRef.current.get(uploadId)?.abort()
          uploadAbortControllersRef.current.delete(uploadId)
          await API.files.cancelUpload(uploadId).catch(() => {})
        }))
      }
    }

    const uploadedKeys = new Set(entriesToUpload.map((entry) => `${entry.targetPath}::${entry.filename}`))
    entries.forEach((entry) => {
      const key = `${entry.targetPath}::${entry.filename}`
      if (!uploadedKeys.has(key)) {
        addAlert(`Skipped upload for "${entry.filename}"`, 'info', { persistent: false })
      }
    })

    try {
      for (const entry of entriesToUpload) {
        if (bulkCancelledRef.current) break
        // Upload sequentially to preserve stable progress/cancel behavior.
        // This also avoids opening too many concurrent requests at once.
        // eslint-disable-next-line no-await-in-loop
        const uploadId = await uploadSingleFile(entry.file, entry.targetPath, defaultConflictMode)
        dirFileNamesCacheRef.current.delete(normalizeUploadPath(entry.targetPath))
        if (showBulkProgress) {
          setBulkOperation((prev) => (prev
            ? { ...prev, completed: prev.completed + 1, transferIds: [...(prev.transferIds || []), uploadId] }
            : prev))
        }
      }
      if (showBulkProgress) {
        setBulkOperation((prev) => (prev ? { ...prev, status: bulkCancelledRef.current ? 'cancelled' : 'done' } : prev))
        setTimeout(() => setBulkOperation(null), 1500)
      }
    } catch (err) {
      if (showBulkProgress) {
        setBulkOperation((prev) => (prev ? { ...prev, status: 'error' } : prev))
        setTimeout(() => setBulkOperation(null), 3000)
      }
      throw err
    } finally {
      if (showBulkProgress) bulkCancelRef.current = null
    }
  }, [addAlert, askUploadConflictDecision, hasUploadConflict, uploadStates])

  const handleUpload = async (e) => {
    const files = Array.from(e.target.files || [])
    if (files.length === 0) return
    await runUploadBatch(buildUploadEntries(files, currentPath))
    e.target.value = ''
  }

  const handleUploadDirectory = async (e) => {
    const files = Array.from(e.target.files || [])
    if (files.length === 0) return

    await runUploadBatch(buildUploadEntries(files, currentPath))
    e.target.value = ''
  }

  const handleCancelUpload = async (uploadId) => {
    if (!uploadId) return
    try {
      uploadProgressCancelsRef.current.get(uploadId)?.()
      uploadProgressCancelsRef.current.delete(uploadId)
      uploadAbortControllersRef.current.get(uploadId)?.abort()
      uploadAbortControllersRef.current.delete(uploadId)
      await API.files.cancelUpload(uploadId)
      addAlert('Upload cancelled', 'info')
    } catch (err) {
      addAlert(err.message, 'error')
    }
    setUploadStates((prev) => prev.filter((u) => u.id !== uploadId))
    loadTree()
  }

  const handleDownload = async (item) => {
    try {
      const downloadId = createOperationId()
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
        }))

        if (data.status === 'done') {
          downloadProgressCancelsRef.current.get(downloadId)?.()
          downloadProgressCancelsRef.current.delete(downloadId)
          setTimeout(() => {
            setDownloadStates((prev) => prev.filter((d) => d.id !== downloadId))
          }, 2000)
        }
        if (data.status === 'error') {
          downloadProgressCancelsRef.current.get(downloadId)?.()
          downloadProgressCancelsRef.current.delete(downloadId)
          addAlert('Download failed unexpectedly. Please try again.', 'error', { persistent: true })
          setTimeout(() => {
            setDownloadStates((prev) => prev.filter((d) => d.id !== downloadId))
          }, 3000)
        }
        if (data.status === 'cancelled') {
          downloadProgressCancelsRef.current.get(downloadId)?.()
          downloadProgressCancelsRef.current.delete(downloadId)
          addAlert('Download cancelled', 'info')
          setTimeout(() => {
            setDownloadStates((prev) => prev.filter((d) => d.id !== downloadId))
          }, 1500)
        }
      })
      downloadProgressCancelsRef.current.set(downloadId, cancel)

      const url = item.is_file
        ? API.files.downloadFileUrl(storageId, item.path, downloadId)
        : API.files.downloadDirUrl(storageId, item.path, downloadId)

      const a = document.createElement('a')
      a.href = url
      a.download = filename
      a.rel = 'noopener'
      a.click()
      return downloadId
    } catch (err) {
      addAlert(err.message, 'error')
      return null
    }
  }

  const handleCancelDownload = async (downloadId) => {
    if (!downloadId) return
    try {
      await API.files.cancelDownload(downloadId)
      downloadProgressCancelsRef.current.get(downloadId)?.()
      downloadProgressCancelsRef.current.delete(downloadId)
      updateDownloadState(downloadId, (prev) => (prev ? { ...prev, status: 'cancelled' } : prev))
      addAlert('Download cancelled', 'info')
      setTimeout(() => {
        setDownloadStates((prev) => prev.filter((d) => d.id !== downloadId))
      }, 1500)
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const getMediaType = (name) => {
    const ext = name?.split('.').pop()?.toLowerCase() || ''
    if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'svg'].includes(ext)) return 'image'
    if (['mp4', 'webm', 'ogg', 'mov', 'm4v'].includes(ext)) return 'video'
    return null
  }

  const handlePreview = (item) => {
    const mediaType = getMediaType(item.name)
    if (!mediaType) {
      handleDownload(item)
      return
    }
    setPreviewFile(item)
  }

  const handleDelete = async () => {
    const target = deleteTarget
    if (!target) return

    setDeleteTarget(null)

    try {
      const path = (target.path || target.name).replace(/\/+$/, '')
      const deleteId = createOperationId()
      if (cancelDeleteProgressRef.current) cancelDeleteProgressRef.current()
      setDeleteState({
        label: target.name || path,
        totalChunks: 0,
        deletedChunks: 0,
        status: 'deleting',
        workersStatus: 'active',
      })

      const cancel = API.files.subscribeDeleteProgress(deleteId, (data) => {
        setDeleteState((prev) => ({
          ...prev,
          totalChunks: data.total || prev?.totalChunks || 0,
          deletedChunks: data.deleted || 0,
          status: data.status,
          workersStatus: data.workers_status || prev?.workersStatus || 'active',
        }))

        if (data.status === 'done') {
          cancel()
          cancelDeleteProgressRef.current = null
          loadTree()
          setTimeout(() => setDeleteState(null), 1500)
        }
        if (data.status === 'error') {
          cancel()
          cancelDeleteProgressRef.current = null
          setTimeout(() => setDeleteState(null), 3000)
        }
      })
      cancelDeleteProgressRef.current = cancel

      await API.files.delete(storageId, path, deleteId, forceDelete)
      addAlert('Deleted', 'success')
      loadTree()
    } catch (err) {
      if (cancelDeleteProgressRef.current) {
        cancelDeleteProgressRef.current()
        cancelDeleteProgressRef.current = null
      }
      setDeleteState((prev) => (prev ? { ...prev, status: 'error' } : null))
      setTimeout(() => setDeleteState(null), 3000)
      addAlert(err.message, 'error')
    } finally {
      setForceDelete(false)
      loadTree()
    }
  }

  const handleMove = async (item, newPath) => {
    try {
      const oldPath = item.is_file
        ? (item.path || item.name)
        : (item.path || item.name).replace(/\/$/, '')
      await API.files.move(storageId, oldPath, newPath)
      addAlert('Moved successfully', 'success')
      setMoveTarget(null)
      loadTree()
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const handleRename = async (item, newName) => {
    try {
      const sourcePath = (item.path || item.name).replace(/\/$/, '')
      const pathParts = sourcePath.split('/').filter(Boolean)
      pathParts[pathParts.length - 1] = newName
      const targetPath = pathParts.join('/')
      await API.files.move(storageId, sourcePath, targetPath)
      addAlert('Folder renamed', 'success')
      setRenameTarget(null)
      loadTree()
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const pathParts = currentPath.split('/').filter(Boolean)
  const breadcrumbs = [
    <MuiLink
      key="root"
      underline="hover"
      color="inherit"
      sx={{ cursor: 'pointer', fontSize: '0.8125rem' }}
      onClick={() => navigate(prefix)}
    >
      Root
    </MuiLink>,
    ...pathParts.map((part, i) => {
      const pathTo = prefix + pathParts.slice(0, i + 1).join('/') + '/'
      return (
        <MuiLink
          key={pathTo}
          underline="hover"
          color="inherit"
          sx={{ cursor: 'pointer', fontSize: '0.8125rem' }}
          onClick={() => navigate(pathTo)}
        >
          {part}
        </MuiLink>
      )
    }),
  ]

  const displayItems = searchResults || items
  const selectableFiles = displayItems.filter((item) => item.is_file)
  const selectedFiles = selectableFiles.filter((item) => selectedFilePaths.includes(item.path))
  const allFilesSelected = selectableFiles.length > 0 && selectedFiles.length === selectableFiles.length
  const bulkTransferStates = bulkOperation?.operation === 'upload'
    ? uploadStates.filter((u) => (bulkOperation.transferIds || []).includes(u.id))
    : bulkOperation?.operation === 'download'
      ? downloadStates.filter((d) => (bulkOperation.transferIds || []).includes(d.id))
      : []
  const bulkTotalBytes = bulkOperation?.operation === 'move'
    ? (bulkOperation.total || 0)
    : bulkTransferStates.reduce((sum, t) => sum + (t.totalBytes || 0), 0)
  const bulkProcessedBytes = bulkOperation?.operation === 'move'
    ? (bulkOperation.completed || 0)
    : bulkTransferStates.reduce((sum, t) => sum + ((t.uploadedBytes ?? t.downloadedBytes) || 0), 0)
  const bulkTotalChunks = bulkOperation?.operation === 'move'
    ? (bulkOperation.total || 0)
    : bulkTransferStates.reduce((sum, t) => sum + (t.totalChunks || 0), 0)
  const bulkProcessedChunks = bulkOperation?.operation === 'move'
    ? (bulkOperation.completed || 0)
    : bulkTransferStates.reduce((sum, t) => sum + ((t.uploadedChunks ?? t.downloadedChunks) || 0), 0)
  const bulkWorkersStatus = bulkOperation?.operation === 'move'
    ? 'active'
    : bulkTransferStates.some((t) => t.workersStatus === 'waiting_rate_limit') ? 'waiting_rate_limit' : 'active'

  useEffect(() => {
    const visiblePaths = new Set(selectableFiles.map((item) => item.path))
    setSelectedFilePaths((prev) => {
      const next = prev.filter((path) => visiblePaths.has(path))
      if (next.length === prev.length && next.every((path, i) => path === prev[i])) {
        return prev
      }
      return next
    })
  }, [selectableFiles])

  const toggleFileSelection = (item) => {
    if (!item?.is_file || !item.path) return
    setSelectedFilePaths((prev) => (
      prev.includes(item.path)
        ? prev.filter((path) => path !== item.path)
        : [...prev, item.path]
    ))
  }

  const toggleSelectAllFiles = () => {
    if (allFilesSelected) {
      setSelectedFilePaths([])
      return
    }
    setSelectedFilePaths(selectableFiles.map((item) => item.path))
  }

  const handleBulkDownload = async () => {
    const targets = [...selectedFiles]
    const bulkCancelledRef = { current: false }
    setBulkOperation({
      operation: 'download',
      status: 'running',
      total: targets.length,
      completed: 0,
      transferIds: [],
    })
    bulkCancelRef.current = async () => {
      bulkCancelledRef.current = true
      const currentIds = downloadStates
        .filter((d) => d.status === 'downloading')
        .map((d) => d.id)
      await Promise.all(currentIds.map(async (downloadId) => {
        await API.files.cancelDownload(downloadId).catch(() => {})
        downloadProgressCancelsRef.current.get(downloadId)?.()
        downloadProgressCancelsRef.current.delete(downloadId)
      }))
    }
    try {
      for (let i = 0; i < targets.length; i += 1) {
        if (bulkCancelledRef.current) break
        // Keep stable progress state behavior.
        // eslint-disable-next-line no-await-in-loop
        const downloadId = await handleDownload(targets[i])
        setBulkOperation((prev) => (prev ? {
          ...prev,
          completed: i + 1,
          transferIds: downloadId ? [...(prev.transferIds || []), downloadId] : (prev.transferIds || []),
        } : prev))
      }
      setBulkOperation((prev) => (prev ? { ...prev, status: bulkCancelledRef.current ? 'cancelled' : 'done' } : prev))
      setTimeout(() => setBulkOperation(null), 1500)
    } catch (err) {
      setBulkOperation((prev) => (prev ? { ...prev, status: 'error' } : prev))
      setTimeout(() => setBulkOperation(null), 3000)
      addAlert(err.message, 'error')
    } finally {
      bulkCancelRef.current = null
    }
  }

  const deleteSingleItem = async (item) => {
    const path = (item.path || item.name).replace(/\/+$/, '')
    await API.files.delete(storageId, path)
  }

  const handleBulkDelete = async () => {
    setBulkDeleteOpen(false)
    const targets = [...selectedFiles]
    const bulkCancelledRef = { current: false }
    setBulkOperation({
      operation: 'delete',
      status: 'running',
      total: targets.length,
      completed: 0,
      transferIds: [],
    })
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

  const handleBulkMove = async (targetPath) => {
    setBulkMoveOpen(false)
    const targets = [...selectedFiles]
    const bulkCancelledRef = { current: false }
    const moveAbortController = new AbortController()
    setBulkOperation({
      operation: 'move',
      status: 'running',
      total: targets.length,
      completed: 0,
      transferIds: [],
    })
    bulkCancelRef.current = () => {
      bulkCancelledRef.current = true
      moveAbortController.abort()
    }
    try {
      let movedCount = 0
      for (let i = 0; i < targets.length; i += 1) {
        if (bulkCancelledRef.current) break
        const item = targets[i]
        const newPath = targetPath ? `${targetPath}/${item.name}` : item.name
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

  return (
    <Box>
      <Breadcrumbs sx={{ mb: 2 }}>{breadcrumbs}</Breadcrumbs>

      {uploadStates.map((uploadState) => (
        <UploadProgress
          key={uploadState.id}
          filename={uploadState.filename}
          totalBytes={uploadState.totalBytes}
          uploadedBytes={uploadState.uploadedBytes}
          totalChunks={uploadState.totalChunks}
          uploadedChunks={uploadState.uploadedChunks}
          status={uploadState.status}
          workersStatus={uploadState.workersStatus}
          onCancel={() => handleCancelUpload(uploadState.id)}
        />
      ))}
      {downloadStates.map((downloadState) => (
        <DownloadProgress
          key={downloadState.id}
          filename={downloadState.filename}
          totalBytes={downloadState.totalBytes}
          downloadedBytes={downloadState.downloadedBytes}
          totalChunks={downloadState.totalChunks}
          downloadedChunks={downloadState.downloadedChunks}
          status={downloadState.status}
          workersStatus={downloadState.workersStatus}
          onCancel={() => handleCancelDownload(downloadState.id)}
        />
      ))}
      {deleteState && (
        <DeleteProgress
          label={deleteState.label}
          totalChunks={deleteState.totalChunks}
          deletedChunks={deleteState.deletedChunks}
          status={deleteState.status}
          workersStatus={deleteState.workersStatus}
        />
      )}
      {bulkOperation && (
        <BulkOperationProgress
          operation={bulkOperation.operation}
          status={bulkOperation.status}
          total={bulkOperation.total}
          completed={bulkOperation.completed}
          totalBytes={bulkTotalBytes}
          processedBytes={bulkProcessedBytes}
          totalChunks={bulkTotalChunks}
          processedChunks={bulkProcessedChunks}
          workersStatus={bulkWorkersStatus}
          onCancel={bulkOperation.status === 'running' ? () => bulkCancelRef.current?.() : null}
        />
      )}

      <Box component="form" onSubmit={handleSearch} sx={{ mb: 2, display: 'flex', alignItems: 'center', gap: 1 }}>
        <TextField
          size="small"
          placeholder="Search files..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
              </InputAdornment>
            ),
          }}
          sx={{ width: 260 }}
        />
        {searchResults && (
          <Button
            size="small"
            onClick={() => { setSearchResults(null); setSearch('') }}
            sx={{ color: 'text.secondary', fontSize: '0.8125rem' }}
          >
            Clear
          </Button>
        )}
      </Box>

      {selectableFiles.length > 0 && (
        <Box sx={{ mb: 1.5, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
          <FormControlLabel
            control={<Checkbox checked={allFilesSelected} onChange={toggleSelectAllFiles} />}
            label={`Select all files (${selectableFiles.length})`}
          />
          <Button size="small" onClick={() => setSelectedFilePaths([])} disabled={selectedFiles.length === 0}>
            Clear
          </Button>
          <Button size="small" onClick={() => setBulkMoveOpen(true)} disabled={selectedFiles.length === 0 || isBulkOperating}>
            Move selected
          </Button>
          <Button size="small" onClick={handleBulkDownload} disabled={selectedFiles.length === 0 || isBulkOperating}>
            Download selected
          </Button>
          <Button
            size="small"
            color="error"
            onClick={() => setBulkDeleteOpen(true)}
            disabled={selectedFiles.length === 0 || isBulkOperating}
          >
            Delete selected
          </Button>
          {selectedFiles.length > 0 && (
            <Typography variant="body2" color="text.secondary">
              {`${selectedFiles.length} selected`}
            </Typography>
          )}
        </Box>
      )}

      <Box sx={{
        bgcolor: 'background.paper',
        borderRadius: 3,
        border: '1px solid',
        borderColor: 'divider',
        overflow: 'hidden',
      }}>
        <List disablePadding>
          {displayItems.map((item, i) => (
            <Box key={item.path || item.name}>
              {i > 0 && <Divider />}
              <FSListItem
                item={item}
                storageId={storageId}
                onInfo={setInfoFile}
                onPreview={handlePreview}
                onDelete={setDeleteTarget}
                onDownload={handleDownload}
                onMove={setMoveTarget}
                onRename={setRenameTarget}
                selectionEnabled
                isSelected={selectedFilePaths.includes(item.path)}
                onToggleSelect={toggleFileSelection}
              />
            </Box>
          ))}
          {displayItems.length === 0 && (
            <Box sx={{ p: 4, textAlign: 'center' }}>
              <Typography color="text.secondary" variant="body2">
                {searchResults ? 'No results found' : 'Empty folder'}
              </Typography>
            </Box>
          )}
        </List>
      </Box>

      <FloatingMenu>
        {(close) => [
          <MenuItem key="folder" onClick={() => { close(); setFolderDialogOpen(true) }}>
            <FolderAddIcon sx={{ mr: 1.5, fontSize: 18, color: 'text.secondary' }} /> New Folder
          </MenuItem>,
          <MenuItem key="upload" component="label">
            <UploadIcon sx={{ mr: 1.5, fontSize: 18, color: 'text.secondary' }} /> Upload File
            <input type="file" hidden multiple onChange={(e) => { close(); handleUpload(e) }} />
          </MenuItem>,
          <MenuItem key="upload-folder" component="label">
            <FolderUploadIcon sx={{ mr: 1.5, fontSize: 18, color: 'text.secondary' }} /> Upload Folder
            <input
              type="file"
              hidden
              multiple
              webkitdirectory=""
              directory=""
              onChange={(e) => { close(); handleUploadDirectory(e) }}
            />
          </MenuItem>,
        ]}
      </FloatingMenu>

      <FileInfo file={infoFile} open={!!infoFile} onClose={() => setInfoFile(null)} />
      <MediaPreviewDialog
        open={!!previewFile}
        file={previewFile}
        mediaType={getMediaType(previewFile?.name)}
        src={previewFile ? API.files.previewFileUrl(storageId, previewFile.path) : ''}
        onClose={() => setPreviewFile(null)}
        onDownload={() => {
          if (!previewFile) return
          const file = previewFile
          setPreviewFile(null)
          handleDownload(file)
        }}
      />

      <CreateFolderDialog
        open={folderDialogOpen}
        onCreate={handleCreateFolder}
        onClose={() => setFolderDialogOpen(false)}
      />

      <ActionConfirmDialog
        open={!!deleteTarget}
        entity={deleteTarget?.name || 'item'}
        action="Delete"
        description={`Are you sure you want to delete "${deleteTarget?.name}"?`}
        onConfirm={handleDelete}
        onCancel={() => { setDeleteTarget(null); setForceDelete(false) }}
      >
        <Box sx={{ mt: 2 }}>
          <FormControlLabel
            control={(
              <Checkbox
                checked={forceDelete}
                onChange={(e) => setForceDelete(e.target.checked)}
                color="error"
              />
            )}
            label="Force delete"
          />
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
            This operation deletes file records from the database only and skips backend storage cleanup.
          </Typography>
          <Typography variant="caption" color="error.main" sx={{ display: 'block', mt: 1 }}>
            Warning: this is irreversible and leaves orphaned data in backend storage.
          </Typography>
        </Box>
      </ActionConfirmDialog>

      <ActionConfirmDialog
        open={bulkDeleteOpen}
        entity={`${selectedFiles.length} file(s)`}
        action="Delete"
        description={`Are you sure you want to delete ${selectedFiles.length} selected file(s)?`}
        onConfirm={handleBulkDelete}
        onCancel={() => setBulkDeleteOpen(false)}
      />

      <NavigationBlockDialog
        blocker={blocker}
        isUploading={isUploading || isBulkUpload}
        isDownloading={isDownloading || isBulkDownload}
        isDeleting={isDeleting || isBulkDelete}
        isMoving={isBulkMove}
      />

      <MoveDialog
        open={!!moveTarget}
        item={moveTarget}
        storageId={storageId}
        onMove={handleMove}
        onClose={() => setMoveTarget(null)}
      />

      <BulkMoveDialog
        open={bulkMoveOpen}
        count={selectedFiles.length}
        storageId={storageId}
        onConfirm={handleBulkMove}
        onClose={() => setBulkMoveOpen(false)}
      />

      <RenameFolderDialog
        open={!!renameTarget}
        folder={renameTarget}
        onRename={handleRename}
        onClose={() => setRenameTarget(null)}
      />
      <UploadConflictDialog
        open={uploadConflictDialog.open}
        filename={uploadConflictDialog.filename}
        targetPath={uploadConflictDialog.targetPath}
        applyForAll={uploadConflictDialog.applyForAll}
        onApplyForAllChange={(checked) => setUploadConflictDialog((prev) => ({ ...prev, applyForAll: checked }))}
        onDecision={handleUploadConflictDecision}
      />
    </Box>
  )
}
