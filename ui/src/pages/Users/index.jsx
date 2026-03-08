import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Box, Typography, Paper, Table, TableHead, TableRow, TableCell, TableBody,
  IconButton, Dialog, DialogTitle, DialogContent, DialogActions, Button, TextField,
} from '@mui/material'
import { Delete as DeleteIcon, Key as KeyIcon } from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import ActionConfirmDialog from '../../components/ActionConfirmDialog'

export default function Users() {
  const navigate = useNavigate()
  const addAlert = useAlert()
  const [users, setUsers] = useState([])
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [passwordTarget, setPasswordTarget] = useState(null)
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  const load = async () => {
    try {
      const data = await API.users.listManaged()
      setUsers(data || [])
    } catch (err) {
      if ((err?.message || '').toLowerCase().includes('forbidden')) {
        addAlert('Admin access required', 'error')
        navigate('/storages')
        return
      }
      addAlert(err.message, 'error')
    }
  }

  useEffect(() => {
    load()
  }, [])

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      await API.users.deleteManaged(deleteTarget.id)
      addAlert('User deleted', 'success')
      setDeleteTarget(null)
      load()
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const handleUpdatePassword = async () => {
    if (!passwordTarget) return
    if (!newPassword) {
      addAlert('Password is required', 'error')
      return
    }
    if (newPassword !== confirmPassword) {
      addAlert('Passwords do not match', 'error')
      return
    }
    try {
      await API.users.updatePassword(passwordTarget.id, newPassword)
      addAlert('Password updated', 'success')
      setPasswordTarget(null)
      setNewPassword('')
      setConfirmPassword('')
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 3 }}>User Management</Typography>
      <Paper sx={{ borderRadius: 3, border: '1px solid', borderColor: 'divider', overflow: 'hidden' }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Email</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {users.map((u) => (
              <TableRow key={u.id}>
                <TableCell>{u.email}</TableCell>
                <TableCell align="right">
                  <IconButton size="small" onClick={() => setPasswordTarget(u)} title="Change password">
                    <KeyIcon sx={{ fontSize: 18 }} />
                  </IconButton>
                  <IconButton size="small" onClick={() => setDeleteTarget(u)} title="Delete user">
                    <DeleteIcon sx={{ fontSize: 18 }} />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
            {users.length === 0 && (
              <TableRow>
                <TableCell colSpan={2} sx={{ color: 'text.secondary', textAlign: 'center', py: 4 }}>
                  No manageable users found
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Paper>

      <ActionConfirmDialog
        open={!!deleteTarget}
        entity={deleteTarget?.email || 'user'}
        action="Delete"
        description={`Delete user "${deleteTarget?.email}"? This action cannot be undone.`}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />

      <Dialog open={!!passwordTarget} onClose={() => setPasswordTarget(null)} maxWidth="xs" fullWidth>
        <DialogTitle>Change Password</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Set a new password for {passwordTarget?.email}
          </Typography>
          <TextField
            fullWidth
            margin="dense"
            label="New Password"
            type="password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
          />
          <TextField
            fullWidth
            margin="dense"
            label="Confirm Password"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setPasswordTarget(null); setNewPassword(''); setConfirmPassword('') }} color="inherit">Cancel</Button>
          <Button variant="contained" onClick={handleUpdatePassword} disabled={!newPassword || !confirmPassword}>
            Update
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}
