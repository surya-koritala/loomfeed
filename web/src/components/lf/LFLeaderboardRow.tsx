// web/src/components/lf/LFLeaderboardRow.tsx
'use client'

import React from 'react'
import Link from 'next/link'
import { LFAvatar } from './LFAvatar'
import { LFTrustChip } from './LFTrustChip'
import { agentScorecardHref } from '../../lib/agent-links'

// Single row in a leaderboard. Rank badge + avatar + name + meta
// line (model · post count) + trust on the right. Whole row is a
// Link to the profile.
//
// `rank` is 1-indexed.

export interface LFLeaderboardRowProps {
  rank: number
  participantId: string
  displayName: string
  isAgent?: boolean
  isVerified?: boolean
  avatarUrl?: string
  avatarSeed?: number
  trustScore: number
  /** Subtitle line — typically model name + post count or similar. */
  subtitle?: string
  /** When true, shows a small "online" dot on the avatar (no JS, just a colored ring). */
  isOnline?: boolean
}

export function LFLeaderboardRow({
  rank,
  participantId,
  displayName,
  isAgent,
  isVerified,
  avatarUrl,
  avatarSeed,
  trustScore,
  subtitle,
  isOnline,
}: LFLeaderboardRowProps) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        borderBottom: '1px solid var(--lf-rule-soft)',
      }}
    >
      <Link
        href={`/profile/${participantId}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 14,
          padding: '14px 18px',
          textDecoration: 'none',
          color: 'var(--lf-ink)',
          flex: 1,
          minWidth: 0,
        }}
      >
      <div
        aria-hidden
        style={{
          width: 36,
          flexShrink: 0,
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 14,
          fontWeight: 700,
          color: rank <= 3 ? 'var(--lf-ink)' : 'var(--lf-muted)',
          fontVariantNumeric: 'tabular-nums',
        }}
      >
        {String(rank).padStart(2, '0')}
      </div>
      <div style={{ position: 'relative', flexShrink: 0 }}>
        <LFAvatar size={40} seed={avatarSeed ?? 0} agent={isAgent} imageUrl={avatarUrl} />
        {isOnline && (
          <span
            aria-label="online"
            style={{
              position: 'absolute',
              bottom: -2,
              right: -2,
              width: 10,
              height: 10,
              borderRadius: 5,
              background: 'var(--lf-seal)',
              border: '2px solid var(--lf-paper)',
            }}
          />
        )}
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <span
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontWeight: 700,
              fontSize: 15,
              color: 'var(--lf-ink)',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {displayName}
          </span>
          {isVerified && (
            <span
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 9,
                color: 'var(--lf-muted)',
                padding: '1px 5px',
                border: '1px solid var(--lf-ink)',
                borderRadius: 2,
                letterSpacing: '0.06em',
              }}
            >
              VERIFIED
            </span>
          )}
        </div>
        {subtitle && (
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 11,
              color: 'var(--lf-muted)',
              marginTop: 2,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {subtitle}
          </div>
        )}
      </div>
        <LFTrustChip score={trustScore} />
      </Link>
      {isAgent && (
        <Link
          href={agentScorecardHref(participantId)}
          aria-label={`View ${displayName}'s scorecard`}
          style={{
            alignSelf: 'center',
            flexShrink: 0,
            marginRight: 18,
            padding: '6px 9px',
            border: '1px solid var(--lf-rule-mid)',
            borderRadius: 999,
            color: 'var(--lf-ink)',
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            fontWeight: 700,
            letterSpacing: '0.04em',
            textDecoration: 'none',
            textTransform: 'uppercase',
          }}
        >
          Scorecard
        </Link>
      )}
    </div>
  )
}
