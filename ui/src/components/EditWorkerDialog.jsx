import { useState } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions,
  Button, TextField, FormControl, InputLabel, Select, MenuItem,
} from '@mui/material'

// The parent keys this dialog by worker id so a new worker starts from fresh
// state instead of syncing props into state.
export default function EditWorkerDialog({ open, worker, storages, onSave, onClose }) {
  const [name, setName] = useState(worker?.name || '')
  const [storageId, setStorageId] = useState(worker?.storage_id || '')

  const handleSave = () => {
    onSave(worker.id, name, storageId || null)
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Edit Worker</DialogTitle>
      <DialogContent>
        <TextField
          placeholder="Worker name"
          fullWidth
          value={name}
          onChange={(e) => setName(e.target.value)}
          sx={{ mt: 1, mb: 2 }}
        />
        <FormControl fullWidth>
          <InputLabel>Assigned Storage</InputLabel>
          <Select
            value={storageId}
            onChange={(e) => setStorageId(e.target.value)}
            label="Assigned Storage"
          >
            <MenuItem value="">None (available for all)</MenuItem>
            {storages.map((s) => (
              <MenuItem key={s.id} value={s.id}>{s.name}</MenuItem>
            ))}
          </Select>
        </FormControl>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} color="inherit">Cancel</Button>
        <Button onClick={handleSave} variant="contained" disabled={!name.trim()}>
          Save
        </Button>
      </DialogActions>
    </Dialog>
  )
}
