import test from 'node:test'
import assert from 'node:assert/strict'
import {
  createWatchReporter,
  formatPlaybackTime,
  readablePosition,
  resumePosition,
  watchActions,
  watchState,
  watchSummary,
  withWatch,
} from './watch_progress.js'

function makeReporter() {
  let clock = 0
  const saves = []
  const reporter = createWatchReporter({
    save: (position, duration) => { saves.push([position, duration]); return Promise.resolve() },
    intervalMs: 10_000,
    now: () => clock,
  })
  return { reporter, saves, advance: (ms) => { clock += ms } }
}

test('reporter saves at most once per interval while playing', () => {
  const { reporter, saves, advance } = makeReporter()
  reporter.update(1, 100)
  advance(5_000)
  reporter.update(6, 100)
  assert.deepEqual(saves, [])
  advance(5_000)
  reporter.update(11, 100)
  assert.deepEqual(saves, [[11, 100]])
  advance(1_000)
  reporter.update(12, 100)
  assert.deepEqual(saves, [[11, 100]])
})

test('flush saves the latest position once, and nothing when unchanged', async () => {
  const { reporter, saves } = makeReporter()
  assert.equal(await reporter.flush(), false)
  reporter.update(42, 100)
  assert.equal(await reporter.flush(), true)
  assert.equal(await reporter.flush(), false)
  assert.deepEqual(saves, [[42, 100]])
})

test('reporter ignores unusable positions', async () => {
  const { reporter, saves } = makeReporter()
  reporter.update(NaN, 100)
  reporter.update(10, Infinity)
  reporter.update(10, 0)
  assert.equal(await reporter.flush(), false)
  assert.deepEqual(saves, [])
})

test('a failed save resolves to false instead of throwing', async () => {
  const reporter = createWatchReporter({ save: () => Promise.reject(new Error('offline')), now: () => 0 })
  reporter.update(42, 100)
  assert.equal(await reporter.flush(), false)
})

test('readablePosition skips players that are seeking or have no frame yet', () => {
  const video = { seeking: false, readyState: 4, currentTime: 42, duration: 100 }
  assert.deepEqual(readablePosition(video), { position: 42, duration: 100 })
  assert.equal(readablePosition({ ...video, seeking: true }), null)
  assert.equal(readablePosition({ ...video, readyState: 1 }), null)
  assert.equal(readablePosition({ ...video, duration: NaN }), null)
  assert.equal(readablePosition({ ...video, duration: Infinity }), null)
  assert.equal(readablePosition(null), null)
})

test('resumePosition and watchSummary read the server progress', () => {
  assert.equal(resumePosition(undefined), 0)
  assert.equal(resumePosition({ position_seconds: 0, watched: true }), 0)
  assert.equal(resumePosition({ position_seconds: 2530 }), 2530)

  assert.equal(watchSummary(undefined), null)
  assert.deepEqual(watchSummary({ position_seconds: 2530, duration_seconds: 5400, watched: false }), {
    watched: false, inProgress: true, percent: 47, label: '42:10 / 1:30:00',
  })
  assert.deepEqual(watchSummary({ position_seconds: 0, duration_seconds: 5400, watched: true }), {
    watched: true, inProgress: false, percent: 0, label: '',
  })
})

test('formatPlaybackTime uses m:ss and h:mm:ss', () => {
  assert.equal(formatPlaybackTime(0), '0:00')
  assert.equal(formatPlaybackTime(65.9), '1:05')
  assert.equal(formatPlaybackTime(3725), '1:02:05')
  assert.equal(formatPlaybackTime(-3), '0:00')
})

test('watchActions offers the marks that change something', () => {
  assert.deepEqual(watchActions(undefined), ['watched'])
  assert.deepEqual(watchActions({ position_seconds: 0, duration_seconds: 100, watched: false }), ['watched'])
  assert.deepEqual(watchActions({ position_seconds: 42, duration_seconds: 100, watched: false }), ['watched', 'pending'])
  assert.deepEqual(watchActions({ position_seconds: 0, duration_seconds: 100, watched: true }), ['pending'])
  assert.deepEqual(watchActions({ position_seconds: 42, duration_seconds: 100, watched: true }), ['pending'])
})

test('withWatch replaces the progress of one file only', () => {
  const items = [
    { path: 'a.mkv', is_file: true },
    { path: 'b.mkv', is_file: true, watch: { watched: false } },
    { path: 'a.mkv/', is_file: false },
  ]
  const watch = { watched: true, position_seconds: 0 }
  const updated = withWatch(items, 'a.mkv', watch)
  assert.equal(updated[0].watch, watch)
  assert.equal(updated[1], items[1])
  assert.equal(updated[2], items[2])
  assert.equal(items[0].watch, undefined)
  assert.equal(withWatch(null, 'a.mkv', watch), null)
})

test('watchState picks the icon state of a listed video', () => {
  assert.equal(watchState(undefined), 'unplayed')
  assert.equal(watchState({ position_seconds: 0, duration_seconds: 100, watched: false }), 'unplayed')
  assert.equal(watchState({ position_seconds: 42, duration_seconds: 100, watched: false }), 'in_progress')
  assert.equal(watchState({ position_seconds: 0, duration_seconds: 100, watched: true }), 'watched')
  assert.equal(watchState({ position_seconds: 42, duration_seconds: 100, watched: true }), 'watched')
})
