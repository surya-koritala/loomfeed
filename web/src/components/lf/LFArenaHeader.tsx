// web/src/components/lf/LFArenaHeader.tsx
'use client'

import React from 'react'
import { lfColor } from '../../lib/lf-tokens'

// Header for /arena/[id]. Eyebrow with arena name + community,
// status pill (Live / Voting / Completed) with a pulsing dot,
// topic h1, then meta line below.

export type LFArenaPhase = 'pending' | 'in_progress' | 'voting' | 'completed' | string

export interface LFArenaHeaderProps {
  topic: string
  /** Eyebrow text — e.g. "The Arena · climate-lab". */
  scope?: string
  phase: LFArenaPhase
  /** Display string for time remaining or phase context, e.g. "47m left". */
  phaseDetail?: string
  /** Round info, e.g. "Round 3 of 5". */
  roundInfo?: string
  /** Watching count — total spectators. */
  watching?: number
  /** Total votes cast across all rounds. */
  totalVotes?: number
}

const PHASE_STYLES: Record<string, { label: string; color: string }> = {
  pending:     { label: 'Pending',  color: lfColor.muted },
  in_progress: { label: 'Live',     color: lfColor.accent2 },
  live:        { label: 'Live',     color: lfColor.accent2 },
  voting:      { label: 'Voting',   color: lfColor.contested },
  completed:   { label: 'Completed', color: lfColor.seal },
}

function phaseOf(p: LFArenaPhase) {
  return PHASE_STYLES[p] ?? { label: p, color: lfColor.muted }
}

function fmt(n: number | undefined): string {
  if (n == null) return '—'
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return n.toLocaleString()
}

export function LFArenaHeader({
  topic,
  scope,
  phase,
  phaseDetail,
  roundInfo,
  watching,
  totalVotes,
}: LFArenaHeaderProps) {
  const ph = phaseOf(phase)
  const metaParts = [
    roundInfo,
    watching != null ? `${fmt(watching)} watching` : null,
    totalVotes != null ? `${fmt(totalVotes)} votes cast` : null,
  ].filter(Boolean)

  return (
    <header
      style={{
        padding: '28px 0 24px',
        borderBottom: 'var(--lf-border-w) solid var(--lf-ink)',
        background: 'var(--lf-paper)',
        marginBottom: 24,
      }}
    >
      {/* Eyebrow + status pill */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 12,
          marginBottom: 8,
          flexWrap: 'wrap',
        }}
      >
        {scope && (
          <span
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 11,
              color: 'var(--lf-muted)',
              letterSpacing: '0.08em',
              textTransform: 'uppercase',
            }}
          >
            {scope}
          </span>
        )}
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 6,
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            color: ph.color,
            letterSpacing: '0.04em',
            textTransform: 'uppercase',
            fontWeight: 700,
          }}
        >
          <span
            aria-hidden
            style={{
              width: 6,
              height: 6,
              borderRadius: 'var(--lf-radius-tag)',
              background: ph.color,
              boxShadow: `0 0 0 4px ${ph.color}33`,
            }}
          />
          {ph.label}
          {phaseDetail ? ` · ${phaseDetail}` : ''}
        </span>
      </div>

      {/* Topic */}
      <h1
        style={{
          fontFamily: 'var(--lf-font-display)',
          fontWeight: 800,
          fontSize: 'clamp(24px, 5.5vw, 38px)',
          letterSpacing: '-0.025em',
          color: 'var(--lf-ink)',
          margin: 0,
          lineHeight: 1.15,
          wordBreak: 'break-word',
        }}
      >
        {topic}
      </h1>

      {/* Meta */}
      {metaParts.length > 0 && (
        <div
          style={{
            marginTop: 10,
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 12,
            color: 'var(--lf-muted)',
          }}
        >
          {metaParts.join(' · ')}
        </div>
      )}
    </header>
  )
}
