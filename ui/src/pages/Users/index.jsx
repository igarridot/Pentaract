import { useEffect, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Box, Typography, Table, TableHead, TableRow, TableCell, TableBody,
  IconButton, Dialog, DialogTitle, DialogContent, DialogActions, Button, TextField,
} from '@mui/material'
import { Delete as DeleteIcon, Key as KeyIcon } from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import { useApiAction } from '../../common/use_api_action'
import ActionConfirmDialog from '../../components/ActionConfirmDialog'
import Panel from '../../components/Panel'

export default function Users() {
  const navigate = useNavigate()
  const addAlert = useAlert()
  const run = useApiAction(addAlert)
  const [users, setUsers] = useState([])
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [passwordTarget, setPasswordTarget] = useState(null)
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  const load = useCallback(() => run(() => API.users.listManaged(), {
    onSuccess: (data) => setUsers(data || []),
    onError: (err) => {
      if (!(err?.message || '').toLowerCase().includes('forbidden')) return false
      addAlert('Admin access required', 'error')
      navigate('/storages')
      return true
    },
  }), [run, addAlert, navigate])

  useEffect(() => { load() }, [load])

  const closePasswordDialog = () => {
    setPasswordTarget(null)
    setNewPassword('')
    setConfirmPassword('')
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    return run(() => API.users.deleteManaged(deleteTarget.id), {
      success: 'User deleted',
      onSuccess: () => { setDeleteTarget(null); load() },
    })
  }

  const handleUpdatePassword = () => {
    if (!passwordTarget) return
    if (!newPassword) {
      addAlert('Password is required', 'error')
      return
    }
    if (newPassword !== confirmPassword) {
      addAlert('Passwords do not match', 'error')
      return
    }
    return run(() => API.users.updatePassword(passwordTarget.id, newPassword), {
      success: 'Password updated',
      onSuccess: closePasswordDialog,
    })
  }

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 3 }}>User Management</Typography>
      <Panel>
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
      </Panel>

      <ActionConfirmDialog
        open={!!deleteTarget}
        entity={deleteTarget?.email || 'user'}
        action="Delete"
        description={`Delete user "${deleteTarget?.email}"? This action cannot be undone.`}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />

      <Dialog open={!!passwordTarget} onClose={closePasswordDialog} maxWidth="xs" fullWidth>
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
          <Button onClick={closePasswordDialog} color="inherit">Cancel</Button>
          <Button variant="contained" onClick={handleUpdatePassword} disabled={!newPassword || !confirmPassword}>
            Update
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}
