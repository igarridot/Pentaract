import { useState, useEffect, useCallback } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions,
  Button, List, ListItemButton, ListItemIcon, ListItemText,
  Typography, Breadcrumbs, Link as MuiLink, Box,
} from '@mui/material'
import { Folder as FolderIcon } from '@mui/icons-material'
import API from '../api'

export default function RemoteFolderPickerDialog({
  open,
  storageId,
  initialPath = '',
  title = 'Select destination folder',
  description = 'Select destination folder',
  confirmLabel = 'Use folder',
  confirmDisabled = false,
  onPathChange,
  onConfirm,
  onClose,
}) {
  const [targetPath, setTargetPath] = useState(initialPath)
  const [folders, setFolders] = useState([])

  const loadFolders = useCallback(async (path) => {
    if (!storageId) {
      setFolders([])
      setTargetPath('')
      return
    }

    try {
      const data = await API.files.tree(storageId, path)
      const dirs = (data || []).filter((item) => !item.is_file)
      setFolders(dirs)
      setTargetPath(path)
      onPathChange?.(path)
    } catch {
      setFolders([])
    }
  }, [onPathChange, storageId])

  useEffect(() => {
    if (open) {
      loadFolders(initialPath || '')
    }
  }, [initialPath, loadFolders, open])

  const pathParts = targetPath.split('/').filter(Boolean)

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          {description}
        </Typography>

        <Breadcrumbs sx={{ mb: 1.5 }}>
          <MuiLink
            underline="hover"
            color="inherit"
            sx={{ cursor: 'pointer', fontSize: '0.8125rem' }}
            onClick={() => loadFolders('')}
          >
            Root
          </MuiLink>
          {pathParts.map((part, index) => {
            const pathTo = pathParts.slice(0, index + 1).join('/')
            return (
              <MuiLink
                key={pathTo}
                underline="hover"
                color="inherit"
                sx={{ cursor: 'pointer', fontSize: '0.8125rem' }}
                onClick={() => loadFolders(pathTo)}
              >
                {part}
              </MuiLink>
            )
          })}
        </Breadcrumbs>

        <Box sx={{
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 2,
          maxHeight: 280,
          overflow: 'auto',
          bgcolor: 'background.default',
        }}>
          <List dense disablePadding>
            {folders.length === 0 && (
              <Typography variant="body2" color="text.secondary" sx={{ p: 3, textAlign: 'center' }}>
                No subfolders
              </Typography>
            )}
            {folders.map((folder) => (
              <ListItemButton
                key={folder.path}
                onClick={() => loadFolders(folder.path.replace(/\/$/, ''))}
                sx={{ py: 0.75 }}
              >
                <ListItemIcon sx={{ minWidth: 36 }}>
                  <FolderIcon sx={{ color: 'primary.main', fontSize: 20 }} />
                </ListItemIcon>
                <ListItemText
                  primary={folder.name}
                  primaryTypographyProps={{ fontSize: '0.875rem' }}
                />
              </ListItemButton>
            ))}
          </List>
        </Box>

        <Typography variant="body2" color="text.secondary" sx={{ mt: 1.5, fontSize: '0.8125rem' }}>
          Destination: <strong>/{targetPath || 'Root'}</strong>
        </Typography>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} color="inherit">Cancel</Button>
        <Button onClick={() => onConfirm(targetPath)} variant="contained" disabled={!storageId || confirmDisabled}>
          {confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  )
}
