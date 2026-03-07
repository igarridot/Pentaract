import { useState } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions, Button, TextField,
} from '@mui/material'

export default function CreateFolderDialog({ open, onCreate, onClose }) {
  const [name, setName] = useState('')

  const handleCreate = () => {
    if (name && !name.includes('/')) {
      onCreate(name)
      setName('')
      onClose()
    }
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>Create Folder</DialogTitle>
      <DialogContent>
        <TextField
          autoFocus
          fullWidth
          margin="dense"
          label="Folder name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={name.includes('/')}
          helperText={name.includes('/') ? 'Folder name cannot contain /' : ''}
          onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={handleCreate} variant="contained" disabled={!name || name.includes('/')}>
          Create
        </Button>
      </DialogActions>
    </Dialog>
  )
}
