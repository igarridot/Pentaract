import test from 'node:test'
import assert from 'node:assert/strict'
import API from './index.js'

function makeStorage(token = null) {
  return {
    getItem(key) {
      if (key === 'access_token') return token
      return null
    },
    setItem() {},
    removeItem() {},
  }
}

function sseResponse(chunks) {
  const encoder = new TextEncoder()
  let idx = 0
  return {
    body: {
      getReader() {
        return {
          async read() {
            if (idx >= chunks.length) return { done: true, value: undefined }
            const value = encoder.encode(chunks[idx++])
            return { done: false, value }
          },
        }
      },
    },
  }
}

test('files url builders include auth token and query params', () => {
  globalThis.localStorage = makeStorage('tok')
  const d = API.files.downloadFileUrl('s1', 'folder/a b.txt', 'd1')
  assert.match(d, /^\/api\/storages\/s1\/files\/download\//)
  assert.match(d, /access_token=tok/)
  assert.match(d, /download_id=d1/)

  const p = API.files.previewFileUrl('s1', 'x.mp4')
  assert.match(p, /inline=1/)

  const dd = API.files.downloadDirUrl('s1', 'folder', 'dir1')
  assert.match(dd, /download_id=dir1/)
})

test('url builders and progress subscriptions throw when not authenticated', () => {
  globalThis.localStorage = makeStorage(null)
  assert.throws(() => API.files.downloadFileUrl('s1', 'a.txt', 'd1'), /Not authenticated/)
  assert.throws(() => API.files.subscribeProgress('u1', () => {}), /Not authenticated/)
})

test('files.upload builds multipart body with optional upload id', async () => {
  globalThis.localStorage = makeStorage('tok')
  globalThis.fetch = async (url, opts) => {
    assert.equal(url, '/api/storages/s1/files/upload')
    const fd = opts.body
    assert.equal(fd.get('path'), 'folder')
    assert.equal(fd.get('upload_id'), 'up-1')
    assert.equal(fd.get('on_conflict'), 'keep_both')
    assert.equal(fd.get('file').name, 'a.txt')
    return {
      ok: true,
      status: 200,
      headers: { get: () => null },
      async text() {
        return '{"ok":true}'
      },
    }
  }

  const f = new File(['hello'], 'a.txt')
  const res = await API.files.upload('s1', 'folder', f, 'up-1')
  assert.deepEqual(res, { ok: true })
})

test('files.upload accepts explicit on_conflict policy', async () => {
  globalThis.localStorage = makeStorage('tok')
  globalThis.fetch = async (url, opts) => {
    assert.equal(url, '/api/storages/s1/files/upload')
    const fd = opts.body
    assert.equal(fd.get('on_conflict'), 'skip')
    return {
      ok: true,
      status: 200,
      headers: { get: () => null },
      async text() {
        return '{"ok":true}'
      },
    }
  }

  const f = new File(['hello'], 'a.txt')
  await API.files.upload('s1', 'folder', f, 'up-1', { onConflict: 'skip' })
})

test('files.move sends old/new paths and forwards request options', async () => {
  globalThis.localStorage = makeStorage('tok')
  let gotSignal = false
  globalThis.fetch = async (url, opts) => {
    assert.equal(url, '/api/storages/s1/files/move')
    const body = JSON.parse(opts.body)
    assert.deepEqual(body, { old_path: 'a.txt', new_path: 'dst/a.txt' })
    gotSignal = Boolean(opts.signal)
    return {
      ok: true,
      status: 200,
      headers: { get: () => null },
      async text() {
        return '{"ok":true}'
      },
    }
  }
  const controller = new AbortController()
  await API.files.move('s1', 'a.txt', 'dst/a.txt', { signal: controller.signal })
  assert.equal(gotSignal, true)
})

test('subscribeProgress consumes SSE and stops on done status', async () => {
  globalThis.localStorage = makeStorage('tok')
  let called = null
  globalThis.fetch = async (url, opts) => {
    assert.equal(url, '/api/upload_progress?upload_id=u1')
    assert.equal(opts.headers.Authorization, 'Bearer tok')
    return sseResponse(['data: {"status":"done","uploaded":1}\n\n'])
  }

  const stop = API.files.subscribeProgress('u1', (data) => { called = data })
  await new Promise((r) => setTimeout(r, 0))
  stop()
  assert.deepEqual(called, { status: 'done', uploaded: 1 })
})

test('subscribeDownloadProgress stops cleanly on AbortError', async () => {
  globalThis.localStorage = makeStorage('tok')
  let status = null
  globalThis.fetch = async () => {
    throw Object.assign(new Error('aborted'), { name: 'AbortError' })
  }

  const stop = API.files.subscribeDownloadProgress('d1', (data) => { status = data.status })
  await new Promise((r) => setTimeout(r, 0))
  stop()
  assert.equal(status, null)
})

test('subscribeDeleteProgress retries after transient error and then completes', async () => {
  globalThis.localStorage = makeStorage('tok')
  const originalSetTimeout = globalThis.setTimeout
  globalThis.setTimeout = (fn) => {
    fn()
    return 0
  }

  let attempts = 0
  let resolveDone
  const donePromise = new Promise((resolve) => { resolveDone = resolve })
  globalThis.fetch = async () => {
    attempts++
    if (attempts === 1) {
      throw Object.assign(new Error('temporary failure'), { name: 'TypeError' })
    }
    return sseResponse(['data: {"status":"done"}\n\n'])
  }

  const stop = API.files.subscribeDeleteProgress('del-1', (data) => {
    if (data.status === 'done') resolveDone()
  })
  await donePromise
  stop()
  globalThis.setTimeout = originalSetTimeout
  assert.equal(attempts, 2)
})

test('localFs.browse sends path as query parameter', async () => {
  globalThis.localStorage = makeStorage('tok')
  globalThis.fetch = async (url, opts) => {
    assert.equal(url, '/api/local_fs/browse?path=%2Fdata%2Fmydir')
    assert.equal(opts.method, 'GET')
    assert.equal(opts.headers.Authorization, 'Bearer tok')
    return {
      ok: true,
      status: 200,
      headers: { get: () => null },
      async text() {
        return '[{"name":"a.txt","path":"a.txt","is_file":true,"size":5}]'
      },
    }
  }

  const res = await API.localFs.browse('/data/mydir')
  assert.equal(res.length, 1)
  assert.equal(res[0].name, 'a.txt')
})

test('localFs.browse sends empty path when omitted', async () => {
  globalThis.localStorage = makeStorage('tok')
  globalThis.fetch = async (url) => {
    assert.equal(url, '/api/local_fs/browse?path=')
    return {
      ok: true,
      status: 200,
      headers: { get: () => null },
      async text() { return '[]' },
    }
  }

  const res = await API.localFs.browse()
  assert.deepEqual(res, [])
})

test('files.uploadLocal sends correct JSON body', async () => {
  globalThis.localStorage = makeStorage('tok')
  globalThis.fetch = async (url, opts) => {
    assert.equal(url, '/api/storages/s1/files/upload_local')
    assert.equal(opts.method, 'POST')
    const body = JSON.parse(opts.body)
    assert.equal(body.local_path, 'photos/a.jpg')
    assert.equal(body.dest_path, 'backup')
    assert.equal(body.upload_id, 'up-local-1')
    assert.equal(body.on_conflict, 'keep_both')
    return {
      ok: true,
      status: 202,
      headers: { get: () => null },
      async text() { return '{"upload_id":"up-local-1"}' },
    }
  }

  const res = await API.files.uploadLocal('s1', 'photos/a.jpg', 'backup', 'up-local-1')
  assert.equal(res.upload_id, 'up-local-1')
})

test('files.uploadLocal uses custom on_conflict policy', async () => {
  globalThis.localStorage = makeStorage('tok')
  globalThis.fetch = async (url, opts) => {
    const body = JSON.parse(opts.body)
    assert.equal(body.on_conflict, 'skip')
    return {
      ok: true,
      status: 202,
      headers: { get: () => null },
      async text() { return '{"upload_id":"up-2"}' },
    }
  }

  await API.files.uploadLocal('s1', 'a.txt', '', 'up-2', 'skip')
})

test('files.uploadLocalBatch sends items and on_conflict', async () => {
  globalThis.localStorage = makeStorage('tok')
  globalThis.fetch = async (url, opts) => {
    assert.equal(url, '/api/storages/s1/files/upload_local_batch')
    assert.equal(opts.method, 'POST')
    const body = JSON.parse(opts.body)
    assert.equal(body.items.length, 2)
    assert.equal(body.items[0].local_path, 'a.txt')
    assert.equal(body.items[1].local_path, 'b.txt')
    assert.equal(body.on_conflict, 'overwrite')
    return {
      ok: true,
      status: 202,
      headers: { get: () => null },
      async text() {
        return '{"uploads":[{"local_path":"a.txt","upload_id":"u1"},{"local_path":"b.txt","upload_id":"u2"}]}'
      },
    }
  }

  const items = [
    { local_path: 'a.txt', dest_path: '' },
    { local_path: 'b.txt', dest_path: 'docs' },
  ]
  const res = await API.files.uploadLocalBatch('s1', items, 'overwrite')
  assert.equal(res.uploads.length, 2)
})

test('files.uploadLocalBatch defaults on_conflict to keep_both', async () => {
  globalThis.localStorage = makeStorage('tok')
  globalThis.fetch = async (url, opts) => {
    const body = JSON.parse(opts.body)
    assert.equal(body.on_conflict, 'keep_both')
    return {
      ok: true,
      status: 202,
      headers: { get: () => null },
      async text() { return '{"uploads":[]}' },
    }
  }

  await API.files.uploadLocalBatch('s1', [{ local_path: 'a.txt', dest_path: '' }])
})

test('files.delete builds query params for delete_id and force_delete', async () => {
  globalThis.localStorage = makeStorage('tok')
  globalThis.fetch = async (url, opts) => {
    assert.equal(url, '/api/storages/s1/files/path/to/file?delete_id=del1&force_delete=1')
    assert.equal(opts.method, 'DELETE')
    return {
      ok: true,
      status: 204,
      headers: { get: () => null },
      async text() {
        return ''
      },
    }
  }

  const res = await API.files.delete('s1', 'path/to/file', 'del1', true)
  assert.equal(res, null)
})
