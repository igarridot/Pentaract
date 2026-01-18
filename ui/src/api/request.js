import { alertStore } from '../components/AlertStack'
import { uploadStore } from '../stores/uploadStore'

const API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8000/api'

/**
 * @typedef {'get' | 'post' | 'patch' | 'delete'} Method
 */

/**
 *
 * @param {string} path
 * @param {Method} method
 * @param {string | null | undefined} auth_token
 * @param {any} body
 * @param {boolean} return_response
 * @returns
 */
const apiRequest = async (
	path,
	method,
	auth_token,
	body,
	return_response = false
) => {
	const { addAlert } = alertStore

	const fullpath = `${API_BASE}${path}`

	const headers = new Headers()
	headers.append('Content-Type', 'application/json')
	if (auth_token) {
		headers.append('Authorization', auth_token)
	}

	try {
		const response = await fetch(fullpath, {
			method,
			body: JSON.stringify(body),
			headers,
		})

		if (!response.ok) {
			throw new Error(await response.text())
		}

		if (return_response) {
			return response
		}

		try {
			return await response.json()
		} catch {}
	} catch (err) {
		addAlert(err.message, 'error')

		throw err
	}
}

/**
 *
 * @param {string} path
 * @param {string | null | undefined} auth_token
 * @param {FormData} form
 * @returns
 */
export const apiMultipartRequest = async (path, auth_token, form) => {
	const { addAlert } = alertStore

	const fullpath = `${API_BASE}${path}`

	const headers = new Headers()
	// headers.append("Content-Type", "multipart/form-data");
	if (auth_token) {
		headers.append('Authorization', auth_token)
	}

	try {
		const response = await fetch(fullpath, {
			method: 'post',
			body: form,
			headers,
		})

		if (!response.ok) {
			throw new Error(await response.text())
		}

		try {
			return await response.json()
		} catch {}
	} catch (err) {
		addAlert(err.message, 'error')

		throw err
	}
}

/**
 * Upload a file with progress tracking using XMLHttpRequest
 * @param {string} path
 * @param {string | null | undefined} auth_token
 * @param {FormData} form
 * @param {string} fileName
 * @param {number} fileSize
 * @returns {Promise<any>}
 */
export const apiMultipartRequestWithProgress = (path, auth_token, form, fileName, fileSize) => {
	const { addAlert } = alertStore
	const { startUpload, updateProgress, setProcessing, completeUpload, failUpload } = uploadStore

	const fullpath = `${API_BASE}${path}`
	const uploadId = startUpload(fileName, fileSize)

	return new Promise((resolve, reject) => {
		const xhr = new XMLHttpRequest()

		xhr.upload.addEventListener('progress', (event) => {
			if (event.lengthComputable) {
				const percentComplete = Math.round((event.loaded / event.total) * 100)
				updateProgress(uploadId, percentComplete)
			}
		})

		xhr.upload.addEventListener('load', () => {
			// Upload to server complete, now server is processing (uploading to Telegram)
			setProcessing(uploadId)
		})

		xhr.addEventListener('load', () => {
			if (xhr.status >= 200 && xhr.status < 300) {
				completeUpload(uploadId)
				try {
					const response = JSON.parse(xhr.responseText)
					resolve(response)
				} catch {
					resolve(undefined)
				}
			} else {
				const errorMsg = xhr.responseText || `Upload failed with status ${xhr.status}`
				failUpload(uploadId, errorMsg)
				addAlert(errorMsg, 'error')
				reject(new Error(errorMsg))
			}
		})

		xhr.addEventListener('error', () => {
			const errorMsg = 'Network error during upload'
			failUpload(uploadId, errorMsg)
			addAlert(errorMsg, 'error')
			reject(new Error(errorMsg))
		})

		xhr.addEventListener('abort', () => {
			const errorMsg = 'Upload cancelled'
			failUpload(uploadId, errorMsg)
			reject(new Error(errorMsg))
		})

		xhr.open('POST', fullpath)

		if (auth_token) {
			xhr.setRequestHeader('Authorization', auth_token)
		}

		xhr.send(form)
	})
}

export default apiRequest
