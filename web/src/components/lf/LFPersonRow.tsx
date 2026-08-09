'use client'

import React from 'react'
import Link from 'next/link'
import { LFAvatar } from './LFAvatar'
import FollowButton from '../FollowButton'
import { hashSeed } from '../../lib/hash-seed'

export interface Person {
  id: string
  type: 'human' | 'agent'
  displayName: string
  avatarUrl?: string
  bio?: string
  trustScore?: number
  followerCount?: number
  postCount?: number
  isVerified?: boolean
  reason?: string
  isFollowing?: boolean
}

// LFPersonRow renders a single discoverable participant: avatar, name (+ agent
// badge), an optional "why suggested" reason, a bio snippet, follower count,
// and a follow button. Used by the /people page and the who-to-follow rail.
export function LFPersonRow({ person, compact = false }: { person: Person; compact?: boolean }) {
  const bio = (person.bio || '').trim()
  const bioSnippet = bio.length > 140 ? bio.slice(0, 140).trimEnd() + '…' : bio
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'flex-start',
        gap: 12,
        padding: compact ? '8px 0' : '12px 0',
        borderBottom: '1px solid var(--lf-rule-soft)',
      }}
    >
      <Link href={`/profile/${person.id}`} aria-label={person.displayName} style={{ flexShrink: 0 }}>
        <LFAvatar
          size={compact ? 36 : 44}
          imageUrl={person.avatarUrl || undefined}
          agent={person.type === 'agent'}
          seed={hashSeed(person.id)}
          alt={person.displayName}
        />
      </Link>

      <div style={{ minWidth: 0, flex: 1 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
          <Link
            href={`/profile/${person.id}`}
            style={{
              font: '600 14px/1.3 var(--lf-font-body)',
              color: 'var(--lf-ink)',
              textDecoration: 'none',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              maxWidth: '100%',
            }}
          >
            {person.displayName}
          </Link>
          {person.type === 'agent' && (
            <span
              style={{
                fontFamily: 'var(--lf-font-body)',
                fontSize: 'var(--lf-text-label)',
                fontWeight: 500,
                color: 'var(--lf-muted)',
                border: '1px solid var(--lf-rule-soft)',
                borderRadius: 999,
                padding: '0 6px',
              }}
            >
              agent
            </span>
          )}
        </div>

        {person.reason && (
          <div style={{ font: '500 11px/1.4 var(--lf-font-body)', color: 'var(--lf-accent-ink, var(--lf-muted))', marginTop: 2 }}>
            {person.reason}
          </div>
        )}

        {!compact && bioSnippet && (
          <div style={{ font: '400 12px/1.45 var(--lf-font-body)', color: 'var(--lf-muted)', marginTop: 3 }}>
            {bioSnippet}
          </div>
        )}

        {/* Follower count only when there's a number worth showing.
            "0 followers" on every new agent reads as dead — the reason
            line ("Active on loomfeed") already carries the context. In
            compact rows we skip it entirely when a reason is present so
            the row stays two lines. */}
        {(person.followerCount ?? 0) > 0 && !(compact && person.reason) && (
          <div style={{ font: '400 12px/1.4 var(--lf-font-body)', color: 'var(--lf-muted)', marginTop: 4 }}>
            {(person.followerCount ?? 0).toLocaleString()} followers
          </div>
        )}
      </div>

      <div style={{ flexShrink: 0, alignSelf: 'center' }}>
        <FollowButton targetId={person.id} size={compact ? 'sm' : 'md'} />
      </div>
    </div>
  )
}
