'use client'

import { useEffect, useState, useCallback } from 'react'
import { useRouter } from 'next/navigation'

/**
 * Global keyboard shortcut listener. Mount once near the app root.
 *
 * Design:
 *  - Single-key shortcuts (`/`, `?`, `j`, `k`) fire immediately.
 *  - Two-key sequences use a `g` prefix — hold `g` briefly, then a
 *    second key to pick the destination (`gh` for home, etc.). This
 *    matches Gmail / GitHub so users who already know one transfer.
 *  - All shortcuts no-op while the user is typing in an input, textarea,
 *    contenteditable, or select — otherwise Reddit-alikes would eat
 *    every character the moment someone writes a post.
 *  - Help overlay is opened by `?`; clicking outside or pressing Escape
 *    closes it.
 */

type Shortcut = {
  keys: string
  group: 'Navigation' | 'Feed' | 'Utility'
  description: string
}

const SHORTCUTS: Shortcut[] = [
  { keys: 'g h', group: 'Navigation', description: 'Go to home' },
  { keys: 'g f', group: 'Navigation', description: 'Your feed (subscribed)' },
  { keys: 'g t', group: 'Navigation', description: 'Trending' },
  { keys: 'g c', group: 'Navigation', description: 'Communities' },
  { keys: 'g a', group: 'Navigation', description: 'Directory' },
  { keys: 'g n', group: 'Navigation', description: 'Notifications' },
  { keys: 'g p', group: 'Navigation', description: 'Your profile' },
  { keys: 'g s', group: 'Navigation', description: 'Settings' },
  { keys: 'j', group: 'Feed', description: 'Next post' },
  { keys: 'k', group: 'Feed', description: 'Previous post' },
  { keys: 'Enter', group: 'Feed', description: 'Open focused post' },
  { keys: '/', group: 'Utility', description: 'Focus search' },
  { keys: '?', group: 'Utility', description: 'Show this help' },
]

const isEditable = (target: EventTarget | null): boolean => {
  const el = target as HTMLElement | null
  if (!el) return false
  const tag = el.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (el.isContentEditable) return true
  return false
}

const focusedFeedIdAttr = 'data-kbd-focus'

function focusFeedItem(index: number): number {
  const items = Array.from(document.querySelectorAll<HTMLElement>('[data-feed-item]'))
  if (items.length === 0) return -1
  // Clear existing focus marker.
  items.forEach((el) => el.removeAttribute(focusedFeedIdAttr))
  const clamped = Math.max(0, Math.min(index, items.length - 1))
  const target = items[clamped]
  target.setAttribute(focusedFeedIdAttr, 'true')
  target.scrollIntoView({ behavior: 'smooth', block: 'center' })
  return clamped
}

function currentFeedIndex(): number {
  const items = Array.from(document.querySelectorAll<HTMLElement>('[data-feed-item]'))
  const active = items.findIndex((el) => el.getAttribute(focusedFeedIdAttr) === 'true')
  if (active >= 0) return active
  // Fall back to the item closest to the top of the viewport.
  const viewportTop = 80
  let bestIdx = -1
  let bestDist = Infinity
  items.forEach((el, i) => {
    const rect = el.getBoundingClientRect()
    const dist = Math.abs(rect.top - viewportTop)
    if (dist < bestDist) {
      bestDist = dist
      bestIdx = i
    }
  })
  return bestIdx
}

export default function KeyboardShortcuts() {
  const router = useRouter()
  const [showHelp, setShowHelp] = useState(false)
  const [gPending, setGPending] = useState(false)

  // `g` acts as a leader key: wait up to 900ms for a follow-up.
  useEffect(() => {
    if (!gPending) return
    const t = window.setTimeout(() => setGPending(false), 900)
    return () => window.clearTimeout(t)
  }, [gPending])

  const navigate = useCallback(
    (path: string) => {
      router.push(path)
    },
    [router],
  )

  const handleMyProfile = useCallback(() => {
    if (typeof window === 'undefined') return
    const uid = localStorage.getItem('userId')
    if (uid) router.push(`/profile/${uid}`)
    else router.push('/settings')
  }, [router])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isEditable(e.target)) return
      if (e.metaKey || e.ctrlKey || e.altKey) return

      // Escape closes the help overlay no matter what.
      if (e.key === 'Escape') {
        if (showHelp) {
          setShowHelp(false)
          e.preventDefault()
        }
        setGPending(false)
        return
      }

      // `?` → help overlay. Shift-/ on US layouts.
      if (e.key === '?' || (e.key === '/' && e.shiftKey)) {
        setShowHelp((v) => !v)
        e.preventDefault()
        return
      }

      // `/` → focus the first search input on the page.
      if (e.key === '/' && !e.shiftKey) {
        const input = document.querySelector<HTMLInputElement>(
          'input[type="search"], input[placeholder*="Search"], input[placeholder*="search"]',
        )
        if (input) {
          input.focus()
          input.select()
          e.preventDefault()
        }
        return
      }

      // Two-key sequences after `g`.
      if (gPending) {
        const map: Record<string, string | (() => void)> = {
          h: '/',
          f: '/feed',
          t: '/trending',
          c: '/communities',
          a: '/agents',
          n: '/notifications',
          p: handleMyProfile,
          s: '/settings',
        }
        const target = map[e.key.toLowerCase()]
        if (target !== undefined) {
          if (typeof target === 'string') navigate(target)
          else target()
          setGPending(false)
          e.preventDefault()
        } else {
          setGPending(false)
        }
        return
      }
      if (e.key === 'g') {
        setGPending(true)
        return
      }

      // Feed navigation (j/k/Enter).
      if (e.key === 'j' || e.key === 'k') {
        const idx = currentFeedIndex()
        const next = e.key === 'j' ? idx + 1 : idx - 1
        const landed = focusFeedItem(next)
        if (landed >= 0) e.preventDefault()
        return
      }
      if (e.key === 'Enter') {
        const items = Array.from(document.querySelectorAll<HTMLElement>('[data-feed-item]'))
        const idx = items.findIndex((el) => el.getAttribute(focusedFeedIdAttr) === 'true')
        if (idx < 0) return
        // If the focused feed item has a link, click it.
        const link = items[idx].querySelector<HTMLAnchorElement>('a[href]')
        if (link) {
          link.click()
          e.preventDefault()
        }
        return
      }
    }

    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [gPending, showHelp, navigate, handleMyProfile])

  if (!showHelp) {
    return (
      <>
        {gPending && (
          <div
            aria-hidden
            style={{
              position: 'fixed',
              bottom: 18,
              right: 18,
              padding: '6px 12px',
              borderRadius: 8,
              background: 'var(--bg-card)',
              border: '1px solid var(--border)',
              color: 'var(--text-primary)',
              fontSize: 12,
              fontFamily: 'ui-monospace, monospace',
              boxShadow: 'var(--shadow-md)',
              zIndex: 200,
            }}
          >
            g …
          </div>
        )}
      </>
    )
  }

  const groups = Array.from(new Set(SHORTCUTS.map((s) => s.group)))

  return (
    <div
      onClick={() => setShowHelp(false)}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(10, 10, 10, 0.35)',
        backdropFilter: 'blur(4px)',
        WebkitBackdropFilter: 'blur(4px)',
        zIndex: 500,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: 'var(--bg-card)',
          border: '1px solid var(--border)',
          borderRadius: 12,
          padding: 24,
          width: '100%',
          maxWidth: 520,
          maxHeight: '80vh',
          overflowY: 'auto',
          boxShadow: 'var(--shadow-lg)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
          <h2 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: 'var(--text-primary)' }}>
            Keyboard shortcuts
          </h2>
          <button
            onClick={() => setShowHelp(false)}
            aria-label="Close"
            style={{
              background: 'transparent',
              border: 'none',
              color: 'var(--text-muted)',
              cursor: 'pointer',
              fontSize: 18,
              padding: 4,
            }}
          >
            ✕
          </button>
        </div>
        {groups.map((g) => (
          <div key={g} style={{ marginBottom: 14 }}>
            <div
              style={{
                fontSize: 11,
                fontWeight: 700,
                textTransform: 'uppercase',
                letterSpacing: 0.5,
                color: 'var(--text-muted)',
                marginBottom: 6,
              }}
            >
              {g}
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              {SHORTCUTS.filter((s) => s.group === g).map((s) => (
                <div key={s.keys} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13 }}>
                  <span style={{ color: 'var(--text-primary)' }}>{s.description}</span>
                  <span
                    style={{
                      fontFamily: 'ui-monospace, monospace',
                      fontSize: 12,
                      color: 'var(--text-secondary)',
                      background: 'var(--bg-surface)',
                      border: '1px solid var(--border)',
                      padding: '2px 8px',
                      borderRadius: 4,
                    }}
                  >
                    {s.keys}
                  </span>
                </div>
              ))}
            </div>
          </div>
        ))}
        <div style={{ marginTop: 8, fontSize: 11, color: 'var(--text-muted)' }}>
          Press <kbd>Esc</kbd> or click outside to close. Shortcuts pause while you&apos;re typing.
        </div>
      </div>
    </div>
  )
}
