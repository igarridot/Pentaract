import { For, Show } from 'solid-js'
import Box from '@suid/material/Box'
import Paper from '@suid/material/Paper'
import Stack from '@suid/material/Stack'
import Typography from '@suid/material/Typography'
import LinearProgress from '@suid/material/LinearProgress'
import IconButton from '@suid/material/IconButton'
import CloseIcon from '@suid/icons-material/Close'
import CheckCircleIcon from '@suid/icons-material/CheckCircle'
import ErrorIcon from '@suid/icons-material/Error'
import CloudUploadIcon from '@suid/icons-material/CloudUpload'
import HourglassEmptyIcon from '@suid/icons-material/HourglassEmpty'

import { uploadStore } from '../stores/uploadStore'

/**
 * Format file size to human readable string
 * @param {number} bytes
 * @returns {string}
 */
const formatFileSize = (bytes) => {
	if (bytes === 0) return '0 B'
	const k = 1024
	const sizes = ['B', 'KB', 'MB', 'GB']
	const i = Math.floor(Math.log(bytes) / Math.log(k))
	return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

/**
 * Get status text based on upload status
 * @param {import('../stores/uploadStore').UploadStatus} status
 * @param {number} progress
 * @returns {string}
 */
const getStatusText = (status, progress) => {
	switch (status) {
		case 'pending':
			return 'Preparing...'
		case 'uploading':
			return `Uploading... ${progress}%`
		case 'processing':
			return 'Processing on server...'
		case 'completed':
			return 'Completed'
		case 'error':
			return 'Failed'
		default:
			return ''
	}
}

/**
 * Get status icon based on upload status
 * @param {import('../stores/uploadStore').UploadStatus} status
 */
const StatusIcon = (props) => {
	return (
		<Show
			when={props.status === 'completed'}
			fallback={
				<Show
					when={props.status === 'error'}
					fallback={
						<Show
							when={props.status === 'processing'}
							fallback={<CloudUploadIcon color="primary" fontSize="small" />}
						>
							<HourglassEmptyIcon color="warning" fontSize="small" />
						</Show>
					}
				>
					<ErrorIcon color="error" fontSize="small" />
				</Show>
			}
		>
			<CheckCircleIcon color="success" fontSize="small" />
		</Show>
	)
}

/**
 * Single upload item component
 */
const UploadItem = (props) => {
	const { removeUpload } = uploadStore
	const upload = () => props.upload

	return (
		<Paper
			elevation={3}
			sx={{
				p: 1.5,
				mb: 1,
				backgroundColor:
					upload().status === 'error'
						? 'error.light'
						: upload().status === 'completed'
						? 'success.light'
						: 'background.paper',
			}}
		>
			<Stack direction="row" alignItems="center" spacing={1}>
				<StatusIcon status={upload().status} />
				<Box sx={{ flexGrow: 1, minWidth: 0 }}>
					<Typography
						variant="body2"
						noWrap
						sx={{ fontWeight: 500 }}
						title={upload().fileName}
					>
						{upload().fileName}
					</Typography>
					<Typography variant="caption" color="text.secondary">
						{formatFileSize(upload().fileSize)} - {getStatusText(upload().status, upload().progress)}
					</Typography>
					<Show when={upload().status === 'uploading' || upload().status === 'pending'}>
						<LinearProgress
							variant="determinate"
							value={upload().progress}
							sx={{ mt: 0.5, height: 4, borderRadius: 2 }}
						/>
					</Show>
					<Show when={upload().status === 'processing'}>
						<LinearProgress
							variant="indeterminate"
							sx={{ mt: 0.5, height: 4, borderRadius: 2 }}
						/>
					</Show>
				</Box>
				<Show when={upload().status === 'error' || upload().status === 'completed'}>
					<IconButton
						size="small"
						onClick={() => removeUpload(upload().id)}
						sx={{ ml: 'auto' }}
					>
						<CloseIcon fontSize="small" />
					</IconButton>
				</Show>
			</Stack>
		</Paper>
	)
}

/**
 * Upload progress panel that shows all active uploads
 */
const UploadProgress = () => {
	const { uploads } = uploadStore

	return (
		<Show when={uploads().length > 0}>
			<Box
				sx={{
					position: 'fixed',
					bottom: '1rem',
					right: '1rem',
					width: 320,
					maxHeight: '50vh',
					overflowY: 'auto',
					zIndex: 9999,
				}}
			>
				<Paper elevation={4} sx={{ p: 1.5 }}>
					<Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>
						Uploads ({uploads().length})
					</Typography>
					<For each={uploads()}>
						{(upload) => <UploadItem upload={upload} />}
					</For>
				</Paper>
			</Box>
		</Show>
	)
}

export default UploadProgress
