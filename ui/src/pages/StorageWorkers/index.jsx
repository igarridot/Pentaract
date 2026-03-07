import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import {
  Typography, List, ListItem, ListItemText, Paper, Box, Fab, Divider, Chip,
  IconButton,
} from '@mui/material'
import { Add as AddIcon, Delete as DeleteIcon } from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import ActionConfirmDialog from '../../components/ActionConfirmDialog'

export default function StorageWorkers() {
  const addAlert = useAlert()
  const [workers, setWorkers] = useState([])
  const [deleteTarget, setDeleteTarget] = useState(null)

  const load = async () => {
    try {
      const data = await API.storageWorkers.list()
      setWorkers(data || [])
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  useEffect(() => { load() }, [])

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
                  <IconButton onClick={() => setDeleteTarget(w)} title="Delete worker">
                    <DeleteIcon />
                  </IconButton>
                }
              >
                <ListItemText
                  primary={w.name}
                  secondary={`Token: ${w.token.substring(0, 10)}...`}
                />
                {w.storage_id && (
                  <Chip label="Bound" size="small" color="primary" variant="outlined" sx={{ mr: 4 }} />
                )}
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
    </Box>
  )
}
