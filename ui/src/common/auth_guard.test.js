import test from 'node:test'
import assert from 'node:assert/strict'
import { checkAuth, getRedirectPath, isAuthenticated, logout } from './auth_guard.js'

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
