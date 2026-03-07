import {
  Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography,
} from '@mui/material'
import { convertSize } from '../common/size_converter'

export default function FileInfo({ file, open, onClose }) {
  if (!file) return null

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>File Info</DialogTitle>
      <DialogContent>
        <Typography><strong>Name:</strong> {file.name}</Typography>
        <Typography><strong>Path:</strong> {file.path}</Typography>
        <Typography><strong>Size:</strong> {convertSize(file.size)}</Typography>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  )
}
