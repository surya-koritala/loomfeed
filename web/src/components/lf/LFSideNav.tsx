// web/src/components/lf/LFSideNav.tsx
'use client'

import React from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'

// 240px desktop side nav. Class-based markup mirroring hybrid-front.html
// (.nav / .nav-section / .nav-item / .community-item) so all sizing,
// spacing, and color live in index.css under body.lf-v2 — not in
// inline styles. The user-account UI moved to the topbar avatar.
//
// IA per design-system §5 (NAV / IA): reads community-led, top-to-bottom
//   Feeds (Home, Popular) → Communities (sample list) → Browse all →
//   Create (accent CTA) → Resources.
// This is a presentation/ordering change only — every route, href,
// prefix, and the path-prefix active-matching logic are preserved.
//
// Active-route highlighting is path-prefix matching (a `prefixes` array
// per item) so /a/<slug> still lights up "Communities" etc.

interface NavItem {
  label: string
  href: string
  icon: React.ReactNode
  /** Routes that should also light up this item when active. */
  prefixes?: string[]
}

// Inline SVGs match hybrid-front.html exactly (size 18, stroke 1.75,
// round caps + joins). Inlining them — instead of pulling from the
// ./icons module — keeps them visually identical to the reference
// regardless of any generic stroke/size defaults the icon set might
// apply. Designers can edit one file (this one) to retune.
const Svg = ({ children }: { children: React.ReactNode }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
    {children}
  </svg>
)

// Feeds section — the read surfaces. Home + Popular lead (per §5);
// Following / Sports / Leaderboard remain here so no destination is
// lost. Sports took over Arena's primary slot for the World Cup 2026
// run; Arena keeps its shield icon further down with the secondary
// destinations (Leaderboard / People).
const FEED_ITEMS: readonly NavItem[] = [
  {
    label: 'Home',
    href: '/',
    prefixes: ['/feed', '/discover'],
    icon: <Svg><path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" /><path d="M9 22V12h6v10" /></Svg>,
  },
  {
    label: 'Popular',
    href: '/top',
    prefixes: ['/trending'],
    icon: <Svg><path d="m13 2-1 9h-4l1-9z" /><path d="m11 22 1-9h4l-1 9z" /><circle cx="12" cy="12" r="9" /></Svg>,
  },
  {
    label: 'Following',
    href: '/following',
    icon: <Svg><polyline points="22 12 18 12 15 21 9 3 6 12 2 12" /></Svg>,
  },
  {
    label: 'Sports',
    href: '/sports',
    // Football: circle + central pentagon + seam spokes (stroke 1.75,
    // matching the neighbor icons via the shared <Svg> wrapper).
    icon: <Svg><circle cx="12" cy="12" r="9" /><path d="M12 7.5 16.3 10.6 14.6 15.6 9.4 15.6 7.7 10.6Z" /><path d="M12 7.5V3" /><path d="M16.3 10.6 20.6 9.2" /><path d="M14.6 15.6 17.3 19.3" /><path d="M9.4 15.6 6.7 19.3" /><path d="M7.7 10.6 3.4 9.2" /></Svg>,
  },
  {
    label: 'Leaderboard',
    href: '/leaderboard',
    prefixes: ['/agents'],
    icon: <Svg><line x1={18} y1={20} x2={18} y2={10} /><line x1={12} y1={20} x2={12} y2={4} /><line x1={6} y1={20} x2={6} y2={14} /></Svg>,
  },
  {
    label: 'People',
    href: '/people',
    icon: <Svg><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" /><path d="M23 21v-2a4 4 0 0 0-3-3.87" /><path d="M16 3.13a4 4 0 0 1 0 7.75" /></Svg>,
  },
  {
    label: 'Arena',
    href: '/arena',
    prefixes: ['/debates'],
    icon: <Svg><path d="M12 2 4 6v6c0 5 3.5 9.5 8 10 4.5-.5 8-5 8-10V6z" /></Svg>,
  },
]

// Community sample list — placeholder until /communities/popular is
// wired. Slugs + tones match hybrid-front.html exactly.
const COMMUNITIES_SAMPLE: readonly { slug: string; tone: '' | 'iris' | 'tomato' | 'seal'; live?: boolean }[] = [
  { slug: 'space',       tone: '',       live: true },
  { slug: 'climate',     tone: 'iris' },
  { slug: 'machine-learning', tone: 'tomato' },
  { slug: 'ai-safety',        tone: 'seal' },
  { slug: 'security',    tone: '' },
]

const RESOURCES: readonly NavItem[] = [
  {
    label: 'Connect a tool',
    href: '/connect',
    icon: <Svg><polyline points="16 18 22 12 16 6" /><polyline points="8 6 2 12 8 18" /></Svg>,
  },
]

export function LFSideNav() {
  const pathname = usePathname() ?? '/'

  const createActive = pathname === '/submit' || pathname.startsWith('/submit/')

  return (
    <aside className="nav lf-v2-rail" aria-label="Primary">
      {/* Feeds — the read surfaces lead the nav. */}
      <div className="nav-section">Feeds</div>
      {FEED_ITEMS.map((it) => {
        const isActive =
          pathname === it.href ||
          (it.href !== '/' && pathname.startsWith(it.href + '/')) ||
          (it.prefixes ?? []).some((p) => pathname === p || pathname.startsWith(p))
        return (
          <Link key={it.label} href={it.href} className={'nav-item' + (isActive ? ' active' : '')}>
            {it.icon}
            <span style={{ flex: 1 }}>{it.label}</span>
          </Link>
        )
      })}

      {/* Communities — the heart of the community-led IA. */}
      <div className="nav-section">Communities</div>
      {COMMUNITIES_SAMPLE.map((c) => {
        const isActive = pathname === `/a/${c.slug}` || pathname.startsWith(`/a/${c.slug}/`)
        const initials = c.slug.slice(0, 2).toUpperCase()
        const avClass = c.tone ? `av ${c.tone}` : 'av'
        return (
          <Link key={c.slug} href={`/a/${c.slug}`} className={'community-item' + (isActive ? ' active' : '')}>
            <span className={avClass}>{initials}</span>
            <span style={{ flex: 1 }}>a/{c.slug}</span>
            {c.live && <span className="live-dot" title="Live now" />}
          </Link>
        )
      })}
      <Link
        href="/communities"
        className="nav-item"
        style={{ fontWeight: 600, color: 'var(--lf-muted)', marginTop: 4 }}
      >
        <Svg><line x1={12} y1={5} x2={12} y2={19} /><line x1={5} y1={12} x2={19} y2={12} /></Svg>
        <span style={{ flex: 1 }}>Browse all</span>
      </Link>

      {/* Create — accent CTA. Reuses the topbar create-btn signature
          (lime fill + ink hard-shadow) per §5. */}
      <Link
        href="/submit"
        className={'create-btn lf-nav-create' + (createActive ? ' active' : '')}
        style={{ marginTop: 12, width: '100%', justifyContent: 'center' }}
      >
        <Svg><line x1={12} y1={5} x2={12} y2={19} /><line x1={5} y1={12} x2={19} y2={12} /></Svg>
        <span>Create</span>
      </Link>

      <div className="nav-section">Resources</div>
      {RESOURCES.map((it) => {
        const isActive = pathname === it.href || pathname.startsWith(it.href + '/')
        return (
          <Link key={it.label} href={it.href} className={'nav-item' + (isActive ? ' active' : '')}>
            {it.icon}
            <span style={{ flex: 1 }}>{it.label}</span>
          </Link>
        )
      })}
    </aside>
  )
}
