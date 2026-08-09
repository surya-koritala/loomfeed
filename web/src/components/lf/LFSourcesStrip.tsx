// web/src/components/lf/LFSourcesStrip.tsx
'use client'

import React from 'react'
import Link from 'next/link'

// Single horizontal strip showing the post's sources summary —
// matches the hybrid-post.html `.verify` shape: lime-tint icon on
// the left, two-line text middle, action button right. The detailed
// claim → citation tree opens behind the View button (drawer or
// dedicated /post/<id>/sources page).

export interface LFSourcesStripProps {
  /** Total number of distinct sources cited across all claims. */
  sourceCount: number
  /** Post-level confidence score (0..1). */
  confidence?: number
  /** Optional per-stance breakdown for the sub-line. */
  breakdown?: {
    supports?: number
    extends?: number
    contradicts?: number
    quotes?: number
  }
  /** "synthesis" / "original" / "summary" / "translation". */
  generationMethod?: string
  /** ISO timestamp the post was drafted. */
  createdAt?: string
  /** Where the View button goes — usually `/post/<id>/sources`. */
  viewHref?: string
  /** Click-to-expand handler — set if you want an inline drawer
   *  instead of a navigation. When provided, viewHref is ignored. */
  onView?: () => void
}

export function LFSourcesStrip({
  sourceCount,
  confidence,
  breakdown,
  generationMethod,
  createdAt,
  viewHref,
  onView,
}: LFSourcesStripProps) {
  if (sourceCount === 0) return null
  const confPct = confidence != null ? Math.round(confidence * 100) : null

  // Build the sub-line from the per-stance breakdown + method + date.
  // Mirrors the mockup's "3 supports · 1 extends · 1 contradicts ·
  // synthesis · drafted 23h ago" — each part renders only when the
  // value is present so older posts (no breakdown) still look clean.
  const subParts: React.ReactNode[] = []
  if (breakdown?.supports) subParts.push(<span key="s">{breakdown.supports} supports</span>)
  if (breakdown?.extends) subParts.push(<span key="e">{breakdown.extends} extends</span>)
  if (breakdown?.contradicts) subParts.push(<span key="c">{breakdown.contradicts} contradicts</span>)
  if (breakdown?.quotes) subParts.push(<span key="q">{breakdown.quotes} quotes</span>)
  if (generationMethod) subParts.push(<span key="m">{generationMethod}</span>)
  if (createdAt) subParts.push(<span key="t">drafted {relativeTime(createdAt)}</span>)

  return (
    <section className="verify">
      <div className="left">
        <span className="icon">
          <SourcesIcon />
        </span>
        <div className="text">
          <div className="top">
            {sourceCount} {sourceCount === 1 ? 'source' : 'sources'} cited
            {confPct != null && <> · {confPct}% confidence</>}
          </div>
          <div className="sub">
            {subParts.map((node, i) => (
              <React.Fragment key={i}>
                {i > 0 && ' · '}
                {node}
              </React.Fragment>
            ))}
          </div>
        </div>
      </div>
      {onView ? (
        <button type="button" className="verify-btn" onClick={onView}>
          <ListIcon />
          View sources
        </button>
      ) : (
        <Link className="verify-btn" href={viewHref ?? '#'}>
          <ListIcon />
          View sources
        </Link>
      )}
    </section>
  )
}

function SourcesIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9A2 2 0 0 0 19.66 9H14z" />
      <line x1="2" y1="20" x2="2" y2="9" />
    </svg>
  )
}

function ListIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M3 6h18" />
      <path d="M3 12h18" />
      <path d="M3 18h12" />
    </svg>
  )
}

function relativeTime(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  if (ms < 60_000) return 'just now'
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m ago`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h ago`
  if (ms < 7 * 86_400_000) return `${Math.floor(ms / 86_400_000)}d ago`
  return new Date(iso).toLocaleDateString()
}
