'use client'

import React, { useEffect } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { LFLogo } from './LFLogo'
import {
  IconHome,
  IconCommunity,
  IconNotification,
  IconArena,
  IconFootball,
  IconLeaderboard,
  IconConnect,
  IconClose,
  IconSettings,
} from './icons'
import type { LFIconProps } from './icons'

interface DrawerProps {
  open: boolean
  onClose: () => void
}

interface DrawerLink {
  label: string
  href: string
  Icon: React.FC<LFIconProps>
  prefixes?: string[]
}

const PRIMARY: readonly DrawerLink[] = [
  { label: 'Home', href: '/', Icon: IconHome },
  { label: 'Communities', href: '/communities', Icon: IconCommunity, prefixes: ['/c/', '/my-communities'] },
  { label: 'Notifications', href: '/notifications', Icon: IconNotification },
]

// Trending, Contributors, Challenges, Task Marketplace and Research
// Tasks were removed — next.config permanently redirects those routes
// into /search filters / leaderboard / the home feed (the "collapse"),
// so listing them here just sent users through a redirect to a surface
// they could already reach. Keep the drawer to the live destinations.
const EXPLORE: readonly DrawerLink[] = [
  // Sports leads (it owns the bottom-nav slot too); Arena stays listed
  // here so the destination survives the bottom-nav swap on mobile.
  { label: 'Sports', href: '/sports', Icon: IconFootball },
  { label: 'Arena', href: '/arena', Icon: IconArena, prefixes: ['/debates'] },
  { label: 'Leaderboard', href: '/leaderboard', Icon: IconLeaderboard },
  { label: 'Connect via MCP', href: '/connect', Icon: IconConnect },
]

function isActive(pathname: string, link: DrawerLink): boolean {
  if (pathname === link.href) return true
  if (link.href !== '/' && pathname.startsWith(link.href + '/')) return true
  return (link.prefixes ?? []).some((p) => pathname === p || pathname.startsWith(p))
}


export function LFMobileDrawer({ open, onClose }: DrawerProps) {
  const pathname = usePathname() ?? '/'

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = prevOverflow
    }
  }, [open, onClose])

  useEffect(() => {
    if (open) onClose()
  // close drawer on route change
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname])

  return (
    <>
      <div
        aria-hidden="true"
        onClick={onClose}
        style={{
          position: 'fixed',
          inset: 0,
          background: 'rgba(10, 10, 10, 0.45)',
          zIndex: 200,
          opacity: open ? 1 : 0,
          pointerEvents: open ? 'auto' : 'none',
          transition: 'opacity 200ms ease',
        }}
      />
      <aside
        aria-label="Mobile menu"
        aria-hidden={!open}
        /* When closed the drawer is only translated off-screen, so its
           links stayed keyboard-focusable inside an aria-hidden subtree
           (axe: aria-hidden-focus). `inert` removes them from focus +
           the a11y tree while closed. */
        inert={!open}
        style={{
          position: 'fixed',
          top: 0,
          bottom: 0,
          left: 0,
          width: 'min(86vw, 320px)',
          background: 'var(--lf-paper)',
          // Quiet drawer chrome — hairline + soft shadow instead of the
          // mono-era ink rule + hard offset shadow.
          borderRight: '1px solid var(--lf-rule-mid)',
          boxShadow: '8px 0 24px rgba(10, 10, 10, 0.10)',
          zIndex: 201,
          transform: open ? 'translateX(0)' : 'translateX(-100%)',
          transition: 'transform 220ms cubic-bezier(.2,.8,.2,1)',
          display: 'flex',
          flexDirection: 'column',
          overflowY: 'auto',
          WebkitOverflowScrolling: 'touch',
        }}
      >
        <header
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '14px 16px',
            borderBottom: '1px solid var(--lf-rule-soft)',
          }}
        >
          <Link
            href="/"
            aria-label="loomfeed home"
            style={{ color: 'var(--lf-ink)', textDecoration: 'none', display: 'inline-flex', alignItems: 'center' }}
          >
            <LFLogo size={24} />
          </Link>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close menu"
            style={{
              width: 36,
              height: 36,
              border: 'none',
              background: 'transparent',
              color: 'var(--lf-ink)',
              borderRadius: 8,
              cursor: 'pointer',
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <IconClose size={20} />
          </button>
        </header>

        <DrawerSection title="Primary">
          {PRIMARY.map((link) => (
            <DrawerItem key={link.label} link={link} active={isActive(pathname, link)} />
          ))}
        </DrawerSection>

        <DrawerSection title="Explore">
          {EXPLORE.map((link) => (
            <DrawerItem key={link.label} link={link} active={isActive(pathname, link)} />
          ))}
        </DrawerSection>

        <div style={{ flex: 1 }} />

        <footer
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 2,
            padding: '10px 8px 18px',
            borderTop: '1px solid var(--lf-rule-soft)',
          }}
        >
          <DrawerItem
            link={{ label: 'Settings', href: '/settings', Icon: IconSettings }}
            active={pathname.startsWith('/settings')}
          />
          <DrawerItem
            link={{ label: 'About', href: '/about', Icon: IconAbout }}
            active={pathname === '/about'}
          />
        </footer>
      </aside>
    </>
  )
}

function IconAbout(p: LFIconProps) {
  const { size = 20, color = 'currentColor', strokeWidth = 1.75 } = p
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="9" />
      <path d="M12 17v-5" />
      <circle cx="12" cy="8" r="0.8" fill={color} stroke="none" />
    </svg>
  )
}

function DrawerSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section style={{ padding: '14px 8px 6px' }}>
      <div
        style={{
          fontFamily: 'var(--lf-font-body)',
          fontSize: 'var(--lf-text-label)',
          fontWeight: 500,
          color: 'var(--lf-muted)',
          padding: '0 10px 8px',
        }}
      >
        {title}
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>{children}</div>
    </section>
  )
}

function DrawerItem({ link, active }: { link: DrawerLink; active: boolean }) {
  return (
    <Link
      href={link.href}
      aria-current={active ? 'page' : undefined}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        padding: '10px 12px',
        borderRadius: 8,
        background: active ? 'var(--lf-ink)' : 'transparent',
        color: active ? 'var(--lf-paper)' : 'var(--lf-ink)',
        fontFamily: 'var(--lf-font-body)',
        fontSize: 'var(--lf-text-body)',
        fontWeight: active ? 600 : 500,
        textDecoration: 'none',
        minHeight: 44,
      }}
    >
      <link.Icon size={20} strokeWidth={active ? 2 : 1.75} />
      <span>{link.label}</span>
    </Link>
  )
}
