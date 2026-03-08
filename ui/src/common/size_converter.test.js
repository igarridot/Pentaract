import test from 'node:test'
import assert from 'node:assert/strict'
import { convertSize } from './size_converter.js'

test('convertSize handles zero and bytes', () => {
  assert.equal(convertSize(0), '0 bytes')
  assert.equal(convertSize(1), '1 bytes')
  assert.equal(convertSize(512), '512 bytes')
})

test('convertSize handles kibibytes', () => {
  assert.equal(convertSize(1024), '1 KiB')
  assert.equal(convertSize(1536), '1.5 KiB')
})

test('convertSize handles larger units', () => {
  assert.equal(convertSize(1024 * 1024), '1 MiB')
  assert.equal(convertSize(1024 * 1024 * 1024), '1 GiB')
  assert.equal(convertSize(1024 ** 4), '1 TiB')
})
