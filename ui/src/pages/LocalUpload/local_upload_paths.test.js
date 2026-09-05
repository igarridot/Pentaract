import test from 'node:test'
import assert from 'node:assert/strict'
import {
  sortLocalEntries, localEntryKey, relativeLocalPath, dirOf, buildDestDir,
  collectLocalFiles, buildLocalBatchItems,
} from './local_upload_paths.js'

test('sortLocalEntries lists directories before files, each by name', () => {
  const sorted = sortLocalEntries([
    { name: 'b.txt', is_file: true },
    { name: 'zeta', is_file: false },
    { name: 'a.txt', is_file: true },
    { name: 'alpha', is_file: false },
  ])
  assert.deepEqual(sorted.map((e) => e.name), ['alpha', 'zeta', 'a.txt', 'b.txt'])
})

test('localEntryKey prefers the path and falls back to the name', () => {
  assert.equal(localEntryKey({ path: 'x/y', name: 'y' }), 'x/y')
  assert.equal(localEntryKey({ path: '', name: 'y' }), 'y')
})

test('relativeLocalPath strips the browse base', () => {
  assert.equal(relativeLocalPath('/a/b/test/file.txt', '/a/b/'), 'test/file.txt')
  assert.equal(relativeLocalPath('a/b/file.txt', ''), 'a/b/file.txt')
  assert.equal(relativeLocalPath('other/file.txt', 'a/b'), 'other/file.txt')
})

test('dirOf and buildDestDir compose the destination directory', () => {
  assert.equal(dirOf('test/file.txt'), 'test')
  assert.equal(dirOf('file.txt'), '')
  assert.equal(buildDestDir('/media/', 'test/file.txt'), 'media/test')
  assert.equal(buildDestDir('', 'test/file.txt'), 'test')
  assert.equal(buildDestDir('media', 'file.txt'), 'media')
  assert.equal(buildDestDir('', 'file.txt'), '')
})

const tree = {
  '': [{ name: 'root.txt', path: 'root.txt', is_file: true }, { name: 'dir', path: 'dir', is_file: false }],
  dir: [{ name: 'inner.txt', path: 'dir/inner.txt', is_file: true }, { name: 'sub', path: 'dir/sub', is_file: false }, { name: 'locked', path: 'dir/locked', is_file: false }],
  'dir/sub': [{ name: 'deep.txt', path: 'dir/sub/deep.txt', is_file: true }],
}
async function browse(path) {
  if (path === 'dir/locked') throw new Error('permission denied')
  return tree[path] || []
}

test('collectLocalFiles walks directories and skips unreadable ones', async () => {
  const files = await collectLocalFiles('dir', browse)
  assert.deepEqual(files.map((f) => f.path), ['dir/inner.txt', 'dir/sub/deep.txt'])
})

test('buildLocalBatchItems keeps the selected directory name under destPath', async () => {
  const selected = [tree[''][0], tree[''][1]]
  const items = await buildLocalBatchItems(selected, '', 'backup', browse)
  assert.deepEqual(items, [
    { local_path: 'root.txt', dest_path: 'backup' },
    { local_path: 'dir/inner.txt', dest_path: 'backup/dir' },
    { local_path: 'dir/sub/deep.txt', dest_path: 'backup/dir/sub' },
  ])
})
