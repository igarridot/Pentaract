import { useState, useEffect, useCallback } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions,
  Button, List, ListItemButton, ListItemIcon, ListItemText,
  Typography, Breadcrumbs, Link as MuiLink, Box,
} from '@mui/material'
import { Folder as FolderIcon } from '@mui/icons-material'
import API from '../api'

export default function MoveDialog({ open, item, storageId, onMove, onClose }) {
  const [targetPath, setTargetPath] = useState('')
  const [folders, setFolders] = useState([])

  const loadFolders = useCallback(async (path) => {
    try {
      const data = await API.files.tree(storageId, path)
      const dirs = (data || []).filter((f) => !f.is_file)
      setFolders(dirs)
      setTargetPath(path)
    } catch {
      setFolders([])
    }
  }, [storageId])

  useEffect(() => {
    if (open) loadFolders('')
  }, [open, loadFolders])

  const pathParts = targetPath.split('/').filter(Boolean)

  const handleMove = () => {
    if (!item) return
    const itemName = item.name
    const newPath = targetPath ? targetPath + '/' + itemName : itemName
    onMove(item, newPath)
  }

  // Don't allow moving into the same location
  const currentDir = item?.path
    ? item.path.substring(0, item.path.lastIndexOf('/') + 1).replace(/\/$/, '')
    : ''
  const isSameLocation = targetPath === currentDir

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Move "{item?.name}"</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          Select destination folder:
        </Typography>

        <Breadcrumbs sx={{ mb: 1 }}>
          <MuiLink
            underline="hover"
            color="inherit"
            sx={{ cursor: 'pointer' }}
            onClick={() => loadFolders('')}
          >
            Root
          </MuiLink>
          {pathParts.map((part, i) => {
            const pathTo = pathParts.slice(0, i + 1).join('/')
            return (
              <MuiLink
                key={pathTo}
                underline="hover"
                color="inherit"
                sx={{ cursor: 'pointer' }}
                onClick={() => loadFolders(pathTo)}
              >
                {part}
              </MuiLink>
            )
          })}
        </Breadcrumbs>

        <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, maxHeight: 300, overflow: 'auto' }}>
          <List dense>
            {folders.length === 0 && (
              <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
                No subfolders
              </Typography>
            )}
            {folders.map((folder) => (
              <ListItemButton
                key={folder.path}
                onClick={() => loadFolders(folder.path.replace(/\/$/, ''))}
              >
                <ListItemIcon><FolderIcon /></ListItemIcon>
                <ListItemText primary={folder.name} />
              </ListItemButton>
            ))}
          </List>
        </Box>

        <Typography variant="body2" sx={{ mt: 1 }}>
          Destination: <strong>/{targetPath || 'Root'}</strong>
        </Typography>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={handleMove} variant="contained" disabled={isSameLocation}>
          Move here
        </Button>
      </DialogActions>
    </Dialog>
  )
}
