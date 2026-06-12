import { Box, List, Divider, Typography } from '@mui/material'
import FSListItem from '../../components/FSListItem'

// The file/folder list with dividers and empty state. Selection and row
// actions are delegated to callbacks owned by Files/index.jsx.
export default function FileList({
  items,
  storageId,
  selectedFilePaths,
  isSearchResults,
  onInfo,
  onPreview,
  onDelete,
  onDownload,
  onMove,
  onRename,
  onToggleSelect,
}) {
  return (
    <Box sx={{
      bgcolor: 'background.paper',
      borderRadius: 3,
      border: '1px solid',
      borderColor: 'divider',
      overflow: 'hidden',
    }}>
      <List disablePadding>
        {items.map((item, i) => (
          <Box key={item.path || item.name}>
            {i > 0 && <Divider />}
            <FSListItem
              item={item}
              storageId={storageId}
              onInfo={onInfo}
              onPreview={onPreview}
              onDelete={onDelete}
              onDownload={onDownload}
              onMove={onMove}
              onRename={onRename}
              selectionEnabled
              isSelected={selectedFilePaths.includes(item.path)}
              onToggleSelect={onToggleSelect}
            />
          </Box>
        ))}
        {items.length === 0 && (
          <Box sx={{ p: 4, textAlign: 'center' }}>
            <Typography color="text.secondary" variant="body2">
              {isSearchResults ? 'No results found' : 'Empty folder'}
            </Typography>
          </Box>
        )}
      </List>
    </Box>
  )
}
