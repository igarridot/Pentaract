import { apiRequest, apiMultipartRequest } from './request'

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
    hasWorkers: (storageId) => apiRequest(`/storage_workers/has_workers?storage_id=${storageId}`),
  },

  files: {
    createFolder: (storageId, path, folder_name) =>
      apiRequest(`/storages/${storageId}/files/create_folder`, 'POST', { path, folder_name }),

    upload: (storageId, path, file) => {
      const formData = new FormData()
      formData.append('file', file)
      formData.append('path', path || '')
      return apiMultipartRequest(`/storages/${storageId}/files/upload`, 'POST', formData)
    },

    uploadTo: (storageId, path, file) => {
      const formData = new FormData()
      formData.append('file', file)
      formData.append('path', path || '')
      return apiMultipartRequest(`/storages/${storageId}/files/upload_to`, 'POST', formData)
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

    search: (storageId, basePath, searchPath) =>
      apiRequest(`/storages/${storageId}/files/search/${basePath || ''}?search_path=${encodeURIComponent(searchPath)}`),

    delete: (storageId, path) =>
      apiRequest(`/storages/${storageId}/files/${path}`, 'DELETE'),
  },
}

export default API
