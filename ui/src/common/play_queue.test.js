import test from 'node:test'
import assert from 'node:assert/strict'
import { isPlayableVideo, nextVideo, readAutoplay, writeAutoplay, AUTOPLAY_STORAGE_KEY } from './play_queue.js'

const file = (path) => ({ path, name: path.split('/').pop(), is_file: true })
const folder = (path) => ({ path, name: path.split('/').pop(), is_file: false })

function memoryStorage(initial = {}) {
  const data = { ...initial }
  return {
    data,
    getItem: (key) => (key in data ? data[key] : null),
    setItem: (key, value) => { data[key] = String(value) },
  }
}

const throwingStorage = {
  getItem: () => { throw new Error('blocked') },
  setItem: () => { throw new Error('blocked') },
}

test('isPlayableVideo accepts native and HLS videos, not folders or other files', () => {
  assert.equal(isPlayableVideo(file('a.mp4')), true)
  assert.equal(isPlayableVideo(file('a.mkv')), true)
  assert.equal(isPlayableVideo(file('a.jpg')), false)
  assert.equal(isPlayableVideo(file('a.txt')), false)
  assert.equal(isPlayableVideo(folder('a.mkv')), false)
  assert.equal(isPlayableVideo(null), false)
})

test('nextVideo skips folders, images and other files', () => {
  const items = [
    folder('Season 2'),
    file('e01.mkv'),
    file('e01.srt'),
    file('cover.jpg'),
    file('e02.mp4'),
    file('e03.mkv'),
  ]
  assert.equal(nextVideo(items, items[1]).path, 'e02.mp4')
  assert.equal(nextVideo(items, items[4]).path, 'e03.mkv')
})

test('nextVideo returns null after the last video', () => {
  const items = [file('e01.mkv'), file('e02.mkv'), file('notes.txt')]
  assert.equal(nextVideo(items, items[1]), null)
})

test('nextVideo returns null when the current file is not listed', () => {
  const items = [file('e01.mkv'), file('e02.mkv')]
  assert.equal(nextVideo(items, file('other.mkv')), null)
  assert.equal(nextVideo(null, items[0]), null)
  assert.equal(nextVideo(items, null), null)
})

test('nextVideo matches the current file by path, so a reloaded listing still works', () => {
  const items = [file('e01.mkv'), file('e02.mkv')]
  assert.equal(nextVideo(items, { ...file('e01.mkv'), watch: { watched: true } }).path, 'e02.mkv')
})

test('autoplay is on by default and remembers being turned off', () => {
  const storage = memoryStorage()
  assert.equal(readAutoplay(storage), true)
  writeAutoplay(storage, false)
  assert.equal(storage.data[AUTOPLAY_STORAGE_KEY], 'off')
  assert.equal(readAutoplay(storage), false)
  writeAutoplay(storage, true)
  assert.equal(readAutoplay(storage), true)
})

test('autoplay falls back to on when storage is missing or throws', () => {
  assert.equal(readAutoplay(undefined), true)
  assert.equal(readAutoplay(throwingStorage), true)
  assert.doesNotThrow(() => writeAutoplay(throwingStorage, false))
  assert.doesNotThrow(() => writeAutoplay(undefined, false))
})
