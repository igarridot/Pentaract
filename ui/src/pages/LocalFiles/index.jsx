import { useState, useEffect, useCallback } from 'react'
import { useBlocker } from 'react-router-dom'
import {
  Typography, List, Box, Divider, Button, Breadcrumbs,
  Link as MuiLink, Checkbox, FormControlLabel, TextField, MenuItem,
  ListItem, ListItemButton, ListItemIcon, ListItemText,
} from '@mui/material'
import {
  Folder as FolderIcon,
  InsertDriveFile as FileIcon,
  DriveFileMove as BrowseIcon,
} from '@mui/icons-material'
import API from '../../api'
import { useUploadManager } from '../../common/use_upload_manager'
import { useAlert } from '../../components/AlertStack'
import UploadProgress from '../../components/UploadProgress'
import BulkOperationProgress from '../../components/BulkOperationProgress'
import UploadConflictDialog from '../../components/UploadConflictDialog'
import NavigationBlockDialog from '../../components/NavigationBlockDialog'
import RemoteFolderPickerDialog from '../../components/RemoteFolderPickerDialog'
import { convertSize } from '../../common/size_converter'
import { normalizeUploadPath } from '../Files/upload_conflicts'
import { runSequentialUploadPipeline } from '../Files/operations.js'
import { buildLocalUploadEntries, normalizeLocalPath } from './entries'

export default function LocalFiles() {
  const addAlert = useAlert()
  const [storages, setStorages] = useState([])
  const [storageId, setStorageId] = useState('')
  const [items, setItems] = useState([])
  const [currentPath, setCurrentPath] = useState('')
  const [selectedPaths, setSelectedPaths] = useState([])
  const [targetPath, setTargetPath] = useState('')
  const [destinationPickerOpen, setDestinationPickerOpen] = useState(false)
  const [loadError, setLoadError] = useState('')

  useEffect(() => {
    let cancelled = false
    API.storages.list()
      .then((data) => {
        if (cancelled) return
        setStorages(data || [])
      })
      .catch((err) => {
        if (!cancelled) addAlert(err.message, 'error')
      })
    return () => { cancelled = true }
  }, [addAlert])

  const {
    uploadStates,
    bulkOperation,
    bulkMetrics,
    isUploading,
    isBulkUpload,
    runUploadBatch,
    cancelUpload,
    cancelBulkUpload,
    uploadConflictDialog,
    handleUploadConflictDecision,
    clearConflictCache,
  } = useUploadManager({
    storageId,
    addAlert,
    createRequest: (entry, uploadId, signal, onConflict) => API.localFiles.upload(
      storageId,
      entry.sourcePath,
      entry.targetPath,
      uploadId,
      { signal, onConflict },
    ),
    successMessage: (entry) => `Uploaded "${entry.filename}"`,
    skippedMessage: (entry) => `Skipped upload for "${entry.filename}"`,
    errorMessage: (entry) => `Upload failed unexpectedly for "${entry.filename}". Please try again.`,
    interruptedMessage: (_, err) => `Upload interrupted: ${err.message}`,
    pipelineRunner: runSequentialUploadPipeline,
  })

  const loadTree = useCallback(async () => {
    if (!storageId) {
      setItems([])
      setLoadError('')
      return
    }

    try {
      const data = await API.localFiles.tree(storageId, currentPath)
      setItems(data || [])
      setLoadError('')
    } catch (err) {
      setItems([])
      setLoadError(err.message)
    }
  }, [currentPath, storageId])

  useEffect(() => {
    setSelectedPaths([])
    loadTree()
  }, [loadTree])

  const hasActiveUpload = isUploading || isBulkUpload
  const blocker = useBlocker(hasActiveUpload)

  useEffect(() => {
    if (!hasActiveUpload) return

    const handler = (event) => {
      event.preventDefault()
      event.returnValue = 'Upload in progress. Leaving will cancel it.'
      return event.returnValue
    }

    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [hasActiveUpload])

  const handleUploadSelected = useCallback(async () => {
    if (!storageId) {
      addAlert('Select a storage first', 'error')
      return
    }
    if (selectedPaths.length === 0) {
      addAlert('Select at least one file or folder', 'error')
      return
    }

    let expandedFiles
    try {
      expandedFiles = await API.localFiles.expand(storageId, selectedPaths)
    } catch (err) {
      addAlert(err.message, 'error')
      return
    }

    if (!expandedFiles || expandedFiles.length === 0) {
      addAlert('No uploadable files were found in the selected items', 'info')
      return
    }

    await runUploadBatch(buildLocalUploadEntries(expandedFiles, currentPath, targetPath))
  }, [addAlert, currentPath, runUploadBatch, selectedPaths, storageId, targetPath])

  const toggleSelection = (itemPath) => {
    setSelectedPaths((prev) => (
      prev.includes(itemPath)
        ? prev.filter((path) => path !== itemPath)
        : [...prev, itemPath]
    ))
  }

  const allSelected = items.length > 0 && selectedPaths.length === items.length
  const selectedCount = selectedPaths.length
  const currentPathParts = normalizeLocalPath(currentPath).split('/').filter(Boolean)
  const {
    totalBytes: bulkTotalBytes,
    processedBytes: bulkProcessedBytes,
    totalChunks: bulkTotalChunks,
    processedChunks: bulkProcessedChunks,
    workersStatus: bulkWorkersStatus,
  } = bulkMetrics

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 1.5 }}>Local Files</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Browse the files mounted inside the Pentaract container and upload them into a storage without changing the existing browser or CLI flows.
      </Typography>

      {uploadStates.map((uploadState) => (
        <UploadProgress
          key={uploadState.id}
          filename={uploadState.filename}
          totalBytes={uploadState.totalBytes}
          uploadedBytes={uploadState.uploadedBytes}
          totalChunks={uploadState.totalChunks}
          uploadedChunks={uploadState.uploadedChunks}
          verificationTotal={uploadState.verificationTotal}
          verifiedChunks={uploadState.verifiedChunks}
          status={uploadState.status}
          workersStatus={uploadState.workersStatus}
          onCancel={() => cancelUpload(uploadState.id)}
        />
      ))}

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
          onCancel={bulkOperation.status === 'running' ? cancelBulkUpload : null}
        />
      )}

      <Box sx={{
        mb: 2.5,
        p: 2,
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 3,
        bgcolor: 'background.paper',
        display: 'grid',
        gridTemplateColumns: { xs: '1fr', md: 'minmax(240px, 320px) minmax(220px, 1fr) auto' },
        gap: 1.5,
        alignItems: 'center',
      }}>
        <TextField
          select
          label="Storage"
          value={storageId}
          onChange={(event) => {
            setStorageId(event.target.value)
            clearConflictCache()
          }}
          size="small"
          fullWidth
        >
          <MenuItem value="">Select a storage</MenuItem>
          {storages.map((storage) => (
            <MenuItem key={storage.id} value={storage.id}>{storage.name}</MenuItem>
          ))}
        </TextField>

        <TextField
          label="Destination path"
          value={targetPath}
          onChange={(event) => setTargetPath(normalizeUploadPath(event.target.value))}
          size="small"
          fullWidth
          placeholder="Root"
        />

        <Button
          variant="outlined"
          startIcon={<BrowseIcon />}
          disabled={!storageId}
          onClick={() => setDestinationPickerOpen(true)}
        >
          Browse destination
        </Button>
      </Box>

      <Breadcrumbs sx={{ mb: 2 }}>
        <MuiLink
          underline="hover"
          color="inherit"
          sx={{ cursor: 'pointer', fontSize: '0.8125rem' }}
          onClick={() => setCurrentPath('')}
        >
          Mounted root
        </MuiLink>
        {currentPathParts.map((part, index) => {
          const pathTo = currentPathParts.slice(0, index + 1).join('/')
          return (
            <MuiLink
              key={pathTo}
              underline="hover"
              color="inherit"
              sx={{ cursor: 'pointer', fontSize: '0.8125rem' }}
              onClick={() => setCurrentPath(pathTo)}
            >
              {part}
            </MuiLink>
          )
        })}
      </Breadcrumbs>

      <Box sx={{ mb: 1.5, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
        <FormControlLabel
          control={(
            <Checkbox
              checked={allSelected}
              indeterminate={selectedCount > 0 && !allSelected}
              onChange={() => {
                if (allSelected) {
                  setSelectedPaths([])
                  return
                }
                setSelectedPaths(items.map((item) => item.path))
              }}
            />
          )}
          label={`Select all visible items (${items.length})`}
        />
        <Button size="small" onClick={() => setSelectedPaths([])} disabled={selectedCount === 0}>
          Clear
        </Button>
        <Button
          size="small"
          variant="contained"
          onClick={handleUploadSelected}
          disabled={selectedCount === 0 || !storageId || hasActiveUpload}
        >
          Upload selected
        </Button>
        {selectedCount > 0 && (
          <Typography variant="body2" color="text.secondary">
            {`${selectedCount} selected`}
          </Typography>
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
          {items.map((item, index) => (
            <Box key={item.path}>
              {index > 0 && <Divider />}
              <ListItem
                disablePadding
                secondaryAction={(
                  <Checkbox
                    checked={selectedPaths.includes(item.path)}
                    onChange={() => toggleSelection(item.path)}
                    onClick={(event) => event.stopPropagation()}
                    size="small"
                  />
                )}
              >
                <ListItemButton
                  onClick={() => {
                    if (item.is_file) {
                      toggleSelection(item.path)
                      return
                    }
                    setCurrentPath(item.path)
                  }}
                  sx={{ py: 1 }}
                >
                  <ListItemIcon sx={{ minWidth: 40 }}>
                    {item.is_file
                      ? <FileIcon sx={{ color: 'text.secondary', fontSize: 20 }} />
                      : <FolderIcon sx={{ color: 'primary.main', fontSize: 20 }} />}
                  </ListItemIcon>
                  <ListItemText
                    primary={item.name}
                    secondary={item.is_file ? convertSize(item.size || 0) : 'Folder'}
                    primaryTypographyProps={{ fontSize: '0.875rem', fontWeight: item.is_file ? 400 : 500 }}
                    secondaryTypographyProps={{ fontSize: '0.75rem' }}
                  />
                </ListItemButton>
              </ListItem>
            </Box>
          ))}

          {!storageId && (
            <Box sx={{ p: 4, textAlign: 'center' }}>
              <Typography color="text.secondary" variant="body2">
                Select a storage to browse the mounted local files.
              </Typography>
            </Box>
          )}

          {storageId && items.length === 0 && (
            <Box sx={{ p: 4, textAlign: 'center' }}>
              <Typography color="text.secondary" variant="body2">
                {loadError || 'Empty folder'}
              </Typography>
            </Box>
          )}
        </List>
      </Box>

      <RemoteFolderPickerDialog
        open={destinationPickerOpen}
        storageId={storageId}
        initialPath={targetPath}
        title="Upload selected files"
        confirmLabel="Use this folder"
        onConfirm={(path) => {
          setTargetPath(path)
          setDestinationPickerOpen(false)
        }}
        onClose={() => setDestinationPickerOpen(false)}
      />

      <UploadConflictDialog
        open={uploadConflictDialog.open}
        filename={uploadConflictDialog.filename}
        targetPath={uploadConflictDialog.targetPath}
        applyForAll={uploadConflictDialog.applyForAll}
        onApplyForAllChange={(checked) => setUploadConflictDialog((prev) => ({ ...prev, applyForAll: checked }))}
        onDecision={handleUploadConflictDecision}
      />

      <NavigationBlockDialog
        blocker={blocker}
        isUploading={isUploading || isBulkUpload}
        isDownloading={false}
        isDeleting={false}
        isMoving={false}
      />
    </Box>
  )
}
