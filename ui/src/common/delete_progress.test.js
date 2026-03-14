import test from 'node:test'
import assert from 'node:assert/strict'
import {
  DELETE_PROGRESS_ERROR_DELAY_MS,
  DELETE_PROGRESS_SUCCESS_DELAY_MS,
  createDeleteProgressState,
  applyDeleteProgressUpdate,
  getDeleteProgressResetDelay,
} from './delete_progress.js'

test('createDeleteProgressState builds the default in-flight shape', () => {
  assert.deepEqual(createDeleteProgressState('Docs'), {
    label: 'Docs',
    totalChunks: 0,
    deletedChunks: 0,
    status: 'deleting',
    workersStatus: 'active',
  })
})

test('applyDeleteProgressUpdate keeps previous totals when telegram does not send them again', () => {
  const previous = {
    label: 'Docs',
    totalChunks: 4,
    deletedChunks: 1,
    status: 'deleting',
    workersStatus: 'active',
  }

  assert.deepEqual(applyDeleteProgressUpdate(previous, {
    deleted: 3,
    status: 'done',
  }), {
    label: 'Docs',
    totalChunks: 4,
    deletedChunks: 3,
    status: 'done',
    workersStatus: 'active',
  })
})

test('applyDeleteProgressUpdate prefers the latest worker status when present', () => {
  const previous = createDeleteProgressState('Docs')

  assert.deepEqual(applyDeleteProgressUpdate(previous, {
    total: 7,
    deleted: 2,
    status: 'deleting',
    workers_status: 'waiting_rate_limit',
  }), {
    label: 'Docs',
    totalChunks: 7,
    deletedChunks: 2,
    status: 'deleting',
    workersStatus: 'waiting_rate_limit',
  })
})

test('getDeleteProgressResetDelay maps terminal states to the expected timeout', () => {
  assert.equal(getDeleteProgressResetDelay('done'), DELETE_PROGRESS_SUCCESS_DELAY_MS)
  assert.equal(getDeleteProgressResetDelay('error'), DELETE_PROGRESS_ERROR_DELAY_MS)
})
