import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate, useLocation, useBlocker } from 'react-router-dom'
import {
  Typography, List, Paper, Box, TextField, InputAdornment,
  MenuItem, Divider, Breadcrumbs, Link as MuiLink, Button,
} from '@mui/material'
import {
  Search as SearchIcon,
  CreateNewFolder as FolderAddIcon,
  Upload as UploadIcon,
} from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import FSListItem from '../../components/FSListItem'
import FileInfo from '../../components/FileInfo'
import CreateFolderDialog from '../../components/CreateFolderDialog'
import ActionConfirmDialog from '../../components/ActionConfirmDialog'
import NavigationBlockDialog from '../../components/NavigationBlockDialog'
import FloatingMenu from '../../components/Menu'
import UploadProgress from '../../components/UploadProgress'

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
  const [folderDialogOpen, setFolderDialogOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [uploadState, setUploadState] = useState(null)
  const cancelProgressRef = useRef(null)
  const uploadIdRef = useRef(null)

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

  // Cleanup SSE on unmount
  useEffect(() => {
    return () => {
      if (cancelProgressRef.current) cancelProgressRef.current()
    }
  }, [])

  // Warn before closing/navigating away during upload
  const isUploading = uploadState?.status === 'uploading'

  useEffect(() => {
    if (!isUploading) return
    const handler = (e) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [isUploading])

  // Block SPA navigation during upload
  const blocker = useBlocker(isUploading)

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
    const uploadId = crypto.randomUUID()
    uploadIdRef.current = uploadId
    setUploadState({ filename, totalBytes: file.size, uploadedBytes: 0, totalChunks: 0, uploadedChunks: 0, status: 'uploading' })

    // Subscribe to SSE progress immediately (before the upload POST completes)
    const cancel = API.files.subscribeProgress(uploadId, (data) => {
      setUploadState((prev) => ({
        ...prev,
        filename,
        totalBytes: data.total_bytes || prev?.totalBytes || file.size,
        uploadedBytes: data.uploaded_bytes || 0,
        totalChunks: data.total || prev?.totalChunks || 0,
        uploadedChunks: data.uploaded || 0,
        status: data.status,
      }))
      if (data.status === 'done') {
        uploadIdRef.current = null
        addAlert('File uploaded', 'success')
        loadTree()
        setTimeout(() => setUploadState(null), 2000)
      }
      if (data.status === 'error') {
        uploadIdRef.current = null
        addAlert('Upload failed', 'error')
        setTimeout(() => setUploadState(null), 3000)
      }
    })
    cancelProgressRef.current = cancel

    // Start upload (this blocks until file body is fully sent)
    try {
      await API.files.upload(storageId, currentPath, file, uploadId)
    } catch (err) {
      if (uploadIdRef.current === uploadId) {
        setUploadState(null)
        uploadIdRef.current = null
        addAlert(err.message, 'error')
      }
    }
    e.target.value = ''
  }

  const handleCancelUpload = async () => {
    const id = uploadIdRef.current
    if (!id) return
    try {
      if (cancelProgressRef.current) cancelProgressRef.current()
      await API.files.cancelUpload(id)
      addAlert('Upload cancelled', 'info')
    } catch (err) {
      addAlert(err.message, 'error')
    }
    setUploadState(null)
    uploadIdRef.current = null
    loadTree()
  }

  const handleDownload = async (item) => {
    try {
      const blob = await API.files.download(storageId, item.path)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = item.name
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const handleDelete = async () => {
    try {
      const path = deleteTarget.path || deleteTarget.name
      await API.files.delete(storageId, path)
      addAlert('Deleted', 'success')
      setDeleteTarget(null)
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
      sx={{ cursor: 'pointer' }}
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
          sx={{ cursor: 'pointer' }}
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

      {uploadState && (
        <UploadProgress
          filename={uploadState.filename}
          totalBytes={uploadState.totalBytes}
          uploadedBytes={uploadState.uploadedBytes}
          totalChunks={uploadState.totalChunks}
          uploadedChunks={uploadState.uploadedChunks}
          status={uploadState.status}
          onCancel={handleCancelUpload}
        />
      )}

      <Box component="form" onSubmit={handleSearch} sx={{ mb: 2 }}>
        <TextField
          size="small"
          placeholder="Search files..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start"><SearchIcon /></InputAdornment>
            ),
          }}
        />
        {searchResults && (
          <Button size="small" onClick={() => { setSearchResults(null); setSearch('') }} sx={{ ml: 1 }}>
            Clear
          </Button>
        )}
      </Box>

      <Paper variant="outlined">
        <List>
          {displayItems.map((item, i) => (
            <Box key={item.path || item.name}>
              {i > 0 && <Divider />}
              <FSListItem
                item={item}
                storageId={storageId}
                currentPath={currentPath}
                onInfo={setInfoFile}
                onDelete={setDeleteTarget}
                onDownload={handleDownload}
              />
            </Box>
          ))}
          {displayItems.length === 0 && (
            <Box sx={{ p: 2, textAlign: 'center' }}>
              <Typography color="text.secondary">
                {searchResults ? 'No results found' : 'Empty folder'}
              </Typography>
            </Box>
          )}
        </List>
      </Paper>

      <FloatingMenu>
        {(close) => [
          <MenuItem key="folder" onClick={() => { close(); setFolderDialogOpen(true) }}>
            <FolderAddIcon sx={{ mr: 1 }} /> New Folder
          </MenuItem>,
          <MenuItem key="upload" component="label">
            <UploadIcon sx={{ mr: 1 }} /> Upload File
            <input type="file" hidden onChange={(e) => { close(); handleUpload(e) }} />
          </MenuItem>,
        ]}
      </FloatingMenu>

      <FileInfo file={infoFile} open={!!infoFile} onClose={() => setInfoFile(null)} />

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
    </Box>
  )
}
