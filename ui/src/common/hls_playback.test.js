import test from 'node:test'
import assert from 'node:assert/strict'
import {
  audioTrackLabel,
  audioTracks,
  buildHlsConfig,
  channelsLabel,
  codecLabel,
  defaultAudioTrack,
  describePlayback,
  playbackErrorMessage,
  playbackUnavailableReason,
  shouldFallbackToTranscode,
} from './hls_playback.js'

const info = {
  container: 'matroska',
  duration_seconds: 5400,
  tracks: [
    { number: 1, type: 'video', codec: 'hevc', width: 3840, height: 2160, default: true },
    { number: 2, type: 'audio', codec: 'eac3', channels: 6, language: 'eng', default: false },
    { number: 3, type: 'audio', codec: 'aac', channels: 2, language: 'spa', name: 'Castellano', default: true },
    { number: 4, type: 'subtitle', codec: 'pgs', default: true },
  ],
  hls: { available: true, video_copy: true, audio_copy: false, segment_seconds: 6 },
}

test('codecLabel and channelsLabel map known values and fall back sensibly', () => {
  assert.equal(codecLabel('h264'), 'H.264')
  assert.equal(codecLabel('eac3'), 'E-AC3')
  assert.equal(codecLabel('x_weird'), 'X_WEIRD')
  assert.equal(codecLabel(undefined), 'unknown')
  assert.equal(channelsLabel(2), 'stereo')
  assert.equal(channelsLabel(6), '5.1')
  assert.equal(channelsLabel(3), '3ch')
  assert.equal(channelsLabel(undefined), '')
})

test('audio track helpers pick the default track and label entries', () => {
  assert.deepEqual(audioTracks(info).map((t) => t.number), [2, 3])
  assert.equal(defaultAudioTrack(info), 3)
  assert.equal(defaultAudioTrack({ tracks: [{ number: 7, type: 'audio', codec: 'ac3' }] }), 7)
  assert.equal(defaultAudioTrack({ tracks: [] }), null)
  assert.equal(defaultAudioTrack(null), null)
  assert.equal(audioTrackLabel(info.tracks[2]), 'Castellano (AAC stereo)')
  assert.equal(audioTrackLabel(info.tracks[1]), 'eng (E-AC3 5.1)')
  assert.equal(audioTrackLabel({ codec: 'dts', channels: 6, language: 'und' }), 'DTS 5.1')
})

test('describePlayback says what is copied and what is converted', () => {
  assert.equal(describePlayback(info, 3), 'HEVC 3840×2160 · AAC stereo')
  assert.equal(describePlayback(info, 2), 'HEVC 3840×2160 · E-AC3 5.1 → AAC stereo')
  assert.equal(describePlayback({ tracks: [{ type: 'video', codec: 'vp9' }] }), 'VP9 → H.264')
  assert.equal(describePlayback({ tracks: [] }), '')
  assert.equal(describePlayback(null), '')
  assert.equal(describePlayback(info, 3, { transcodeVideo: true }), 'HEVC 3840×2160 → H.264 · AAC stereo')
})

test('shouldFallbackToTranscode retries a decode error once, on copied video only', () => {
  const MEDIA_ERR_NETWORK = 2
  const MEDIA_ERR_DECODE = 3
  // Chrome on macOS: VideoToolbox rejected a Blu-ray H.264 copy.
  assert.equal(shouldFallbackToTranscode(MEDIA_ERR_DECODE, { videoCopy: true, transcoding: false }), true)
  // Already re-encoded: a second decode error is the user's to see.
  assert.equal(shouldFallbackToTranscode(MEDIA_ERR_DECODE, { videoCopy: true, transcoding: true }), false)
  // The server already transcodes this codec; nothing else to try.
  assert.equal(shouldFallbackToTranscode(MEDIA_ERR_DECODE, { videoCopy: false, transcoding: false }), false)
  assert.equal(shouldFallbackToTranscode(MEDIA_ERR_NETWORK, { videoCopy: true, transcoding: false }), false)
  assert.equal(shouldFallbackToTranscode(undefined, { videoCopy: true, transcoding: false }), false)
})

test('playbackUnavailableReason explains server answers', () => {
  assert.equal(playbackUnavailableReason(info), null)
  assert.equal(playbackUnavailableReason(null), null)
  assert.equal(playbackUnavailableReason({ container: 'unsupported', hls: { available: false } }), 'This file is not a Matroska video.')
  assert.equal(
    playbackUnavailableReason({ container: 'unknown', error: 'telegram down', hls: { available: false } }),
    'Could not read the video header: telegram down',
  )
  assert.equal(
    playbackUnavailableReason({ container: 'matroska', hls: { available: false, reason: 'ffmpeg not found' } }),
    'ffmpeg not found',
  )
  assert.equal(
    playbackUnavailableReason({ container: 'matroska', hls: { available: false } }),
    'This video cannot be played in the browser.',
  )
})

test('buildHlsConfig authenticates requests and waits long enough for Telegram', () => {
  const config = buildHlsConfig('tok')
  const headers = {}
  config.xhrSetup({ setRequestHeader: (name, value) => { headers[name] = value } })
  assert.equal(headers.Authorization, 'Bearer tok')
  assert.ok(config.fragLoadPolicy.default.maxTimeToFirstByteMs >= 120000)
  assert.ok(config.fragLoadPolicy.default.maxLoadTimeMs >= config.fragLoadPolicy.default.maxTimeToFirstByteMs)
  assert.equal(config.manifestLoadPolicy, config.fragLoadPolicy)
  assert.ok(config.backBufferLength >= 60)
})

test('playbackErrorMessage only speaks on fatal errors', () => {
  assert.equal(playbackErrorMessage({ fatal: false, type: 'networkError' }), null)
  assert.equal(playbackErrorMessage(null), null)
  assert.equal(playbackErrorMessage({ fatal: true, type: 'networkError', response: { code: 401 } }), 'Your session is no longer valid. Sign in again.')
  assert.equal(playbackErrorMessage({ fatal: true, type: 'networkError', response: { code: 503 } }), 'The server cannot transcode right now (ffmpeg unavailable).')
  assert.equal(playbackErrorMessage({ fatal: true, type: 'networkError' }), 'Lost the connection to the server while loading the video.')
  assert.equal(playbackErrorMessage({ fatal: true, type: 'mediaError' }), 'Your browser could not decode this video.')
  assert.equal(playbackErrorMessage({ fatal: true, type: 'otherError' }), 'Playback failed.')
})
