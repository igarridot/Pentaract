import RemoteFolderPickerDialog from './RemoteFolderPickerDialog'

export default function BulkMoveDialog({ open, count = 0, storageId, onConfirm, onClose }) {
  return (
    <RemoteFolderPickerDialog
      open={open}
      storageId={storageId}
      title={`Move ${count} selected file(s)`}
      confirmLabel="Move here"
      onConfirm={onConfirm}
      onClose={onClose}
    />
  )
}
