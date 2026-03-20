import test from 'node:test'
import assert from 'node:assert/strict'

import {
  applyUploadProgressState,
  createUploadState,
  getSkippedUploadEntries,
} from './upload_manager_helpers.js'
import { createUploadEntryKey } from './upload_manager_helpers.js'

test('createUploadState builds the initial tracking shape', () => {
  assert.deepEqual(createUploadState({
    filename: 'report.txt',
    size: 12,
    targetPath: 'docs',
  }, 'up-1'), {
    id: 'up-1',
    filename: 'report.txt',
    totalBytes: 12,
    uploadedBytes: 0,
    totalChunks: 0,
    uploadedChunks: 0,
    verificationTotal: 0,
    verifiedChunks: 0,
    status: 'uploading',
    workersStatus: 'active',
  })
})

test('applyUploadProgressState merges server progress without dropping fallbacks', () => {
  const next = applyUploadProgressState({
    id: 'up-1',
    totalBytes: 12,
    totalChunks: 3,
    workersStatus: 'active',
  }, {
    filename: 'report.txt',
    size: 12,
  }, {
    uploaded_bytes: 7,
    uploaded: 2,
    status: 'verifying',
    verification_total: 3,
    verified: 1,
  })

  assert.deepEqual(next, {
    id: 'up-1',
    totalBytes: 12,
    totalChunks: 3,
    workersStatus: 'active',
    filename: 'report.txt',
    uploadedBytes: 7,
    uploadedChunks: 2,
    verificationTotal: 3,
    verifiedChunks: 1,
    status: 'verifying',
  })
})

test('getSkippedUploadEntries compares uploads by target path and filename', () => {
  const entries = [
    { filename: 'a.txt', targetPath: 'docs' },
    { filename: 'b.txt', targetPath: 'docs' },
    { filename: 'a.txt', targetPath: 'archive' },
  ]
  const uploaded = [
    { filename: 'b.txt', targetPath: 'docs' },
  ]

  assert.deepEqual(getSkippedUploadEntries(entries, uploaded), [
    { filename: 'a.txt', targetPath: 'docs' },
    { filename: 'a.txt', targetPath: 'archive' },
  ])
  assert.equal(createUploadEntryKey(entries[0]), 'docs::a.txt')
})
