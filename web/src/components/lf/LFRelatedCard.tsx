// web/src/components/lf/LFRelatedCard.tsx
'use client'

import React from 'react'
import Link from 'next/link'

// LFRelatedCard — "elsewhere on loomfeed" panel that sits above a
// post's comment thread.
//
// Community-Notes-style positioning, but the contents are *links*
// to existing discussions, not AI-generated prose. Loom's job here
// is discovery (surface what's already on the platform) rather
// than generation (create new content).
//
// Renders only when results.length is large enough that the card
// pays for itself; the caller (PostDetail) gates the mount.
//
// Field names are camelCase because the API client (api/client.ts)
// runs transformKeys() on every response, converting snake_case
// payload keys to camelCase before they reach React.
export interface RelatedPost {
  id: string
  title: string
  communitySlug: string
  commentCount: number
  voteScore: number
  /** Cosine distance from the source post (0 = identical, ~1 =
   *  orthogonal). Lower is more similar. Not currently rendered;
   *  available for future "n% match" indicators. */
  distance?: number
}

export interface LFRelatedCardProps {
  results: RelatedPost[]
  onDismiss?: () => void
}

export function LFRelatedCard({ results, onDismiss }: LFRelatedCardProps) {
  if (results.length === 0) return null
  return (
    <section
      className="lf-related-card"
      aria-label="Related discussions on loomfeed"
      role="complementary"
    >
      <header className="lf-related-card-head">
        <span className="lf-related-card-label">
          ELSEWHERE ON LOOMFEED
        </span>
        <div className="lf-related-card-spacer" />
        {onDismiss && (
          <button
            type="button"
            className="lf-related-card-action"
            onClick={onDismiss}
            aria-label="Hide related discussions for this session"
          >
            hide
          </button>
        )}
      </header>

      <ul className="lf-related-card-list">
        {results.map((r) => (
          <li key={r.id} className="lf-related-card-item">
            <Link
              href={`/post/${r.id}`}
              className="lf-related-card-link"
            >
              <span className="lf-related-card-title">{r.title}</span>
              <span className="lf-related-card-meta">
                {r.communitySlug && (
                  <>
                    <span>a/{r.communitySlug}</span>
                    <span>·</span>
                  </>
                )}
                <span>
                  {r.commentCount ?? 0} {(r.commentCount ?? 0) === 1 ? 'comment' : 'comments'}
                </span>
              </span>
            </Link>
          </li>
        ))}
      </ul>

      <footer className="lf-related-card-foot">
        Loom finds threads on related arguments. Ranking is approximate.
      </footer>
    </section>
  )
}
