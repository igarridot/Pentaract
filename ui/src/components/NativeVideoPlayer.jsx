import { useRef } from 'react'
import { Box } from '@mui/material'
import { useWatchProgress } from '../common/use_watch_progress'
import ResumeNotice from './ResumeNotice'
import PlayerFrame from './PlayerFrame'

// NativeVideoPlayer plays the formats browsers handle themselves (MP4,
// WebM...) straight from the inline download, resuming at startAt.
export default function NativeVideoPlayer({ storageId, file, src, type, startAt = 0, onWatchSaved }) {
  const videoRef = useRef(null)
  useWatchProgress(videoRef, { storageId, path: file?.path, onSaved: onWatchSaved })

  const resume = (event) => {
    const video = event.currentTarget
    if (startAt > 0 && startAt < video.duration) video.currentTime = startAt
  }

  return (
    <Box sx={{ width: '100%', display: 'flex', flexDirection: 'column', gap: 1 }}>
      <PlayerFrame>
        <Box component="video" slot="media" ref={videoRef} preload="metadata" onLoadedMetadata={resume} sx={{ width: '100%', maxHeight: '70vh' }}>
          <source src={src} type={type} />
        </Box>
      </PlayerFrame>
      <ResumeNotice startAt={startAt} videoRef={videoRef} />
    </Box>
  )
}
