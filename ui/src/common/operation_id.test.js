import test from 'node:test'
import assert from 'node:assert/strict'
import { createOperationId } from './operation_id.js'

const uuidV4Regex = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

test('createOperationId returns a valid UUID string', () => {
  const id = createOperationId()
  assert.match(id, uuidV4Regex)
})

test('createOperationId returns unique values', () => {
  const a = createOperationId()
  const b = createOperationId()
  assert.notEqual(a, b)
})

test('createOperationId fallback produces valid UUIDs when crypto.randomUUID is unavailable', () => {
  const original = crypto.randomUUID
  try {
    crypto.randomUUID = undefined
    const id = createOperationId()
    assert.match(id, uuidV4Regex)
    const id2 = createOperationId()
    assert.notEqual(id, id2)
  } finally {
    crypto.randomUUID = original
  }
})
