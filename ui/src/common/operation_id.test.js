import test from 'node:test'
import assert from 'node:assert/strict'
import { createOperationId } from './operation_id.js'

function withCrypto(mockCrypto, fn) {
  const original = Object.getOwnPropertyDescriptor(globalThis, 'crypto')
  Object.defineProperty(globalThis, 'crypto', {
    value: mockCrypto,
    configurable: true,
  })
  try {
    fn()
  } finally {
    if (original) {
      Object.defineProperty(globalThis, 'crypto', original)
    } else {
      delete globalThis.crypto
    }
  }
}

test('createOperationId uses crypto.randomUUID when available', () => {
  withCrypto(
    {
      randomUUID: () => '11111111-2222-4333-8444-555555555555',
    },
    () => {
      assert.equal(createOperationId(), '11111111-2222-4333-8444-555555555555')
    },
  )
})

test('createOperationId builds UUID-like value with getRandomValues fallback', () => {
  withCrypto(
    {
      getRandomValues: (arr) => {
        for (let i = 0; i < arr.length; i++) arr[i] = i
        return arr
      },
    },
    () => {
      const id = createOperationId()
      assert.match(id, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
    },
  )
})

test('createOperationId falls back to Date.now + Math.random when crypto is missing', () => {
  const originalNow = Date.now
  const originalRandom = Math.random
  withCrypto(undefined, () => {
    Date.now = () => 1700000000000
    Math.random = () => 0.5
    const id = createOperationId()
    assert.match(id, /^1700000000000-[0-9a-f]+$/)
  })
  Date.now = originalNow
  Math.random = originalRandom
})
