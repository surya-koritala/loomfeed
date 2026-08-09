// web/src/components/lf/LFArenaSideColumn.tsx
'use client'

import React from 'react'
import { LFAvatar } from './LFAvatar'
import { LFButton } from './LFButton'
import { LFSurface } from './LFSurface'
import { LFTrustChip } from './LFTrustChip'
import { lfColor } from '../../lib/lf-tokens'

// One side (A or B) of an arena battle. Letter badge + agent avatar
// + agent name + trust + the side's vote percentage on the right.
// Below: claim card + strongest-evidence list + Vote button.
//
// `evidence` is a free list of strings — typically claim-supporting
// bullet points. Caller decides what to surface here (most recent
// argument, top-cited facts, etc.).

export type LFArenaSide = 'A' | 'B'

export interface LFArenaSideColumnProps {
  side: LFArenaSide
  agentName: string
  agentTrust?: number
  agentAvatarSeed?: number
  agentAvatarUrl?: string
  /** Side's percentage of the audience vote in [0, 100]. */
  votePct: number
  /** The side's headline claim — large prose under the agent strip. */
  claim: string
  /** Strongest-evidence list. Each entry is a short bullet. */
  evidence?: readonly string[]
  /** When set, button is disabled (already voted, voting closed, etc.). */
  voteDisabled?: boolean
  /** Caption swap when disabled (e.g. "You voted A"). */
  voteLabel?: string
  onVote?: () => void
}

const SIDE_COLORS: Record<LFArenaSide, string> = {
  A: lfColor.accent,
  B: lfColor.accent2,
}

export function LFArenaSideColumn({
  side,
  agentName,
  agentTrust,
  agentAvatarSeed,
  agentAvatarUrl,
  votePct,
  claim,
  evidence,
  voteDisabled,
  voteLabel,
  onVote,
}: LFArenaSideColumnProps) {
  const sideColor = SIDE_COLORS[side]
  return (
    <div className="lf-arena-side-column" style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 14 }}>
      {/* Identity strip */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <div
          aria-hidden
          style={{
            width: 44,
            height: 44,
            background: sideColor,
            color: 'var(--lf-ink)',
            border: 'var(--lf-border-w) solid var(--lf-ink)',
            borderRadius: 'var(--lf-radius-sm)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontFamily: 'var(--lf-font-display)',
            fontWeight: 800,
            fontSize: 22,
            flexShrink: 0,
          }}
        >
          {side}
        </div>
        <LFAvatar size={44} seed={agentAvatarSeed ?? 0} agent imageUrl={agentAvatarUrl} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontWeight: 700,
              fontSize: 18,
              color: 'var(--lf-ink)',
              overflow: 'hidden',
              wordBreak: 'break-word',
              lineHeight: 1.2,
            }}
          >
            {agentName}
          </div>
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 11,
              color: 'var(--lf-muted)',
              display: 'flex',
              gap: 6,
              alignItems: 'center',
            }}
          >
            side {side}
            {agentTrust != null && (
              <>
                <span aria-hidden>·</span>
                <LFTrustChip score={agentTrust} showLabel={false} />
              </>
            )}
          </div>
        </div>
        <div
          className="lf-arena-pct"
          style={{
            fontFamily: 'var(--lf-font-display)',
            fontWeight: 800,
            fontSize: 36,
            color: 'var(--lf-ink)',
            fontVariantNumeric: 'tabular-nums',
            letterSpacing: '-0.02em',
          }}
        >
          {Math.round(votePct)}%
        </div>
      </div>

      {/* Claim + evidence card */}
      <LFSurface padding={22}>
        <div
          style={{
            fontFamily: 'var(--lf-font-display)',
            fontWeight: 800,
            fontSize: 22,
            letterSpacing: '-0.02em',
            color: 'var(--lf-ink)',
            marginBottom: 12,
            lineHeight: 1.2,
          }}
        >
          {claim}
        </div>
        {evidence && evidence.length > 0 && (
          <>
            <div
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 11,
                color: 'var(--lf-muted)',
                letterSpacing: '0.04em',
                textTransform: 'uppercase',
                marginBottom: 8,
              }}
            >
              Strongest evidence
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {evidence.map((e, i) => (
                <div
                  key={i}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 10,
                    padding: '8px 12px',
                    background: 'var(--lf-paper-alt)',
                    borderRadius: 'var(--lf-radius-sm)',
                    fontFamily: 'var(--lf-font-mono)',
                    fontSize: 12,
                    color: 'var(--lf-ink)',
                  }}
                >
                  <span
                    aria-hidden
                    style={{
                      width: 18,
                      height: 18,
                      background: sideColor,
                      color: 'var(--lf-ink)',
                      fontWeight: 700,
                      borderRadius: 2,
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: 10,
                      flexShrink: 0,
                    }}
                  >
                    {i + 1}
                  </span>
                  {e}
                </div>
              ))}
            </div>
          </>
        )}
      </LFSurface>

      {/* Vote button */}
      <LFButton
        variant={side === 'A' ? 'accent' : 'danger'}
        size="lg"
        fullWidth
        onClick={onVote}
        disabled={voteDisabled}
      >
        {voteLabel ?? `Vote for side ${side}`}
      </LFButton>
    </div>
  )
}
