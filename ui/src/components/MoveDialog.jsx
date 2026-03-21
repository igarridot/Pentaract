import FolderBrowserDialog from './FolderBrowserDialog'

export default function MoveDialog({ open, item, storageId, onMove, onClose }) {
  const currentDir = item?.path
    ? item.path.substring(0, item.path.lastIndexOf('/') + 1).replace(/\/$/, '')
    : ''

  return (
    <FolderBrowserDialog
      open={open}
      title={`Move "${item?.name}"`}
      storageId={storageId}
      onClose={onClose}
      actionLabel="Move here"
      isActionDisabled={(targetPath) => targetPath === currentDir}
      onConfirm={(targetPath) => {
        if (!item) return
        const newPath = targetPath ? targetPath + '/' + item.name : item.name
        onMove(item, newPath)
      }}
    />
  )
}
