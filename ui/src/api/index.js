import { API_BASE, getRawToken, apiRequest, apiMultipartRequest } from './request'

function buildAuthUrl(path, params = {}) {
  const token = getRawToken()
  if (!token) throw new Error('Not authenticated')
  const searchParams = new URLSearchParams({ access_token: token, ...params })
  return `${API_BASE}${path}?${searchParams.toString()}`
}

function subscribeSSE(url, token, onProgress) {
  let stopped = false
  let currentController = null

  const fetchSSE = async () => {
    while (!stopped) {
      const controller = new AbortController()
      currentController = controller
      try {
        const resp = await fetch(url, {
          headers: { Authorization: `Bearer ${token}` },
          signal: controller.signal,
        })
        const reader = resp.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''

        while (true) {
          const { done, value } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })

          const lines = buffer.split('\n')
          buffer = lines.pop() || ''

          for (const line of lines) {
            if (line.startsWith('data: ')) {
              try {
                const data = JSON.parse(line.slice(6))
                onProgress(data)
                if (data.status === 'done' || data.status === 'error' || data.status === 'cancelled') {
                  stopped = true
                  return
                }
              } catch {}
            }
          }
        }
      } catch (err) {
        if (err.name === 'AbortError' || stopped) return
      }
      if (!stopped) {
        await new Promise((r) => setTimeout(r, 1000))
      }
    }
  }

  fetchSSE()
  return () => { stopped = true; if (currentController) currentController.abort() }
}

function subscribeAuthSSE(path, onProgress) {
  const token = getRawToken()
  if (!token) throw new Error('Not authenticated')
  return subscribeSSE(`${API_BASE}${path}`, token, onProgress)
}

const API = {
  auth: {
    login: (email, password) => apiRequest('/auth/login', 'POST', { email, password }, false),
  },

  users: {
    register: (email, password) => apiRequest('/users', 'POST', { email, password }, false),
  },

  storages: {
    list: () => apiRequest('/storages'),
    create: (name, chat_id) => apiRequest('/storages', 'POST', { name, chat_id }),
    get: (id) => apiRequest(`/storages/${id}`),
    delete: (id, deleteId) =>
      apiRequest(`/storages/${id}${deleteId ? `?delete_id=${encodeURIComponent(deleteId)}` : ''}`, 'DELETE'),
  },

  access: {
    list: (storageId) => apiRequest(`/storages/${storageId}/access`),
    grant: (storageId, email, access_type) =>
      apiRequest(`/storages/${storageId}/access`, 'POST', { email, access_type }),
    revoke: (storageId, user_id) =>
      apiRequest(`/storages/${storageId}/access`, 'DELETE', { user_id }),
  },

  storageWorkers: {
    list: () => apiRequest('/storage_workers'),
    create: (name, token, storage_id) =>
      apiRequest('/storage_workers', 'POST', { name, token, storage_id: storage_id || null }),
    update: (id, name, storage_id) =>
      apiRequest(`/storage_workers/${id}`, 'PUT', { name, storage_id: storage_id || null }),
    delete: (id) => apiRequest(`/storage_workers/${id}`, 'DELETE'),
    hasWorkers: (storageId) => apiRequest(`/storage_workers/has_workers?storage_id=${storageId}`),
  },

  files: {
    createFolder: (storageId, path, folder_name) =>
      apiRequest(`/storages/${storageId}/files/create_folder`, 'POST', { path, folder_name }),

    move: (storageId, oldPath, newPath) =>
      apiRequest(`/storages/${storageId}/files/move`, 'POST', { old_path: oldPath, new_path: newPath }),

    upload: (storageId, path, file, uploadId, options = {}) => {
      const formData = new FormData()
      formData.append('path', path || '')
      if (uploadId) formData.append('upload_id', uploadId)
      formData.append('file', file)
      return apiMultipartRequest(`/storages/${storageId}/files/upload`, 'POST', formData, true, options)
    },

    tree: (storageId, path) =>
      apiRequest(`/storages/${storageId}/files/tree/${path || ''}`),

    downloadFileUrl: (storageId, path, downloadId) =>
      buildAuthUrl(`/storages/${storageId}/files/download/${encodeURI(path || '')}`, { download_id: downloadId }),

    previewFileUrl: (storageId, path) =>
      buildAuthUrl(`/storages/${storageId}/files/download/${encodeURI(path || '')}`, { inline: '1' }),

    downloadDirUrl: (storageId, path, downloadId) =>
      buildAuthUrl(`/storages/${storageId}/files/download_dir/${encodeURI(path || '')}`, { download_id: downloadId }),

    search: (storageId, basePath, searchPath) =>
      apiRequest(`/storages/${storageId}/files/search/${basePath || ''}?search_path=${encodeURIComponent(searchPath)}`),

    delete: (storageId, path, deleteId, forceDelete = false) => {
      const params = new URLSearchParams()
      if (deleteId) params.set('delete_id', deleteId)
      if (forceDelete) params.set('force_delete', '1')
      const query = params.toString()
      return apiRequest(`/storages/${storageId}/files/${path}${query ? `?${query}` : ''}`, 'DELETE')
    },

    cancelUpload: (uploadId) =>
      apiRequest(`/upload_cancel/${uploadId}`, 'POST'),

    cancelDownload: (downloadId) =>
      apiRequest(`/download_cancel/${downloadId}`, 'POST'),

    subscribeProgress: (uploadId, onProgress) =>
      subscribeAuthSSE(`/upload_progress?upload_id=${uploadId}`, onProgress),

    subscribeDownloadProgress: (downloadId, onProgress) =>
      subscribeAuthSSE(`/download_progress?download_id=${downloadId}`, onProgress),

    subscribeDeleteProgress: (deleteId, onProgress) =>
      subscribeAuthSSE(`/delete_progress?delete_id=${deleteId}`, onProgress),
  },
}

export default API
