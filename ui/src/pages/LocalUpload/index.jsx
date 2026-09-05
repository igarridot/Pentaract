import { useState, useEffect, useCallback } from 'react'
import {
  Box, Typography, Button,
  Select, MenuItem, TextField, FormControl, InputLabel,
  Alert,
} from '@mui/material'
import {
  CloudUpload as CloudUploadIcon,
  FolderOpen as FolderOpenIcon,
} from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import PathBreadcrumbs from '../../components/PathBreadcrumbs'
import UploadProgress from '../../components/UploadProgress'
import BulkOperationProgress from '../../components/BulkOperationProgress'
import FolderBrowserDialog from '../../components/FolderBrowserDialog'
import NavigationBlockDialog from '../../components/NavigationBlockDialog'
import UploadConflictDialog from '../../components/UploadConflictDialog'
import { useNavigationBlock } from '../Files/useNavigationBlock'
import { useLocalUploads } from './useLocalUploads'
import LocalFsBrowser from './LocalFsBrowser'
import { sortLocalEntries, localEntryKey, buildLocalBatchItems } from './local_upload_paths'

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
  const {
    uploadStates, isUploading, launchLocalBatch, cancelUpload,
    resolveLocalConflicts,
    conflictDialog, setConflictDialog, handleConflictDecision,
  } = useLocalUploads(addAlert)
  const [batchUploading, setBatchUploading] = useState(false)

  // Destination folder picker
  const [folderDialogOpen, setFolderDialogOpen] = useState(false)

  // Navigation blocking while uploads are active
  const { blocker } = useNavigationBlock({
    hasActiveFileOperation: isUploading || batchUploading,
    isDeleting: false,
    isBulkDelete: false,
  })

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
      setEntries(sortLocalEntries(await API.localFs.browse(path)))
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

  const pathParts = browsePath.split('/').filter(Boolean)

  // Selection handlers
  const toggleSelect = (entry) => {
    setSelected((prev) => {
      const next = new Set(prev)
      const key = localEntryKey(entry)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  const allSelected = entries.length > 0 && entries.every((e) => selected.has(localEntryKey(e)))

  const toggleSelectAll = () => {
    if (allSelected) {
      setSelected(new Set())
    } else {
      setSelected(new Set(entries.map(localEntryKey)))
    }
  }

  // Upload selected items
  const handleUpload = async () => {
    if (!storageId || selected.size === 0) return
    setBatchUploading(true)

    try {
      const selectedEntries = entries.filter((e) => selected.has(localEntryKey(e)))
      const allFiles = await buildLocalBatchItems(selectedEntries, browsePath, destPath, API.localFs.browse)

      if (allFiles.length === 0) {
        addAlert('No files found in selection', 'info')
        setBatchUploading(false)
        return
      }

      // Check for conflicts before uploading (same UX as browser uploads)
      const resolved = await resolveLocalConflicts(storageId, allFiles)
      if (resolved.length === 0) {
        setBatchUploading(false)
        return
      }

      // Map resolved entries back to batch items
      const itemsToUpload = resolved.map((e) => ({
        local_path: e.local_path,
        dest_path: e.dest_path,
      }))

      if (itemsToUpload.length > 1) {
        setBulkProgress({ operation: 'upload', status: 'running', total: itemsToUpload.length, completed: 0 })
      }

      await launchLocalBatch(storageId, itemsToUpload, 'keep_both')

      if (itemsToUpload.length > 1) {
        setBulkProgress((prev) => prev ? { ...prev, status: 'done', completed: itemsToUpload.length } : prev)
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
      <PathBreadcrumbs
        parts={pathParts}
        sx={{ mb: 2 }}
        onSelect={(parts) => browse(parts.length ? `/${parts.join('/')}` : '')}
      />

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

      <LocalFsBrowser
        entries={entries}
        selected={selected}
        loading={loading}
        onToggle={toggleSelect}
        onOpenDir={browse}
      />

      <UploadConflictDialog
        open={conflictDialog.open}
        filename={conflictDialog.filename}
        targetPath={conflictDialog.targetPath}
        applyForAll={conflictDialog.applyForAll}
        onApplyForAllChange={(checked) => setConflictDialog((prev) => ({ ...prev, applyForAll: checked }))}
        onDecision={handleConflictDecision}
      />

      <NavigationBlockDialog
        blocker={blocker}
        isUploading={isUploading || batchUploading}
      />
    </Box>
  )
}
