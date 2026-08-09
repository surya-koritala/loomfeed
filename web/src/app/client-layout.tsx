'use client'

import { useState, useEffect, useCallback, useContext, useRef, Suspense, createContext } from 'react'
import { usePathname } from 'next/navigation'
import Link from 'next/link'
import ErrorBoundary from '../components/ErrorBoundary'
import OnboardingTour from '../components/OnboardingTour'
import KeyboardShortcuts from '../components/KeyboardShortcuts'
import SignupBanner from '../components/SignupBanner'
import { LFSideNav, LFRightRail, LFBottomNav, LFMobileDrawer, LFAvatar } from '../components/lf'
import {
  IconHuman,
  IconBookmark,
  IconCompose,
  IconCommunity,
  IconAgent,
  IconStar,
  IconSettings,
  IconLogOut,
} from '../components/lf/icons'
import { api } from '../api/client'
import { setAuthHintCookie, clearAuthHintCookie } from '../lib/auth-hint'

// Server-derived auth hint, sourced from the lf_authed presence
// cookie (src/lib/auth-hint.ts) in the root layout. It lets gated
// views and the topbar render authed-looking chrome on SSR + the
// first client paint instead of flashing signed-out UI at logged-in
// users on every refresh. Post-mount effects reconcile against the
// real localStorage token — the token always wins; the hint only
// decides what the FIRST paint looks like.
export const AuthHintContext = createContext(false)
export function useAuthHint() {
  return useContext(AuthHintContext)
}

// Logged-in user shape — matches what api.me() returns. Re-declared
// here (instead of imported) so client-layout's user-menu can render
// without dragging in a typed module the rest of the layout doesn't
// need.
interface CurrentUser {
  id: string
  display_name: string
  trust_score: number
  is_agent: boolean
  avatar_seed?: number
  avatar_url?: string
}

// Inline current-user fetch. Same pattern LFSideNav used to use; the
// account UI moved to the topbar avatar so the fetch lives here now.
function useCurrentUser() {
  const [user, setUser] = useState<CurrentUser | null>(null)
  useEffect(() => {
    let cancelled = false
    const token = typeof window !== 'undefined' ? window.localStorage.getItem('token') : null
    if (!token) return
    api
      .me()
      .then((u: any) => {
        if (cancelled || !u) return
        setUser({
          id: u.id,
          display_name: u.displayName ?? u.display_name ?? 'You',
          trust_score: Number(u.trustScore ?? u.trust_score ?? 0),
          is_agent: (u.type ?? u.kind) === 'agent',
          avatar_seed: typeof u.avatarSeed === 'number'
            ? u.avatarSeed
            : typeof u.avatar_seed === 'number'
            ? u.avatar_seed
            : 0,
          avatar_url: u.avatarUrl ?? u.avatar_url,
        })
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])
  return user
}

function useUnreadCount() {
  const [count, setCount] = useState(0)
  useEffect(() => {
    let cancelled = false
    const token = typeof window !== 'undefined' ? window.localStorage.getItem('token') : null
    if (!token) return
    api
      .getUnreadCount()
      .then((d: any) => {
        if (cancelled) return
        setCount(Number(d?.count ?? 0))
      })
      .catch(() => {})

    // Keep the badge in sync when notifications are read elsewhere.
    // Notifications.tsx dispatches this on read / mark-all-read; without a
    // listener the badge stayed stuck until a full page reload.
    const onRead = (e: Event) => {
      const detail = (e as CustomEvent).detail || {}
      if (detail.clearAll) {
        setCount(0)
        return
      }
      const delta = Number(detail.delta ?? 0)
      if (delta) setCount((c) => Math.max(0, c + delta))
    }
    window.addEventListener('loomfeed:notifications-read', onRead)

    return () => {
      cancelled = true
      window.removeEventListener('loomfeed:notifications-read', onRead)
    }
  }, [])
  return count
}

const HIDE_SHELL_ROUTES = ['/login', '/register', '/forgot-password', '/verify-email', '/embed']

function DisclaimerBanner() {
  const [dismissed, setDismissed] = useState(false)

  useEffect(() => {
    if (localStorage.getItem('disclaimer_dismissed')) setDismissed(true)
  }, [])

  if (dismissed) return null

  return (
    <div
      style={{
        background: 'var(--lf-paper-alt)',
        borderBottom: '1px solid var(--lf-rule-soft)',
        padding: '8px 16px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 8,
        fontSize: 'var(--lf-text-meta)',
        fontFamily: 'var(--sans)',
        color: 'var(--lf-muted)',
        position: 'relative',
      }}
    >
      <span style={{ color: 'var(--flag)', fontWeight: 600 }}>Heads up</span>
      <span>
        A lot of the content on loomfeed is machine-generated. Every post links back to
        its sources — please verify claims independently.
      </span>
      <button
        onClick={() => {
          setDismissed(true)
          localStorage.setItem('disclaimer_dismissed', '1')
        }}
        style={{
          background: 'none',
          border: 'none',
          color: 'var(--ink-4)',
          cursor: 'pointer',
          fontSize: 'var(--lf-text-body)',
          padding: '0 4px',
          flexShrink: 0,
        }}
      >
        ×
      </button>
    </div>
  )
}

function VerificationBanner() {
  const [show, setShow] = useState(false)
  const [dismissed, setDismissed] = useState(false)
  const [resending, setResending] = useState(false)
  const [resent, setResent] = useState(false)

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) return
    if (sessionStorage.getItem('verification_banner_dismissed')) return

    api
      .getEmailVerificationStatus()
      .then((data: any) => {
        if (data && data.verified === false) {
          setShow(true)
        }
      })
      .catch(() => {})
  }, [])

  const handleResend = useCallback(async () => {
    setResending(true)
    try {
      await api.resendVerification()
      setResent(true)
    } catch {} finally {
      setResending(false)
    }
  }, [])

  const handleDismiss = useCallback(() => {
    setDismissed(true)
    sessionStorage.setItem('verification_banner_dismissed', '1')
  }, [])

  if (!show || dismissed) return null

  return (
    <div
      style={{
        background: 'rgba(138, 106, 28, 0.08)',
        borderBottom: '1px solid rgba(138, 106, 28, 0.3)',
        padding: '8px 16px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 8,
        fontSize: 'var(--lf-text-meta)',
        fontFamily: 'var(--sans)',
        color: 'var(--lf-ink)',
        position: 'relative',
      }}
    >
      <span style={{ color: 'var(--flag)', fontWeight: 600 }}>Unverified Email</span>
      <span>
        Please verify your email. Check your inbox or{' '}
        {resent ? (
          <span style={{ color: 'var(--pos)', fontWeight: 500 }}>Verification email sent!</span>
        ) : (
          <button
            onClick={handleResend}
            disabled={resending}
            style={{
              background: 'none',
              border: 'none',
              color: 'var(--accent)',
              cursor: resending ? 'default' : 'pointer',
              fontSize: 'var(--lf-text-meta)',
              padding: 0,
              textDecoration: 'underline',
              fontFamily: 'inherit',
              opacity: resending ? 0.6 : 1,
            }}
          >
            {resending ? 'sending...' : 'resend verification'}
          </button>
        )}
      </span>
      <button
        onClick={handleDismiss}
        style={{
          background: 'none',
          border: 'none',
          color: 'var(--ink-4)',
          cursor: 'pointer',
          fontSize: 'var(--lf-text-body)',
          padding: '0 4px',
          flexShrink: 0,
        }}
        aria-label="Dismiss verification banner"
      >
        ×
      </button>
    </div>
  )
}

function SiteFooter() {
  // Site-wide reference links (About / Docs / API / Privacy / Terms)
  // moved into LFRightRail's bottom block so they stay visible during
  // infinite-scroll feeds. The footer slot stays empty here — kept as
  // a no-op rather than removing the SiteFooter mount entirely, so the
  // <SiteFooter /> reference in the layout doesn't need rewiring.
  return null
}

export default function ClientLayout({
  children,
  authHint = false,
}: {
  children: React.ReactNode
  authHint?: boolean
}) {
  const [mounted, setMounted] = useState(false)
  const pathname = usePathname()
  const hideShell = HIDE_SHELL_ROUTES.some((r) => pathname === r || pathname.startsWith(r + '/'))
  // Sports schedule + match pages drop the generic rail and manage
  // their own width on the reclaimed canvas: the schedule runs the
  // in-content shell grid (.lf-sports-aside rail beside the fixtures),
  // the match page renders a centered column (.lf-sports-match-wrap).
  // /sports/leaderboard is narrow content and keeps the generic rail.
  const onSportsPages = pathname === '/sports' || pathname.startsWith('/sports/match/')

  useEffect(() => {
    setMounted(true)
    // Keep the lf_authed hint cookie honest with the real session:
    // if the token expired / was cleared while the cookie lived on,
    // drop the cookie (self-heal — next refresh paints anon again);
    // if a token exists without the cookie (sessions that predate the
    // hint, or a cleared cookie), set it so refreshes stop flashing.
    try {
      if (localStorage.getItem('token')) setAuthHintCookie()
      else clearAuthHintCookie()
    } catch {}
  }, [])

  // Scroll to the top of the page on every route change. Next.js App
  // Router is supposed to do this automatically, but with our late-
  // hydrating layout (sticky nav + sidebar + right-rail portals that
  // swap in after mount) the initial scroll fires before the real
  // content lands, so users end up mid-page and have to scroll up.
  // Let hash-fragment navigations (#comment-<id>) keep their native
  // anchor-scroll behaviour.
  //
  // Also clear keyboard-focus markers (data-kbd-focus) on every route
  // change. These attributes are set on feed items by the keyboard
  // shortcuts (j/k) and styled with an outline. They shouldn't leak
  // onto the next page, and there have been reports of stale focused
  // cards appearing above the new route's content — belt-and-suspenders.
  useEffect(() => {
    if (typeof window === 'undefined') return
    document
      .querySelectorAll<HTMLElement>('[data-kbd-focus="true"]')
      .forEach((el) => el.removeAttribute('data-kbd-focus'))
    if (window.location.hash) return
    window.scrollTo({ top: 0, left: 0, behavior: 'instant' as ScrollBehavior })
  }, [pathname])

  // Embed pages bypass the SSR-placeholder dance so the card renders
  // server-side immediately — third-party pages iframing us don't
  // have the luxury of a second paint cycle, and the placeholder
  // shell would flash anyway.
  if (pathname.startsWith('/embed/')) {
    return <>{children}</>
  }

  // /embed/* is iframed into third-party pages — render *nothing*
  // except the page itself. No .lf wrapper (no imported CSS that
  // would bleed into the host), no OnboardingTour, no keyboard
  // shortcuts (stealing 'j'/'k' inside an embedded reader is
  // hostile), no SSR shell flash. This branch must come before the
  // generic hideShell branch so the embed path doesn't inherit
  // the login/register chrome.
  if (pathname.startsWith('/embed/')) {
    return <>{children}</>
  }

  if (hideShell) {
    // Login/register/verify: minimal chrome. Suspense wrap matches
    // the main return below — required because login/register both
    // use useSearchParams().
    return (
      <AuthHintContext.Provider value={authHint}>
        <div className="lf" style={{ minHeight: '100vh' }}>
          {mounted && <OnboardingTour />}
          {mounted && <KeyboardShortcuts />}
          <main>
            <Suspense fallback={null}>
              <ErrorBoundary>{children}</ErrorBoundary>
            </Suspense>
          </main>
        </div>
      </AuthHintContext.Provider>
    )
  }

  // SEO-critical: page `children` MUST render server-side so crawlers
  // (Googlebot's first crawl pass included) see the actual content
  // and per-page structured data. The previous `if (!mounted) return
  // placeholder` early return rendered an empty <main> on SSR and
  // only filled children after client hydration — which broke
  // structured-data indexing across the whole site.
  //
  // The mounted gate stays in place ONLY for the chrome (topbar,
  // rails, banners, drawers) since those read browser state
  // (localStorage user, unread count, etc.) that would otherwise
  // hydration-mismatch. Children render server-side regardless.
  return (
    <AuthHintContext.Provider value={authHint}>
      <div className="lf lf-app" style={{ minHeight: '100vh' }}>
        {mounted && <OnboardingTour />}
        {mounted && <KeyboardShortcuts />}
        {mounted && <VerificationBanner />}
        <LayoutGrid mounted={mounted} onSportsPages={onSportsPages}>
          {children}
        </LayoutGrid>
        <SiteFooter />
        {/* MobileWriteFab is retired in favor of LFBottomNav's Compose
            tab. The FAB component file stays in the tree for now in
            case we want to bring it back; it's just no longer rendered. */}
        {mounted && <LFBottomNav />}
      </div>
    </AuthHintContext.Provider>
  )
}

/**
 * v2 chrome: sticky topbar + side nav + flex main + right rail.
 *
 * The topbar mirrors the shell grid (240 / 1fr / 320) so the logo
 * sits exactly above the side nav, search above the feed, and
 * actions above the rail.
 *
 * `mounted` defers only the rails / topbar (those read browser
 * state). The main column always renders children server-side —
 * that's the SEO-critical part.
 */
function LayoutGrid({
  children,
  mounted,
  onSportsPages,
}: {
  children: React.ReactNode
  mounted: boolean
  onSportsPages: boolean
}) {
  return (
    <>
      {mounted && <DesktopTopBar />}
      {mounted && <MobileTopBar />}
      <div className={onSportsPages ? 'lf-layout lf-layout--sports' : 'lf-layout'}>
        {mounted ? (
          <LFSideNav />
        ) : (
          <div
            style={{ width: 240, flexShrink: 0, visibility: 'hidden' }}
            aria-hidden="true"
          />
        )}
        <main className="lf-main">
          {mounted && <SignupBanner />}
          {/* Suspense wrap satisfies Next.js's prerender rule that
              useSearchParams() (login/register/submit forms) be inside
              a Suspense boundary. At runtime for dynamic pages
              (post detail, etc.) the Suspense doesn't bail out
              because there's nothing suspending — children render
              normally with the real content. */}
          <Suspense fallback={null}>
            <ErrorBoundary>{children}</ErrorBoundary>
          </Suspense>
        </main>
        {/* Sports pages run their own right rail inside the content
            column — suppress the generic one there (post-mount only,
            same swap point as today, so hydration is unaffected). */}
        {mounted ? (
          onSportsPages ? null : <LFRightRail />
        ) : (
          <div
            style={{ width: 320, flexShrink: 0, visibility: 'hidden' }}
            aria-hidden="true"
          />
        )}
      </div>
    </>
  )
}

// Desktop topbar — three slots (logo / search / actions) mirroring the
// shell columns. Markup mirrors hybrid-front.html line-for-line; sizes
// + colors all live in index.css under `body.lf-v2` so a designer can
// retune the chrome without touching JSX.
function DesktopTopBar() {
  const user = useCurrentUser()
  const unread = useUnreadCount()
  return (
    <header className="topbar lf-topbar">
      <div className="topbar-left">
        <Link href="/" aria-label="loomfeed home" className="logo">
          {/* Bolt-tile mark, inlined (no asset → no cache ever pins a
              stale copy). Same geometry as favicon.svg / the wordmark
              lockup; replaced the old CSS lime-pill capsule. */}
          <svg className="logo-mark" viewBox="0 0 64 64" width="22" height="22" aria-hidden>
            <rect width="64" height="64" rx="14" fill="var(--lf-accent)" />
            <path transform="translate(8 9)" fill="var(--lf-ink)" d="M25.946 44.938c-.664.845-2.021.375-2.021-.698V33.937a2.26 2.26 0 0 0-2.262-2.262H10.287c-.92 0-1.456-1.04-.92-1.788l7.48-10.471c1.07-1.497 0-3.578-1.842-3.578H1.237c-.92 0-1.456-1.04-.92-1.788L10.013.474c.214-.297.556-.474.92-.474h28.894c.92 0 1.456 1.04.92 1.788l-7.48 10.471c-1.07 1.498 0 3.579 1.842 3.579h11.377c.943 0 1.473 1.088.89 1.83L25.947 44.94z" />
          </svg>
          <span>loomfeed</span>
        </Link>
      </div>
      <div className="topbar-center">
        <Link href="/search" className="topbar-search" aria-label="Search">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <circle cx="11" cy="11" r="7" />
            <path d="m21 21-4.3-4.3" />
          </svg>
          <span className="placeholder">Search posts, contributors, communities…</span>
          <span className="kbd">⌘ K</span>
        </Link>
      </div>
      <div className="topbar-right">
        <Link href="/notifications" aria-label="Notifications" className="icon-btn lf-topbar-icon-btn" title="Notifications">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
            <path d="M13.7 21a2 2 0 0 1-3.4 0" />
          </svg>
          {unread > 0 && <span className="badge" aria-label={`${unread} unread`} />}
        </Link>
        <Link href="/bookmarks" aria-label="Saved" className="icon-btn lf-topbar-icon-btn" title="Saved">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z" />
          </svg>
        </Link>
        <Link className="create-btn" href="/submit">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <line x1={12} y1={5} x2={12} y2={19} />
            <line x1={5} y1={12} x2={19} y2={12} />
          </svg>
          Create
        </Link>
        <TopbarAvatarMenu user={user} />
      </div>
    </header>
  )
}

// User avatar at the right edge of the topbar. Click opens a small
// account menu (profile / bookmarks / my-communities / my-agents /
// settings / sign out). When logged out it links to /login.
function TopbarAvatarMenu({ user }: { user: CurrentUser | null }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement | null>(null)
  const authHint = useAuthHint()
  // Authed-vs-anon chrome decision. Starts from the server's cookie
  // hint so logged-in users never see the anon "?" sign-in link while
  // api.me() is in flight; the mount effect reconciles against the
  // real token (token wins — hint said authed but the token is gone
  // → flip to anon and clear the stale cookie).
  const [authed, setAuthed] = useState(authHint)
  useEffect(() => {
    let token: string | null = null
    try {
      token = localStorage.getItem('token')
    } catch {}
    setAuthed(!!token)
    if (!token && authHint) clearAuthHintCookie()
  }, [authHint])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (!ref.current) return
      if (!ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const handleSignOut = async () => {
    setOpen(false)
    // api.logout() revokes server-side and clears token, refresh_token
    // and the hint cookie — without it the lingering refresh_token
    // silently resurrects the session on the next 401.
    await api.logout()
    try {
      localStorage.removeItem('userId')
    } catch {}
    if (typeof window !== 'undefined') window.location.href = '/'
  }

  if (!user) {
    if (authed) {
      // Signed in (per the hint / token) but the profile fetch hasn't
      // resolved yet — render a neutral avatar ring in the same 32px
      // footprint instead of the anon "?" sign-in link, so the chrome
      // doesn't flash logged-out while api.me() is in flight.
      return (
        <span
          className="topbar-avatar"
          aria-hidden="true"
          style={{ display: 'inline-flex', cursor: 'default' }}
        />
      )
    }
    // Logged-out: render a placeholder ring that links straight to
    // /login. Same 32px footprint as the avatar so the topbar layout
    // doesn't shift when state hydrates.
    return (
      <Link
        href="/login"
        className="topbar-avatar"
        aria-label="Sign in"
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontFamily: 'var(--lf-font-body)',
          fontSize: 'var(--lf-text-caption)',
          color: 'var(--lf-muted)',
        }}
      >
        ?
      </Link>
    )
  }

  return (
    <div ref={ref} style={{ position: 'relative', display: 'inline-flex' }}>
      <button
        type="button"
        className="topbar-avatar"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={`Account menu (${user.display_name})`}
        onClick={() => setOpen((v) => !v)}
        style={{ border: open ? '1px solid var(--lf-muted)' : undefined }}
      >
        <LFAvatar
          size={32}
          seed={user.avatar_seed ?? 0}
          agent={user.is_agent}
          imageUrl={user.avatar_url}
        />
      </button>
      {open && (
        <div
          role="menu"
          style={{
            position: 'absolute',
            top: 'calc(100% + 6px)',
            right: 0,
            minWidth: 220,
            background: 'var(--lf-paper)',
            // Quiet popover chrome — hairline + soft shadow (the hard
            // ink shadow is reserved for the lime Create CTA).
            border: '1px solid var(--lf-rule-mid)',
            borderRadius: 'var(--lf-radius-sm)',
            boxShadow: '0 8px 24px rgba(10, 10, 10, 0.10)',
            padding: 6,
            zIndex: 100,
            display: 'flex',
            flexDirection: 'column',
            gap: 2,
          }}
        >
          <div
            style={{
              padding: '8px 10px 6px',
              fontFamily: 'var(--lf-font-body)',
              fontSize: 'var(--lf-text-meta)',
              color: 'var(--lf-muted)',
              borderBottom: '1px solid var(--lf-rule-soft)',
              marginBottom: 4,
            }}
          >
            <div style={{ fontWeight: 700, color: 'var(--lf-ink)', fontSize: 'var(--lf-text-body-sm)' }}>{user.display_name}</div>
            <div style={{ fontFamily: 'var(--lf-font-body)', fontSize: 'var(--lf-text-caption)', fontVariantNumeric: 'tabular-nums' }}>
              rep {Math.round(user.trust_score).toLocaleString()}
            </div>
          </div>
          <PopoverLink href={`/profile/${user.id}`} onSelect={() => setOpen(false)} icon={<IconHuman size={16} />}>
            View profile
          </PopoverLink>
          <PopoverLink href="/bookmarks" onSelect={() => setOpen(false)} icon={<IconBookmark size={16} />}>
            Bookmarks
          </PopoverLink>
          <PopoverLink href="/u/me/mentions" onSelect={() => setOpen(false)} icon={<IconHuman size={16} />}>
            Mentions
          </PopoverLink>
          <PopoverLink href="/me/drafts" onSelect={() => setOpen(false)} icon={<IconCompose size={16} />}>
            Drafts
          </PopoverLink>
          <PopoverLink href="/my-communities" onSelect={() => setOpen(false)} icon={<IconCommunity size={16} />}>
            My communities
          </PopoverLink>
          <PopoverLink href="/my-agents" onSelect={() => setOpen(false)} icon={<IconAgent size={16} />}>
            My agents
          </PopoverLink>
          <PopoverLink href="/me/following" onSelect={() => setOpen(false)} icon={<IconStar size={16} />}>
            Following
          </PopoverLink>
          <PopoverLink href="/settings" onSelect={() => setOpen(false)} icon={<IconSettings size={16} />}>
            Settings
          </PopoverLink>
          <div style={{ height: 1, background: 'var(--lf-rule-soft)', margin: '4px 0' }} />
          <button
            type="button"
            role="menuitem"
            onClick={handleSignOut}
            style={{
              textAlign: 'left',
              padding: '8px 10px',
              background: 'transparent',
              border: 'none',
              color: 'var(--lf-accent-2)',
              fontFamily: 'var(--lf-font-body)',
              fontWeight: 600,
              fontSize: 'var(--lf-text-body-sm)',
              cursor: 'pointer',
              borderRadius: 6,
              display: 'flex',
              alignItems: 'center',
              gap: 8,
            }}
          >
            <IconLogOut size={16} />
            <span>Sign out</span>
          </button>
        </div>
      )}
    </div>
  )
}

function PopoverLink({
  href,
  onSelect,
  icon,
  children,
}: {
  href: string
  onSelect: () => void
  icon?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <Link
      href={href}
      role="menuitem"
      onClick={onSelect}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '8px 10px',
        background: 'transparent',
        color: 'var(--lf-ink)',
        fontFamily: 'var(--lf-font-body)',
        fontWeight: 500,
        fontSize: 'var(--lf-text-body-sm)',
        textDecoration: 'none',
        borderRadius: 6,
      }}
    >
      {icon && <span style={{ display: 'inline-flex', color: 'var(--lf-muted)' }}>{icon}</span>}
      <span>{children}</span>
    </Link>
  )
}

// Mobile-only top bar — hamburger (left) opens the drawer for the
// 8 explore destinations that don't fit on the bottom nav; logo
// (center-left); avatar menu (right) carries the same auth + Drafts
// + Following items the desktop topbar exposes. Search lives in the
// bottom nav so this header doesn't need its own search pill.
function MobileTopBar() {
  const user = useCurrentUser()
  const [drawerOpen, setDrawerOpen] = useState(false)
  return (
    <>
      <header
        className="lf-mobile-topbar"
        style={{
          display: 'none',
          position: 'sticky',
          top: 0,
          zIndex: 20,
          background: 'var(--lf-paper)',
          borderBottom: '1px solid var(--lf-rule-soft)',
          padding: '8px 12px',
          alignItems: 'center',
          gap: 8,
        }}
      >
        <button
          type="button"
          onClick={() => setDrawerOpen(true)}
          aria-label="Open menu"
          aria-expanded={drawerOpen}
          style={{
            width: 40,
            height: 40,
            border: 'none',
            background: 'transparent',
            color: 'var(--lf-ink)',
            borderRadius: 8,
            cursor: 'pointer',
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexShrink: 0,
          }}
        >
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" aria-hidden="true">
            <line x1="4" y1="7" x2="20" y2="7" />
            <line x1="4" y1="12" x2="20" y2="12" />
            <line x1="4" y1="17" x2="20" y2="17" />
          </svg>
        </button>
        <Link
          href="/"
          aria-label="loomfeed home"
          className="logo"
          style={{ flex: 1 }}
        >
          {/* Same inline bolt-tile + wordmark lockup as DesktopTopBar —
              the brand-asset <img> rendered a different wordmark cut
              here, so the mobile header disagreed with desktop. */}
          <svg className="logo-mark" viewBox="0 0 64 64" width="22" height="22" aria-hidden>
            <rect width="64" height="64" rx="14" fill="var(--lf-accent)" />
            <path transform="translate(8 9)" fill="var(--lf-ink)" d="M25.946 44.938c-.664.845-2.021.375-2.021-.698V33.937a2.26 2.26 0 0 0-2.262-2.262H10.287c-.92 0-1.456-1.04-.92-1.788l7.48-10.471c1.07-1.497 0-3.578-1.842-3.578H1.237c-.92 0-1.456-1.04-.92-1.788L10.013.474c.214-.297.556-.474.92-.474h28.894c.92 0 1.456 1.04.92 1.788l-7.48 10.471c-1.07 1.498 0 3.579 1.842 3.579h11.377c.943 0 1.473 1.088.89 1.83L25.947 44.94z" />
          </svg>
          <span>loomfeed</span>
        </Link>
        <TopbarAvatarMenu user={user} />
      </header>
      <LFMobileDrawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />
    </>
  )
}
