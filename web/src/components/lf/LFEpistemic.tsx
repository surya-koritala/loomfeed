// web/src/components/lf/LFEpistemic.tsx
import React from 'react'
import { lfColor } from '../../lib/lf-tokens'

// Five-state epistemic chip — the core of loomfeed's signal layer.
// Tells the reader at a glance whether the post is a hypothesis,
// supported by evidence, contested, refuted, or community consensus.
// Mono font, uppercase, with a tinted background and a tiny color dot.
//
// Color choices match Direction A spec §4:
//   - hypothesis → iris (5B5BFF)  — "we don't know yet"
//   - supported  → green (0E8463) — "evidence backs this"
//   - contested  → amber (B45309) — "actively disputed"
//   - refuted    → red (B91C1C)   — "evidence against"
//   - consensus  → lime fill      — "community agrees"
export type LFEpistemicKind = 'hypothesis' | 'supported' | 'contested' | 'refuted' | 'consensus'

interface KindStyle {
  fg: string
  bg: string
  label: string
}

const STYLES: Record<LFEpistemicKind, KindStyle> = {
  hypothesis: { fg: lfColor.accent3, bg: 'color-mix(in srgb, var(--lf-accent-3) 12%, transparent)', label: 'Hypothesis' },
  supported:  { fg: lfColor.seal,    bg: 'color-mix(in srgb, var(--lf-pos) 12%, transparent)',  label: 'Supported' },
  contested:  { fg: lfColor.contested, bg: 'rgba(180,83,9,0.12)', label: 'Contested' },
  refuted:    { fg: lfColor.refuted, bg: 'rgba(185,28,28,0.10)',  label: 'Refuted' },
  consensus:  { fg: lfColor.ink,     bg: lfColor.accent,           label: 'Consensus' },
}

export interface LFEpistemicProps {
  kind: LFEpistemicKind
  className?: string
}

export function LFEpistemic({ kind, className }: LFEpistemicProps) {
  const s = STYLES[kind]
  return (
    <span
      className={className}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 5,
        padding: '2px 8px',
        background: s.bg,
        color: s.fg,
        fontFamily: 'var(--lf-font-mono)',
        fontSize: 11,
        fontWeight: 600,
        letterSpacing: '0.04em',
        borderRadius: 'var(--lf-radius-sm)',
        textTransform: 'uppercase',
      }}
    >
      <span
        aria-hidden
        style={{
          width: 5,
          height: 5,
          background: s.fg,
          borderRadius: '50%',
        }}
      />
      {s.label}
    </span>
  )
}
