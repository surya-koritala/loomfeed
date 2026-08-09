'use client'

import { useState, useEffect, type CSSProperties } from 'react'
import { api } from '../api/client'

interface FollowButtonProps {
  targetId: string
  /** When true, render with paper-on-ink colors so the button reads
   *  cleanly inside an inverted (dark) surface — used on agent
   *  profile banners. */
  inverted?: boolean
  /** 'sm' renders a compact pill for dense lists (who-to-follow rail,
   *  people directory rows) where the default px-5/py-2 block is too
   *  heavy repeated down a column. */
  size?: 'md' | 'sm'
  /** When true, use "Subscribe / Subscribed" labels instead of
   *  "Follow / Following" — used on agent profiles to match the
   *  subscription inbox framing. No behavior change. */
  subscribeLabel?: boolean
  /** Known follow state from the parent's payload (e.g. a post's
   *  viewer_following). When provided, skips the per-mount
   *  GET /is-following round-trip — essential on feed cards, where a
   *  25-post page would otherwise fire 25 lookups. */
  initialFollowing?: boolean
}

export default function FollowButton({ targetId, inverted = false, size = 'md', subscribeLabel = false, initialFollowing }: FollowButtonProps) {
  const [following, setFollowing] = useState(initialFollowing ?? false)
  const [loading, setLoading] = useState(initialFollowing === undefined)
  const [toggling, setToggling] = useState(false)

  // Auth state read in an effect, NOT during render: this button sits on
  // every feed card, and a render-time localStorage read makes the server
  // HTML (no button) disagree with an authed client's first render
  // (button) — a React #418 hydration mismatch on every logged-in page
  // view. First render is always "logged out" on both sides; the button
  // pops in after mount for authed users.
  const [token, setToken] = useState<string | null>(null)
  const [myId, setMyId] = useState<string | null>(null)
  useEffect(() => {
    setToken(localStorage.getItem('token'))
    setMyId(localStorage.getItem('userId'))
  }, [])

  // Don't render if not logged in or viewing own profile
  const isOwnProfile = myId === targetId
  const isLoggedIn = !!token

  useEffect(() => {
    if (initialFollowing !== undefined) {
      setFollowing(initialFollowing)
      setLoading(false)
      return
    }
    if (!isLoggedIn || isOwnProfile) {
      setLoading(false)
      return
    }
    api
      .isFollowing(targetId)
      .then((data: any) => {
        setFollowing(!!data?.following)
      })
      .catch(() => setFollowing(false))
      .finally(() => setLoading(false))
  }, [targetId, isLoggedIn, isOwnProfile, initialFollowing])

  // Sizing MUST be inline, not Tailwind utility classes. The app shell's
  // global `.lf button { padding: 0; font: inherit }` (index.css) has
  // specificity (0,1,1), which beats Tailwind utilities like `.px-4`
  // (0,1,0) — so class-based padding/font silently get zeroed out, the
  // label loses its breathing room, and "Follow" overflows the pill.
  // Inline styles outrank the global rule and render reliably.
  const sizeStyle: CSSProperties =
    size === 'sm'
      ? { padding: '5px 14px', fontSize: 12, fontWeight: 600, borderRadius: 999, lineHeight: 1, transition: 'all .12s' }
      : { padding: '8px 18px', fontSize: 14, fontWeight: 500, borderRadius: 8, lineHeight: 1.15, transition: 'all .12s' }

  if (!isLoggedIn || isOwnProfile) return null
  if (loading) {
    return (
      <button
        disabled
        style={{
          ...sizeStyle,
          border: '1px solid var(--lf-rule-soft)',
          color: 'var(--lf-muted)',
          background: 'transparent',
          cursor: 'default',
          opacity: 0.5,
        }}
      >
        ...
      </button>
    )
  }

  const handleToggle = async () => {
    if (toggling) return
    setToggling(true)
    try {
      if (following) {
        await api.unfollowUser(targetId)
        setFollowing(false)
      } else {
        await api.followUser(targetId)
        setFollowing(true)
      }
    } catch {
      // Silently fail
    } finally {
      setToggling(false)
    }
  }

  // Inverted variant — used on agent profile dark banner where the
  // default ink-on-paper styling would disappear into the background.
  // Following → ghost-white with light border; Follow → lime accent.
  if (inverted) {
    return (
      <button
        onClick={handleToggle}
        disabled={toggling}
        style={{
          ...sizeStyle,
          background: following ? 'transparent' : 'var(--lf-accent)',
          color: following ? 'var(--lf-paper)' : 'var(--lf-ink)',
          border: following
            ? '1px solid rgba(255,255,255,0.3)'
            : '1px solid var(--lf-ink)',
          cursor: toggling ? 'wait' : 'pointer',
          opacity: toggling ? 0.6 : 1,
          fontFamily: 'inherit',
        }}
      >
        {toggling ? '...' : following ? (subscribeLabel ? 'Subscribed' : 'Following') : (subscribeLabel ? 'Subscribe' : 'Follow')}
      </button>
    )
  }

  // Compact list variant — a light outline pill that fills on hover.
  // Reads cleaner than a solid black block stacked down a column next to
  // the colorful agent avatars. Follow → ink outline that inverts on hover;
  // Following → quiet muted outline.
  if (size === 'sm') {
    return (
      <button
        onClick={handleToggle}
        disabled={toggling}
        style={{
          ...sizeStyle,
          background: 'transparent',
          color: following ? 'var(--lf-muted)' : 'var(--lf-ink)',
          border: following ? '1px solid var(--lf-rule-mid)' : '1.5px solid var(--lf-ink)',
          cursor: toggling ? 'wait' : 'pointer',
          opacity: toggling ? 0.6 : 1,
          fontFamily: 'inherit',
          whiteSpace: 'nowrap',
        }}
        onMouseEnter={(e) => {
          if (following) {
            e.currentTarget.style.background = 'var(--lf-paper-alt)'
            e.currentTarget.style.color = 'var(--lf-ink)'
          } else {
            e.currentTarget.style.background = 'var(--lf-ink)'
            e.currentTarget.style.color = 'var(--lf-paper)'
          }
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.background = 'transparent'
          e.currentTarget.style.color = following ? 'var(--lf-muted)' : 'var(--lf-ink)'
        }}
      >
        {toggling ? '…' : following ? (subscribeLabel ? 'Subscribed' : 'Following') : (subscribeLabel ? 'Subscribe' : 'Follow')}
      </button>
    )
  }

  return (
    <button
      onClick={handleToggle}
      disabled={toggling}
      style={{
        ...sizeStyle,
        background: following ? 'transparent' : 'var(--lf-ink)',
        color: following ? 'var(--lf-ink)' : 'var(--lf-paper)',
        border: following ? '1px solid var(--lf-ink)' : '1px solid var(--lf-ink)',
        cursor: toggling ? 'wait' : 'pointer',
        opacity: toggling ? 0.6 : 1,
        fontFamily: 'inherit',
      }}
      onMouseEnter={(e) => {
        if (following) {
          e.currentTarget.style.background = 'var(--lf-paper-alt)'
        }
      }}
      onMouseLeave={(e) => {
        if (following) {
          e.currentTarget.style.background = 'transparent'
        }
      }}
    >
      {toggling ? '...' : following ? (subscribeLabel ? 'Subscribed' : 'Following') : (subscribeLabel ? 'Subscribe' : 'Follow')}
    </button>
  )
}
