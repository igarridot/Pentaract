import { createRoot, createSignal } from 'solid-js'

/**
 * @typedef {'pending' | 'uploading' | 'processing' | 'completed' | 'error'} UploadStatus
 */

/**
 * @typedef {Object} UploadItem
 * @property {string} id
 * @property {string} fileName
 * @property {number} fileSize
 * @property {number} progress - 0 to 100
 * @property {UploadStatus} status
 * @property {string} [error]
 */

let uploadIdCounter = 0

export const uploadStore = createRoot(() => {
	/**
	 * @type {[import("solid-js").Accessor<UploadItem[]>, import("solid-js").Setter<UploadItem[]>]}
	 */
	const [uploads, setUploads] = createSignal([])

	/**
	 * Start tracking a new upload
	 * @param {string} fileName
	 * @param {number} fileSize
	 * @returns {string} uploadId
	 */
	const startUpload = (fileName, fileSize) => {
		const id = `upload-${++uploadIdCounter}-${Date.now()}`
		const newUpload = {
			id,
			fileName,
			fileSize,
			progress: 0,
			status: 'pending',
		}
		setUploads((prev) => [newUpload, ...prev])
		return id
	}

	/**
	 * Update upload progress
	 * @param {string} id
	 * @param {number} progress
	 */
	const updateProgress = (id, progress) => {
		setUploads((prev) =>
			prev.map((upload) =>
				upload.id === id
					? { ...upload, progress, status: 'uploading' }
					: upload
			)
		)
	}

	/**
	 * Mark upload as processing (sent to server, waiting for Telegram upload)
	 * @param {string} id
	 */
	const setProcessing = (id) => {
		setUploads((prev) =>
			prev.map((upload) =>
				upload.id === id
					? { ...upload, progress: 100, status: 'processing' }
					: upload
			)
		)
	}

	/**
	 * Mark upload as completed
	 * @param {string} id
	 */
	const completeUpload = (id) => {
		setUploads((prev) =>
			prev.map((upload) =>
				upload.id === id ? { ...upload, status: 'completed' } : upload
			)
		)
		// Remove completed upload after 3 seconds
		setTimeout(() => {
			removeUpload(id)
		}, 3000)
	}

	/**
	 * Mark upload as failed
	 * @param {string} id
	 * @param {string} error
	 */
	const failUpload = (id, error) => {
		setUploads((prev) =>
			prev.map((upload) =>
				upload.id === id ? { ...upload, status: 'error', error } : upload
			)
		)
	}

	/**
	 * Remove an upload from the list
	 * @param {string} id
	 */
	const removeUpload = (id) => {
		setUploads((prev) => prev.filter((upload) => upload.id !== id))
	}

	/**
	 * Check if there are any active uploads
	 * @returns {boolean}
	 */
	const hasActiveUploads = () => {
		return uploads().some(
			(u) => u.status === 'pending' || u.status === 'uploading' || u.status === 'processing'
		)
	}

	return {
		uploads,
		startUpload,
		updateProgress,
		setProcessing,
		completeUpload,
		failUpload,
		removeUpload,
		hasActiveUploads,
	}
})
