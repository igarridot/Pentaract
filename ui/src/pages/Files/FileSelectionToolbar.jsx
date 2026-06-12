import { Box, Typography, Button, FormControlLabel, Checkbox } from '@mui/material'

// Bulk-selection toolbar shown above the file list when there are selectable
// files. All actions are delegated to callbacks owned by Files/index.jsx.
export default function FileSelectionToolbar({
  selectableCount,
  selectedCount,
  allSelected,
  isBulkOperating,
  onToggleSelectAll,
  onClear,
  onMove,
  onDownload,
  onDelete,
}) {
  const noneSelected = selectedCount === 0
  return (
    <Box sx={{ mb: 1.5, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
      <FormControlLabel
        control={<Checkbox checked={allSelected} onChange={onToggleSelectAll} />}
        label={`Select all files (${selectableCount})`}
      />
      <Button size="small" onClick={onClear} disabled={noneSelected}>
        Clear
      </Button>
      <Button size="small" onClick={onMove} disabled={noneSelected || isBulkOperating}>
        Move selected
      </Button>
      <Button size="small" onClick={onDownload} disabled={noneSelected || isBulkOperating}>
        Download selected
      </Button>
      <Button size="small" color="error" onClick={onDelete} disabled={noneSelected || isBulkOperating}>
        Delete selected
      </Button>
      {selectedCount > 0 && (
        <Typography variant="body2" color="text.secondary">
          {`${selectedCount} selected`}
        </Typography>
      )}
    </Box>
  )
}
