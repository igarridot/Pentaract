// Play queue: once a video ends, the preview moves on to the next playable
// video in the listing the user is looking at (folder or search results), in
// the order it is shown. Whether it does is a per-browser preference.

import { getMediaType } from '../pages/Files/operations.js'

export const AUTOPLAY_STORAGE_KEY = 'autoplay_next'

export function isPlayableVideo(item) {
  return !!item?.is_file && ['video', 'hls'].includes(getMediaType(item.name))
}

// nextVideo returns the first playable video listed after current, or null
// when current is the last one or is no longer listed.
export function nextVideo(items, current) {
  if (!Array.isArray(items) || !current) return null
  const index = items.findIndex((item) => item.path === current.path)
  if (index < 0) return null
  return items.slice(index + 1).find(isPlayableVideo) || null
}

// Autoplay is on unless the user turned it off. Storage can be missing or
// throw (private windows, blocked site data), which leaves the default.
export function readAutoplay(storage) {
  try {
    return storage?.getItem(AUTOPLAY_STORAGE_KEY) !== 'off'
  } catch {
    return true
  }
}

export function writeAutoplay(storage, enabled) {
  try {
    storage?.setItem(AUTOPLAY_STORAGE_KEY, enabled ? 'on' : 'off')
  } catch {
    // Not remembered; the toggle still applies to this page.
  }
}
