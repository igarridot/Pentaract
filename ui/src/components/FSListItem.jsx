import { useState } from 'react'
import {
  ListItem, ListItemButton, ListItemIcon, ListItemText,
  IconButton, Menu, MenuItem,
} from '@mui/material'
import {
  InsertDriveFile as FileIcon,
  Folder as FolderIcon,
  MoreHoriz as MoreIcon,
} from '@mui/icons-material'
import { useNavigate } from 'react-router-dom'

export default function FSListItem({ item, storageId, currentPath, onInfo, onDelete, onDownload, onMove }) {
  const navigate = useNavigate()
  const [anchorEl, setAnchorEl] = useState(null)

  const handleClick = () => {
    if (item.is_file) {
      if (onDownload) onDownload(item)
    } else {
      navigate(`/storages/${storageId}/files/${item.path}`)
    }
  }

  const handleDownload = async () => {
    setAnchorEl(null)
    if (onDownload) onDownload(item)
  }

  return (
    <ListItem
      disablePadding
      secondaryAction={
        <IconButton
          onClick={(e) => setAnchorEl(e.currentTarget)}
          size="small"
          sx={{ opacity: 0.5, '&:hover': { opacity: 1 } }}
        >
          <MoreIcon fontSize="small" />
        </IconButton>
      }
    >
      <ListItemButton onClick={handleClick} sx={{ py: 1 }}>
        <ListItemIcon sx={{ minWidth: 40 }}>
          {item.is_file
            ? <FileIcon sx={{ color: 'text.secondary', fontSize: 20 }} />
            : <FolderIcon sx={{ color: 'primary.main', fontSize: 20 }} />
          }
        </ListItemIcon>
        <ListItemText
          primary={item.name}
          primaryTypographyProps={{
            fontSize: '0.875rem',
            fontWeight: item.is_file ? 400 : 500,
          }}
        />
      </ListItemButton>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={() => setAnchorEl(null)}>
        {item.is_file && (
          <MenuItem onClick={() => { setAnchorEl(null); onInfo(item) }}>
            Info
          </MenuItem>
        )}
        <MenuItem onClick={handleDownload}>
          {item.is_file ? 'Download' : 'Download as ZIP'}
        </MenuItem>
        <MenuItem onClick={() => { setAnchorEl(null); onMove(item) }}>
          Move
        </MenuItem>
        <MenuItem onClick={() => { setAnchorEl(null); onDelete(item) }} sx={{ color: 'error.main' }}>
          Delete
        </MenuItem>
      </Menu>
    </ListItem>
  )
}
