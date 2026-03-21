import FolderBrowserDialog from './FolderBrowserDialog'

export default function MoveDialog({ open, item, count, storageId, onMove, onConfirm, onClose }) {
  const isBulk = count != null && count > 0

  const currentDir = item?.path
    ? item.path.substring(0, item.path.lastIndexOf('/') + 1).replace(/\/$/, '')
    : ''

  return (
    <FolderBrowserDialog
      open={open}
      title={isBulk ? `Move ${count} selected file(s)` : `Move "${item?.name}"`}
      storageId={storageId}
      onClose={onClose}
      actionLabel="Move here"
      isActionDisabled={isBulk ? undefined : (targetPath) => targetPath === currentDir}
      onConfirm={
        isBulk
          ? onConfirm
          : (targetPath) => {
              if (!item) return
              const newPath = targetPath ? targetPath + '/' + item.name : item.name
              onMove(item, newPath)
            }
      }
    />
  )
}
