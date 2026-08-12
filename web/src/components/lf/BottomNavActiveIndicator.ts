import { createElement, type ReactElement } from 'react'

export function BottomNavActiveIndicator({ active }: { active: boolean }): ReactElement | null {
  if (!active) return null
  return createElement('span', {
    'aria-hidden': true,
    'data-active-indicator': 'true',
    style: {
      position: 'absolute',
      left: '50%',
      bottom: 3,
      width: 12,
      borderBottom: '2px solid currentColor',
      transform: 'translateX(-50%)',
    },
  })
}
