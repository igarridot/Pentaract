import { lazy, Suspense } from 'react'
import { Dialog, DialogTitle, DialogContent, DialogActions, Button, Box, CircularProgress } from '@mui/material'
import { getVideoMime } from '../pages/Files/operations'

// hls.js is only needed for Matroska files, so it stays out of the main bundle.
const HlsPlayer = lazy(() => import('./HlsPlayer'))

export default function MediaPreviewDialog({ open, file, mediaType, src, storageId, onClose, onDownload }) {
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
          {mediaType === 'hls' && open && file && (
            <Suspense fallback={<CircularProgress size={24} />}>
              <HlsPlayer storageId={storageId} file={file} />
            </Suspense>
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
