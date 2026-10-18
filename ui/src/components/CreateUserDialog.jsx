import { useState } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions, Button, TextField, Typography,
} from '@mui/material'
import { newUserError } from '../pages/Users/new_user'

// The superuser adding an account. The parent resets the form through key.
export default function CreateUserDialog({ open, onCreate, onClose }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')

  const handleSubmit = (e) => {
    e.preventDefault()
    const problem = newUserError({ email, password, confirm })
    setError(problem || '')
    if (!problem) onCreate(email.trim(), password)
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <form onSubmit={handleSubmit}>
        <DialogTitle>Add User</DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            autoFocus
            margin="dense"
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <TextField
            fullWidth
            margin="dense"
            label="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <TextField
            fullWidth
            margin="dense"
            label="Confirm Password"
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
          />
          {error && (
            <Typography color="error" variant="body2" sx={{ mt: 1, fontSize: '0.8rem' }}>
              {error}
            </Typography>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose} color="inherit">Cancel</Button>
          <Button type="submit" variant="contained" disabled={!email || !password || !confirm}>
            Create
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  )
}
