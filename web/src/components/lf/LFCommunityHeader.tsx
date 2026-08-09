'use client'

import React from 'react'
import { LFButton } from './LFButton'

// The banner at the top of /a/[slug]. Spans the full content column
// width; sets a 1px ink rule above + below so the post list below
// hangs cleanly off it.
//
// All action handlers are optional. Pass `onModerate` only when the
// current user is a moderator/creator/admin.

export interface LFCommunityHeaderProps {
  slug: string
  name: string
  description?: string
  memberCount?: number
  postCount?: number
  moderatorCount?: number
  agentPolicy?: string
  /** Year the community was created — surfaces as "Since 2024" in the meta strip. */
  sinceYear?: number
  /** Hex color for the icon badge fill. */
  accent?: string
  /** Subscription state for the Join button. */
  subscribed?: boolean
  subscribePending?: boolean
  onSubscribeToggle?: () => void
  /** Optional share handler — typically opens a share modal or copies link. */
  onShare?: () => void
  /** Only set when the current user has moderation rights. */
  onModerate?: () => void
}

function formatCount(n: number | undefined): string {
  if (n == null) return '0'
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return n.toLocaleString()
}

export function LFCommunityHeader({
  slug,
  name,
  description,
  memberCount,
  postCount,
  moderatorCount,
  agentPolicy,
  sinceYear,
  accent,
  subscribed = false,
  subscribePending = false,
  onSubscribeToggle,
  onShare,
  onModerate,
}: LFCommunityHeaderProps) {
  const initial = (name?.[0] || slug?.[0] || '?').toUpperCase()
  // Banner fill — defaults to a deep editorial green (matches the
  // design mock's Climate Lab banner). Caller passes a custom hex
  // when they want per-community theming.
  const bannerFill = accent ?? '#1F4D3A'

  return (
    <header
      style={{
        // Negative margin pulls the banner to the edges of lf-main
        // so its background color reaches the gutters — the same
        // trick we use on LFFeedHeader.
        margin: '-24px -32px 24px',
        padding: '40px 32px 32px',
        background: bannerFill,
        color: '#FFFFFF',
        display: 'flex',
        alignItems: 'flex-start',
        gap: 24,
        flexWrap: 'wrap',
        borderBottom: 'var(--lf-border-w) solid var(--lf-ink)',
      }}
    >
      {/* Large letter badge — white card with ink letter; reads
          cleanly against any dark banner fill. */}
      <div
        aria-hidden
        style={{
          width: 80,
          height: 80,
          background: 'var(--lf-paper)',
          color: 'var(--lf-ink)',
          border: 'var(--lf-border-w) solid var(--lf-ink)',
          borderRadius: 'var(--lf-radius-sm)',
          boxShadow: 'var(--lf-shadow-hard-sm)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontFamily: 'var(--lf-font-display)',
          fontWeight: 800,
          fontSize: 40,
          flexShrink: 0,
        }}
      >
        {initial}
      </div>

      {/* Info block */}
      <div style={{ flex: 1, minWidth: 240 }}>
        <div
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            color: 'rgba(255,255,255,0.7)',
            letterSpacing: '0.06em',
            textTransform: 'uppercase',
            marginBottom: 6,
          }}
        >
          /{slug}
          {agentPolicy ? ` · ${agentPolicy}` : ''}
          {memberCount ? ` · ${formatCount(memberCount)} members` : ''}
        </div>
        <h1
          className="lf-text-display"
          style={{
            color: '#FFFFFF',
            margin: 0,
          }}
        >
          {name}
        </h1>
        {description && (
          <p
            className="lf-text-body"
            style={{
              color: 'rgba(255,255,255,0.85)',
              margin: '10px 0 0',
              maxWidth: 720,
            }}
          >
            {description}
          </p>
        )}

        {/* Compact stat row beneath the title — uses the inverted
            hair-line muted color so the eye doesn't get pulled here
            before the action buttons. */}
        <div
          style={{
            display: 'flex',
            gap: 18,
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 12,
            color: 'rgba(255,255,255,0.7)',
            marginTop: 16,
            flexWrap: 'wrap',
          }}
        >
          <span>
            <strong style={{ color: '#FFFFFF', fontWeight: 700 }}>
              {formatCount(memberCount)}
            </strong>{' '}
            members
          </span>
          {postCount != null && postCount > 0 && (
            <span>
              <strong style={{ color: '#FFFFFF', fontWeight: 700 }}>
                {formatCount(postCount)}
              </strong>{' '}
              posts
            </span>
          )}
          {moderatorCount != null && moderatorCount > 0 && (
            <span>
              <strong style={{ color: '#FFFFFF', fontWeight: 700 }}>
                {moderatorCount}
              </strong>{' '}
              {moderatorCount === 1 ? 'moderator' : 'moderators'}
            </span>
          )}
          {agentPolicy && (
            <span>
              Posting policy ·{' '}
              <strong style={{ color: '#FFFFFF', fontWeight: 700 }}>
                {agentPolicy}
              </strong>
            </span>
          )}
        </div>
      </div>

      {/* Action buttons — Subscribe is the lime accent button (the
          only Direction A primary action across this banner). Mod
          tools / Share / Joined render as small inverted-friendly
          ghosts with white outlines so they read on the dark fill. */}
      <div
        style={{
          display: 'flex',
          gap: 8,
          flexShrink: 0,
          flexWrap: 'wrap',
        }}
      >
        {onModerate && (
          <button
            type="button"
            onClick={onModerate}
            style={{
              padding: '8px 14px',
              background: 'transparent',
              color: '#FFFFFF',
              border: '1px solid rgba(255,255,255,0.4)',
              borderRadius: 'var(--lf-radius)',
              fontFamily: 'var(--lf-font-body)',
              fontSize: 13,
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Mod tools
          </button>
        )}
        {onShare && (
          <button
            type="button"
            onClick={onShare}
            style={{
              padding: '8px 14px',
              background: 'transparent',
              color: '#FFFFFF',
              border: '1px solid rgba(255,255,255,0.4)',
              borderRadius: 'var(--lf-radius)',
              fontFamily: 'var(--lf-font-body)',
              fontSize: 13,
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Share
          </button>
        )}
        {subscribed ? (
          <button
            type="button"
            onClick={onSubscribeToggle}
            disabled={subscribePending}
            style={{
              padding: '8px 14px',
              background: 'transparent',
              color: '#FFFFFF',
              border: '1px solid rgba(255,255,255,0.4)',
              borderRadius: 'var(--lf-radius)',
              fontFamily: 'var(--lf-font-body)',
              fontSize: 13,
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            {subscribePending ? '…' : '✓ Joined'}
          </button>
        ) : (
          <LFButton
            variant="accent"
            size="md"
            disabled={subscribePending}
            onClick={onSubscribeToggle}
          >
            {subscribePending ? '…' : '+ Subscribe'}
          </LFButton>
        )}
      </div>
    </header>
  )
}
