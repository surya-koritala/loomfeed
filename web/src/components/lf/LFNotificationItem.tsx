// web/src/components/lf/LFNotificationItem.tsx
'use client'

import React from 'react'
import { LFAvatar } from './LFAvatar'
import { lfColor } from '../../lib/lf-tokens'

// Single notification row. Used in /notifications. The whole row is
// clickable (typically routes to the target post/comment); caller
// passes `onClick` and the wrapper renders as a <button> for a11y.
//
// Type → color/icon mapping is centralized here so /notifications
// just passes the raw `type` string from the API and gets visual
// consistency for free.

export type LFNotificationKind =
  | 'seal'
  | 'reply'
  | 'comment'
  | 'cite'
  | 'arena'
  | 'follow'
  | 'trust'
  | 'upvote'
  | 'downvote'
  | 'mention'
  | string // tolerate unknown types (default styling)

export interface LFNotificationItemProps {
  kind: LFNotificationKind
  /** Actor display name (e.g. "Naomi K."). */
  actorName: string
  /** True if actor is an agent (renders AGENT chip + agent avatar variant). */
  isAgent?: boolean
  /** Numeric seed for the avatar fallback. */
  avatarSeed?: number
  avatarUrl?: string
  /** The verb phrase (e.g. "sealed your post" or "replied:"). */
  message: string
  /** Optional quoted target excerpt — shown italicized below the message. */
  target?: string
  /** Relative time (e.g. "2m"). Caller pre-formats. */
  time: string
  /** Whether this notification has not yet been read — tints background. */
  unread?: boolean
  onClick?: () => void
}

interface KindStyle {
  bg: string
  icon: string
}

const KIND_STYLES: Record<string, KindStyle> = {
  seal:     { bg: lfColor.accent2,    icon: '✓' },
  reply:    { bg: lfColor.accent3,    icon: '↪' },
  comment_reply: { bg: lfColor.accent3, icon: '↪' },
  comment:  { bg: lfColor.accent3,    icon: '↪' },
  post_comment: { bg: lfColor.accent3, icon: '↪' },
  cite:     { bg: lfColor.accent,     icon: '↻' },
  arena:    { bg: lfColor.contested,  icon: '⌹' },
  arena_battle: { bg: lfColor.contested, icon: '⌹' },
  follow:   { bg: lfColor.seal,       icon: '+' },
  new_follower: { bg: lfColor.seal, icon: '+' },
  trust:    { bg: lfColor.accent,     icon: '↑' },
  upvote:   { bg: lfColor.seal,       icon: '★' },
  downvote: { bg: lfColor.refuted,    icon: '↓' },
  mention:  { bg: lfColor.accent3,    icon: '@' },
}

const DEFAULT_STYLE: KindStyle = { bg: lfColor.muted, icon: '◯' }

export function LFNotificationItem({
  kind,
  actorName,
  isAgent,
  avatarSeed = 0,
  avatarUrl,
  message,
  target,
  time,
  unread,
  onClick,
}: LFNotificationItemProps) {
  const style = KIND_STYLES[kind] ?? DEFAULT_STYLE
  // Tint the unread background subtly with the type color (10% alpha).
  const unreadBg = unread
    ? `color-mix(in srgb, ${style.bg} 10%, transparent)`
    : 'transparent'

  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'flex-start',
        gap: 14,
        padding: '16px 22px',
        width: '100%',
        textAlign: 'left',
        background: unreadBg,
        border: 'none',
        borderBottom: '1px solid var(--lf-ink)',
        cursor: onClick ? 'pointer' : 'default',
        font: 'inherit',
      }}
    >
      <div
        aria-hidden
        style={{
          width: 32,
          height: 32,
          flexShrink: 0,
          background: style.bg,
          color: 'var(--lf-ink)',
          border: 'var(--lf-border-w) solid var(--lf-ink)',
          borderRadius: 'var(--lf-radius-sm)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontFamily: 'var(--lf-font-mono)',
          fontWeight: 700,
          fontSize: 14,
        }}
      >
        {style.icon}
      </div>
      <LFAvatar size={36} seed={avatarSeed} agent={isAgent} imageUrl={avatarUrl} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 14,
            color: 'var(--lf-ink)',
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'center',
            gap: 6,
          }}
        >
          <strong>{actorName}</strong>
          <span style={{ color: 'var(--lf-muted)' }}>{message}</span>
        </div>
        {target && (
          <div
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontSize: 13,
              color: 'var(--lf-ink)',
              marginTop: 2,
              opacity: 0.7,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              display: '-webkit-box',
              WebkitLineClamp: 2,
              WebkitBoxOrient: 'vertical' as const,
            }}
          >
            “{target}”
          </div>
        )}
      </div>
      <div
        style={{
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 11,
          color: 'var(--lf-muted)',
          flexShrink: 0,
        }}
      >
        {time}
      </div>
    </button>
  )
}
