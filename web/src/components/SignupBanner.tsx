'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'

// Soft conversion banner shown to anonymous visitors. The platform
// doesn't wall content (which would kill SEO, press cite-ability,
// and HN/Twitter click-throughs) — instead, this banner makes the
// next step obvious without blocking the read.
//
// Hides on:
//   - any /login, /register, /forgot-password page (avoid recursion)
//   - when user has an auth token in localStorage
//   - when user has dismissed it (localStorage preference, persistent)
//
// Mobile-first: stacks on narrow viewports per CLAUDE.md.

const DISMISSED_KEY = 'signup_banner_dismissed_v1'
const AUTH_PATH_PREFIXES = [
  '/login',
  '/register',
  '/forgot-password',
  '/reset-password',
  '/auth',
  '/oauth',
  '/verify',
]

export default function SignupBanner() {
  const pathname = usePathname() ?? '/'
  const [show, setShow] = useState(false)

  useEffect(() => {
    if (typeof window === 'undefined') return
    // Authed users never see it.
    if (localStorage.getItem('token')) return
    // Dismissed users never see it again (until localStorage cleared).
    if (localStorage.getItem(DISMISSED_KEY)) return
    setShow(true)
  }, [])

  // Hide on auth pages so we don't recurse into the signup flow.
  for (const prefix of AUTH_PATH_PREFIXES) {
    if (pathname === prefix || pathname.startsWith(prefix + '/')) return null
  }
  if (!show) return null

  const dismiss = () => {
    try {
      localStorage.setItem(DISMISSED_KEY, '1')
    } catch {}
    setShow(false)
  }

  return (
    <div
      role="region"
      aria-label="Sign up"
      // Single-row strip on every viewport — no flex-wrap, no
      // multi-line stacking that ate half the mobile fold. Inline
      // text with a compact CTA + dismiss "×" all on one line.
      // Lime accent border instead of solid-ink fill so it reads as
      // a nudge, not an ad takeover.
      style={{
        margin: '0 0 16px',
        padding: '8px 12px',
        background: 'var(--lf-paper)',
        color: 'var(--lf-ink)',
        borderRadius: 10,
        border: '1px solid var(--lf-ink)',
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        boxShadow: 'var(--lf-shadow-hard-xs, 2px 2px 0 var(--lf-ink))',
      }}
    >
      <span
        style={{
          font: '600 13px/1.3 var(--lf-font-body)',
          flex: '1 1 auto',
          minWidth: 0,
          // Truncate on very narrow viewports rather than wrap to
          // a second row — keeps the strip a fixed height so it
          // doesn't push hero/feed content off the fold.
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        Sign up to vote, comment, save.
      </span>
      <Link
        href="/register"
        style={{
          padding: '6px 12px',
          background: 'var(--lf-accent)',
          color: 'var(--lf-ink)',
          border: '1px solid var(--lf-ink)',
          borderRadius: 999,
          font: '700 12px var(--lf-font-body)',
          textDecoration: 'none',
          whiteSpace: 'nowrap',
          flexShrink: 0,
        }}
      >
        Sign up
      </Link>
      <button
        type="button"
        onClick={dismiss}
        aria-label="Dismiss"
        style={{
          width: 24,
          height: 24,
          borderRadius: 999,
          background: 'transparent',
          color: 'var(--lf-muted)',
          border: 0,
          cursor: 'pointer',
          font: '400 16px var(--lf-font-body)',
          lineHeight: 1,
          flexShrink: 0,
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        ×
      </button>
    </div>
  )
}
