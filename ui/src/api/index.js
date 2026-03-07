import { apiRequest, apiMultipartRequest } from './request'

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
                if (data.status === 'done' || data.status === 'error') {
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
    delete: (id) => apiRequest(`/storages/${id}`, 'DELETE'),
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

    upload: (storageId, path, file, uploadId) => {
      const formData = new FormData()
      formData.append('path', path || '')
      if (uploadId) formData.append('upload_id', uploadId)
      formData.append('file', file)
      return apiMultipartRequest(`/storages/${storageId}/files/upload`, 'POST', formData)
    },

    tree: (storageId, path) =>
      apiRequest(`/storages/${storageId}/files/tree/${path || ''}`),

    download: async (storageId, path) => {
      const token = localStorage.getItem('access_token')
      const base = import.meta.env.VITE_API_BASE || '/api'
      const resp = await fetch(`${base}/storages/${storageId}/files/download/${path}`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!resp.ok) throw new Error('Download failed')
      return resp.blob()
    },

    downloadDirUrl: (storageId, path, downloadId) => {
      const token = localStorage.getItem('access_token')
      if (!token) throw new Error('Not authenticated')
      const base = import.meta.env.VITE_API_BASE || '/api'
      const safePath = encodeURI(path || '')
      const params = new URLSearchParams({
        download_id: downloadId,
        access_token: token,
      })
      return `${base}/storages/${storageId}/files/download_dir/${safePath}?${params.toString()}`
    },

    search: (storageId, basePath, searchPath) =>
      apiRequest(`/storages/${storageId}/files/search/${basePath || ''}?search_path=${encodeURIComponent(searchPath)}`),

    delete: (storageId, path) =>
      apiRequest(`/storages/${storageId}/files/${path}`, 'DELETE'),

    cancelUpload: (uploadId) =>
      apiRequest(`/upload_cancel/${uploadId}`, 'POST'),

    subscribeProgress: (uploadId, onProgress) => {
      const token = localStorage.getItem('access_token')
      const base = import.meta.env.VITE_API_BASE || '/api'
      const url = `${base}/upload_progress?upload_id=${uploadId}`
      return subscribeSSE(url, token, onProgress)
    },

    subscribeDownloadProgress: (downloadId, onProgress) => {
      const token = localStorage.getItem('access_token')
      const base = import.meta.env.VITE_API_BASE || '/api'
      const url = `${base}/download_progress?download_id=${downloadId}`
      return subscribeSSE(url, token, onProgress)
    },
  },
}

export default API
