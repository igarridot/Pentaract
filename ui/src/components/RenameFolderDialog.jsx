import { useEffect, useState } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions, Button, TextField,
} from '@mui/material'

export default function RenameFolderDialog({ open, folder, onRename, onClose }) {
  const [name, setName] = useState('')

  useEffect(() => {
    if (open) setName(folder?.name || '')
  }, [open, folder])

  const handleRename = () => {
    const trimmed = name.trim()
    if (!trimmed || trimmed.includes('/') || !folder) return
    onRename(folder, trimmed)
  }

  const hasError = name.includes('/')
  const sameName = name.trim() === (folder?.name || '')

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>Rename Folder</DialogTitle>
      <DialogContent>
        <TextField
          autoFocus
          fullWidth
          margin="dense"
          placeholder="Folder name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={hasError}
          helperText={hasError ? 'Name cannot contain /' : ''}
          onKeyDown={(e) => e.key === 'Enter' && handleRename()}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} color="inherit">Cancel</Button>
        <Button onClick={handleRename} variant="contained" disabled={!name.trim() || hasError || sameName}>
          Rename
        </Button>
      </DialogActions>
    </Dialog>
  )
}
