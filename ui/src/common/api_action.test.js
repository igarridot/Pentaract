import test from 'node:test'
import assert from 'node:assert/strict'
import { runWithAlert } from './api_action.js'

function recorder() {
  const calls = []
  const addAlert = (message, severity) => calls.push({ message, severity })
  return { calls, addAlert }
}

test('runWithAlert returns the result and reports success when asked', async () => {
  const { calls, addAlert } = recorder()
  const seen = []
  const result = await runWithAlert(addAlert, async () => 42, { success: 'Saved', onSuccess: (r) => seen.push(r) })
  assert.equal(result, 42)
  assert.deepEqual(seen, [42])
  assert.deepEqual(calls, [{ message: 'Saved', severity: 'success' }])
})

test('runWithAlert skips onSuccess when the call fails', async () => {
  const { addAlert } = recorder()
  const seen = []
  await runWithAlert(addAlert, async () => { throw new Error('x') }, { onSuccess: (r) => seen.push(r) })
  assert.deepEqual(seen, [])
})

test('runWithAlert stays quiet on success without a message', async () => {
  const { calls, addAlert } = recorder()
  await runWithAlert(addAlert, async () => 'ok')
  assert.deepEqual(calls, [])
})

test('runWithAlert reports the error message and returns undefined', async () => {
  const { calls, addAlert } = recorder()
  const result = await runWithAlert(addAlert, async () => { throw new Error('nope') }, { success: 'Saved' })
  assert.equal(result, undefined)
  assert.deepEqual(calls, [{ message: 'nope', severity: 'error' }])
})

test('runWithAlert lets onError take over the failure', async () => {
  const { calls, addAlert } = recorder()
  const seen = []
  await runWithAlert(addAlert, async () => { throw new Error('forbidden') }, {
    onError: (err) => { seen.push(err.message); return true },
  })
  assert.deepEqual(seen, ['forbidden'])
  assert.deepEqual(calls, [])
})
