import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
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
import { getItemPath, getMediaType, buildRenamedPath, getBulkOperationMetrics } from './operations'
import { useFileNavigation } from './useFileNavigation'
import { useUploads } from './useUploads'
import { useDownloads } from './useDownloads'
import { useDeleteOperation } from './useDeleteOperation'
import { useBulkOperations } from './useBulkOperations'
import { useNavigationBlock } from './useNavigationBlock'

export default function Files() {
  const navigate = useNavigate()
  const addAlert = useAlert()

  const [infoFile, setInfoFile] = useState(null)
  const [previewFile, setPreviewFile] = useState(null)
  const [folderDialogOpen, setFolderDialogOpen] = useState(false)
  const [moveTarget, setMoveTarget] = useState(null)
  const [renameTarget, setRenameTarget] = useState(null)

  // Hook 1: File navigation (path, items, search)
  // We pass onPathChange later via updateDirCache from uploads
  const nav = useFileNavigation(addAlert)
  const {
    storageId, prefix, currentPath, pathParts,
    items, search, setSearch, searchResults, setSearchResults,
    loadTree, handleSearch,
  } = nav

  // Hook 4: Bulk operations (needs uploadStates/downloadStates for metrics — passed each render)
  // Callbacks are stable and don't depend on transfer states.
  const bulk = useBulkOperations(addAlert, storageId, loadTree)
  const {
    selectedFilePaths, setSelectedFilePaths,
    bulkDeleteOpen, setBulkDeleteOpen,
    bulkMoveOpen, setBulkMoveOpen,
    bulkOperation, setBulkOperation, bulkCancelRef,
    isBulkOperating, isBulkUpload, isBulkDownload, isBulkDelete, isBulkMove,
    registerBulkTransfer, markBulkTransferTerminal, finalizeBulkTransferLaunch,
    handleBulkDownload, handleBulkDelete, handleBulkMove,
  } = bulk

  // Hook 2: Uploads
  const uploads = useUploads(addAlert, storageId, currentPath, loadTree, {
    registerBulkTransfer,
    markBulkTransferTerminal,
    finalizeBulkTransferLaunch,
    setBulkOperation,
    bulkCancelRef,
  })
  const {
    uploadStates, isUploading,
    startUpload, cancelUpload, cleanupUploads,
    uploadConflictDialog, setUploadConflictDialog, handleUploadConflictDecision,
    updateDirCache,
  } = uploads

  // Hook 3: Downloads
  const downloads = useDownloads(addAlert, storageId, loadTree, {
    markBulkTransferTerminal,
  })
  const {
    downloadStates, downloadStatesRef, isDownloading,
    startDownload, cancelDownload, cleanupDownloads, releaseDownloadTracking,
  } = downloads

  // Hook 5: Delete
  const del = useDeleteOperation(addAlert, storageId, loadTree)
  const {
    deleteTarget, setDeleteTarget,
    forceDelete, setForceDelete,
    deleteState, isDeleting,
    confirmDelete, cleanupDelete,
  } = del

  // Derived flags
  const hasActiveFileOperation = isUploading || isDownloading || isDeleting || isBulkOperating

  // Hook 6: Navigation blocking
  const { blocker } = useNavigationBlock({ hasActiveFileOperation, isDeleting, isBulkDelete })

  // Wire navigation's onPathChange to upload's dir cache
  // (useFileNavigation calls onPathChange on loadTree; we update cache + reset selection)
  // Since we can't change the callback ref after hook init, we use an effect approach:
  // The navigation hook passes data via loadTree -> setItems. We handle cache update here.
  useEffect(() => {
    if (items.length > 0 || currentPath !== undefined) {
      updateDirCache(currentPath, items)
    }
  }, [items, currentPath, updateDirCache])

  // Reset selection on path change
  useEffect(() => {
    setSelectedFilePaths([])
  }, [loadTree])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      bulk.cleanupBulk()
      cleanupUploads()
      cleanupDownloads()
      cleanupDelete()
    }
  }, [])

  // Compute bulk metrics with actual current transfer states
  const {
    totalBytes: bulkTotalBytes,
    processedBytes: bulkProcessedBytes,
    totalChunks: bulkTotalChunks,
    processedChunks: bulkProcessedChunks,
    workersStatus: bulkWorkersStatus,
  } = getBulkOperationMetrics(bulkOperation, uploadStates, downloadStates)

  // --- Local handlers that compose hooks ---
  const handleCreateFolder = async (name) => {
    try {
      await API.files.createFolder(storageId, currentPath, name)
      addAlert('Folder created', 'success')
      loadTree()
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const handlePreview = (item) => {
    const mediaType = getMediaType(item.name)
    if (!mediaType) {
      startDownload(item)
      return
    }
    setPreviewFile(item)
  }

  const handleMove = async (item, newPath) => {
    try {
      await API.files.move(storageId, getItemPath(item), newPath)
      addAlert('Moved successfully', 'success')
      setMoveTarget(null)
      loadTree()
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const handleRename = async (item, newName) => {
    try {
      const sourcePath = getItemPath(item)
      const targetPath = buildRenamedPath(item, newName)
      await API.files.move(storageId, sourcePath, targetPath)
      addAlert('Folder renamed', 'success')
      setRenameTarget(null)
      loadTree()
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  // --- Display items and selection ---
  const displayItems = searchResults || items
  const selectableFiles = displayItems.filter((item) => item.is_file)
  const selectedFiles = selectableFiles.filter((item) => selectedFilePaths.includes(item.path))
  const allFilesSelected = selectableFiles.length > 0 && selectedFiles.length === selectableFiles.length

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

  // --- Breadcrumbs ---
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
          verificationTotal={uploadState.verificationTotal}
          verifiedChunks={uploadState.verifiedChunks}
          status={uploadState.status}
          workersStatus={uploadState.workersStatus}
          onCancel={() => cancelUpload(uploadState.id)}
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
          errorMessage={downloadState.errorMessage}
          onCancel={() => cancelDownload(downloadState.id)}
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
          <Button
            size="small"
            onClick={() => handleBulkDownload(selectedFiles, { startDownload, downloadStatesRef, releaseDownloadTracking })}
            disabled={selectedFiles.length === 0 || isBulkOperating}
          >
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
                onDownload={startDownload}
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
            <input type="file" hidden multiple onChange={(e) => { close(); startUpload(e) }} />
          </MenuItem>,
          <MenuItem key="upload-folder" component="label">
            <FolderUploadIcon sx={{ mr: 1.5, fontSize: 18, color: 'text.secondary' }} /> Upload Folder
            <input
              type="file"
              hidden
              multiple
              webkitdirectory=""
              directory=""
              onChange={(e) => { close(); startUpload(e) }}
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
          startDownload(file)
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
        onConfirm={confirmDelete}
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
        onConfirm={() => handleBulkDelete(selectedFiles)}
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
        onConfirm={(targetPath) => handleBulkMove(targetPath, selectedFiles)}
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
