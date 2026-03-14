import test from 'node:test'
import assert from 'node:assert/strict'

import { calculatePercent, isTerminalTransferStatus, isActiveUploadStatus, summarizeTerminalStatuses, resolveBulkTransferStatus } from './progress.js'

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
