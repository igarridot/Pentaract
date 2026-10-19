import { useEffect, useRef, useState } from 'react'
import { Alert, Box, CircularProgress, FormControl, InputLabel, MenuItem, Select, Typography } from '@mui/material'
import Hls from 'hls.js'
import API from '../api'
import { getRawToken } from '../api/request'
import { useWatchProgress } from '../common/use_watch_progress'
import ResumeNotice from './ResumeNotice'
import PlayerFrame from './PlayerFrame'
import {
  audioTrackLabel,
  audioTracks,
  buildHlsConfig,
  defaultAudioTrack,
  describePlayback,
  playbackErrorMessage,
  playbackUnavailableReason,
  shouldFallbackToTranscode,
} from '../common/hls_playback'

// HlsPlayer plays a Matroska file through the server's on-the-fly HLS
// pipeline. It first asks what the file contains (codecs, audio tracks),
// then feeds the playlist to hls.js, or to the browser's own HLS support
// when Media Source Extensions are missing (iOS Safari). Playback starts at
// startAt, where the user last stopped, and on its own if autoPlay (next
// video in the play queue). onEnded runs when it played to the end.
export default function HlsPlayer({ storageId, file, startAt = 0, autoPlay = false, onEnded, onWatchSaved }) {
  const videoRef = useRef(null)
  // Where playback was when the player was last torn down, so switching
  // audio track resumes at the same point instead of from the start.
  const positionRef = useRef(0)
  const [info, setInfo] = useState(null)
  const [infoError, setInfoError] = useState(null)
  const [audioTrack, setAudioTrack] = useState(null)
  const [playbackError, setPlaybackError] = useState(null)
  // Set when the browser could not decode the copied video: the server then
  // re-encodes it, from where playback stopped.
  const [transcodeVideo, setTranscodeVideo] = useState(false)

  const path = file?.path

  // Describe the file once per preview.
  useEffect(() => {
    let cancelled = false
    setInfo(null)
    setInfoError(null)
    setAudioTrack(null)
    setPlaybackError(null)
    setTranscodeVideo(false)
    positionRef.current = startAt
    if (!storageId || !path) return undefined
    API.files.mediaInfo(storageId, path)
      .then((result) => {
        if (cancelled) return
        setInfo(result)
        setAudioTrack(defaultAudioTrack(result))
      })
      .catch((err) => { if (!cancelled) setInfoError(err.message || 'Could not read the video') })
    return () => { cancelled = true }
  }, [storageId, path, startAt])

  const playable = !!info?.hls?.available
  const videoCopy = !!info?.hls?.video_copy
  const unavailableReason = infoError || playbackUnavailableReason(info)

  useWatchProgress(videoRef, { storageId, path, enabled: playable, onSaved: onWatchSaved })

  // Attach the player once the file is known to be playable, and again when
  // the audio track changes or the video falls back to transcoding (each is
  // its own server session).
  useEffect(() => {
    const video = videoRef.current
    if (!playable || !video || !storageId || !path) return undefined
    setPlaybackError(null)

    const resumeAt = positionRef.current
    const urlOptions = { transcode: transcodeVideo }
    let hls = null
    if (Hls.isSupported()) {
      hls = new Hls({ ...buildHlsConfig(getRawToken()), startPosition: resumeAt > 0 ? resumeAt : -1 })
      hls.on(Hls.Events.ERROR, (_event, data) => {
        const message = playbackErrorMessage(data)
        if (message) setPlaybackError(message)
      })
      hls.loadSource(API.files.hlsPlaylistUrl(storageId, path, audioTrack, urlOptions))
      hls.attachMedia(video)
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = API.files.hlsPlaylistAuthUrl(storageId, path, audioTrack, urlOptions)
      if (resumeAt > 0) {
        video.addEventListener('loadedmetadata', () => { video.currentTime = resumeAt }, { once: true })
      }
    } else {
      setPlaybackError('This browser cannot play HLS video.')
      return undefined
    }

    const onVideoError = () => {
      if (shouldFallbackToTranscode(video.error?.code, { videoCopy, transcoding: transcodeVideo })) {
        setTranscodeVideo(true)
      } else if (video.error?.code === video.error?.MEDIA_ERR_DECODE) {
        setPlaybackError('Your browser could not decode this video.')
      }
    }
    video.addEventListener('error', onVideoError)

    return () => {
      video.removeEventListener('error', onVideoError)
      positionRef.current = video.currentTime || positionRef.current
      if (hls) hls.destroy()
      else video.removeAttribute('src')
    }
  }, [playable, videoCopy, storageId, path, audioTrack, transcodeVideo])

  // Let the server drop its ffmpeg as soon as the preview closes.
  useEffect(() => () => {
    if (storageId && path) API.files.hlsStop(storageId, path).catch(() => {})
  }, [storageId, path])

  const tracks = audioTracks(info)

  return (
    <Box sx={{ width: '100%', display: 'flex', flexDirection: 'column', gap: 1 }}>
      {!info && !infoError && (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, justifyContent: 'center', py: 4 }}>
          <CircularProgress size={20} />
          <Typography variant="body2" color="text.secondary">Reading video header…</Typography>
        </Box>
      )}
      {unavailableReason && <Alert severity="warning">{unavailableReason}</Alert>}
      {playable && (
        <>
          <PlayerFrame maxHeight="60vh">
            <Box component="video" slot="media" ref={videoRef} playsInline autoPlay={autoPlay} onEnded={onEnded} sx={{ width: '100%', bgcolor: 'black' }} />
          </PlayerFrame>
          <ResumeNotice startAt={startAt} videoRef={videoRef} />
          {playbackError && <Alert severity="error">{playbackError}</Alert>}
          {transcodeVideo && videoCopy && !playbackError && (
            <Alert severity="info">Your browser could not decode the original video, so the server is converting it.</Alert>
          )}
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
            <Typography variant="caption" color="text.secondary">{describePlayback(info, audioTrack, { transcodeVideo })}</Typography>
            {tracks.length > 1 && (
              <FormControl size="small" sx={{ minWidth: 200 }}>
                <InputLabel id="hls-audio-track-label">Audio</InputLabel>
                <Select
                  labelId="hls-audio-track-label"
                  label="Audio"
                  value={audioTrack ?? ''}
                  onChange={(e) => setAudioTrack(Number(e.target.value))}
                >
                  {tracks.map((track) => (
                    <MenuItem key={track.number} value={track.number}>{audioTrackLabel(track)}</MenuItem>
                  ))}
                </Select>
              </FormControl>
            )}
          </Box>
        </>
      )}
    </Box>
  )
}
