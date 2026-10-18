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
export default function PlayerFrame({ children }) {
  return (
    <Box
      component={MediaController}
      sx={{
        display: 'block',
        width: '100%',
        bgcolor: 'black',
        '--media-primary-color': '#fff',
        '--media-range-bar-color': (theme) => theme.palette.primary.main,
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
