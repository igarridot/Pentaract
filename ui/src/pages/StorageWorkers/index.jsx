import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import {
  Typography, List, ListItem, ListItemText, Box, Fab, Divider,
  IconButton,
} from '@mui/material'
import { Add as AddIcon, Delete as DeleteIcon, Edit as EditIcon } from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import ActionConfirmDialog from '../../components/ActionConfirmDialog'
import EditWorkerDialog from '../../components/EditWorkerDialog'

export default function StorageWorkers() {
  const addAlert = useAlert()
  const [workers, setWorkers] = useState([])
  const [storages, setStorages] = useState([])
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [editTarget, setEditTarget] = useState(null)

  const load = async () => {
    try {
      const [workersData, storagesData] = await Promise.all([
        API.storageWorkers.list(),
        API.storages.list(),
      ])
      setWorkers(workersData || [])
      setStorages(storagesData || [])
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  useEffect(() => { load() }, [])

  const storageMap = Object.fromEntries(storages.map((s) => [s.id, s.name]))

  const handleDelete = async () => {
    try {
      await API.storageWorkers.delete(deleteTarget.id)
      setDeleteTarget(null)
      addAlert('Worker deleted', 'success')
      load()
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  const handleEdit = async (id, name, storageId) => {
    try {
      await API.storageWorkers.update(id, name, storageId)
      setEditTarget(null)
      addAlert('Worker updated', 'success')
      load()
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 3 }}>Storage Workers</Typography>

      <Box sx={{
        bgcolor: 'background.paper',
        borderRadius: 3,
        border: '1px solid',
        borderColor: 'divider',
        overflow: 'hidden',
      }}>
        <List disablePadding>
          {workers.map((w, i) => (
            <Box key={w.id}>
              {i > 0 && <Divider />}
              <ListItem
                secondaryAction={
                  <Box sx={{ display: 'flex', gap: 0.25 }}>
                    <IconButton
                      size="small"
                      onClick={() => setEditTarget(w)}
                      title="Edit worker"
                      sx={{ opacity: 0.4, '&:hover': { opacity: 1 } }}
                    >
                      <EditIcon sx={{ fontSize: 18 }} />
                    </IconButton>
                    <IconButton
                      size="small"
                      onClick={() => setDeleteTarget(w)}
                      title="Delete worker"
                      sx={{ opacity: 0.4, '&:hover': { opacity: 1, color: 'error.main' } }}
                    >
                      <DeleteIcon sx={{ fontSize: 18 }} />
                    </IconButton>
                  </Box>
                }
                sx={{ py: 1.25, px: 2.5 }}
              >
                <ListItemText
                  primary={w.name}
                  secondary={w.storage_id
                    ? `Assigned to ${storageMap[w.storage_id] || 'Unknown'}`
                    : 'Available for all storages'}
                  primaryTypographyProps={{ fontWeight: 500, fontSize: '0.875rem' }}
                  secondaryTypographyProps={{ fontSize: '0.75rem' }}
                />
              </ListItem>
            </Box>
          ))}
          {workers.length === 0 && (
            <Box sx={{ p: 4, textAlign: 'center' }}>
              <Typography color="text.secondary" variant="body2">
                No workers yet
              </Typography>
              <Typography color="text.secondary" variant="caption">
                Create one to enable file operations
              </Typography>
            </Box>
          )}
        </List>
      </Box>

      <Fab
        color="primary"
        component={Link}
        to="/storage_workers/register"
        sx={{ position: 'fixed', bottom: 28, right: 28, width: 52, height: 52 }}
      >
        <AddIcon />
      </Fab>

      <ActionConfirmDialog
        open={!!deleteTarget}
        entity={deleteTarget?.name || 'worker'}
        action="Delete"
        description={`Are you sure you want to delete worker "${deleteTarget?.name}"?`}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />

      <EditWorkerDialog
        open={!!editTarget}
        worker={editTarget}
        storages={storages}
        onSave={handleEdit}
        onClose={() => setEditTarget(null)}
      />
    </Box>
  )
}
