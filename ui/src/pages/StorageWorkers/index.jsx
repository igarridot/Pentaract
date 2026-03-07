import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import {
  Typography, List, ListItem, ListItemText, Paper, Box, Fab, Divider, Chip,
} from '@mui/material'
import { Add as AddIcon } from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'

export default function StorageWorkers() {
  const addAlert = useAlert()
  const [workers, setWorkers] = useState([])

  useEffect(() => {
    const load = async () => {
      try {
        const data = await API.storageWorkers.list()
        setWorkers(data || [])
      } catch (err) {
        addAlert(err.message, 'error')
      }
    }
    load()
  }, [])

  return (
    <Box>
      <Typography variant="h5" gutterBottom>Storage Workers</Typography>

      <Paper variant="outlined">
        <List>
          {workers.map((w, i) => (
            <Box key={w.id}>
              {i > 0 && <Divider />}
              <ListItem>
                <ListItemText
                  primary={w.name}
                  secondary={`Token: ${w.token.substring(0, 10)}...`}
                />
                {w.storage_id && (
                  <Chip label="Bound" size="small" color="primary" variant="outlined" />
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
    </Box>
  )
}
