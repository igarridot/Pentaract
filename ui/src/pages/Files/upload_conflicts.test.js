import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildUploadEntries, normalizeUploadPath, resolveUploadEntries,
  fileNameFromPath, fileNamesOf, findSkippedEntries,
} from './upload_conflicts.js'

test('normalizeUploadPath trims slashes and handles empty path', () => {
  assert.equal(normalizeUploadPath('/a/b/'), 'a/b')
  assert.equal(normalizeUploadPath('a/b'), 'a/b')
  assert.equal(normalizeUploadPath('/'), '')
  assert.equal(normalizeUploadPath(''), '')
})

test('buildUploadEntries resolves target path for single files and directory uploads', () => {
  const files = [
    { name: 'one.txt' },
    { name: 'nested.txt', webkitRelativePath: 'photos/2026/nested.txt' },
  ]

  const entries = buildUploadEntries(files, '/root/base/')

  assert.deepEqual(entries.map((e) => ({ filename: e.filename, targetPath: e.targetPath })), [
    { filename: 'one.txt', targetPath: 'root/base' },
    { filename: 'nested.txt', targetPath: 'root/base/photos/2026' },
  ])
})

test('resolveUploadEntries asks once and applies keep_both to all conflicts when applyForAll is enabled', async () => {
  const entries = [
    { file: { name: 'a.txt' }, filename: 'a.txt', targetPath: 'docs' },
    { file: { name: 'b.txt' }, filename: 'b.txt', targetPath: 'docs' },
    { file: { name: 'c.txt' }, filename: 'c.txt', targetPath: 'docs' },
  ]

  const asked = []
  const resolved = await resolveUploadEntries(
    entries,
    async (_, filename) => filename !== 'c.txt',
    async (filename, targetPath) => {
      asked.push({ filename, targetPath })
      return { action: 'keep_both', applyForAll: true }
    },
  )

  assert.equal(asked.length, 1)
  assert.deepEqual(asked[0], { filename: 'a.txt', targetPath: 'docs' })
  assert.equal(resolved.length, 3)
})

test('resolveUploadEntries skips conflicted files when decision is skip with applyForAll', async () => {
  const entries = [
    { file: { name: 'a.txt' }, filename: 'a.txt', targetPath: '' },
    { file: { name: 'b.txt' }, filename: 'b.txt', targetPath: '' },
    { file: { name: 'c.txt' }, filename: 'c.txt', targetPath: '' },
  ]

  const resolved = await resolveUploadEntries(
    entries,
    async (_, filename) => filename !== 'c.txt',
    async () => ({ action: 'skip', applyForAll: true }),
  )

  assert.deepEqual(resolved.map((e) => e.filename), ['c.txt'])
})

test('fileNameFromPath returns the last path segment', () => {
  assert.equal(fileNameFromPath('media/clips/a.mp4'), 'a.mp4')
  assert.equal(fileNameFromPath('a.mp4'), 'a.mp4')
  assert.equal(fileNameFromPath(''), '')
  assert.equal(fileNameFromPath(undefined), '')
})

test('fileNamesOf collects only file names from a listing', () => {
  const names = fileNamesOf([
    { name: 'a.txt', is_file: true },
    { name: 'docs', is_file: false },
    { name: 'b.txt', is_file: true },
  ])
  assert.deepEqual([...names].sort(), ['a.txt', 'b.txt'])
  assert.equal(fileNamesOf(null).size, 0)
})

test('findSkippedEntries reports entries dropped by conflict resolution', () => {
  const entries = [
    { targetPath: 'docs', filename: 'a.txt' },
    { targetPath: 'docs', filename: 'b.txt' },
    { targetPath: '', filename: 'a.txt' },
  ]
  const skipped = findSkippedEntries(entries, [entries[0], entries[2]])
  assert.deepEqual(skipped, [entries[1]])
  assert.deepEqual(findSkippedEntries(entries, entries), [])
})
