'use client'

import React from 'react'
import Link from 'next/link'
import { LFSurface } from './LFSurface'
import { LFButton } from './LFButton'

// Community card shown in the /communities directory grid. Click
// anywhere on the title or icon to navigate to /a/[slug]; the
// "Subscribe" button has its own handler so the click doesn't
// propagate to the parent navigation.
//
// `accent` is the colored fill on the letter-icon badge. Caller
// computes it (e.g. from a slug → palette lookup) and passes it in.
// We don't bake a hash function in; the design's intent is that
// each community has a distinct, manually-chosen accent.

export interface LFCommunityCardCommunity {
  slug: string
  name: string
  description?: string
  memberCount?: number
  agentPolicy?: string
}

export interface LFCommunityCardProps {
  community: LFCommunityCardCommunity
  /** Optional secondary stat ("12 agents" etc.). */
  agentCount?: number
  /** Optional moderator handle (e.g. "naomi.k"). */
  moderator?: string
  /** Hex color used for the letter-icon badge fill. */
  accent?: string
  /** True if the current user has already subscribed. */
  subscribed?: boolean
  /** Subscribe / unsubscribe button is disabled while pending. */
  subscribePending?: boolean
  /** Fires when the user clicks Subscribe / Joined. */
  onSubscribeToggle?: () => void
}

function formatCount(n: number | undefined): string {
  if (n == null) return '—'
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return String(n)
}

export function LFCommunityCard({
  community,
  agentCount,
  moderator,
  accent,
  subscribed = false,
  subscribePending = false,
  onSubscribeToggle,
}: LFCommunityCardProps) {
  const initial = (community.name?.[0] || community.slug?.[0] || '?').toUpperCase()
  const accentColor = accent ?? 'var(--lf-accent-3)' // iris fallback

  return (
    <LFSurface padding={22} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      {/* Header — icon + name + slug. The whole header is a link. */}
      <Link
        href={`/a/${community.slug}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 12,
          textDecoration: 'none',
          color: 'inherit',
        }}
      >
        <div
          aria-hidden
          style={{
            width: 44,
            height: 44,
            background: accentColor,
            color: 'var(--lf-ink)',
            border: 'var(--lf-border-w) solid var(--lf-ink)',
            borderRadius: 'var(--lf-radius-sm)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontFamily: 'var(--lf-font-display)',
            fontWeight: 800,
            fontSize: 18,
            flexShrink: 0,
          }}
        >
          {initial}
        </div>
        <div style={{ minWidth: 0 }}>
          <div
            style={{
              fontFamily: 'var(--lf-font-display)',
              fontWeight: 800,
              fontSize: 22,
              letterSpacing: '-0.02em',
              color: 'var(--lf-ink)',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {community.name}
          </div>
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 11,
              color: 'var(--lf-muted)',
            }}
          >
            /{community.slug}
          </div>
        </div>
      </Link>

      {/* Description */}
      {community.description && (
        <div
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 13,
            color: 'var(--lf-ink)',
            opacity: 0.78,
            lineHeight: 1.45,
          }}
        >
          {community.description}
        </div>
      )}

      {/* Stats row */}
      <div
        style={{
          display: 'flex',
          gap: 14,
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 11,
          color: 'var(--lf-muted)',
          marginTop: 4,
          flexWrap: 'wrap',
        }}
      >
        <span>{formatCount(community.memberCount)} members</span>
        {agentCount != null && <span>{agentCount} agents</span>}
        {moderator && (
          <span style={{ marginLeft: 'auto' }}>mod @{moderator}</span>
        )}
      </div>

      {/* Subscribe button */}
      <LFButton
        variant={subscribed ? 'ghost' : 'accent'}
        size="sm"
        fullWidth
        disabled={subscribePending}
        onClick={(e) => {
          // Prevent the parent <Link>-as-card navigation when the
          // user clicks Subscribe. The header Link is a separate
          // element, but defensive in case the card itself becomes
          // clickable in a future version.
          e.preventDefault()
          e.stopPropagation()
          onSubscribeToggle?.()
        }}
        style={{ justifyContent: 'center', marginTop: 4 }}
      >
        {subscribePending ? '…' : subscribed ? '✓ Subscribed' : 'Subscribe'}
      </LFButton>
    </LFSurface>
  )
}
