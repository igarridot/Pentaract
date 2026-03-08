import { useState, useEffect, useRef } from 'react'
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
import DeleteProgress from '../../components/DeleteProgress'

export default function Storages() {
  const navigate = useNavigate()
  const addAlert = useAlert()
  const [storages, setStorages] = useState([])
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [accessStorageId, setAccessStorageId] = useState(null)
  const [accessUsers, setAccessUsers] = useState([])
  const [grantOpen, setGrantOpen] = useState(false)
  const [editUser, setEditUser] = useState(null)
  const [deleteState, setDeleteState] = useState(null)
  const cancelDeleteProgressRef = useRef(null)

  const createOperationId = () => {
    const cryptoObj = globalThis.crypto
    if (cryptoObj?.randomUUID) return cryptoObj.randomUUID()
    if (cryptoObj?.getRandomValues) {
      const bytes = cryptoObj.getRandomValues(new Uint8Array(16))
      bytes[6] = (bytes[6] & 0x0f) | 0x40
      bytes[8] = (bytes[8] & 0x3f) | 0x80
      const hex = [...bytes].map((b) => b.toString(16).padStart(2, '0')).join('')
      return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
    }
    return `${Date.now()}-${Math.random().toString(16).slice(2)}`
  }

  const load = async () => {
    try {
      const data = await API.storages.list()
      setStorages(data || [])
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  useEffect(() => { load() }, [])
  useEffect(() => () => {
    if (cancelDeleteProgressRef.current) cancelDeleteProgressRef.current()
  }, [])

  const handleDelete = async () => {
    try {
      const deleteId = createOperationId()
      if (cancelDeleteProgressRef.current) cancelDeleteProgressRef.current()
      setDeleteState({
        label: deleteTarget?.name || 'storage',
        totalChunks: 0,
        deletedChunks: 0,
        status: 'deleting',
        workersStatus: 'active',
      })

      const cancel = API.files.subscribeDeleteProgress(deleteId, (data) => {
        setDeleteState((prev) => ({
          ...prev,
          totalChunks: data.total || prev?.totalChunks || 0,
          deletedChunks: data.deleted || 0,
          status: data.status,
          workersStatus: data.workers_status || prev?.workersStatus || 'active',
        }))

        if (data.status === 'done') {
          cancel()
          setTimeout(() => setDeleteState(null), 1500)
        }
        if (data.status === 'error') {
          cancel()
          setTimeout(() => setDeleteState(null), 3000)
        }
      })
      cancelDeleteProgressRef.current = cancel

      await API.storages.delete(deleteTarget.id, deleteId)
      cancel()
      cancelDeleteProgressRef.current = null
      setDeleteTarget(null)
      addAlert('Storage deleted', 'success')
      load()
    } catch (err) {
      if (cancelDeleteProgressRef.current) {
        cancelDeleteProgressRef.current()
        cancelDeleteProgressRef.current = null
      }
      setDeleteState((prev) => (prev ? { ...prev, status: 'error' } : null))
      setTimeout(() => setDeleteState(null), 3000)
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
      {deleteState && (
        <DeleteProgress
          label={deleteState.label}
          totalChunks={deleteState.totalChunks}
          deletedChunks={deleteState.deletedChunks}
          status={deleteState.status}
          workersStatus={deleteState.workersStatus}
        />
      )}

      <Box sx={{
        bgcolor: 'background.paper',
        borderRadius: 3,
        border: '1px solid',
        borderColor: 'divider',
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
