const viteEnv = typeof import.meta !== 'undefined' ? import.meta.env : undefined
export const API_BASE = viteEnv?.VITE_API_BASE || '/api'

export function getRawToken() {
  return localStorage.getItem('access_token')
}

// ApiError carries the HTTP status so callers can branch on it instead of
// matching text in the message.
export class ApiError extends Error {
  constructor(message, status) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function parseResponse(resp) {
  if (!resp.ok) {
    const data = await resp.json().catch(() => ({ error: 'Unknown error' }))
    throw new ApiError(data.error || `Request failed with status ${resp.status}`, resp.status)
  }
  if (resp.status === 204 || resp.headers.get('Content-Length') === '0') return null
  const text = await resp.text()
  if (!text) return null
  return JSON.parse(text)
}

// Adds the bearer token to headers when auth is requested and a token exists.
function authHeaders(auth, headers = {}) {
  const token = auth ? getRawToken() : null
  return token ? { ...headers, Authorization: `Bearer ${token}` } : headers
}

export async function apiRequest(path, method = 'GET', body = null, auth = true, options = {}) {
  const opts = { method, headers: authHeaders(auth, { 'Content-Type': 'application/json' }), ...options }
  if (body) opts.body = JSON.stringify(body)
  return parseResponse(await fetch(`${API_BASE}${path}`, opts))
}

export async function apiMultipartRequest(path, method, formData, auth = true, options = {}) {
  return parseResponse(await fetch(`${API_BASE}${path}`, { method, headers: authHeaders(auth), body: formData, ...options }))
}
