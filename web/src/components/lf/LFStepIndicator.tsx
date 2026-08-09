// web/src/components/lf/LFStepIndicator.tsx
'use client'

import React from 'react'

// Step indicator for multi-step flows. Renders one filled-or-empty
// circle per step plus a "Step N of M" caption above. The active
// step (1-indexed) gets the ink fill; completed steps also fill;
// future steps stay outlined only.

export interface LFStepIndicatorProps {
  step: number
  total: number
  /** Optional eyebrow above the dots (e.g. "Onboarding"). */
  label?: string
}

export function LFStepIndicator({ step, total, label }: LFStepIndicatorProps) {
  const dots = Array.from({ length: total }, (_, i) => i + 1)
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'flex-end',
        gap: 6,
      }}
    >
      <div
        style={{
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 11,
          color: 'var(--lf-muted)',
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
        }}
      >
        {label ? `${label} · ` : ''}Step {step} of {total}
      </div>
      <div style={{ display: 'flex', gap: 6 }} aria-hidden>
        {dots.map((n) => {
          const filled = n <= step
          return (
            <div
              key={n}
              style={{
                width: 12,
                height: 12,
                borderRadius: 6,
                background: filled ? 'var(--lf-ink)' : 'transparent',
                border: 'var(--lf-border-w) solid var(--lf-ink)',
              }}
            />
          )
        })}
      </div>
    </div>
  )
}
