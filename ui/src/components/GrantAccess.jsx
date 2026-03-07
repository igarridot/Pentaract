import { useState, useEffect } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions,
  Button, TextField, FormControl, InputLabel, Select, MenuItem,
} from '@mui/material'

export default function GrantAccess({ open, onClose, onGrant, editUser }) {
  const [email, setEmail] = useState('')
  const [accessType, setAccessType] = useState('r')

  useEffect(() => {
    if (editUser) {
      setEmail(editUser.email)
      setAccessType(editUser.access_type)
    } else {
      setEmail('')
      setAccessType('r')
    }
  }, [editUser, open])

  const handleSubmit = () => {
    onGrant(email, accessType)
    onClose()
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>{editUser ? 'Change Access' : 'Grant Access'}</DialogTitle>
      <DialogContent>
        <TextField
          fullWidth
          margin="dense"
          placeholder="Email address"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          disabled={!!editUser}
          sx={{ mb: 2 }}
        />
        <FormControl fullWidth>
          <InputLabel>Access Type</InputLabel>
          <Select value={accessType} onChange={(e) => setAccessType(e.target.value)} label="Access Type">
            <MenuItem value="r">Viewer (Read)</MenuItem>
            <MenuItem value="w">Editor (Write)</MenuItem>
            <MenuItem value="a">Admin</MenuItem>
          </Select>
        </FormControl>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} color="inherit">Cancel</Button>
        <Button onClick={handleSubmit} variant="contained" disabled={!email}>
          {editUser ? 'Update' : 'Grant'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}
