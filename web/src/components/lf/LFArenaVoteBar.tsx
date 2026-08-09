// web/src/components/lf/LFArenaVoteBar.tsx
'use client'

import React from 'react'
import { lfColor } from '../../lib/lf-tokens'

// Bottom-of-page distribution bar. Shows the live A/B vote split as a
// stacked horizontal bar plus an optional human/agent breakdown.

export interface LFArenaVoteBarProps {
  pctA: number
  pctB: number
  humansPct?: number
  agentsPct?: number
}

export function LFArenaVoteBar({
  pctA,
  pctB,
  humansPct,
  agentsPct,
}: LFArenaVoteBarProps) {
  const a = Math.max(0, Math.min(100, pctA))
  const b = Math.max(0, Math.min(100, pctB))
  return (
    <div
      style={{
        padding: '20px 0',
        borderTop: 'var(--lf-border-w) solid var(--lf-ink)',
        marginTop: 24,
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 16,
          marginBottom: 8,
          flexWrap: 'wrap',
        }}
      >
        <span
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            color: 'var(--lf-muted)',
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
          }}
        >
          Live tally
        </span>
        {humansPct != null && agentsPct != null && (
          <span
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 11,
              color: 'var(--lf-muted)',
            }}
          >
            · humans {Math.round(humansPct)}% · contributors {Math.round(agentsPct)}%
          </span>
        )}
      </div>
      <div
        style={{
          height: 12,
          display: 'flex',
          borderRadius: 'var(--lf-radius-sm)',
          overflow: 'hidden',
          border: 'var(--lf-border-w) solid var(--lf-ink)',
        }}
      >
        <div style={{ width: `${a}%`, background: lfColor.accent }} />
        <div style={{ width: `${b}%`, background: lfColor.accent2 }} />
      </div>
    </div>
  )
}
