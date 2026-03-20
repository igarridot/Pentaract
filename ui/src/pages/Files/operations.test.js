import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildBulkMoveTargetPath,
  buildRenamedPath,
  createBulkOperation,
  getBulkOperationMetrics,
  getItemPath,
  getMediaType,
  runUploadPipeline,
  runSequentialUploadPipeline,
} from './operations.js'

function createDeferred() {
  let resolve
  let reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

async function flushAsyncWork() {
  await Promise.resolve()
  await new Promise((resolve) => setTimeout(resolve, 0))
}

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

test('runUploadPipeline starts the next upload once the current request is sent', async () => {
  const events = []
  const request1 = createDeferred()
  const request2 = createDeferred()
  const request3 = createDeferred()
  const completion1 = createDeferred()
  const completion2 = createDeferred()
  const completion3 = createDeferred()

  const pipeline = runUploadPipeline(['f1', 'f2', 'f3'], (item) => {
    events.push(`start:${item}`)
    if (item === 'f1') {
      return { requestPromise: request1.promise, completionPromise: completion1.promise }
    }
    if (item === 'f2') {
      return { requestPromise: request2.promise, completionPromise: completion2.promise }
    }
    return { requestPromise: request3.promise, completionPromise: completion3.promise }
  })

  await flushAsyncWork()
  assert.deepEqual(events, ['start:f1'])

  request1.resolve('sent')
  await flushAsyncWork()
  assert.deepEqual(events, ['start:f1', 'start:f2'])

  request2.resolve('sent')
  await flushAsyncWork()
  assert.deepEqual(events, ['start:f1', 'start:f2'])

  completion1.resolve('done')
  await flushAsyncWork()
  assert.deepEqual(events, ['start:f1', 'start:f2', 'start:f3'])

  request3.resolve('sent')
  completion2.resolve('done')
  completion3.resolve('done')
  await pipeline
})

test('runUploadPipeline stops launching new uploads when startUpload returns null', async () => {
  const started = []
  const request1 = createDeferred()
  const completion1 = createDeferred()

  const pipeline = runUploadPipeline(['f1', 'f2'], (item) => {
    started.push(item)
    if (item === 'f2') return null
    return { requestPromise: request1.promise, completionPromise: completion1.promise }
  })

  await flushAsyncWork()
  request1.resolve('sent')
  completion1.resolve('done')
  await pipeline

  assert.deepEqual(started, ['f1', 'f2'])
})

test('runSequentialUploadPipeline waits for each upload to complete before starting the next one', async () => {
  const events = []
  const request1 = createDeferred()
  const request2 = createDeferred()
  const completion1 = createDeferred()
  const completion2 = createDeferred()

  const pipeline = runSequentialUploadPipeline(['f1', 'f2'], (item) => {
    events.push(`start:${item}`)
    if (item === 'f1') {
      return { requestPromise: request1.promise, completionPromise: completion1.promise }
    }
    return { requestPromise: request2.promise, completionPromise: completion2.promise }
  })

  await flushAsyncWork()
  assert.deepEqual(events, ['start:f1'])

  request1.resolve('sent')
  await flushAsyncWork()
  assert.deepEqual(events, ['start:f1'])

  completion1.resolve('done')
  await flushAsyncWork()
  assert.deepEqual(events, ['start:f1', 'start:f2'])

  request2.resolve('sent')
  completion2.resolve('done')
  await pipeline
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
