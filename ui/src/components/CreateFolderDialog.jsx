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
      <DialogTitle>New Folder</DialogTitle>
      <DialogContent>
        <TextField
          autoFocus
          fullWidth
          margin="dense"
          placeholder="Folder name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={name.includes('/')}
          helperText={name.includes('/') ? 'Name cannot contain /' : ''}
          onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} color="inherit">Cancel</Button>
        <Button onClick={handleCreate} variant="contained" disabled={!name || name.includes('/')}>
          Create
        </Button>
      </DialogActions>
    </Dialog>
  )
}
