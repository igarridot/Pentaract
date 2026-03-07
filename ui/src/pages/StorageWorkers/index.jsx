import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import {
  Typography, List, ListItem, ListItemText, Paper, Box, Fab, Divider, Chip,
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
      <Typography variant="h5" gutterBottom>Storage Workers</Typography>

      <Paper variant="outlined">
        <List>
          {workers.map((w, i) => (
            <Box key={w.id}>
              {i > 0 && <Divider />}
              <ListItem
                secondaryAction={
                  <Box>
                    <IconButton onClick={() => setEditTarget(w)} title="Edit worker">
                      <EditIcon />
                    </IconButton>
                    <IconButton onClick={() => setDeleteTarget(w)} title="Delete worker">
                      <DeleteIcon />
                    </IconButton>
                  </Box>
                }
              >
                <ListItemText
                  primary={w.name}
                  secondary={w.storage_id
                    ? `Assigned to: ${storageMap[w.storage_id] || 'Unknown'}`
                    : 'Available for all storages'}
                />
              </ListItem>
            </Box>
          ))}
          {workers.length === 0 && (
            <ListItem>
              <ListItemText primary="No workers yet" secondary="Create one to enable file operations" />
            </ListItem>
          )}
        </List>
      </Paper>

      <Fab
        color="secondary"
        component={Link}
        to="/storage_workers/register"
        sx={{ position: 'fixed', bottom: 24, right: 24 }}
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
