import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Typography, Box, TextField, InputAdornment,
  MenuItem, Button, FormControlLabel, Checkbox,
} from '@mui/material'
import {
  Search as SearchIcon,
  CreateNewFolder as FolderAddIcon,
  Upload as UploadIcon,
  DriveFolderUpload as FolderUploadIcon,
} from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import { useApiAction } from '../../common/use_api_action'
import FileInfo from '../../components/FileInfo'
import CreateFolderDialog from '../../components/CreateFolderDialog'
import ActionConfirmDialog from '../../components/ActionConfirmDialog'
import NavigationBlockDialog from '../../components/NavigationBlockDialog'
import FloatingMenu from '../../components/Menu'
import MoveDialog from '../../components/MoveDialog'
import MediaPreviewDialog from '../../components/MediaPreviewDialog'
import RenameFolderDialog from '../../components/RenameFolderDialog'
import UploadConflictDialog from '../../components/UploadConflictDialog'
import FileBreadcrumbs from './FileBreadcrumbs'
import TransferProgressStack from './TransferProgressStack'
import FileSelectionToolbar from './FileSelectionToolbar'
import FileList from './FileList'
import { getItemPath, getMediaType, buildRenamedPath, getBulkOperationMetrics } from './operations'
import { useFileNavigation } from './useFileNavigation'
import { useUploads } from './useUploads'
import { useDownloads } from './useDownloads'
import { useDeleteOperation } from './useDeleteOperation'
import { useBulkOperations } from './useBulkOperations'
import { useFileSelection } from './useFileSelection'
import { useNavigationBlock } from './useNavigationBlock'

export default function Files() {
  const navigate = useNavigate()
  const addAlert = useAlert()
  const run = useApiAction(addAlert)

  const [infoFile, setInfoFile] = useState(null)
  const [previewFile, setPreviewFile] = useState(null)
  const [folderDialogOpen, setFolderDialogOpen] = useState(false)
  const [moveTarget, setMoveTarget] = useState(null)
  const [renameTarget, setRenameTarget] = useState(null)

  // Navigation: current path, listing and search.
  const {
    storageId, prefix, currentPath, pathParts,
    items, search, setSearch, searchResults, setSearchResults,
    loadTree, handleSearch,
  } = useFileNavigation(addAlert)

  const displayItems = searchResults || items
  const {
    selectedFilePaths, selectableFiles, selectedFiles, allFilesSelected,
    clearSelection, toggleFileSelection, toggleSelectAllFiles,
  } = useFileSelection(displayItems, loadTree)

  // Bulk operations read upload/download states for metrics each render.
  const bulk = useBulkOperations(addAlert, storageId, loadTree, clearSelection)
  const {
    bulkDeleteOpen, setBulkDeleteOpen,
    bulkMoveOpen, setBulkMoveOpen,
    bulkOperation, setBulkOperation, bulkCancelRef,
    isBulkOperating, isBulkUpload, isBulkDownload, isBulkDelete, isBulkMove,
    registerBulkTransfer, markBulkTransferTerminal, finalizeBulkTransferLaunch,
    handleBulkDownload, handleBulkDelete, handleBulkMove,
  } = bulk

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

  const downloads = useDownloads(addAlert, storageId, loadTree, {
    markBulkTransferTerminal,
  })
  const {
    downloadStates, downloadStatesRef, isDownloading,
    startDownload, cancelDownload, cleanupDownloads, releaseDownloadTracking,
  } = downloads

  const del = useDeleteOperation(addAlert, storageId, loadTree)
  const {
    deleteTarget, setDeleteTarget,
    forceDelete, setForceDelete,
    deleteState, isDeleting,
    confirmDelete, cleanupDelete,
  } = del

  // Derived flags
  const hasActiveFileOperation = isUploading || isDownloading || isDeleting || isBulkOperating

  const { blocker } = useNavigationBlock({ hasActiveFileOperation, isDeleting, isBulkDelete })

  // The current listing doubles as the upload conflict cache for this directory.
  useEffect(() => {
    updateDirCache(currentPath, items)
  }, [items, currentPath, updateDirCache])

  // Cancel whatever is still running when leaving the page.
  useEffect(() => () => {
    bulk.cleanupBulk()
    cleanupUploads()
    cleanupDownloads()
    cleanupDelete()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps -- unmount only

  // Compute bulk metrics with actual current transfer states
  const {
    totalBytes: bulkTotalBytes,
    processedBytes: bulkProcessedBytes,
    totalChunks: bulkTotalChunks,
    processedChunks: bulkProcessedChunks,
    workersStatus: bulkWorkersStatus,
  } = getBulkOperationMetrics(bulkOperation, uploadStates, downloadStates)

  const handleCreateFolder = (name) => run(
    () => API.files.createFolder(storageId, currentPath, name),
    { success: 'Folder created', onSuccess: loadTree },
  )

  const handlePreview = (item) => {
    const mediaType = getMediaType(item.name)
    if (!mediaType) {
      startDownload(item)
      return
    }
    setPreviewFile(item)
  }

  const handleMove = (item, newPath) => run(
    () => API.files.move(storageId, getItemPath(item), newPath),
    { success: 'Moved successfully', onSuccess: () => { setMoveTarget(null); loadTree() } },
  )

  const handleRename = (item, newName) => run(
    () => API.files.move(storageId, getItemPath(item), buildRenamedPath(item, newName)),
    { success: 'Folder renamed', onSuccess: () => { setRenameTarget(null); loadTree() } },
  )

  return (
    <Box>
      <FileBreadcrumbs prefix={prefix} pathParts={pathParts} onNavigate={navigate} />

      <TransferProgressStack
        uploadStates={uploadStates}
        onCancelUpload={cancelUpload}
        downloadStates={downloadStates}
        onCancelDownload={cancelDownload}
        deleteState={deleteState}
        bulkOperation={bulkOperation}
        bulkMetrics={{
          totalBytes: bulkTotalBytes,
          processedBytes: bulkProcessedBytes,
          totalChunks: bulkTotalChunks,
          processedChunks: bulkProcessedChunks,
          workersStatus: bulkWorkersStatus,
        }}
        onCancelBulk={() => bulkCancelRef.current?.()}
      />

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
        <FileSelectionToolbar
          selectableCount={selectableFiles.length}
          selectedCount={selectedFiles.length}
          allSelected={allFilesSelected}
          isBulkOperating={isBulkOperating}
          onToggleSelectAll={toggleSelectAllFiles}
          onClear={clearSelection}
          onMove={() => setBulkMoveOpen(true)}
          onDownload={() => handleBulkDownload(selectedFiles, { startDownload, downloadStatesRef, releaseDownloadTracking })}
          onDelete={() => setBulkDeleteOpen(true)}
        />
      )}

      <FileList
        items={displayItems}
        storageId={storageId}
        selectedFilePaths={selectedFilePaths}
        isSearchResults={!!searchResults}
        onInfo={setInfoFile}
        onPreview={handlePreview}
        onDelete={setDeleteTarget}
        onDownload={startDownload}
        onMove={setMoveTarget}
        onRename={setRenameTarget}
        onToggleSelect={toggleFileSelection}
      />

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

      <MoveDialog
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
