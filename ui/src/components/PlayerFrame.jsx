import { Box } from '@mui/material'
import {
  MediaController,
  MediaControlBar,
  MediaFullscreenButton,
  MediaLoadingIndicator,
  MediaMuteButton,
  MediaPipButton,
  MediaPlayButton,
  MediaPlaybackRateButton,
  MediaSeekBackwardButton,
  MediaSeekForwardButton,
  MediaTimeDisplay,
  MediaTimeRange,
  MediaVolumeRange,
} from 'media-chrome/react'

// PlayerFrame puts Media Chrome controls around a <video slot="media">.
// Unlike the browser's own controls, hovering the time bar shows the time a
// click would seek to. The video stays a plain element React renders, so
// hls.js and the watch progress hook keep working on it unchanged.
// maxHeight caps the video in the page only: in fullscreen it fills the
// screen (a cap set on the video itself would outrank Media Chrome's
// fullscreen sizing and leave it small at the top).
export default function PlayerFrame({ maxHeight, children }) {
  return (
    <Box
      component={MediaController}
      sx={{
        display: 'block',
        width: '100%',
        bgcolor: 'black',
        '--media-primary-color': '#fff',
        '--media-range-bar-color': (theme) => theme.palette.primary.main,
        '&:not([mediaisfullscreen]) > video': { maxHeight },
        '&[mediaisfullscreen] > video': { width: '100%', height: '100%', maxHeight: 'none', objectFit: 'contain' },
      }}
    >
      {children}
      <MediaLoadingIndicator slot="centered-chrome" noAutohide />
      <MediaControlBar>
        <MediaPlayButton />
        <MediaSeekBackwardButton seekOffset={10} />
        <MediaSeekForwardButton seekOffset={10} />
        <MediaTimeRange />
        <MediaTimeDisplay showDuration />
        <MediaMuteButton />
        <MediaVolumeRange />
        <MediaPlaybackRateButton />
        <MediaPipButton />
        <MediaFullscreenButton />
      </MediaControlBar>
    </Box>
  )
}
