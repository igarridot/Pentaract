export function calculatePercent(processed, total) {
  if (!Number.isFinite(processed) || !Number.isFinite(total) || total <= 0) return 0
  const bounded = Math.min(Math.max(processed, 0), total)
  return Math.round((bounded / total) * 100)
}

export function isTerminalTransferStatus(status) {
  return status === 'done' || status === 'skipped' || status === 'error' || status === 'cancelled'
}

export function isActiveUploadStatus(status) {
  return status === 'uploading' || status === 'verifying'
}

// "interrupted" means the browser dropped the connection (typically after
// blocking the file as an insecure download) and the server is waiting for the
// browser to re-request it once the user allows the download.
export function isActiveDownloadStatus(status) {
  return status === 'downloading' || status === 'interrupted'
}

export function workersStatusText(workersStatus) {
  return workersStatus === 'waiting_rate_limit' ? 'Workers waiting (rate limit)' : 'Workers active'
}

// Initial card state for an upload, before the first SSE event arrives.
export function createUploadState(id, filename, totalBytes = 0) {
  return {
    id,
    filename,
    totalBytes,
    uploadedBytes: 0,
    totalChunks: 0,
    uploadedChunks: 0,
    verificationTotal: 0,
    verifiedChunks: 0,
    status: 'uploading',
    workersStatus: 'active',
  }
}

// Merges an upload_progress SSE event into the card state. Totals fall back
// to what we already knew so a sparse event never blanks the bar.
export function applyUploadProgressUpdate(previous, data) {
  return {
    ...previous,
    totalBytes: data.total_bytes ?? previous?.totalBytes ?? 0,
    uploadedBytes: data.uploaded_bytes ?? 0,
    totalChunks: data.total ?? previous?.totalChunks ?? 0,
    uploadedChunks: data.uploaded ?? 0,
    verificationTotal: data.verification_total ?? previous?.verificationTotal ?? 0,
    verifiedChunks: data.verified ?? 0,
    status: data.status,
    workersStatus: data.workers_status ?? previous?.workersStatus ?? 'active',
  }
}

export function summarizeTerminalStatuses(terminalStatuses = {}) {
  const summary = {
    completed: 0,
    doneCount: 0,
    skippedCount: 0,
    errorCount: 0,
    cancelledCount: 0,
  }

  Object.values(terminalStatuses).forEach((status) => {
    if (!isTerminalTransferStatus(status)) return
    summary.completed += 1
    if (status === 'done') summary.doneCount += 1
    if (status === 'skipped') summary.skippedCount += 1
    if (status === 'error') summary.errorCount += 1
    if (status === 'cancelled') summary.cancelledCount += 1
  })

  return summary
}

export function resolveBulkTransferStatus(summary) {
  if (summary.errorCount > 0) return 'error'
  if (summary.cancelledCount > 0) return 'cancelled'
  return 'done'
}
