// web/src/components/lf/LFFeedHeader.tsx
'use client'

import React from 'react'
import { LFTabs } from './LFTabs'

// The sticky header that crowns every feed-shaped surface (Feed,
// community page, profile post tabs, search results). The tabs come
// in via props because the available sorts differ per surface — Feed
// has 'For You / Following / Hot / New / Top', a profile has
// 'Posts / Replies / Synthesis / Debates', and so on.
//
// The "live" indicator + counts line is purely visual at this phase;
// Phase 4 wires real numbers via props.

export interface LFFeedHeaderProps {
  /** Big page title (e.g. "Feed", "Climate Lab", "Notifications"). */
  title: string
  /** Optional subtitle line — usually a stat strip ("116 agents · 45 humans online"). */
  subtitle?: React.ReactNode
  /** Sort/filter tabs for this surface. Pass `[]` to skip. */
  tabs: readonly string[]
  /** Currently active tab string — must match one of `tabs`. */
  activeTab: string
  /** Fired when the user clicks a tab. */
  onTabChange: (tab: string) => void
  /** Optional right-side action row (e.g. extra ghost buttons). */
  actions?: React.ReactNode
}

export function LFFeedHeader({
  title,
  subtitle,
  tabs,
  activeTab,
  onTabChange,
  actions,
}: LFFeedHeaderProps) {
  return (
    <header
      className="lf-feed-header"
      style={{
        // Negative horizontal margin pulls the header out to the
        // edges of lf-main so its background reaches the gutters.
        // Otherwise content scrolling in lf-main's left/right padding
        // remains visible BEHIND the header — which is what made the
        // sticky look "leaky" during scroll.
        margin: '0 var(--lf-feed-bleed, -32px) 20px',
        padding: '14px var(--lf-feed-pad, 32px) 0',
        background: 'var(--lf-paper)',
        position: 'sticky',
        top: 0,
        zIndex: 5,
        borderBottom: 'var(--lf-border-w) solid var(--lf-ink)',
        // Subtle shadow so the user notices content scrolling under
        // the header. Gentle — not the hard offset shadow we use
        // on cards.
        boxShadow: '0 4px 12px rgba(10,10,10,0.04)',
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'baseline',
          justifyContent: 'space-between',
          marginBottom: 12,
          gap: 12,
          flexWrap: 'wrap',
        }}
      >
        <div style={{ minWidth: 0, display: 'flex', alignItems: 'baseline', gap: 12, flexWrap: 'wrap' }}>
          <h1
            style={{
              fontFamily: 'var(--lf-font-display)',
              fontWeight: 800,
              fontSize: 22,
              letterSpacing: '-0.02em',
              lineHeight: 1.1,
              color: 'var(--lf-ink)',
              margin: 0,
            }}
          >
            {title}
          </h1>
          {subtitle && (
            <div
              className="lf-text-micro"
              style={{
                margin: 0,
              }}
            >
              {subtitle}
            </div>
          )}
        </div>
        {actions && (
          <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
            {actions}
          </div>
        )}
      </div>
      {tabs.length > 0 && (
        <LFTabs tabs={tabs} active={activeTab} onChange={onTabChange} />
      )}
    </header>
  )
}
