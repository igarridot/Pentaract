const API_BASE = import.meta.env.VITE_API_BASE || '/api'

export function getAuthToken() {
  const token = localStorage.getItem('access_token')
  return token ? `Bearer ${token}` : null
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

export async function apiRequest(path, method = 'GET', body = null, auth = true) {
  const headers = { 'Content-Type': 'application/json' }
  if (auth) {
    const token = getAuthToken()
    if (token) headers['Authorization'] = token
  }

  const opts = { method, headers }
  if (body) opts.body = JSON.stringify(body)

  const resp = await fetch(`${API_BASE}${path}`, opts)
  return parseResponse(resp)
}

export async function apiMultipartRequest(path, method, formData, auth = true) {
  const headers = {}
  if (auth) {
    const token = getAuthToken()
    if (token) headers['Authorization'] = token
  }

  const resp = await fetch(`${API_BASE}${path}`, { method, headers, body: formData })
  return parseResponse(resp)
}
