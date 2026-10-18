// Watch progress: where the user is in a video, saved on the server so the
// web UI and Kodi resume from the same point. The server decides what counts
// as watched or worth resuming; this module only decides when to report.

export const WATCH_SAVE_INTERVAL_MS = 10_000

// HTMLMediaElement.HAVE_CURRENT_DATA, spelled out so this stays DOM-free.
const HAVE_CURRENT_DATA = 2

// createWatchReporter batches a player's positions into one save every
// intervalMs while playing, plus one on flush (pause, end, close). save
// receives (positionSeconds, durationSeconds) and returns a promise.
export function createWatchReporter({ save, intervalMs = WATCH_SAVE_INTERVAL_MS, now = Date.now }) {
  let latest = null
  let sent = null
  let lastSaveAt = now()

  const send = () => {
    if (!latest || (sent && sent.position === latest.position && sent.duration === latest.duration)) {
      return Promise.resolve(false)
    }
    sent = latest
    lastSaveAt = now()
    return Promise.resolve(save(latest.position, latest.duration)).then(() => true, () => false)
  }

  return {
    update(position, duration) {
      if (!Number.isFinite(position) || !Number.isFinite(duration) || duration <= 0) return
      latest = { position, duration }
      if (now() - lastSaveAt >= intervalMs) send()
    },
    // Resolves to whether something was saved.
    flush: send,
  }
}

// readablePosition returns the video's position when it means something:
// not while it is seeking or has no frame yet (a fresh or reset player sits
// at 0 then, which would wipe the resume point).
export function readablePosition(video) {
  if (!video || video.seeking || video.readyState < HAVE_CURRENT_DATA) return null
  if (!(video.duration > 0) || !Number.isFinite(video.duration)) return null
  return { position: video.currentTime, duration: video.duration }
}

export function resumePosition(watch) {
  const position = watch?.position_seconds
  return position > 0 ? position : 0
}

// watchSummary describes a listed file's progress, or null if never played.
export function watchSummary(watch) {
  if (!watch) return null
  const position = resumePosition(watch)
  const duration = watch.duration_seconds > 0 ? watch.duration_seconds : 0
  const percent = duration ? Math.min(100, Math.round((position / duration) * 100)) : 0
  return {
    watched: !!watch.watched,
    inProgress: position > 0,
    percent,
    label: position > 0 ? `${formatPlaybackTime(position)} / ${formatPlaybackTime(duration)}` : '',
  }
}

// watchActions lists the hand marks that make sense for a listed video:
// "watched" unless it already is, "pending" when it is watched or has a
// resume point to forget.
export function watchActions(watch) {
  const summary = watchSummary(watch)
  const actions = []
  if (!summary?.watched) actions.push('watched')
  if (summary?.watched || summary?.inProgress) actions.push('pending')
  return actions
}

// withWatch returns items with the file at path carrying watch instead.
export function withWatch(items, path, watch) {
  if (!items) return items
  return items.map((item) => (item.is_file && item.path === path ? { ...item, watch } : item))
}

// formatPlaybackTime renders seconds as m:ss or h:mm:ss.
export function formatPlaybackTime(seconds) {
  const total = Math.max(0, Math.floor(Number(seconds) || 0))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = String(total % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`
}
