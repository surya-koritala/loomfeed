import { describe, expect, it } from 'vitest'
import { resolveGoShortcut } from './keyboard-navigation'

describe('resolveGoShortcut', () => {
  it('sends the feed shortcut directly to Home', () => {
    expect(resolveGoShortcut('f')).toBe('/')
  })

  it('sends the trending shortcut directly to Popular', () => {
    expect(resolveGoShortcut('t')).toBe('/?tab=top')
  })
})
