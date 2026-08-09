import React from 'react'

// Loading placeholder shaped like LFPostCard — a vote rail + meta line +
// title + two body lines + an action row — drawn with the shimmer
// `.skeleton` kit (defined in index.css). Render a few while the feed or
// a post list is fetching so rows shimmer in place instead of the page
// snapping from empty text to full content.
//
// Decorative: aria-hidden so screen readers announce nothing while the
// real content loads (the live region / heading does the announcing).

const frame: React.CSSProperties = {
  display: 'flex',
  gap: 12,
  padding: 14,
  border: '1px solid var(--lf-rule-mid)',
  borderRadius: 'var(--lf-radius)',
  background: 'var(--lf-paper)',
}

export function LFPostCardSkeleton() {
  return (
    <div style={frame} aria-hidden="true">
      {/* vote rail */}
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6, flexShrink: 0 }}>
        <div className="skeleton" style={{ width: 22, height: 22, borderRadius: 'var(--lf-radius-sm)' }} />
        <div className="skeleton" style={{ width: 16, height: 10 }} />
        <div className="skeleton" style={{ width: 22, height: 22, borderRadius: 'var(--lf-radius-sm)' }} />
      </div>
      {/* content */}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="skeleton skeleton-text-sm" style={{ width: '42%' }} />
        <div className="skeleton skeleton-title" style={{ width: '88%' }} />
        <div className="skeleton skeleton-text" style={{ width: '100%' }} />
        <div className="skeleton skeleton-text" style={{ width: '68%' }} />
        <div style={{ display: 'flex', gap: 10, marginTop: 12 }}>
          {[64, 64, 56].map((w, i) => (
            <div key={i} className="skeleton" style={{ width: w, height: 22, borderRadius: 'var(--lf-radius-pill)' }} />
          ))}
        </div>
      </div>
    </div>
  )
}

export function LFPostListSkeleton({ count = 5 }: { count?: number }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }} aria-hidden="true">
      {Array.from({ length: count }).map((_, i) => (
        <LFPostCardSkeleton key={i} />
      ))}
    </div>
  )
}
