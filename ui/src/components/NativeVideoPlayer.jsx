import { useRef } from 'react'
import { Box } from '@mui/material'
import { useWatchProgress } from '../common/use_watch_progress'
import ResumeNotice from './ResumeNotice'
import PlayerFrame from './PlayerFrame'

// NativeVideoPlayer plays the formats browsers handle themselves (MP4,
// WebM...) straight from the inline download, resuming at startAt. autoPlay
// starts it on its own (next video in the play queue); onEnded runs when it
// played to the end.
export default function NativeVideoPlayer({ storageId, file, src, type, startAt = 0, autoPlay = false, onEnded, onWatchSaved }) {
  const videoRef = useRef(null)
  useWatchProgress(videoRef, { storageId, path: file?.path, onSaved: onWatchSaved })

  const resume = (event) => {
    const video = event.currentTarget
    if (startAt > 0 && startAt < video.duration) video.currentTime = startAt
  }

  return (
    <Box sx={{ width: '100%', display: 'flex', flexDirection: 'column', gap: 1 }}>
      <PlayerFrame maxHeight="70vh">
        <Box component="video" slot="media" ref={videoRef} preload="metadata" autoPlay={autoPlay} onLoadedMetadata={resume} onEnded={onEnded} sx={{ width: '100%' }}>
          <source src={src} type={type} />
        </Box>
      </PlayerFrame>
      <ResumeNotice startAt={startAt} videoRef={videoRef} />
    </Box>
  )
}
