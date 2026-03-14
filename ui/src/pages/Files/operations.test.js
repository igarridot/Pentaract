import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildBulkMoveTargetPath,
  buildRenamedPath,
  createBulkOperation,
  getBulkOperationMetrics,
  getItemPath,
  getMediaType,
} from './operations.js'

test('createBulkOperation enables transfer tracking only for upload and download', () => {
  assert.deepEqual(createBulkOperation('upload', 3), {
    operation: 'upload',
    status: 'running',
    total: 3,
    completed: 0,
    transferIds: [],
    terminalStatuses: {},
    startedAll: false,
  })

  assert.deepEqual(createBulkOperation('move', 2), {
    operation: 'move',
    status: 'running',
    total: 2,
    completed: 0,
    transferIds: [],
  })
})

test('getBulkOperationMetrics aggregates tracked uploads and propagates rate limit status', () => {
  const bulkOperation = {
    operation: 'upload',
    transferIds: ['u1', 'u2'],
  }

  const metrics = getBulkOperationMetrics(bulkOperation, [
    { id: 'u1', totalBytes: 10, uploadedBytes: 4, totalChunks: 3, uploadedChunks: 1, workersStatus: 'active' },
    { id: 'u2', totalBytes: 20, uploadedBytes: 15, totalChunks: 5, uploadedChunks: 4, workersStatus: 'waiting_rate_limit' },
    { id: 'u3', totalBytes: 99, uploadedBytes: 99, totalChunks: 9, uploadedChunks: 9, workersStatus: 'active' },
  ])

  assert.deepEqual(metrics, {
    totalBytes: 30,
    processedBytes: 19,
    totalChunks: 8,
    processedChunks: 5,
    workersStatus: 'waiting_rate_limit',
  })
})

test('getBulkOperationMetrics handles move operations without transfer state arrays', () => {
  assert.deepEqual(getBulkOperationMetrics({
    operation: 'move',
    total: 5,
    completed: 2,
  }), {
    totalBytes: 5,
    processedBytes: 2,
    totalChunks: 5,
    processedChunks: 2,
    workersStatus: 'active',
  })
})

test('file operation helpers derive paths and media type consistently', () => {
  assert.equal(getMediaType('movie.mp4'), 'video')
  assert.equal(getMediaType('photo.PNG'), 'image')
  assert.equal(getMediaType('notes.txt'), null)

  assert.equal(getItemPath({ path: 'docs/report.pdf/' }), 'docs/report.pdf')
  assert.equal(buildRenamedPath({ path: 'docs/report.pdf' }, 'final.pdf'), 'docs/final.pdf')
  assert.equal(buildBulkMoveTargetPath('archive', 'report.pdf'), 'archive/report.pdf')
  assert.equal(buildBulkMoveTargetPath('', 'report.pdf'), 'report.pdf')
})
