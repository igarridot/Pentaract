import { useEffect, useRef } from 'react'
import API from '../api'
import { createWatchReporter, readablePosition } from './watch_progress'

// useWatchProgress reports where the user is in the video videoRef points
// at, while enabled: every few seconds while playing, and on pause, end and
// close (also of the tab). onSaved runs once the last report landed (player closed or file
// changed), so the listing can show the new progress.
export function useWatchProgress(videoRef, { storageId, path, enabled = true, onSaved }) {
  const onSavedRef = useRef(onSaved)
  useEffect(() => { onSavedRef.current = onSaved })

  useEffect(() => {
    const video = videoRef.current
    if (!enabled || !video || !storageId || !path) return undefined

    const reporter = createWatchReporter({
      save: (position, duration) => API.files.saveWatchProgress(storageId, path, position, duration),
    })
    const record = () => {
      const current = readablePosition(video)
      if (current) reporter.update(current.position, current.duration)
    }
    const onTimeUpdate = () => { if (!video.paused) record() }
    const onPause = () => { record(); reporter.flush() }
    const onEnded = () => {
      if (video.duration > 0) reporter.update(video.duration, video.duration)
      reporter.flush()
    }

    // Closing the tab unmounts nothing: save there too, or the last few
    // seconds (maybe the ones crossing into "watched") would be lost.
    const onPageHide = () => { record(); reporter.flush() }

    video.addEventListener('timeupdate', onTimeUpdate)
    video.addEventListener('pause', onPause)
    video.addEventListener('ended', onEnded)
    window.addEventListener('pagehide', onPageHide)
    return () => {
      video.removeEventListener('timeupdate', onTimeUpdate)
      video.removeEventListener('pause', onPause)
      video.removeEventListener('ended', onEnded)
      window.removeEventListener('pagehide', onPageHide)
      reporter.flush().then((saved) => { if (saved) onSavedRef.current?.() })
    }
  }, [videoRef, storageId, path, enabled])
}
