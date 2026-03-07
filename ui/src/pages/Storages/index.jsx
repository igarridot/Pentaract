import { useState, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import {
  Typography, List, ListItem, ListItemButton, ListItemIcon, ListItemText,
  IconButton, Paper, Box, Fab, Divider,
} from '@mui/material'
import {
  Storage as StorageIcon,
  Delete as DeleteIcon,
  Add as AddIcon,
  People as PeopleIcon,
} from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import { convertSize } from '../../common/size_converter'
import ActionConfirmDialog from '../../components/ActionConfirmDialog'
import Access from '../../components/Access'
import GrantAccess from '../../components/GrantAccess'

export default function Storages() {
  const navigate = useNavigate()
  const addAlert = useAlert()
  const [storages, setStorages] = useState([])
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [accessStorageId, setAccessStorageId] = useState(null)
  const [accessUsers, setAccessUsers] = useState([])
  const [grantOpen, setGrantOpen] = useState(false)
  const [editUser, setEditUser] = useState(null)

  const load = async () => {
    try {
      const data = await API.storages.list()
      setStorages(data || [])
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  useEffect(() => { load() }, [])

  const handleDelete = async () => {
    try {
      await API.storages.delete(deleteTarget.id)
      setDeleteTarget(null)
      addAlert('Storage deleted', 'success')
      load()
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const loadAccess = async (storageId) => {
    try {
      const data = await API.access.list(storageId)
      setAccessUsers(data || [])
      setAccessStorageId(storageId)
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const handleGrant = async (email, accessType) => {
    try {
      await API.access.grant(accessStorageId, email, accessType)
      addAlert('Access granted', 'success')
      loadAccess(accessStorageId)
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const handleRevokeAccess = async (user) => {
    try {
      await API.access.revoke(accessStorageId, user.id)
      addAlert('Access revoked', 'success')
      loadAccess(accessStorageId)
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  // Get current user ID from JWT
  const token = localStorage.getItem('access_token')
  let currentUserId = null
  if (token) {
    try {
      const payload = JSON.parse(atob(token.split('.')[1]))
      currentUserId = payload.sub
    } catch {}
  }

  return (
    <Box>
      <Typography variant="h5" gutterBottom>Storages</Typography>

      <Paper variant="outlined">
        <List>
          {storages.map((s, i) => (
            <Box key={s.id}>
              {i > 0 && <Divider />}
              <ListItem
                secondaryAction={
                  <Box>
                    <IconButton onClick={() => loadAccess(s.id)} title="Manage access">
                      <PeopleIcon />
                    </IconButton>
                    <IconButton onClick={() => setDeleteTarget(s)} title="Delete storage">
                      <DeleteIcon />
                    </IconButton>
                  </Box>
                }
              >
                <ListItemButton onClick={() => navigate(`/storages/${s.id}/files/`)}>
                  <ListItemIcon><StorageIcon /></ListItemIcon>
                  <ListItemText
                    primary={s.name}
                    secondary={`${s.files_amount} files - ${convertSize(s.size)}`}
                  />
                </ListItemButton>
              </ListItem>
            </Box>
          ))}
          {storages.length === 0 && (
            <ListItem>
              <ListItemText primary="No storages yet" secondary="Create one to get started" />
            </ListItem>
          )}
        </List>
      </Paper>

      <Fab
        color="secondary"
        component={Link}
        to="/storages/register"
        sx={{ position: 'fixed', bottom: 24, right: 24 }}
      >
        <AddIcon />
      </Fab>

      <ActionConfirmDialog
        open={!!deleteTarget}
        entity={deleteTarget?.name || 'storage'}
        action="Delete"
        description={`Are you sure you want to delete "${deleteTarget?.name}"? All files will be lost.`}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />

      {accessStorageId && (
        <Box sx={{ mt: 3 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
            <Typography variant="h6">Access Control</Typography>
            <Box>
              <IconButton onClick={() => { setEditUser(null); setGrantOpen(true) }}>
                <AddIcon />
              </IconButton>
              <IconButton onClick={() => setAccessStorageId(null)}>
                <DeleteIcon />
              </IconButton>
            </Box>
          </Box>
          <Access
            users={accessUsers}
            currentUserId={currentUserId}
            onEdit={(user) => { setEditUser(user); setGrantOpen(true) }}
            onDelete={handleRevokeAccess}
          />
        </Box>
      )}

      <GrantAccess
        open={grantOpen}
        onClose={() => { setGrantOpen(false); setEditUser(null) }}
        onGrant={handleGrant}
        editUser={editUser}
      />
    </Box>
  )
}
