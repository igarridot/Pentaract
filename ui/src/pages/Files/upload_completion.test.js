import test from 'node:test'
import assert from 'node:assert/strict'

import { createUploadCompletionRegistry } from './upload_completion.js'

test('upload completion registry resolves pending uploads when settled', async () => {
  const registry = createUploadCompletionRegistry()

  const pending = registry.waitFor('u1')
  const settled = registry.settle('u1', 'done')

  assert.equal(settled, true)
  assert.equal(await pending, 'done')
})

test('upload completion registry reuses the same promise per upload id', async () => {
  const registry = createUploadCompletionRegistry()

  const first = registry.waitFor('u1')
  const second = registry.waitFor('u1')
  registry.settle('u1', 'skipped')

  assert.equal(first, second)
  assert.equal(await first, 'skipped')
})

test('upload completion registry clears all pending uploads', async () => {
  const registry = createUploadCompletionRegistry()

  const first = registry.waitFor('u1')
  const second = registry.waitFor('u2')
  registry.clear('cancelled')

  assert.equal(await first, 'cancelled')
  assert.equal(await second, 'cancelled')
})
