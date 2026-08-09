// web/src/components/lf/LFLoomChip.tsx
import React from 'react'

// Brand chip next to a Loom-authored comment's display name.
//
// Two parts:
//   1. The wordmark "LOOM" — small mono pill marking platform-AI
//      replies so users can distinguish them from regular author
//      content.
//   2. Optional intent tag — "summarize" / "fact-check" / "counter" —
//      shown when the comment row knows which specialty produced this
//      reply. Lets users build a mental model of which specialty
//      they're getting without us ever showing different agent names.
export interface LFLoomChipProps {
  intent?: string | null
  size?: 'sm' | 'md'
  className?: string
}

const SIZES: Record<'sm' | 'md', { fontSize: number; padding: string }> = {
  sm: { fontSize: 9, padding: '1px 5px' },
  md: { fontSize: 11, padding: '3px 8px' },
}

export function LFLoomChip({ intent, size = 'sm', className }: LFLoomChipProps) {
  const s = SIZES[size]
  return (
    <span
      className={className}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        background: 'var(--lf-ink)',
        color: 'var(--lf-paper)',
        fontFamily: 'var(--lf-font-mono)',
        fontWeight: 700,
        fontSize: s.fontSize,
        padding: s.padding,
        letterSpacing: '0.06em',
        borderRadius: 'var(--lf-radius-tag)',
      }}
    >
      <span>LOOM</span>
      {intent ? (
        <span
          style={{
            opacity: 0.75,
            fontWeight: 600,
            textTransform: 'lowercase',
            letterSpacing: '0.02em',
          }}
        >
          · {intent}
        </span>
      ) : null}
    </span>
  )
}
