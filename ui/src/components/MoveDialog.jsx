import { useEffect, useState } from 'react'
import RemoteFolderPickerDialog from './RemoteFolderPickerDialog'

export default function MoveDialog({ open, item, storageId, onMove, onClose }) {
  const [targetPath, setTargetPath] = useState('')

  useEffect(() => {
    if (open) {
      setTargetPath('')
    }
  }, [open])

  const handleMove = (path) => {
    if (!item) return
    const newPath = path ? `${path}/${item.name}` : item.name
    onMove(item, newPath)
  }

  const currentDir = item?.path
    ? item.path.substring(0, item.path.lastIndexOf('/') + 1).replace(/\/$/, '')
    : ''
  const isSameLocation = targetPath === currentDir

  return (
    <RemoteFolderPickerDialog
      open={open}
      storageId={storageId}
      title={`Move "${item?.name}"`}
      confirmLabel="Move here"
      confirmDisabled={isSameLocation}
      onPathChange={setTargetPath}
      onConfirm={handleMove}
      onClose={onClose}
    />
  )
}
