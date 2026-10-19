import test from 'node:test'
import assert from 'node:assert/strict'
import { COMPACT_HIDDEN_CONTROLS, ESSENTIAL_CONTROLS, compactControlsSx } from './player_layout.js'

const hiddenSelectors = (sx) => Object.entries(sx)
  .filter(([, rules]) => rules.display === 'none')
  .flatMap(([selector]) => selector.split(',').map((s) => s.trim()))

test('the compact bar never hides the controls needed to drive the player', () => {
  const hidden = hiddenSelectors(compactControlsSx()).join(' ')
  for (const tag of ESSENTIAL_CONTROLS) {
    assert.equal(hidden.includes(tag), false, `${tag} must stay visible`)
  }
})

test('the compact bar hides the optional controls only when the player is narrow', () => {
  const hidden = hiddenSelectors(compactControlsSx())
  assert.deepEqual(hidden, COMPACT_HIDDEN_CONTROLS.map((tag) => `&:not([breakpointmd]) ${tag}`))
})

test('the compact bar wraps and gives the time bar a row of its own', () => {
  const sx = compactControlsSx()
  assert.equal(sx['&:not([breakpointmd]) media-control-bar'].flexWrap, 'wrap')
  assert.deepEqual(sx['&:not([breakpointmd]) media-time-range'], { order: -1, flexBasis: '100%', minHeight: 'auto' })
})
