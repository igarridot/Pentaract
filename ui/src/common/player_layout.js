// How the player's control bar adapts to the room it has. Media Chrome
// measures its own controller and sets breakpointsm (>= 384 px wide) and
// breakpointmd (>= 576 px) on it, so these rules follow the player's width,
// not the window's: a phone in portrait and a narrow dialog on a desktop get
// the same compact bar.

// Never hidden: without them the player cannot be driven at all.
export const ESSENTIAL_CONTROLS = [
  'media-play-button',
  'media-seek-backward-button',
  'media-seek-forward-button',
  'media-time-range',
  'media-time-display',
  'media-fullscreen-button',
]

// Dropped under 576 px: a phone sets the volume with its own buttons, and
// speed and picture-in-picture are rarely needed there.
export const COMPACT_HIDDEN_CONTROLS = [
  'media-volume-range',
  'media-playback-rate-button',
  'media-pip-button',
]

// compactControlsSx returns the sx rules for the MediaController. Under
// 576 px the time bar moves to a row of its own above the buttons, so it
// keeps a usable length and the buttons never get pushed out of view; under
// 384 px the buttons also lose some padding.
export function compactControlsSx() {
  const narrow = '&:not([breakpointmd])'
  return {
    [COMPACT_HIDDEN_CONTROLS.map((tag) => `${narrow} ${tag}`).join(', ')]: { display: 'none' },
    [`${narrow} media-control-bar`]: { flexWrap: 'wrap', width: '100%' },
    // Media Chrome gives the time bar min-height: 100%, which on a row of its
    // own takes the whole bar and pushes the buttons out of the player.
    [`${narrow} media-time-range`]: { order: -1, flexBasis: '100%', minHeight: 'auto' },
    [`${narrow} media-time-display`]: { marginLeft: 'auto' },
    '&:not([breakpointsm])': { '--media-control-padding': '8px' },
  }
}
