const API_BASE = import.meta.env.VITE_API_BASE || '/api'

export function getAuthToken() {
  const token = localStorage.getItem('access_token')
  return token ? `Bearer ${token}` : null
}

export async function apiRequest(path, method = 'GET', body = null, auth = true, returnResponse = false) {
  const headers = { 'Content-Type': 'application/json' }

  if (auth) {
    const token = getAuthToken()
    if (token) headers['Authorization'] = token
  }

  const opts = { method, headers }
  if (body) opts.body = JSON.stringify(body)

  const resp = await fetch(`${API_BASE}${path}`, opts)

  if (returnResponse) return resp

  if (!resp.ok) {
    const data = await resp.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(data.error || `Request failed with status ${resp.status}`)
  }

  if (resp.status === 204) return null
  return resp.json()
}

export async function apiMultipartRequest(path, method, formData, auth = true) {
  const headers = {}

  if (auth) {
    const token = getAuthToken()
    if (token) headers['Authorization'] = token
  }

  const resp = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    body: formData,
  })

  if (!resp.ok) {
    const data = await resp.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(data.error || `Request failed with status ${resp.status}`)
  }

  if (resp.status === 204) return null
  return resp.json()
}
