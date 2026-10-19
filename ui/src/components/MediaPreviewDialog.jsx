import { lazy, Suspense, useState } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions, Button, Box, CircularProgress, Typography, useMediaQuery,
} from '@mui/material'
import { PlaylistPlay as AutoplayOnIcon, PlaylistRemove as AutoplayOffIcon } from '@mui/icons-material'
import { getVideoMime } from '../pages/Files/operations'
import { resumePosition } from '../common/watch_progress'
import { readAutoplay, writeAutoplay } from '../common/play_queue'

// The players (hls.js, Media Chrome controls) are only needed once a video
// is opened, so they stay out of the main bundle.
const HlsPlayer = lazy(() => import('./HlsPlayer'))
const NativeVideoPlayer = lazy(() => import('./NativeVideoPlayer'))

// Reading localStorage itself can throw when site data is blocked.
function browserStorage() {
  try {
    return window.localStorage
  } catch {
    return null
  }
}

// onWatchSaved runs after a video player saved where the user stopped.
// nextFile is the video that follows in the listing: when the current one
// ends and autoplay is on, onPlayNext(nextFile) opens it, and the dialog
// starts it on its own when autoPlay is set.
export default function MediaPreviewDialog({
  open, file, mediaType, src, storageId, autoPlay = false, nextFile, onPlayNext, onClose, onDownload, onWatchSaved,
}) {
  const videoMime = getVideoMime(file?.name)
  const startAt = resumePosition(file?.watch)
  const isVideo = mediaType === 'video' || mediaType === 'hls'
  const [autoplayNext, setAutoplayNext] = useState(() => readAutoplay(browserStorage()))
  // On a phone the dialog takes the whole screen: its margins would
  // otherwise leave the player too narrow for its controls.
  const phone = useMediaQuery((theme) => theme.breakpoints.down('sm'))

  const toggleAutoplay = () => {
    const enabled = !autoplayNext
    setAutoplayNext(enabled)
    writeAutoplay(browserStorage(), enabled)
  }

  const handleEnded = () => {
    if (autoplayNext && nextFile) onPlayNext?.(nextFile)
  }

  // Each file gets its own player (key), so moving to the next video starts
  // from a clean element, resume notice and server session.
  const playerProps = {
    storageId,
    file,
    startAt,
    autoPlay,
    onEnded: handleEnded,
    onWatchSaved,
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth fullScreen={phone}>
      <DialogTitle noWrap title={file?.name} sx={{ fontSize: '1rem', py: 1.5, px: { xs: 2, sm: 3 } }}>
        {file?.name || 'Preview'}
      </DialogTitle>
      <DialogContent sx={{ px: { xs: 1, sm: 2 }, pb: 1 }}>
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: isVideo ? 0 : 240 }}>
          {mediaType === 'image' && src && (
            <Box component="img" src={src} alt={file?.name || 'Image'} sx={{ maxWidth: '100%', maxHeight: '70vh', objectFit: 'contain' }} />
          )}
          {mediaType === 'video' && src && (
            <Suspense fallback={<CircularProgress size={24} />}>
              <NativeVideoPlayer {...playerProps} key={file?.path} src={src} type={videoMime} />
            </Suspense>
          )}
          {mediaType === 'hls' && open && file && (
            <Suspense fallback={<CircularProgress size={24} />}>
              <HlsPlayer {...playerProps} key={file?.path} />
            </Suspense>
          )}
        </Box>
      </DialogContent>
      <DialogActions disableSpacing sx={{ flexWrap: 'wrap', gap: 1, px: { xs: 1, sm: 2 } }}>
        {isVideo && (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mr: 'auto', minWidth: 0, flexBasis: { xs: '100%', sm: 'auto' } }}>
            <Button
              size="small"
              variant="contained"
              aria-pressed={autoplayNext}
              startIcon={autoplayNext ? <AutoplayOnIcon /> : <AutoplayOffIcon />}
              onClick={toggleAutoplay}
            >
              {autoplayNext ? 'Autoplay on' : 'Autoplay off'}
            </Button>
            {autoplayNext && nextFile && (
              <Typography variant="caption" color="text.secondary" noWrap title={nextFile.name}>
                Up next: {nextFile.name}
              </Typography>
            )}
          </Box>
        )}
        <Button size="small" onClick={onClose} color="inherit" sx={{ ml: { xs: 'auto', sm: 0 } }}>Close</Button>
        <Button size="small" onClick={onDownload} variant="contained">Download</Button>
      </DialogActions>
    </Dialog>
  )
}
