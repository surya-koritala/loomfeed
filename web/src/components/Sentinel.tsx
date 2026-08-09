'use client'

import React, { useEffect, useRef } from 'react'

// Auto-paginating sentinel. Renders a small "Loading more…" stub and
// fires onVisible when it scrolls into view (with 400px rootMargin so
// the next page kicks before the user actually hits the bottom).
//
// Single-fire per loading cycle: while loading=true we won't re-trigger
// even if the sentinel stays intersecting. When loading drops back to
// false the sentinel is ready to fire again on the next intersect.
//
// Used by Home, Community, Profile, AgentDirectory, etc. to replace
// manual "Load more" buttons.

export interface SentinelProps {
  /** Called once when the sentinel intersects the viewport. */
  onVisible: () => void
  /** True while a page is in-flight; suppresses re-fires + shows "Loading more…". */
  loading: boolean
  /** Optional override for the loading text. */
  label?: string
}

export function Sentinel({ onVisible, loading, label = 'Loading more…' }: SentinelProps) {
  const ref = useRef<HTMLDivElement>(null)
  const called = useRef(false)

  useEffect(() => {
    if (!loading) called.current = false
  }, [loading])

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting && !called.current && !loading) {
          called.current = true
          onVisible()
        }
      },
      { rootMargin: '400px' },
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [onVisible, loading])

  return (
    <div ref={ref} style={{ padding: '20px 0', textAlign: 'center' }}>
      {loading && (
        <div
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            letterSpacing: '0.12em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
          }}
        >
          {label}
        </div>
      )}
    </div>
  )
}
