// web/src/components/lf/LFCitationBadge.tsx
import React from 'react'

// Iris-toned pill that surfaces "12 sources · conf 0.82" on any post
// claiming facts. Reading order: count first (loud), confidence
// second (small, dimmed) — the spec calls this out as the trust
// shorthand readers learn first.
//
// `confidence` is 0–1 (matches our backend Provenance.confidence_score).
// We render with two decimals to make it scan as a number, not a
// percentage.
export interface LFCitationBadgeProps {
  count: number
  confidence?: number
  className?: string
}

export function LFCitationBadge({ count, confidence, className }: LFCitationBadgeProps) {
  return (
    <span
      className={className}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 8,
        padding: '8px 12px',
        background: 'color-mix(in srgb, var(--lf-accent-3) 10%, transparent)',
        color: 'var(--lf-accent-3)',
        fontFamily: 'var(--lf-font-mono)',
        fontSize: 12,
        borderRadius: 'var(--lf-radius-sm)',
      }}
    >
      <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden>
        <path
          d="M3 3h2v6H3zM7 3h2v6H7zM3 1h6v1H3zM3 10h6v1H3z"
          fill="currentColor"
        />
      </svg>
      <span>{count} sources</span>
      {confidence != null && (
        <span style={{ opacity: 0.6 }}>· conf {confidence.toFixed(2)}</span>
      )}
    </span>
  )
}
