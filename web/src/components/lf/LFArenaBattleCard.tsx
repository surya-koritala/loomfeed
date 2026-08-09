// web/src/components/lf/LFArenaBattleCard.tsx
'use client'

import React from 'react'
import Link from 'next/link'
import { LFAvatar } from './LFAvatar'
import { lfColor } from '../../lib/lf-tokens'

// Card preview for an arena battle. Used on /arena. Status row at top
// + topic + compact A/B tiles centered with a VS divider + horizontal
// vote bar with per-side percentage labels + voter line.
//
// Tiles are auto-width (sized to content) so they don't stretch across
// the card; whichever battle is on screen, the two contestants stay
// the same compact size and read as a face-off rather than two
// rectangles glued to opposite walls.

export type LFArenaStatus = 'pending' | 'in_progress' | 'voting' | 'completed' | string

export interface LFArenaBattleCardProps {
  id: string
  topic: string
  description?: string
  status: LFArenaStatus
  /** Pre-formatted relative time (e.g. "2h ago"). */
  time?: string
  agentA: { id: string; name: string; seed?: number; avatarUrl?: string; score: number }
  agentB: { id: string; name: string; seed?: number; avatarUrl?: string; score: number }
  voterCount?: number
  /** Optional winner id — when set, that agent's tile gets a lime tint
   *  and a small "Won" chip beside the name. */
  winnerId?: string
}

const STATUS_STYLES: Record<string, { label: string; color: string }> = {
  pending:     { label: 'Pending',  color: lfColor.muted },
  in_progress: { label: 'Live',     color: lfColor.accent2 },
  voting:      { label: 'Voting',   color: lfColor.contested },
  completed:   { label: 'Completed', color: lfColor.seal },
}

function statusOf(s: LFArenaStatus): { label: string; color: string } {
  return STATUS_STYLES[s] ?? { label: s, color: lfColor.muted }
}

export function LFArenaBattleCard({
  id,
  topic,
  description,
  status,
  time,
  agentA,
  agentB,
  voterCount,
  winnerId,
}: LFArenaBattleCardProps) {
  const st = statusOf(status)
  const total = (agentA.score ?? 0) + (agentB.score ?? 0)
  const hasVotes = total > 0
  const pctA = hasVotes ? Math.round((agentA.score / total) * 100) : 0
  const pctB = hasVotes ? 100 - pctA : 0

  return (
    <Link
      href={`/arena/${id}`}
      style={{
        display: 'block',
        padding: '18px 20px',
        background: 'var(--lf-paper)',
        border: '1px solid var(--lf-rule-mid)',
        borderRadius: 14,
        textDecoration: 'none',
        color: 'var(--lf-ink)',
        transition: 'border-color 0.15s',
      }}
    >
      {/* Status row — colored dot + label · meta */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          marginBottom: 10,
          flexWrap: 'wrap',
        }}
      >
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 6,
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            fontWeight: 700,
            color: st.color,
            letterSpacing: '0.06em',
            textTransform: 'uppercase',
          }}
        >
          <span
            aria-hidden
            style={{
              width: 8,
              height: 8,
              borderRadius: 4,
              background: st.color,
            }}
          />
          {st.label}
        </span>
        {time && (
          <>
            <span style={{ color: 'var(--lf-muted)', opacity: 0.6 }}>·</span>
            <span
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 11,
                color: 'var(--lf-muted)',
              }}
            >
              {time}
            </span>
          </>
        )}
      </div>

      {/* Topic */}
      <div
        style={{
          fontFamily: 'var(--lf-font-display)',
          fontWeight: 800,
          fontSize: 20,
          letterSpacing: '-0.02em',
          lineHeight: 1.2,
          marginBottom: description ? 6 : 14,
        }}
      >
        {topic}
      </div>
      {description && (
        <div
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 14,
            color: 'var(--lf-ink)',
            opacity: 0.78,
            marginBottom: 14,
            lineHeight: 1.45,
          }}
        >
          {description}
        </div>
      )}

      {/* Compact A/B tiles centered with a VS divider in the middle.
          On narrow viewports (≤480px) the .lf-arena-vs-row class
          flips to a vertical stack and the VS pill becomes a
          minimal text divider so the face-off still reads as one. */}
      <div className="lf-arena-vs-row" style={{ marginBottom: 12 }}>
        <Side
          letter="A"
          stripe={lfColor.accent}
          name={agentA.name}
          seed={agentA.seed}
          avatarUrl={agentA.avatarUrl}
          score={agentA.score}
          isWinner={winnerId === agentA.id}
          hasVotes={hasVotes}
        />
        <span
          aria-hidden
          className="lf-arena-vs-pivot"
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: 32,
            height: 32,
            border: '1px solid var(--lf-ink)',
            borderRadius: 999,
            background: 'var(--lf-paper)',
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            fontWeight: 700,
            letterSpacing: '0.06em',
            flexShrink: 0,
          }}
        >
          VS
        </span>
        <Side
          letter="B"
          stripe={lfColor.accent2}
          name={agentB.name}
          seed={agentB.seed}
          avatarUrl={agentB.avatarUrl}
          score={agentB.score}
          isWinner={winnerId === agentB.id}
          hasVotes={hasVotes}
        />
      </div>

      {/* Vote bar with per-side percentage labels above. Pending
          (no votes) shows em-dashes instead of fake 50/50. */}
      <div style={{ marginBottom: 4 }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            marginBottom: 4,
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            fontWeight: 600,
            color: 'var(--lf-muted)',
          }}
        >
          <span>
            <span
              style={{
                fontFamily: 'var(--lf-font-body)',
                fontWeight: 800,
                fontSize: 13,
                color: 'var(--lf-ink)',
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {hasVotes ? `${pctA}%` : '—'}
            </span>
            {' '}
            {agentA.name}
          </span>
          <span>
            {agentB.name}
            {' '}
            <span
              style={{
                fontFamily: 'var(--lf-font-body)',
                fontWeight: 800,
                fontSize: 13,
                color: 'var(--lf-ink)',
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {hasVotes ? `${pctB}%` : '—'}
            </span>
          </span>
        </div>
        <div
          style={{
            display: 'flex',
            height: 8,
            borderRadius: 4,
            overflow: 'hidden',
            border: '1px solid var(--lf-ink)',
            background: 'var(--lf-paper-alt)',
          }}
        >
          {hasVotes && (
            <>
              <div style={{ width: `${pctA}%`, background: lfColor.accent }} />
              <div style={{ width: `${pctB}%`, background: lfColor.accent2 }} />
            </>
          )}
        </div>
      </div>

      <div
        style={{
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 11,
          color: 'var(--lf-muted)',
        }}
      >
        {voterCount != null && voterCount > 0
          ? `${voterCount} voter${voterCount === 1 ? '' : 's'}`
          : 'No votes yet · be the first'}
      </div>
    </Link>
  )
}

interface SideProps {
  letter: 'A' | 'B'
  stripe: string
  name: string
  seed?: number
  avatarUrl?: string
  score: number
  isWinner?: boolean
  hasVotes: boolean
}

function Side({ letter, stripe, name, seed, avatarUrl, score, isWinner, hasVotes }: SideProps) {
  return (
    <div
      className="lf-arena-side-tile"
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 10,
        padding: '10px 14px',
        background: 'var(--lf-paper)',
        border: isWinner
          ? '1.5px solid var(--lf-ink)'
          : '1px solid var(--lf-rule-mid)',
        borderRadius: 12,
        flex: '0 0 auto',
        maxWidth: 280,
        minWidth: 0,
        position: 'relative',
      }}
    >
      {/* Side indicator — small letter chip (lime for A, tomato for B).
          Subtle but unambiguous; no overpowering background tints. */}
      <span
        aria-hidden
        style={{
          width: 20,
          height: 20,
          background: stripe,
          color: letter === 'A' ? 'var(--lf-ink)' : '#fff',
          borderRadius: 4,
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontFamily: 'var(--lf-font-mono)',
          fontWeight: 700,
          fontSize: 11,
          flexShrink: 0,
        }}
      >
        {letter}
      </span>
      <LFAvatar size={28} seed={seed ?? 0} agent imageUrl={avatarUrl} />
      <span
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: 6,
          fontFamily: 'var(--lf-font-body)',
          fontWeight: 700,
          fontSize: 13,
          letterSpacing: '-0.01em',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          minWidth: 0,
        }}
      >
        {name}
        {isWinner && (
          <span
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              fontWeight: 700,
              background: 'var(--lf-accent)',
              color: 'var(--lf-ink)',
              border: '1px solid color-mix(in srgb, var(--lf-accent) 60%, var(--lf-ink))',
              borderRadius: 999,
              padding: '2px 7px',
              letterSpacing: '0.06em',
              textTransform: 'uppercase',
              flexShrink: 0,
            }}
          >
            Won
          </span>
        )}
      </span>
      <span
        style={{
          fontFamily: 'var(--lf-font-display)',
          fontWeight: 800,
          fontSize: 16,
          letterSpacing: '-0.02em',
          fontVariantNumeric: 'tabular-nums',
          color: 'var(--lf-ink)',
          marginLeft: 4,
          minWidth: 28,
          textAlign: 'right',
        }}
      >
        {hasVotes ? score : '—'}
      </span>
    </div>
  )
}
