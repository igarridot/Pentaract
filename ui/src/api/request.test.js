import test from 'node:test'
import assert from 'node:assert/strict'
import { apiMultipartRequest, apiRequest, getRawToken } from './request.js'

function mockHeaders(contentLength = null) {
  return {
    get(name) {
      if (name === 'Content-Length') return contentLength
      return null
    },
  }
}

function setLocalStorage(token = null) {
  globalThis.localStorage = {
    getItem(key) {
      if (key === 'access_token') return token
      return null
    },
  }
}

test('getRawToken reads access token from localStorage', () => {
  setLocalStorage('abc123')
  assert.equal(getRawToken(), 'abc123')
})

test('apiRequest sends JSON body and auth header', async () => {
  setLocalStorage('tok')
  globalThis.fetch = async (url, opts) => {
    assert.equal(url, '/api/test')
    assert.equal(opts.method, 'POST')
    assert.equal(opts.headers.Authorization, 'Bearer tok')
    assert.equal(opts.headers['Content-Type'], 'application/json')
    assert.equal(opts.body, JSON.stringify({ a: 1 }))
    return {
      ok: true,
      status: 200,
      headers: mockHeaders(),
      async text() {
        return '{"ok":true}'
      },
    }
  }

  const data = await apiRequest('/test', 'POST', { a: 1 })
  assert.deepEqual(data, { ok: true })
})

test('apiRequest returns null for 204 and empty body', async () => {
  setLocalStorage(null)
  globalThis.fetch = async () => ({
    ok: true,
    status: 204,
    headers: mockHeaders(),
    async text() {
      return ''
    },
  })
  assert.equal(await apiRequest('/no-content'), null)

  globalThis.fetch = async () => ({
    ok: true,
    status: 200,
    headers: mockHeaders('0'),
    async text() {
      return '{"ignored":true}'
    },
  })
  assert.equal(await apiRequest('/zero-len'), null)
})

test('apiRequest throws API error message on non-ok response', async () => {
  setLocalStorage(null)
  globalThis.fetch = async () => ({
    ok: false,
    status: 400,
    headers: mockHeaders(),
    async json() {
      return { error: 'bad request' }
    },
  })

  await assert.rejects(() => apiRequest('/fail'), /bad request/)
})

test('apiRequest throws unknown error when response json is invalid', async () => {
  setLocalStorage(null)
  globalThis.fetch = async () => ({
    ok: false,
    status: 500,
    headers: mockHeaders(),
    async json() {
      throw new Error('invalid json')
    },
  })

  await assert.rejects(() => apiRequest('/fail2'), /Unknown error/)
})

test('apiMultipartRequest sends form data without content-type header', async () => {
  setLocalStorage('tok')
  const fd = new FormData()
  fd.append('k', 'v')

  globalThis.fetch = async (url, opts) => {
    assert.equal(url, '/api/upload')
    assert.equal(opts.method, 'POST')
    assert.equal(opts.headers.Authorization, 'Bearer tok')
    assert.equal(opts.headers['Content-Type'], undefined)
    assert.equal(opts.body, fd)
    return {
      ok: true,
      status: 200,
      headers: mockHeaders(),
      async text() {
        return '{"uploaded":1}'
      },
    }
  }

  const data = await apiMultipartRequest('/upload', 'POST', fd)
  assert.deepEqual(data, { uploaded: 1 })
})
