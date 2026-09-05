import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import {
  Typography, List, ListItem, ListItemText, Box, Fab, Divider,
  IconButton,
} from '@mui/material'
import { Add as AddIcon, Delete as DeleteIcon, Edit as EditIcon } from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import { useApiAction } from '../../common/use_api_action'
import ActionConfirmDialog from '../../components/ActionConfirmDialog'
import Panel from '../../components/Panel'
import EditWorkerDialog from '../../components/EditWorkerDialog'

export default function StorageWorkers() {
  const addAlert = useAlert()
  const run = useApiAction(addAlert)
  const [workers, setWorkers] = useState([])
  const [storages, setStorages] = useState([])
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [editTarget, setEditTarget] = useState(null)

  const load = useCallback(() => run(
    () => Promise.all([API.storageWorkers.list(), API.storages.list()]),
    {
      onSuccess: ([workersData, storagesData]) => {
        setWorkers(workersData || [])
        setStorages(storagesData || [])
      },
    },
  ), [run])

  useEffect(() => { load() }, [load])

  const storageMap = Object.fromEntries(storages.map((s) => [s.id, s.name]))

  const handleDelete = () => run(() => API.storageWorkers.delete(deleteTarget.id), {
    success: 'Worker deleted',
    onSuccess: () => { setDeleteTarget(null); load() },
  })

  const handleEdit = (id, name, storageId) => run(() => API.storageWorkers.update(id, name, storageId), {
    success: 'Worker updated',
    onSuccess: () => { setEditTarget(null); load() },
  })

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 3 }}>Storage Workers</Typography>

      <Panel>
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
      </Panel>

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
        key={editTarget?.id ?? 'closed'}
        open={!!editTarget}
        worker={editTarget}
        storages={storages}
        onSave={handleEdit}
        onClose={() => setEditTarget(null)}
      />
    </Box>
  )
}
