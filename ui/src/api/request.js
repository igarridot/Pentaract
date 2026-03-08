export const API_BASE = import.meta.env.VITE_API_BASE || '/api'

export function getRawToken() {
  return localStorage.getItem('access_token')
}

async function parseResponse(resp) {
  if (!resp.ok) {
    const data = await resp.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(data.error || `Request failed with status ${resp.status}`)
  }
  if (resp.status === 204 || resp.headers.get('Content-Length') === '0') return null
  const text = await resp.text()
  if (!text) return null
  return JSON.parse(text)
}

export async function apiRequest(path, method = 'GET', body = null, auth = true, options = {}) {
  const headers = { 'Content-Type': 'application/json' }
  if (auth) {
    const token = getRawToken()
    if (token) headers['Authorization'] = `Bearer ${token}`
  }

  const opts = { method, headers, ...options }
  if (body) opts.body = JSON.stringify(body)

  const resp = await fetch(`${API_BASE}${path}`, opts)
  return parseResponse(resp)
}

export async function apiMultipartRequest(path, method, formData, auth = true, options = {}) {
  const headers = {}
  if (auth) {
    const token = getRawToken()
    if (token) headers['Authorization'] = `Bearer ${token}`
  }

  const resp = await fetch(`${API_BASE}${path}`, { method, headers, body: formData, ...options })
  return parseResponse(resp)
}
