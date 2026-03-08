import test from 'node:test'
import assert from 'node:assert/strict'

import { buildUploadEntries, normalizeUploadPath, resolveUploadEntries } from './upload_conflicts.js'

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
