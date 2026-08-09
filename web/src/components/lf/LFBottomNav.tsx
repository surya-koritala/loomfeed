'use client'

import React from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import {
  IconHome,
  IconSearch,
  IconCompose,
  IconFootball,
  IconHuman,
  type LFIconProps,
} from './icons'

// Mobile-only bottom tab bar. Renders fixed at the bottom of the
// viewport on screens <768px wide; on tablet/desktop it's hidden
// (the side nav covers navigation there).
//
// 5 tabs:
//   Home · Search · Compose (lime, prominent) · Sports · Profile
// (Sports took over the Arena slot for the World Cup 2026 run; Arena
// stays reachable via the mobile drawer + side nav.)

interface NavTab {
  label: string
  href: string
  Icon: React.FC<LFIconProps>
  /** Routes that should also light up this tab when active. */
  prefixes?: string[]
  /** When true, render with the lime-accent fill (used for Compose). */
  accent?: boolean
}

const TABS: readonly NavTab[] = [
  { label: 'Home',     href: '/',           Icon: IconHome,    prefixes: ['/feed', '/trending', '/discover', '/top'] },
  { label: 'Search',   href: '/search',     Icon: IconSearch },
  { label: 'Compose',  href: '/submit',     Icon: IconCompose, accent: true },
  { label: 'Sports',   href: '/sports',     Icon: IconFootball },
  { label: 'Profile',  href: '/u/me',       Icon: IconHuman,   prefixes: ['/profile/', '/u/'] },
]

function isActive(pathname: string, tab: NavTab): boolean {
  if (pathname === tab.href) return true
  if (tab.href !== '/' && pathname.startsWith(tab.href + '/')) return true
  return (tab.prefixes ?? []).some((p) => pathname === p || pathname.startsWith(p))
}

export function LFBottomNav() {
  const pathname = usePathname() ?? '/'

  return (
    <>
      {/* The nav itself — hidden via CSS at >=768px so we don't ship
          two nav surfaces at once on tablet/desktop. The fixed
          position + safe-area-inset-bottom handles iOS notch. */}
      <nav
        aria-label="Mobile primary"
        style={{
          position: 'fixed',
          bottom: 0,
          left: 0,
          right: 0,
          zIndex: 100,
          background: 'var(--lf-paper)',
          // Quiet hairline instead of the mono-era ink rule — the bar
          // structure, tabs, and lime compose button are unchanged.
          borderTop: '1px solid var(--lf-rule-mid)',
          padding: '8px 16px calc(8px + env(safe-area-inset-bottom)) 16px',
          display: 'none', // overridden via media query below
        }}
        className="lf-bottom-nav"
      >
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-around',
            alignItems: 'center',
            maxWidth: 600,
            margin: '0 auto',
          }}
        >
          {TABS.map((tab) => {
            const active = isActive(pathname, tab)
            const isAccent = tab.accent
            return (
              <Link
                key={tab.label}
                href={tab.href}
                aria-label={tab.label}
                aria-current={active ? 'page' : undefined}
                style={{
                  width: 44,
                  height: 44,
                  borderRadius: 22,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  textDecoration: 'none',
                  background: isAccent
                    ? 'var(--lf-accent)'
                    : active
                    ? 'var(--lf-ink)'
                    : 'transparent',
                  color: isAccent
                    ? 'var(--lf-ink)'
                    : active
                    ? 'var(--lf-paper)'
                    : 'var(--lf-muted)',
                  border: isAccent
                    ? '2px solid var(--lf-ink)'
                    : 'none',
                  boxShadow: isAccent ? 'var(--lf-shadow-hard-sm)' : 'none',
                  flexShrink: 0,
                }}
              >
                <tab.Icon size={22} strokeWidth={isAccent || active ? 2 : 1.75} />
              </Link>
            )
          })}
        </div>
      </nav>
      <style>{`
        @media (max-width: 767px) {
          .lf-bottom-nav { display: block !important; }
        }
        /* Pad the page so content isn't hidden behind the fixed nav. */
        @media (max-width: 767px) {
          body.lf-v2 { padding-bottom: calc(60px + env(safe-area-inset-bottom)); }
        }
      `}</style>
    </>
  )
}
