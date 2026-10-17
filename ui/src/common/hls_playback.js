// Pure helpers behind the HLS (Matroska) player: how to describe what the
// server found in the file, which audio track to start with, and how hls.js
// must be configured to survive Telegram's chunk latency.

const CODEC_LABELS = {
  h264: 'H.264',
  hevc: 'HEVC',
  vp8: 'VP8',
  vp9: 'VP9',
  av1: 'AV1',
  mpeg4: 'MPEG-4',
  mpeg2video: 'MPEG-2',
  aac: 'AAC',
  ac3: 'AC3',
  eac3: 'E-AC3',
  dts: 'DTS',
  truehd: 'TrueHD',
  flac: 'FLAC',
  opus: 'Opus',
  vorbis: 'Vorbis',
  mp3: 'MP3',
  pcm: 'PCM',
  subrip: 'SRT',
  ass: 'ASS',
  ssa: 'SSA',
  pgs: 'PGS',
  vobsub: 'VobSub',
}

export function codecLabel(codec) {
  if (!codec) return 'unknown'
  return CODEC_LABELS[codec] || codec.toUpperCase()
}

export function channelsLabel(channels) {
  switch (channels) {
    case 1: return 'mono'
    case 2: return 'stereo'
    case 6: return '5.1'
    case 8: return '7.1'
    default: return channels ? `${channels}ch` : ''
  }
}

export function audioTracks(info) {
  return (info?.tracks || []).filter((t) => t.type === 'audio')
}

// The server's default choice: the first audio track flagged as default,
// else the first audio track. Returns the Matroska track number or null.
export function defaultAudioTrack(info) {
  const tracks = audioTracks(info)
  if (tracks.length === 0) return null
  const flagged = tracks.find((t) => t.default)
  return (flagged || tracks[0]).number
}

// A short label for an audio track selector entry: "Castellano (AC3 5.1)".
export function audioTrackLabel(track) {
  const codec = [codecLabel(track.codec), channelsLabel(track.channels)].filter(Boolean).join(' ')
  const name = track.name || (track.language && track.language !== 'und' ? track.language : '')
  return name ? `${name} (${codec})` : codec
}

// Describes what the player receives: the video codec with its size, the
// audio codec, and whether each is copied as is or converted to something
// the browser decodes.
export function describePlayback(info, audioTrackNumber) {
  const video = (info?.tracks || []).find((t) => t.type === 'video')
  const tracks = audioTracks(info)
  const audio = tracks.find((t) => t.number === audioTrackNumber) || tracks[0]
  const parts = []
  if (video) {
    const size = video.width && video.height ? ` ${video.width}×${video.height}` : ''
    const copied = video.codec === 'h264' || video.codec === 'hevc'
    parts.push(`${codecLabel(video.codec)}${size}${copied ? '' : ' → H.264'}`)
  }
  if (audio) {
    const channels = channelsLabel(audio.channels)
    const copied = audio.codec === 'aac'
    parts.push(`${codecLabel(audio.codec)}${channels ? ` ${channels}` : ''}${copied ? '' : ' → AAC stereo'}`)
  }
  return parts.join(' · ')
}

// What to tell the user when the media info says playback cannot happen.
export function playbackUnavailableReason(info) {
  if (!info) return null
  if (info.hls?.available) return null
  if (info.container === 'unsupported') return 'This file is not a Matroska video.'
  if (info.error) return `Could not read the video header: ${info.error}`
  return info.hls?.reason || 'This video cannot be played in the browser.'
}

// Telegram hands a chunk over in seconds to minutes, and every segment
// request may have to wait for one, so the loader timeouts are far above
// hls.js' defaults (10 s to first byte). Buffers are generous so a short
// backward seek replays from memory instead of restarting ffmpeg.
export function buildHlsConfig(token) {
  const loadPolicy = {
    default: {
      maxTimeToFirstByteMs: 240000,
      maxLoadTimeMs: 300000,
      timeoutRetry: { maxNumRetry: 2, retryDelayMs: 1000, maxRetryDelayMs: 4000 },
      errorRetry: { maxNumRetry: 4, retryDelayMs: 1000, maxRetryDelayMs: 8000 },
    },
  }
  return {
    maxBufferLength: 60,
    maxMaxBufferLength: 180,
    backBufferLength: 120,
    maxBufferHole: 1,
    fragLoadPolicy: loadPolicy,
    manifestLoadPolicy: loadPolicy,
    playlistLoadPolicy: loadPolicy,
    xhrSetup: (xhr) => {
      xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    },
  }
}

// Maps an hls.js error event to what the user should read. Only fatal
// errors reach the user; hls.js recovers from the rest on its own.
export function playbackErrorMessage(data) {
  if (!data?.fatal) return null
  const status = data.response?.code
  if (status === 401 || status === 403) return 'Your session is no longer valid. Sign in again.'
  if (status === 503) return 'The server cannot transcode right now (ffmpeg unavailable).'
  if (data.type === 'networkError') return 'Lost the connection to the server while loading the video.'
  if (data.type === 'mediaError') return 'Your browser could not decode this video.'
  return 'Playback failed.'
}
