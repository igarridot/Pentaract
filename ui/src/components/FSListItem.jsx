import { useState } from 'react'
import {
  ListItem, ListItemButton, ListItemIcon, ListItemText,
  IconButton, Menu, MenuItem, Checkbox, Box, LinearProgress, Typography,
} from '@mui/material'
import {
  InsertDriveFile as FileIcon,
  Folder as FolderIcon,
  MoreHoriz as MoreIcon,
  CheckCircle as WatchedIcon,
} from '@mui/icons-material'
import { useNavigate } from 'react-router-dom'
import { getMediaType } from '../pages/Files/operations'
import { watchActions, watchSummary } from '../common/watch_progress'

// WatchStatus shows whether a video was watched and where it was left.
function WatchStatus({ watch }) {
  const summary = watchSummary(watch)
  if (!summary || (!summary.watched && !summary.inProgress)) return null
  return (
    <Box component="span" sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
      {summary.watched && (
        <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, color: 'success.main' }}>
          <WatchedIcon sx={{ fontSize: 14 }} />
          <Typography component="span" variant="caption">Watched</Typography>
        </Box>
      )}
      {summary.inProgress && (
        <>
          <LinearProgress variant="determinate" value={summary.percent} sx={{ width: 80, borderRadius: 1 }} />
          <Typography component="span" variant="caption" color="text.secondary">{summary.label}</Typography>
        </>
      )}
    </Box>
  )
}

export default function FSListItem({
  item,
  storageId,
  onInfo,
  onPreview,
  onDelete,
  onDownload,
  onMove,
  onRename,
  onSetWatched,
  selectionEnabled = false,
  isSelected = false,
  onToggleSelect,
}) {
  const navigate = useNavigate()
  const [anchorEl, setAnchorEl] = useState(null)
  const isPreviewable = item.is_file && getMediaType(item.name) !== null
  const isVideo = item.is_file && ['video', 'hls'].includes(getMediaType(item.name))
  const markActions = isVideo && onSetWatched ? watchActions(item.watch) : []

  const handleClick = () => {
    if (item.is_file) {
      if (isPreviewable && onPreview) onPreview(item)
      else if (onDownload) onDownload(item)
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
        secondaryAction={(
          <>
            {selectionEnabled && item.is_file && (
              <Checkbox
                checked={isSelected}
                onChange={(e) => {
                  e.stopPropagation()
                  if (onToggleSelect) onToggleSelect(item)
                }}
                onClick={(e) => e.stopPropagation()}
                size="small"
              />
            )}
            <IconButton
              onClick={(e) => setAnchorEl(e.currentTarget)}
              size="small"
              sx={{ opacity: 0.5, '&:hover': { opacity: 1 } }}
            >
              <MoreIcon fontSize="small" />
            </IconButton>
          </>
        )}
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
          secondary={item.is_file ? <WatchStatus watch={item.watch} /> : null}
          secondaryTypographyProps={{ component: 'span' }}
          primaryTypographyProps={{
            fontSize: '0.875rem',
            fontWeight: item.is_file ? 400 : 500,
          }}
        />
      </ListItemButton>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={() => setAnchorEl(null)}>
        {item.is_file && isPreviewable && onPreview && (
          <MenuItem onClick={() => { setAnchorEl(null); onPreview(item) }}>
            Preview
          </MenuItem>
        )}
        {markActions.includes('watched') && (
          <MenuItem onClick={() => { setAnchorEl(null); onSetWatched(item, true) }}>
            Mark as watched
          </MenuItem>
        )}
        {markActions.includes('pending') && (
          <MenuItem onClick={() => { setAnchorEl(null); onSetWatched(item, false) }}>
            Mark as unwatched
          </MenuItem>
        )}
        {item.is_file && onInfo && (
          <MenuItem onClick={() => { setAnchorEl(null); onInfo(item) }}>
            Info
          </MenuItem>
        )}
        <MenuItem onClick={handleDownload}>
          {item.is_file ? 'Download' : 'Download as ZIP'}
        </MenuItem>
        {!item.is_file && onRename && (
          <MenuItem onClick={() => { setAnchorEl(null); onRename(item) }}>
            Rename
          </MenuItem>
        )}
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
