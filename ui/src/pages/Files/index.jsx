import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate, useLocation, useBlocker } from 'react-router-dom'
import {
  Typography, List, Box, TextField, InputAdornment,
  MenuItem, Divider, Breadcrumbs, Link as MuiLink, Button,
} from '@mui/material'
import {
  Search as SearchIcon,
  CreateNewFolder as FolderAddIcon,
  Upload as UploadIcon,
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
import MoveDialog from '../../components/MoveDialog'
import MediaPreviewDialog from '../../components/MediaPreviewDialog'

export default function Files() {
  const { id: storageId } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const addAlert = useAlert()

  const prefix = `/storages/${storageId}/files/`
  const currentPath = location.pathname.startsWith(prefix)
    ? location.pathname.slice(prefix.length)
    : ''

  const [items, setItems] = useState([])
  const [search, setSearch] = useState('')
  const [searchResults, setSearchResults] = useState(null)
  const [infoFile, setInfoFile] = useState(null)
  const [previewFile, setPreviewFile] = useState(null)
  const [folderDialogOpen, setFolderDialogOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [moveTarget, setMoveTarget] = useState(null)
  const [uploadStates, setUploadStates] = useState([])
  const [downloadStates, setDownloadStates] = useState([])
  const [deleteState, setDeleteState] = useState(null)
  const uploadProgressCancelsRef = useRef(new Map())
  const uploadAbortControllersRef = useRef(new Map())
  const downloadProgressCancelsRef = useRef(new Map())
  const cancelDeleteProgressRef = useRef(null)

  const loadTree = useCallback(async () => {
    try {
      const data = await API.files.tree(storageId, currentPath)
      setItems(data || [])
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }, [storageId, currentPath])

  useEffect(() => {
    loadTree()
    setSearchResults(null)
    setSearch('')
  }, [loadTree])

  useEffect(() => {
    return () => {
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

  useEffect(() => {
    if (!isUploading) return
    const handler = (e) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [isUploading])

  const blocker = useBlocker(isUploading)

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

  const handleUpload = async (e) => {
    const file = e.target.files[0]
    if (!file) return

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
        addAlert('Upload failed unexpectedly. Please try again.', 'error', { persistent: true })
        setTimeout(() => {
          setUploadStates((prev) => prev.filter((u) => u.id !== uploadId))
        }, 3000)
      }
    })
    uploadProgressCancelsRef.current.set(uploadId, cancel)
    const controller = new AbortController()
    uploadAbortControllersRef.current.set(uploadId, controller)

    try {
      await API.files.upload(storageId, currentPath.replace(/\/+$/, ''), file, uploadId, { signal: controller.signal })
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
    } catch (err) {
      addAlert(err.message, 'error')
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
    try {
      const path = (deleteTarget.path || deleteTarget.name).replace(/\/+$/, '')
      const deleteId = createOperationId()
      if (cancelDeleteProgressRef.current) cancelDeleteProgressRef.current()
      setDeleteState({
        label: deleteTarget?.name || path,
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
          setTimeout(() => setDeleteState(null), 1500)
        }
        if (data.status === 'error') {
          cancel()
          cancelDeleteProgressRef.current = null
          setTimeout(() => setDeleteState(null), 3000)
        }
      })
      cancelDeleteProgressRef.current = cancel

      await API.files.delete(storageId, path, deleteId)
      addAlert('Deleted', 'success')
      setDeleteTarget(null)
      loadTree()
    } catch (err) {
      if (cancelDeleteProgressRef.current) {
        cancelDeleteProgressRef.current()
        cancelDeleteProgressRef.current = null
      }
      setDeleteState((prev) => (prev ? { ...prev, status: 'error' } : null))
      setTimeout(() => setDeleteState(null), 3000)
      addAlert(err.message, 'error')
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
            <input type="file" hidden onChange={(e) => { close(); handleUpload(e) }} />
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
        onCancel={() => setDeleteTarget(null)}
      />

      <NavigationBlockDialog blocker={blocker} />

      <MoveDialog
        open={!!moveTarget}
        item={moveTarget}
        storageId={storageId}
        onMove={handleMove}
        onClose={() => setMoveTarget(null)}
      />
    </Box>
  )
}
