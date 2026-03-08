import { Dialog, DialogTitle, DialogContent, DialogActions, Button, Box } from '@mui/material'

function getVideoMime(name) {
  const ext = name?.split('.').pop()?.toLowerCase() || ''
  if (ext === 'mp4' || ext === 'm4v') return 'video/mp4'
  if (ext === 'webm') return 'video/webm'
  if (ext === 'ogg') return 'video/ogg'
  if (ext === 'mov') return 'video/quicktime'
  return undefined
}

export default function MediaPreviewDialog({ open, file, mediaType, src, onClose, onDownload }) {
  const videoMime = getVideoMime(file?.name)

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>{file?.name || 'Preview'}</DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 240 }}>
          {mediaType === 'image' && src && (
            <Box component="img" src={src} alt={file?.name || 'Image'} sx={{ maxWidth: '100%', maxHeight: '70vh', objectFit: 'contain' }} />
          )}
          {mediaType === 'video' && src && (
            <Box component="video" controls preload="metadata" sx={{ width: '100%', maxHeight: '70vh' }}>
              <source src={src} type={videoMime} />
            </Box>
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
