import { useState } from 'react'
import { Box, Button, Typography } from '@mui/material'
import { formatPlaybackTime } from '../common/watch_progress'

// ResumeNotice tells the user playback picked up where they left it, and
// offers to start over instead.
export default function ResumeNotice({ startAt, videoRef }) {
  const [dismissed, setDismissed] = useState(false)
  if (!(startAt > 0) || dismissed) return null

  const startOver = () => {
    if (videoRef.current) videoRef.current.currentTime = 0
    setDismissed(true)
  }

  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
      <Typography variant="caption" color="text.secondary">
        Resumed at {formatPlaybackTime(startAt)}
      </Typography>
      <Button size="small" onClick={startOver}>Start over</Button>
    </Box>
  )
}
