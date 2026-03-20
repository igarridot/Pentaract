import { API_BASE, getRawToken, apiRequest, apiMultipartRequest } from './request.js'

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

function subscribeTransferProgress(endpoint, idName, idValue, onProgress) {
  return subscribeAuthSSE(`/${endpoint}?${idName}=${encodeURIComponent(idValue)}`, onProgress)
}

function cancelTransfer(endpoint, idValue) {
  return apiRequest(`/${endpoint}/${idValue}`, 'POST')
}

function filesPath(storageId, suffix = '') {
  return `/storages/${storageId}/files${suffix}`
}

function filesDownloadAuthUrl(storageId, mode, path, params = {}) {
  return buildAuthUrl(filesPath(storageId, `/${mode}/${encodeURI(path || '')}`), params)
}

const API = {
  auth: {
    login: (email, password) => apiRequest('/auth/login', 'POST', { email, password }, false),
  },

  users: {
    register: (email, password) => apiRequest('/users', 'POST', { email, password }, false),
    adminStatus: () => apiRequest('/users/admin'),
    listManaged: () => apiRequest('/users/manage'),
    updatePassword: (userId, password) => apiRequest(`/users/${userId}/password`, 'PUT', { password }),
    deleteManaged: (userId) => apiRequest(`/users/manage?user_id=${encodeURIComponent(userId)}`, 'DELETE'),
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
    candidates: (storageId) => apiRequest(`/storages/${storageId}/access/candidates`),
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
      apiRequest(filesPath(storageId, '/create_folder'), 'POST', { path, folder_name }),

    move: (storageId, oldPath, newPath, options = {}) =>
      apiRequest(filesPath(storageId, '/move'), 'POST', { old_path: oldPath, new_path: newPath }, true, options),

    upload: (storageId, path, file, uploadId, options = {}) => {
      const formData = new FormData()
      formData.append('path', path || '')
      if (uploadId) formData.append('upload_id', uploadId)
      formData.append('on_conflict', options.onConflict || 'keep_both')
      formData.append('file', file)
      return apiMultipartRequest(filesPath(storageId, '/upload'), 'POST', formData, true, options)
    },

    tree: (storageId, path) =>
      apiRequest(filesPath(storageId, `/tree/${path || ''}`)),

    downloadFileUrl: (storageId, path, downloadId) =>
      filesDownloadAuthUrl(storageId, 'download', path, { download_id: downloadId }),

    previewFileUrl: (storageId, path) =>
      filesDownloadAuthUrl(storageId, 'download', path, { inline: '1' }),

    downloadDirUrl: (storageId, path, downloadId) =>
      filesDownloadAuthUrl(storageId, 'download_dir', path, { download_id: downloadId }),

    search: (storageId, basePath, searchPath) =>
      apiRequest(`${filesPath(storageId, `/search/${basePath || ''}`)}?search_path=${encodeURIComponent(searchPath)}`),

    delete: (storageId, path, deleteId, forceDelete = false) => {
      const params = new URLSearchParams()
      if (deleteId) params.set('delete_id', deleteId)
      if (forceDelete) params.set('force_delete', '1')
      const query = params.toString()
      return apiRequest(`${filesPath(storageId, `/${path}`)}${query ? `?${query}` : ''}`, 'DELETE')
    },

    cancelUpload: (uploadId) => cancelTransfer('upload_cancel', uploadId),

    cancelDownload: (downloadId) => cancelTransfer('download_cancel', downloadId),

    subscribeProgress: (uploadId, onProgress) =>
      subscribeTransferProgress('upload_progress', 'upload_id', uploadId, onProgress),

    subscribeDownloadProgress: (downloadId, onProgress) =>
      subscribeTransferProgress('download_progress', 'download_id', downloadId, onProgress),

    subscribeDeleteProgress: (deleteId, onProgress) =>
      subscribeTransferProgress('delete_progress', 'delete_id', deleteId, onProgress),
  },

  localFiles: {
    tree: (storageId, path) =>
      apiRequest(`/storages/${storageId}/local_files/tree/${path || ''}`),

    expand: (storageId, paths) =>
      apiRequest(`/storages/${storageId}/local_files/expand`, 'POST', { paths }),

    upload: (storageId, sourcePath, targetPath, uploadId, options = {}) =>
      apiRequest(
        `/storages/${storageId}/local_files/upload`,
        'POST',
        {
          source_path: sourcePath,
          target_path: targetPath || '',
          upload_id: uploadId,
          on_conflict: options.onConflict || 'keep_both',
        },
        true,
        options,
      ),
  },
}

export default API
