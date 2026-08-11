import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { BottomNavActiveIndicator } from './BottomNavActiveIndicator'

describe('BottomNavActiveIndicator', () => {
  it('renders a structural marker for the active tab', () => {
    expect(
      renderToStaticMarkup(createElement(BottomNavActiveIndicator, { active: true }))
    ).toContain('data-active-indicator="true"')
  })

  it('renders no structural marker for an inactive tab', () => {
    expect(
      renderToStaticMarkup(createElement(BottomNavActiveIndicator, { active: false }))
    ).toBe('')
  })
})
