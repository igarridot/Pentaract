import { lazy, Suspense } from 'react'
import { Dialog, DialogTitle, DialogContent, DialogActions, Button, Box, CircularProgress } from '@mui/material'
import { getVideoMime } from '../pages/Files/operations'
import { resumePosition } from '../common/watch_progress'
import NativeVideoPlayer from './NativeVideoPlayer'

// hls.js is only needed for Matroska files, so it stays out of the main bundle.
const HlsPlayer = lazy(() => import('./HlsPlayer'))

// onWatchSaved runs after a video player saved where the user stopped.
export default function MediaPreviewDialog({ open, file, mediaType, src, storageId, onClose, onDownload, onWatchSaved }) {
  const videoMime = getVideoMime(file?.name)
  const startAt = resumePosition(file?.watch)

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>{file?.name || 'Preview'}</DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 240 }}>
          {mediaType === 'image' && src && (
            <Box component="img" src={src} alt={file?.name || 'Image'} sx={{ maxWidth: '100%', maxHeight: '70vh', objectFit: 'contain' }} />
          )}
          {mediaType === 'video' && src && (
            <NativeVideoPlayer storageId={storageId} file={file} src={src} type={videoMime} startAt={startAt} onWatchSaved={onWatchSaved} />
          )}
          {mediaType === 'hls' && open && file && (
            <Suspense fallback={<CircularProgress size={24} />}>
              <HlsPlayer storageId={storageId} file={file} startAt={startAt} onWatchSaved={onWatchSaved} />
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
