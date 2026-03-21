import FolderBrowserDialog from './FolderBrowserDialog'

export default function BulkMoveDialog({ open, count = 0, storageId, onConfirm, onClose }) {
  return (
    <FolderBrowserDialog
      open={open}
      title={`Move ${count} selected file(s)`}
      storageId={storageId}
      onClose={onClose}
      actionLabel="Move here"
      onConfirm={onConfirm}
    />
  )
}
