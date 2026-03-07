import { useState } from 'react'
import {
  ListItem, ListItemButton, ListItemIcon, ListItemText,
  IconButton, Menu, MenuItem,
} from '@mui/material'
import {
  InsertDriveFile as FileIcon,
  Folder as FolderIcon,
  MoreVert as MoreIcon,
} from '@mui/icons-material'
import { useNavigate } from 'react-router-dom'

export default function FSListItem({ item, storageId, currentPath, onInfo, onDelete, onDownload }) {
  const navigate = useNavigate()
  const [anchorEl, setAnchorEl] = useState(null)

  const handleClick = () => {
    if (!item.is_file) {
      const newPath = item.path
      navigate(`/storages/${storageId}/files/${newPath}`)
    }
  }

  const handleDownload = async () => {
    setAnchorEl(null)
    if (onDownload) onDownload(item)
  }

  return (
    <ListItem
      secondaryAction={
        <IconButton onClick={(e) => setAnchorEl(e.currentTarget)}>
          <MoreIcon />
        </IconButton>
      }
    >
      <ListItemButton onClick={handleClick}>
        <ListItemIcon>
          {item.is_file ? <FileIcon /> : <FolderIcon />}
        </ListItemIcon>
        <ListItemText primary={item.name} />
      </ListItemButton>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={() => setAnchorEl(null)}>
        {item.is_file && (
          <MenuItem onClick={() => { setAnchorEl(null); onInfo(item) }}>
            Info
          </MenuItem>
        )}
        {item.is_file && (
          <MenuItem onClick={handleDownload}>
            Download
          </MenuItem>
        )}
        <MenuItem onClick={() => { setAnchorEl(null); onDelete(item) }}>
          Delete
        </MenuItem>
      </Menu>
    </ListItem>
  )
}
