// web/src/components/lf/LFConversationListItem.tsx
'use client'

import React from 'react'
import { LFAvatar } from './LFAvatar'

// One row in the DM conversation list. Used in /messages (and in
// a future split-pane embedded list, when one exists).
//
// `lastMessage` is shown as a 2-line clamp; `time` is the relative
// time of the last message; `unread` toggles the tomato dot on the
// right + slightly tints the row background.

export interface LFConversationListItemProps {
  name: string
  isAgent?: boolean
  avatarSeed?: number
  avatarUrl?: string
  /** Color override for the avatar background (human variant). */
  avatarColor?: string
  lastMessage: string
  /** Pre-formatted relative time (e.g. "4m"). */
  time: string
  unread?: boolean
  active?: boolean
  onClick?: () => void
}

export function LFConversationListItem({
  name,
  isAgent,
  avatarSeed = 0,
  avatarUrl,
  avatarColor,
  lastMessage,
  time,
  unread,
  active,
  onClick,
}: LFConversationListItemProps) {
  const bg = active
    ? 'var(--lf-paper-alt)'
    : unread
    ? 'color-mix(in srgb, var(--lf-accent-2) 8%, transparent)'
    : 'transparent'

  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'flex-start',
        gap: 12,
        padding: '14px 18px',
        width: '100%',
        textAlign: 'left',
        background: bg,
        border: 'none',
        borderBottom: '1px solid var(--lf-ink)',
        cursor: onClick ? 'pointer' : 'default',
        font: 'inherit',
      }}
    >
      <LFAvatar
        size={36}
        seed={avatarSeed}
        agent={isAgent}
        imageUrl={avatarUrl}
        color={avatarColor}
      />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
          <strong
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontSize: 13,
              color: 'var(--lf-ink)',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            {name}
          </strong>
          <span
            style={{
              marginLeft: 'auto',
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              color: 'var(--lf-muted)',
              flexShrink: 0,
            }}
          >
            {time}
          </span>
        </div>
        <div
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 12,
            color: unread ? 'var(--lf-ink)' : 'var(--lf-muted)',
            marginTop: 3,
            lineHeight: 1.4,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            display: '-webkit-box',
            WebkitLineClamp: 2,
            WebkitBoxOrient: 'vertical' as const,
          }}
        >
          {lastMessage}
        </div>
      </div>
      {unread && (
        <div
          aria-label="Unread"
          style={{
            width: 7,
            height: 7,
            borderRadius: 4,
            background: 'var(--lf-accent-2)',
            marginTop: 6,
            flexShrink: 0,
          }}
        />
      )}
    </button>
  )
}
