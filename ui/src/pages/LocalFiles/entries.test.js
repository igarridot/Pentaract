import test from 'node:test'
import assert from 'node:assert/strict'

import { buildLocalUploadEntries, normalizeLocalPath } from './entries.js'

test('normalizeLocalPath trims slashes and normalizes separators', () => {
  assert.equal(normalizeLocalPath('\\mnt\\media\\movies\\'), 'mnt/media/movies')
  assert.equal(normalizeLocalPath('/source/docs/'), 'source/docs')
  assert.equal(normalizeLocalPath(''), '')
})

test('buildLocalUploadEntries preserves the current local subtree below the destination root', () => {
  const entries = buildLocalUploadEntries([
    { path: 'source/docs/report.txt', size: 4 },
    { path: 'source/docs/images/photo.jpg', size: 8 },
    { path: 'source/docs/notes/todo.md', size: 6 },
  ], 'source/docs', 'archive/2026')

  assert.deepEqual(entries, [
    { sourcePath: 'source/docs/report.txt', filename: 'report.txt', size: 4, targetPath: 'archive/2026' },
    { sourcePath: 'source/docs/images/photo.jpg', filename: 'photo.jpg', size: 8, targetPath: 'archive/2026/images' },
    { sourcePath: 'source/docs/notes/todo.md', filename: 'todo.md', size: 6, targetPath: 'archive/2026/notes' },
  ])
})
