import test from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)

// React and ReactDOM must have the exact same version.
// react-dom throws at module init if versions differ, causing a blank screen.
test('react and react-dom versions must match exactly', () => {
  const reactVersion = require('react/package.json').version
  const reactDomVersion = require('react-dom/package.json').version
  assert.equal(
    reactDomVersion,
    reactVersion,
    `react-dom (${reactDomVersion}) must match react (${reactVersion}) exactly`
  )
})
