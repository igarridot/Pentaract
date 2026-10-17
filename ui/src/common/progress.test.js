import test from 'node:test'
import assert from 'node:assert/strict'

import {
  calculatePercent, isTerminalTransferStatus, isActiveUploadStatus, isActiveDownloadStatus,
  summarizeTerminalStatuses, resolveBulkTransferStatus, workersStatusText,
  createUploadState, applyUploadProgressUpdate,
  createDownloadState, applyDownloadProgressUpdate,
} from './progress.js'

test('createDownloadState starts an in-flight card', () => {
  assert.deepEqual(createDownloadState('d1', 'a.zip'), {
    id: 'd1',
    filename: 'a.zip',
    totalBytes: 0,
    downloadedBytes: 0,
    totalChunks: 0,
    downloadedChunks: 0,
    status: 'downloading',
    workersStatus: 'active',
    errorMessage: '',
  })
})

test('applyDownloadProgressUpdate keeps known totals and scopes the error message', () => {
  const initial = createDownloadState('d1', 'a.zip')
  const partial = applyDownloadProgressUpdate(initial, {
    total_bytes: 100, downloaded_bytes: 10, total: 5, downloaded: 1, status: 'downloading',
  })
  assert.equal(partial.totalBytes, 100)
  assert.equal(partial.downloadedBytes, 10)
  assert.equal(partial.workersStatus, 'active')

  // A sparse event never blanks the totals already known.
  const sparse = applyDownloadProgressUpdate(partial, { downloaded_bytes: 20, downloaded: 2, status: 'interrupted', workers_status: 'waiting_rate_limit' })
  assert.equal(sparse.totalBytes, 100)
  assert.equal(sparse.totalChunks, 5)
  assert.equal(sparse.status, 'interrupted')
  assert.equal(sparse.workersStatus, 'waiting_rate_limit')
  assert.equal(sparse.filename, 'a.zip')

  const failed = applyDownloadProgressUpdate(sparse, { status: 'error', error_message: 'boom' })
  assert.equal(failed.errorMessage, 'boom')
  const recovered = applyDownloadProgressUpdate(failed, { status: 'downloading' })
  assert.equal(recovered.errorMessage, '')
})

test('calculatePercent clamps values into 0..100', () => {
  assert.equal(calculatePercent(0, 0), 0)
  assert.equal(calculatePercent(5, 10), 50)
  assert.equal(calculatePercent(12, 10), 100)
  assert.equal(calculatePercent(-2, 10), 0)
})

test('isTerminalTransferStatus recognizes final transfer states', () => {
  assert.equal(isTerminalTransferStatus('done'), true)
  assert.equal(isTerminalTransferStatus('skipped'), true)
  assert.equal(isTerminalTransferStatus('error'), true)
  assert.equal(isTerminalTransferStatus('cancelled'), true)
  assert.equal(isTerminalTransferStatus('uploading'), false)
  assert.equal(isTerminalTransferStatus('verifying'), false)
})

test('isActiveUploadStatus keeps verification as an active upload phase', () => {
  assert.equal(isActiveUploadStatus('uploading'), true)
  assert.equal(isActiveUploadStatus('verifying'), true)
  assert.equal(isActiveUploadStatus('done'), false)
})

test('isActiveDownloadStatus keeps a browser-interrupted download alive', () => {
  assert.equal(isActiveDownloadStatus('downloading'), true)
  assert.equal(isActiveDownloadStatus('interrupted'), true)
  assert.equal(isActiveDownloadStatus('done'), false)
  assert.equal(isActiveDownloadStatus('error'), false)
  assert.equal(isActiveDownloadStatus('cancelled'), false)
})

test('interrupted is not a terminal transfer status', () => {
  assert.equal(isTerminalTransferStatus('interrupted'), false)
})

test('summarizeTerminalStatuses counts final results reliably', () => {
  const summary = summarizeTerminalStatuses({
    a: 'done',
    b: 'skipped',
    c: 'error',
    d: 'cancelled',
    e: 'downloading',
  })

  assert.deepEqual(summary, {
    completed: 4,
    doneCount: 1,
    skippedCount: 1,
    errorCount: 1,
    cancelledCount: 1,
  })
})

test('resolveBulkTransferStatus prioritizes error then cancelled then done', () => {
  assert.equal(resolveBulkTransferStatus({ errorCount: 1, cancelledCount: 0 }), 'error')
  assert.equal(resolveBulkTransferStatus({ errorCount: 0, cancelledCount: 2 }), 'cancelled')
  assert.equal(resolveBulkTransferStatus({ errorCount: 0, cancelledCount: 0 }), 'done')
})

test('workersStatusText maps the rate limit state and defaults to active', () => {
  assert.equal(workersStatusText('waiting_rate_limit'), 'Workers waiting (rate limit)')
  assert.equal(workersStatusText('active'), 'Workers active')
  assert.equal(workersStatusText(undefined), 'Workers active')
})

test('createUploadState starts an in-flight card with the known size', () => {
  assert.deepEqual(createUploadState('u1', 'a.bin', 42), {
    id: 'u1',
    filename: 'a.bin',
    totalBytes: 42,
    uploadedBytes: 0,
    totalChunks: 0,
    uploadedChunks: 0,
    verificationTotal: 0,
    verifiedChunks: 0,
    status: 'uploading',
    workersStatus: 'active',
  })
  assert.equal(createUploadState('u2', 'b.bin').totalBytes, 0)
})

test('applyUploadProgressUpdate merges events and keeps known totals', () => {
  const initial = createUploadState('u1', 'a.bin', 100)
  const partial = applyUploadProgressUpdate(initial, { uploaded_bytes: 10, uploaded: 1, status: 'uploading' })
  assert.equal(partial.totalBytes, 100)
  assert.equal(partial.uploadedBytes, 10)
  assert.equal(partial.workersStatus, 'active')

  const verifying = applyUploadProgressUpdate(partial, {
    total_bytes: 100, uploaded_bytes: 100, total: 5, uploaded: 5,
    verification_total: 5, verified: 2, status: 'verifying', workers_status: 'waiting_rate_limit',
  })
  assert.equal(verifying.status, 'verifying')
  assert.equal(verifying.verificationTotal, 5)
  assert.equal(verifying.verifiedChunks, 2)
  assert.equal(verifying.workersStatus, 'waiting_rate_limit')
  assert.equal(verifying.filename, 'a.bin')
})
