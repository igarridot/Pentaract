import test from 'node:test'
import assert from 'node:assert/strict'
import { checkAuth, getRedirectPath, isAuthenticated, logout, getCurrentUserId } from './auth_guard.js'

function makeStorage(initial = {}) {
  const store = { ...initial }
  return {
    getItem(key) {
      return Object.prototype.hasOwnProperty.call(store, key) ? store[key] : null
    },
    setItem(key, value) {
      store[key] = String(value)
    },
    removeItem(key) {
      delete store[key]
    },
    _dump() {
      return { ...store }
    },
  }
}

test('checkAuth redirects to login when token is missing', () => {
  const local = makeStorage()
  globalThis.localStorage = local
  let navigatedTo = null
  const ok = checkAuth((to) => { navigatedTo = to }, { pathname: '/storages/1/files' })
  assert.equal(ok, false)
  assert.equal(local.getItem('redirect'), '/storages/1/files')
  assert.equal(navigatedTo, '/login')
})

test('checkAuth returns true when token exists', () => {
  const local = makeStorage({ access_token: 'tok' })
  globalThis.localStorage = local
  let navigated = false
  const ok = checkAuth(() => { navigated = true }, { pathname: '/' })
  assert.equal(ok, true)
  assert.equal(navigated, false)
})

test('isAuthenticated checks access token presence', () => {
  globalThis.localStorage = makeStorage()
  assert.equal(isAuthenticated(), false)
  globalThis.localStorage = makeStorage({ access_token: 'x' })
  assert.equal(isAuthenticated(), true)
})

test('logout removes token and navigates to login', () => {
  const local = makeStorage({ access_token: 'tok' })
  globalThis.localStorage = local
  let navigatedTo = null
  logout((to) => { navigatedTo = to })
  assert.equal(local.getItem('access_token'), null)
  assert.equal(navigatedTo, '/login')
})

test('getRedirectPath consumes redirect and falls back to /storages', () => {
  const local = makeStorage({ redirect: '/storage_workers' })
  globalThis.localStorage = local
  assert.equal(getRedirectPath(), '/storage_workers')
  assert.equal(local.getItem('redirect'), null)
  assert.equal(getRedirectPath(), '/storages')
})

function fakeJwt(payload) {
  const b64 = (obj) => Buffer.from(JSON.stringify(obj)).toString('base64url')
  return `${b64({ alg: 'HS256' })}.${b64(payload)}.signature`
}

test('getCurrentUserId reads the sub claim from the stored token', () => {
  globalThis.localStorage = makeStorage({ access_token: fakeJwt({ sub: 'user-42', email: 'u@example.com' }) })
  assert.equal(getCurrentUserId(), 'user-42')
})

test('getCurrentUserId returns null without a token or with a malformed one', () => {
  globalThis.localStorage = makeStorage()
  assert.equal(getCurrentUserId(), null)

  globalThis.localStorage = makeStorage({ access_token: 'not-a-jwt' })
  assert.equal(getCurrentUserId(), null)

  globalThis.localStorage = makeStorage({ access_token: fakeJwt({ email: 'no-sub@example.com' }) })
  assert.equal(getCurrentUserId(), null)
})
