import {
  Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography, Box,
} from '@mui/material'
import { convertSize } from '../common/size_converter'

export default function FileInfo({ file, open, onClose }) {
  if (!file) return null

  const rows = [
    { label: 'Name', value: file.name },
    { label: 'Path', value: file.path },
    { label: 'Size', value: convertSize(file.size) },
  ]

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>File Info</DialogTitle>
      <DialogContent>
        {rows.map((row) => (
          <Box key={row.label} sx={{ display: 'flex', py: 1, borderBottom: '1px solid rgba(0,0,0,0.05)', '&:last-child': { borderBottom: 'none' } }}>
            <Typography variant="body2" color="text.secondary" sx={{ width: 64, flexShrink: 0, fontWeight: 500 }}>
              {row.label}
            </Typography>
            <Typography variant="body2" sx={{ wordBreak: 'break-all' }}>
              {row.value}
            </Typography>
          </Box>
        ))}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Done</Button>
      </DialogActions>
    </Dialog>
  )
}
