import { useState, useEffect, useCallback } from 'react'
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
import { useApiAction } from '../../common/use_api_action'
import { useDeleteProgress } from '../../common/use_delete_progress'
import { convertSize } from '../../common/size_converter'
import ActionConfirmDialog from '../../components/ActionConfirmDialog'
import DeleteProgress from '../../components/DeleteProgress'
import Panel from '../../components/Panel'
import StorageAccessPanel from './StorageAccessPanel'

export default function Storages() {
  const navigate = useNavigate()
  const addAlert = useAlert()
  const run = useApiAction(addAlert)
  const [storages, setStorages] = useState([])
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [accessStorageId, setAccessStorageId] = useState(null)
  const { deleteState, runTrackedDelete } = useDeleteProgress()

  const load = useCallback(() => (
    run(() => API.storages.list(), { onSuccess: (data) => setStorages(data || []) })
  ), [run])

  useEffect(() => { load() }, [load])

  const handleDelete = () => {
    const target = deleteTarget
    return run(
      () => runTrackedDelete(target?.name || 'storage', (deleteId) => API.storages.delete(target.id, deleteId)),
      { success: 'Storage deleted', onSuccess: () => { setDeleteTarget(null); load() } },
    )
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

      <Panel>
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
                      onClick={() => setAccessStorageId(s.id)}
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
      </Panel>

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
        <StorageAccessPanel storageId={accessStorageId} onClose={() => setAccessStorageId(null)} />
      )}
    </Box>
  )
}
