import { useState } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions,
  Button, TextField, FormControl, InputLabel, Select, MenuItem, Typography,
} from '@mui/material'

// The parent keys this dialog per opening (and per edited user), so the
// initial state below is recomputed every time it opens.
export default function GrantAccess({ open, onClose, onGrant, editUser, candidates = [] }) {
  const [email, setEmail] = useState(editUser ? editUser.email : candidates[0]?.email || '')
  const [accessType, setAccessType] = useState(editUser ? editUser.access_type : 'r')

  const handleSubmit = () => {
    onGrant(email, accessType)
    onClose()
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>{editUser ? 'Change Access' : 'Grant Access'}</DialogTitle>
      <DialogContent>
        {editUser ? (
          <TextField
            fullWidth
            margin="dense"
            placeholder="Email address"
            value={email}
            disabled
            sx={{ mb: 2 }}
          />
        ) : (
          <FormControl fullWidth sx={{ mb: 2 }}>
            <InputLabel>User</InputLabel>
            <Select
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              label="User"
              disabled={candidates.length === 0}
            >
              {candidates.map((u) => (
                <MenuItem key={u.id} value={u.email}>{u.email}</MenuItem>
              ))}
            </Select>
          </FormControl>
        )}
        {!editUser && candidates.length === 0 && (
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 2 }}>
            No eligible users available.
          </Typography>
        )}
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
        <Button onClick={handleSubmit} variant="contained" disabled={!email || (!editUser && candidates.length === 0)}>
          {editUser ? 'Update' : 'Grant'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}
