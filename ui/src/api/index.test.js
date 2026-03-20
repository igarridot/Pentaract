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
