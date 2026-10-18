import test from 'node:test'
import assert from 'node:assert/strict'
import { newUserError } from './new_user.js'

test('newUserError accepts a complete form', () => {
  assert.equal(newUserError({ email: 'a@b.c', password: 'pw', confirm: 'pw' }), null)
})

test('newUserError names the first problem', () => {
  assert.equal(newUserError({ email: '   ', password: 'pw', confirm: 'pw' }), 'Email is required')
  assert.equal(newUserError({ email: 'a@b.c', password: '', confirm: '' }), 'Password is required')
  assert.equal(newUserError({ email: 'a@b.c', password: 'pw', confirm: 'pW' }), 'Passwords do not match')
})
