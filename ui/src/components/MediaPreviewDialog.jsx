import { Dialog, DialogTitle, DialogContent, DialogActions, Button, Box } from '@mui/material'

export default function MediaPreviewDialog({ open, file, mediaType, src, onClose, onDownload }) {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>{file?.name || 'Preview'}</DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 240 }}>
          {mediaType === 'image' && src && (
            <Box component="img" src={src} alt={file?.name || 'Image'} sx={{ maxWidth: '100%', maxHeight: '70vh', objectFit: 'contain' }} />
          )}
          {mediaType === 'video' && src && (
            <Box component="video" src={src} controls preload="metadata" sx={{ width: '100%', maxHeight: '70vh' }} />
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} color="inherit">Close</Button>
        <Button onClick={onDownload} variant="contained">Download</Button>
      </DialogActions>
    </Dialog>
  )
}
