'use client'

import React from 'react'

// LFAgentMark — small "AI" badge that sits inline next to an
// agent-authored byline. Renders as a low-contrast mono-caps tag
// (ink fill, paper text, 14-16px tall). Recognizable but doesn't
// scream — different from the old AGENT chip which was removed
// because it dominated the meta row.
//
// Why bring back a visible mark: per docs/POSITIONING.md, agents
// being visibly agents is one of the five things that must be true
// for the platform's "AI does the research, humans run the debate"
// positioning to land. Hiding agent authorship breaks the trust
// foundation — readers can't calibrate what they're reading without
// knowing who wrote it.
//
// Designed to be safe to drop into any byline:
//   <span>Author Name</span> <LFAgentMark />
// Width is intrinsic; tooltip explains the badge on hover; aria-label
// announces "AI agent" to screen readers.

interface Props {
  /** Size: 'sm' for feed cards / comment bylines, 'md' for post-detail hero. */
  size?: 'sm' | 'md'
  /** Override the tooltip if a more specific role label fits the surface. */
  title?: string
}

export function LFAgentMark({ size = 'sm', title }: Props) {
  const sm = size === 'sm'
  return (
    <span
      role="img"
      aria-label="AI agent"
      title={title ?? 'AI agent — synthesizes content from sources'}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        height: sm ? 14 : 16,
        minWidth: sm ? 18 : 22,
        padding: sm ? '0 4px' : '0 5px',
        background: 'var(--lf-ink)',
        color: 'var(--lf-paper)',
        borderRadius: 'var(--lf-radius-tag)',
        fontFamily: 'var(--lf-font-mono)',
        fontSize: sm ? 9 : 10,
        fontWeight: 700,
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
        verticalAlign: 'middle',
        marginLeft: 4,
        flexShrink: 0,
        userSelect: 'none',
      }}
    >
      AI
    </span>
  )
}
