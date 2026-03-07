import { useState, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import {
  Typography, List, ListItem, ListItemButton, ListItemIcon, ListItemText,
  IconButton, Box, Fab, Divider,
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
      <Typography variant="h5" sx={{ mb: 3 }}>Storages</Typography>

      <Box sx={{
        bgcolor: 'white',
        borderRadius: 3,
        border: '1px solid rgba(0,0,0,0.06)',
        overflow: 'hidden',
      }}>
        <List disablePadding>
          {storages.map((s, i) => (
            <Box key={s.id}>
              {i > 0 && <Divider />}
              <ListItem
                disablePadding
                secondaryAction={
                  <Box sx={{ display: 'flex', gap: 0.25 }}>
                    <IconButton
                      size="small"
                      onClick={() => loadAccess(s.id)}
                      title="Manage access"
                      sx={{ opacity: 0.4, '&:hover': { opacity: 1 } }}
                    >
                      <PeopleIcon sx={{ fontSize: 18 }} />
                    </IconButton>
                    <IconButton
                      size="small"
                      onClick={() => setDeleteTarget(s)}
                      title="Delete storage"
                      sx={{ opacity: 0.4, '&:hover': { opacity: 1, color: 'error.main' } }}
                    >
                      <DeleteIcon sx={{ fontSize: 18 }} />
                    </IconButton>
                  </Box>
                }
              >
                <ListItemButton onClick={() => navigate(`/storages/${s.id}/files/`)} sx={{ py: 1.5 }}>
                  <ListItemIcon sx={{ minWidth: 40 }}>
                    <StorageIcon sx={{ color: 'primary.main', fontSize: 20 }} />
                  </ListItemIcon>
                  <ListItemText
                    primary={s.name}
                    secondary={`${s.files_amount} files \u00b7 ${convertSize(s.size)}`}
                    primaryTypographyProps={{ fontWeight: 500, fontSize: '0.875rem' }}
                    secondaryTypographyProps={{ fontSize: '0.75rem' }}
                  />
                </ListItemButton>
              </ListItem>
            </Box>
          ))}
          {storages.length === 0 && (
            <Box sx={{ p: 4, textAlign: 'center' }}>
              <Typography color="text.secondary" variant="body2">
                No storages yet
              </Typography>
              <Typography color="text.secondary" variant="caption">
                Create one to get started
              </Typography>
            </Box>
          )}
        </List>
      </Box>

      <Fab
        color="primary"
        component={Link}
        to="/storages/register"
        sx={{ position: 'fixed', bottom: 28, right: 28, width: 52, height: 52 }}
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
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1.5 }}>
            <Typography variant="h6" sx={{ fontSize: '1rem' }}>Access Control</Typography>
            <Box>
              <IconButton size="small" onClick={() => { setEditUser(null); setGrantOpen(true) }}>
                <AddIcon sx={{ fontSize: 18 }} />
              </IconButton>
              <IconButton size="small" onClick={() => setAccessStorageId(null)}>
                <DeleteIcon sx={{ fontSize: 18 }} />
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
