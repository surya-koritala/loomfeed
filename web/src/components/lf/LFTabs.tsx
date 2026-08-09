// web/src/components/lf/LFTabs.tsx
import React from 'react'

// Underline-active tab strip. Used on Feed (For You / Following / Hot
// / New / Top / Sealed / Synthesis / Debates), profile (Posts /
// Replies / etc.), leaderboard, and notifications.
//
// Bottom border is the strip's separator from page content; active
// tab gets a thicker 3px underline that overlaps the border so it
// reads as a "stuck" indicator. `marginBottom: -borderW` is what
// makes the overlap pixel-perfect.
//
// Tab labels must be unique within a single LFTabs — they double as
// React keys and as the active-tab identifier.
export interface LFTabsProps {
  tabs: readonly string[]
  active: string
  onChange?: (tab: string) => void
  className?: string
}

export function LFTabs({ tabs, active, onChange, className }: LFTabsProps) {
  return (
    <div
      className={className}
      role="tablist"
      style={{
        display: 'flex',
        gap: 0,
        borderBottom: `var(--lf-border-w) solid var(--lf-ink)`,
        overflowX: 'auto',
      }}
    >
      {tabs.map((tab) => {
        const isActive = tab === active
        return (
          <button
            key={tab}
            role="tab"
            aria-selected={isActive}
            onClick={() => onChange?.(tab)}
            style={{
              padding: '12px 18px',
              background: 'transparent',
              border: 'none',
              fontFamily: 'var(--lf-font-body)',
              fontWeight: isActive ? 700 : 500,
              fontSize: 14,
              color: isActive ? 'var(--lf-ink)' : 'var(--lf-muted)',
              letterSpacing: '-0.01em',
              cursor: 'pointer',
              borderBottom: `3px solid ${isActive ? 'var(--lf-ink)' : 'transparent'}`,
              marginBottom: `calc(var(--lf-border-w) * -1)`,
              whiteSpace: 'nowrap',
              transition: 'color .12s',
            }}
          >
            {tab}
          </button>
        )
      })}
    </div>
  )
}
