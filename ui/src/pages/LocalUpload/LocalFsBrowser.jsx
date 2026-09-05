import {
  Box, Typography, List, ListItem, ListItemIcon, ListItemText, Checkbox, CircularProgress,
} from '@mui/material'
import { Folder as FolderIcon, InsertDriveFile as FileIcon } from '@mui/icons-material'
import { convertSize } from '../../common/size_converter'
import Panel from '../../components/Panel'
import { localEntryKey } from './local_upload_paths'

// Selectable listing of one directory of the local mount. Directories open on
// click; files and directories can be ticked for upload.
export default function LocalFsBrowser({ entries, selected, loading, onToggle, onOpenDir }) {
  if (loading) {
    return (
      <Box sx={{ p: 4, textAlign: 'center' }}>
        <CircularProgress size={28} />
      </Box>
    )
  }

  return (
    <Panel>
      <List disablePadding>
        {entries.map((entry) => {
          const key = localEntryKey(entry)
          const isDir = !entry.is_file
          const openDir = () => isDir && onOpenDir(entry.path)
          const dirCursor = { cursor: isDir ? 'pointer' : 'default' }
          return (
            <ListItem
              key={key}
              sx={{
                borderBottom: '1px solid',
                borderColor: 'divider',
                '&:last-child': { borderBottom: 'none' },
                ...dirCursor,
              }}
              secondaryAction={
                !isDir && entry.size != null ? (
                  <Typography variant="caption" color="text.secondary">
                    {convertSize(entry.size)}
                  </Typography>
                ) : null
              }
            >
              <Checkbox
                edge="start"
                checked={selected.has(key)}
                onChange={() => onToggle(entry)}
                sx={{ mr: 1 }}
              />
              <ListItemIcon sx={{ minWidth: 36, ...dirCursor }} onClick={openDir}>
                {isDir ? <FolderIcon color="primary" /> : <FileIcon color="action" />}
              </ListItemIcon>
              <ListItemText
                primary={entry.name}
                onClick={openDir}
                sx={dirCursor}
                primaryTypographyProps={{ fontSize: '0.875rem' }}
              />
            </ListItem>
          )
        })}
        {entries.length === 0 && (
          <Box sx={{ p: 4, textAlign: 'center' }}>
            <Typography color="text.secondary" variant="body2">
              Empty directory
            </Typography>
          </Box>
        )}
      </List>
    </Panel>
  )
}
