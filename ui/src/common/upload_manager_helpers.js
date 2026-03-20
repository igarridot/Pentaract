export function createUploadEntryKey(entry) {
  return `${entry.targetPath}::${entry.filename}`
}

export function createUploadState(entry, uploadId) {
  return {
    id: uploadId,
    filename: entry.filename,
    totalBytes: entry.size,
    uploadedBytes: 0,
    totalChunks: 0,
    uploadedChunks: 0,
    verificationTotal: 0,
    verifiedChunks: 0,
    status: 'uploading',
    workersStatus: 'active',
  }
}

export function applyUploadProgressState(previousState, entry, data) {
  return {
    ...previousState,
    filename: entry.filename,
    totalBytes: data.total_bytes ?? previousState?.totalBytes ?? entry.size,
    uploadedBytes: data.uploaded_bytes ?? 0,
    totalChunks: data.total ?? previousState?.totalChunks ?? 0,
    uploadedChunks: data.uploaded ?? 0,
    verificationTotal: data.verification_total ?? previousState?.verificationTotal ?? 0,
    verifiedChunks: data.verified ?? 0,
    status: data.status,
    workersStatus: data.workers_status ?? previousState?.workersStatus ?? 'active',
  }
}

export function getSkippedUploadEntries(entries, uploadedEntries) {
  const uploadedKeys = new Set(uploadedEntries.map(createUploadEntryKey))
  return entries.filter((entry) => !uploadedKeys.has(createUploadEntryKey(entry)))
}
