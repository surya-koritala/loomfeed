// web/src/components/lf/LFTrustChip.tsx
import React from 'react'
import { lfColor } from '../../lib/lf-tokens'

// Reputation chip — mono number with thousands separator. Color is
// contextual against the new uncapped rep system:
//
//   1000+ → seal (elite — top tier on the platform)
//    500+ → muted (strong)
//    100+ → muted (baseline — every new participant starts here)
//    <100 → contested (below baseline; lost ground)
//      0  → refuted (catastrophic — only at hard floor)
//
// The legacy name LFTrustChip is kept for import-stability; it now
// labels itself "rep" and reads uncapped values.
export interface LFTrustChipProps {
  score: number
  showLabel?: boolean
  className?: string
}

function colorForScore(score: number): string {
  if (score >= 1000) return lfColor.seal
  if (score >= 100) return lfColor.muted
  if (score > 0) return lfColor.contested
  return lfColor.refuted
}

export function LFTrustChip({ score, showLabel = true, className }: LFTrustChipProps) {
  const fg = colorForScore(score)
  // Display whole numbers — fractional rep is implementation noise.
  const display = Math.round(score).toLocaleString()
  return (
    <span
      className={className}
      style={{
        display: 'inline-flex',
        alignItems: 'baseline',
        gap: 4,
        fontFamily: 'var(--lf-font-mono)',
        fontSize: 12,
        fontWeight: 700,
        color: fg,
        fontVariantNumeric: 'tabular-nums',
      }}
    >
      {showLabel && (
        <span style={{ color: 'var(--lf-muted)', fontWeight: 500 }}>rep</span>
      )}
      {display}
    </span>
  )
}
