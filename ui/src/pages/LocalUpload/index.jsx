import { useState, useEffect, useCallback } from 'react'
import {
  Box, Typography, List, ListItem, ListItemIcon, ListItemText,
  Checkbox, Button, Breadcrumbs, Link as MuiLink,
  Select, MenuItem, TextField, FormControl, InputLabel,
  CircularProgress, Alert,
} from '@mui/material'
import {
  Folder as FolderIcon,
  InsertDriveFile as FileIcon,
  CloudUpload as CloudUploadIcon,
  FolderOpen as FolderOpenIcon,
} from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import { convertSize } from '../../common/size_converter'
import UploadProgress from '../../components/UploadProgress'
import BulkOperationProgress from '../../components/BulkOperationProgress'
import FolderBrowserDialog from '../../components/FolderBrowserDialog'
import { useLocalUploads } from './useLocalUploads'

export default function LocalUpload() {
  const addAlert = useAlert()

  // Storage selection
  const [storages, setStorages] = useState([])
  const [storageId, setStorageId] = useState('')
  const [destPath, setDestPath] = useState('')

  // Filesystem browser
  const [browsePath, setBrowsePath] = useState('')
  const [entries, setEntries] = useState([])
  const [loading, setLoading] = useState(false)
  const [notConfigured, setNotConfigured] = useState(false)

  // Selection
  const [selected, setSelected] = useState(new Set())

  // Uploads
  const { uploadStates, isUploading, launchLocalBatch, cancelUpload } = useLocalUploads(addAlert)
  const [batchUploading, setBatchUploading] = useState(false)

  // Destination folder picker
  const [folderDialogOpen, setFolderDialogOpen] = useState(false)

  // Bulk progress
  const [bulkProgress, setBulkProgress] = useState(null)

  // Load storages on mount
  useEffect(() => {
    let cancelled = false
    API.storages.list()
      .then((data) => {
        if (!cancelled) setStorages(data || [])
      })
      .catch(() => {
        if (!cancelled) setStorages([])
      })
    return () => { cancelled = true }
  }, [])

  // Browse directory
  const browse = useCallback(async (path) => {
    setLoading(true)
    setNotConfigured(false)
    try {
      const data = await API.localFs.browse(path)
      setEntries(data || [])
      setBrowsePath(path)
      setSelected(new Set())
    } catch (err) {
      if (err.message && (err.message.includes('403') || err.message.toLowerCase().includes('not configured') || err.message.toLowerCase().includes('forbidden'))) {
        setNotConfigured(true)
        setEntries([])
      } else {
        addAlert(err.message, 'error')
      }
    } finally {
      setLoading(false)
    }
  }, [addAlert])

  // Browse root on mount
  useEffect(() => {
    browse('')
  }, [browse])

  // Breadcrumb parts
  const pathParts = browsePath ? browsePath.replace(/^\/+/, '').replace(/\/+$/, '').split('/').filter(Boolean) : []

  const navigateTo = (path) => {
    browse(path)
  }

  // Selection handlers
  const toggleSelect = (entry) => {
    setSelected((prev) => {
      const next = new Set(prev)
      const key = entry.path || entry.name
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  const allSelected = entries.length > 0 && entries.every((e) => selected.has(e.path || e.name))

  const toggleSelectAll = () => {
    if (allSelected) {
      setSelected(new Set())
    } else {
      setSelected(new Set(entries.map((e) => e.path || e.name)))
    }
  }

  // Recursively collect all files from a directory
  const collectFiles = async (dirPath) => {
    const result = []
    try {
      const items = await API.localFs.browse(dirPath)
      for (const item of (items || [])) {
        if (item.is_dir) {
          const subFiles = await collectFiles(item.path)
          result.push(...subFiles)
        } else {
          result.push(item)
        }
      }
    } catch {
      // skip inaccessible directories
    }
    return result
  }

  // Build relative path from base
  const relativePath = (filePath, basePath) => {
    if (!basePath) return filePath
    const base = basePath.endsWith('/') ? basePath : basePath + '/'
    if (filePath.startsWith(base)) {
      return filePath.slice(base.length)
    }
    return filePath
  }

  // Upload selected items
  const handleUpload = async () => {
    if (!storageId || selected.size === 0) return
    setBatchUploading(true)

    try {
      // Collect all files (recursing into directories)
      const allFiles = []
      for (const key of selected) {
        const entry = entries.find((e) => (e.path || e.name) === key)
        if (!entry) continue

        if (entry.is_dir) {
          const dirFiles = await collectFiles(entry.path)
          for (const f of dirFiles) {
            const rel = relativePath(f.path, browsePath)
            const dp = destPath ? (destPath.replace(/\/+$/, '') + '/' + rel) : rel
            allFiles.push({ local_path: f.path, dest_path: dp })
          }
        } else {
          const rel = relativePath(entry.path, browsePath)
          const dp = destPath ? (destPath.replace(/\/+$/, '') + '/' + rel) : rel
          allFiles.push({ local_path: entry.path, dest_path: dp })
        }
      }

      if (allFiles.length === 0) {
        addAlert('No files found in selection', 'info')
        setBatchUploading(false)
        return
      }

      if (allFiles.length > 1) {
        setBulkProgress({ operation: 'upload', status: 'running', total: allFiles.length, completed: 0 })
      }

      await launchLocalBatch(storageId, allFiles, 'keep_both')

      if (allFiles.length > 1) {
        setBulkProgress((prev) => prev ? { ...prev, status: 'done', completed: allFiles.length } : prev)
        setTimeout(() => setBulkProgress(null), 3000)
      }
    } catch (err) {
      addAlert(`Upload failed: ${err.message}`, 'error')
      setBulkProgress((prev) => prev ? { ...prev, status: 'error' } : prev)
      setTimeout(() => setBulkProgress(null), 3000)
    } finally {
      setBatchUploading(false)
    }
  }

  const canUpload = storageId && selected.size > 0 && !batchUploading && !isUploading

  // Not configured state
  if (notConfigured) {
    return (
      <Box sx={{ p: 3 }}>
        <Typography variant="h5" sx={{ mb: 2 }}>Local Upload</Typography>
        <Alert severity="warning" sx={{ maxWidth: 600 }}>
          Local filesystem uploads are not configured. Set <strong>LOCAL_UPLOAD_BASE_PATH</strong> environment variable.
        </Alert>
      </Box>
    )
  }

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 3 }}>Local Upload</Typography>

      {/* Section A: Storage & Destination */}
      <Box sx={{ display: 'flex', gap: 2, mb: 3, flexWrap: 'wrap' }}>
        <FormControl size="small" sx={{ minWidth: 200 }}>
          <InputLabel>Storage</InputLabel>
          <Select
            value={storageId}
            label="Storage"
            onChange={(e) => setStorageId(e.target.value)}
          >
            {storages.length === 0 && (
              <MenuItem value="" disabled>No storages available</MenuItem>
            )}
            {storages.map((s) => (
              <MenuItem key={s.id} value={s.id}>{s.name}</MenuItem>
            ))}
          </Select>
        </FormControl>
        <TextField
          size="small"
          label="Destination path"
          placeholder="/ (root)"
          value={destPath}
          onChange={(e) => setDestPath(e.target.value)}
          sx={{ minWidth: 200 }}
        />
        <Button
          variant="outlined"
          size="small"
          startIcon={<FolderOpenIcon />}
          disabled={!storageId}
          onClick={() => setFolderDialogOpen(true)}
          sx={{ height: 40 }}
        >
          Browse
        </Button>
      </Box>

      <FolderBrowserDialog
        open={folderDialogOpen}
        title="Choose destination folder"
        storageId={storageId}
        onClose={() => setFolderDialogOpen(false)}
        actionLabel="Select"
        onConfirm={(path) => {
          setDestPath(path)
          setFolderDialogOpen(false)
        }}
      />

      {/* Upload progress */}
      {uploadStates.map((u) => (
        <UploadProgress
          key={u.id}
          filename={u.filename}
          totalBytes={u.totalBytes}
          uploadedBytes={u.uploadedBytes}
          totalChunks={u.totalChunks}
          uploadedChunks={u.uploadedChunks}
          verificationTotal={u.verificationTotal}
          verifiedChunks={u.verifiedChunks}
          status={u.status}
          workersStatus={u.workersStatus}
          onCancel={() => cancelUpload(u.id)}
        />
      ))}
      {bulkProgress && (
        <BulkOperationProgress
          operation={bulkProgress.operation}
          status={bulkProgress.status}
          total={bulkProgress.total}
          completed={bulkProgress.completed}
        />
      )}

      {/* Section B: Local Filesystem Browser */}
      <Breadcrumbs sx={{ mb: 2 }}>
        <MuiLink
          underline="hover"
          color="inherit"
          sx={{ cursor: 'pointer', fontSize: '0.8125rem' }}
          onClick={() => navigateTo('')}
        >
          Root
        </MuiLink>
        {pathParts.map((part, i) => {
          const pathTo = '/' + pathParts.slice(0, i + 1).join('/')
          return (
            <MuiLink
              key={pathTo}
              underline="hover"
              color="inherit"
              sx={{ cursor: 'pointer', fontSize: '0.8125rem' }}
              onClick={() => navigateTo(pathTo)}
            >
              {part}
            </MuiLink>
          )
        })}
      </Breadcrumbs>

      {/* Select all + upload button */}
      {entries.length > 0 && (
        <Box sx={{ mb: 1.5, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
          <Button size="small" onClick={toggleSelectAll}>
            {allSelected ? 'Deselect All' : 'Select All'}
          </Button>
          {selected.size > 0 && (
            <Typography variant="body2" color="text.secondary">
              {selected.size} selected
            </Typography>
          )}
          <Box sx={{ flexGrow: 1 }} />
          <Button
            variant="contained"
            startIcon={<CloudUploadIcon />}
            disabled={!canUpload}
            onClick={handleUpload}
          >
            Upload Selected
          </Button>
        </Box>
      )}

      {loading ? (
        <Box sx={{ p: 4, textAlign: 'center' }}>
          <CircularProgress size={28} />
        </Box>
      ) : (
        <Box sx={{
          bgcolor: 'background.paper',
          borderRadius: 3,
          border: '1px solid',
          borderColor: 'divider',
          overflow: 'hidden',
        }}>
          <List disablePadding>
            {entries.map((entry) => {
              const key = entry.path || entry.name
              const isDir = entry.is_dir
              return (
                <ListItem
                  key={key}
                  sx={{
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                    '&:last-child': { borderBottom: 'none' },
                    cursor: isDir ? 'pointer' : 'default',
                  }}
                  secondaryAction={
                    !isDir && entry.size != null ? (
                      <Typography variant="caption" color="text.secondary">
                        {convertSize(entry.size)}
                      </Typography>
                    ) : null
                  }
                >
                  <Checkbox
                    edge="start"
                    checked={selected.has(key)}
                    onChange={() => toggleSelect(entry)}
                    sx={{ mr: 1 }}
                  />
                  <ListItemIcon
                    sx={{ minWidth: 36, cursor: isDir ? 'pointer' : 'default' }}
                    onClick={() => isDir && navigateTo(entry.path)}
                  >
                    {isDir ? <FolderIcon color="primary" /> : <FileIcon color="action" />}
                  </ListItemIcon>
                  <ListItemText
                    primary={entry.name}
                    onClick={() => isDir && navigateTo(entry.path)}
                    sx={{ cursor: isDir ? 'pointer' : 'default' }}
                    primaryTypographyProps={{ fontSize: '0.875rem' }}
                  />
                </ListItem>
              )
            })}
            {entries.length === 0 && (
              <Box sx={{ p: 4, textAlign: 'center' }}>
                <Typography color="text.secondary" variant="body2">
                  Empty directory
                </Typography>
              </Box>
            )}
          </List>
        </Box>
      )}
    </Box>
  )
}
