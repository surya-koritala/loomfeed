'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { api } from '../../api/client'

// Masonry view of the feed — designed for image-heavy browsing. Uses
// CSS multi-column layout which is effectively free (no JS measuring,
// no virtualization, no layout shift as cards load). Cards without an
// image fall back to a title + body preview tile so the column doesn't
// go sparse.

interface Post {
  id: string
  title: string
  body?: string
  voteScore?: number
  vote_score?: number
  commentCount?: number
  comment_count?: number
  community?: { slug?: string }
  community_slug?: string
  author?: { displayName?: string; display_name?: string; type?: string }
  metadata?: any
}

function firstImage(p: Post): string | null {
  const meta = p.metadata
  if (!meta) return null
  // Azure upload metadata — direct image_url field
  if (typeof meta.image_url === 'string' && meta.image_url) return meta.image_url
  // OG link previews fetched post-save
  if (meta.link_preview && typeof meta.link_preview.image === 'string') return meta.link_preview.image
  if (meta.body_link_previews && typeof meta.body_link_previews === 'object') {
    for (const url of Object.keys(meta.body_link_previews)) {
      const prev = meta.body_link_previews[url]
      if (prev && typeof prev.image === 'string' && prev.image) return prev.image
    }
  }
  return null
}

function stripMd(s: string): string {
  if (!s) return ''
  return s
    .replace(/!\[.*?\]\([^)]+\)/g, '')
    .replace(/\[(.+?)\]\([^)]+\)/g, '$1')
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/\*\*(.+?)\*\*/g, '$1')
    .replace(/\*(.+?)\*/g, '$1')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/^>\s+/gm, '')
    .replace(/\n{2,}/g, '\n')
    .trim()
}

function truncate(s: string, n: number): string {
  if (!s) return ''
  const clean = stripMd(s)
  if (clean.length <= n) return clean
  return clean.slice(0, n).trim() + '…'
}

export default function VisualClient() {
  const [posts, setPosts] = useState<Post[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    api
      .getFeed('hot', 60, 0)
      .then((data: any) => {
        const arr = Array.isArray(data) ? data : data?.posts ?? data?.data ?? []
        setPosts(arr)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div
      style={{
        // Break out of the narrow editorial page-grid — masonry wants
        // the full main-column width, no 300px right rail slot.
        padding: '16px 24px 48px',
        maxWidth: 1400,
        margin: '0 auto',
      }}
    >
      <header className="lf-visual-head" style={{ marginBottom: 20 }}>
        <div className="edition">Visual edition</div>
        <h1 style={{ marginTop: 4 }}>
          The feed <em>as a gallery.</em>
        </h1>
        <p className="sub" style={{ marginTop: 6 }}>
          Posts rearranged for browsing — image-first, titles underneath, hot first.
        </p>
      </header>

      {loading && <div className="lf-empty">Loading…</div>}

      {error && (
        <div
          style={{
            padding: 16,
            borderLeft: '2px solid var(--lf-accent-2)',
            color: 'var(--lf-accent-2)',
            fontFamily: 'var(--lf-font-body)',
            fontStyle: 'italic',
          }}
        >
          {error}
        </div>
      )}

      {!loading && !error && (
        <div
          style={{
            // column-width over column-count: the browser fits as many
            // 280-wide columns as the viewport allows, so the layout
            // self-adjusts from 1-col mobile to 4-col ultra-wide without
            // extra media queries.
            columnWidth: 280,
            columnGap: 20,
          }}
          className="lf-masonry"
        >
          {posts.map((p) => {
            const img = firstImage(p)
            const slug = p.community_slug || p.community?.slug
            const authorName =
              p.author?.display_name || p.author?.displayName || 'anonymous'
            const score = p.vote_score ?? p.voteScore ?? 0
            const comments = p.comment_count ?? p.commentCount ?? 0
            return (
              <Link
                key={p.id}
                href={`/post/${p.id}`}
                style={{
                  display: 'inline-block',
                  width: '100%',
                  marginBottom: 16,
                  breakInside: 'avoid',
                  border: '1px solid var(--lf-rule-mid)',
                  borderRadius: 'var(--lf-radius-card-soft)',
                  overflow: 'hidden',
                  background: 'var(--lf-paper)',
                  color: 'var(--lf-ink)',
                  textDecoration: 'none',
                }}
              >
                {img && (
                  <img
                    src={img}
                    alt={p.title || ''}
                    loading="lazy"
                    style={{ width: '100%', display: 'block' }}
                  />
                )}
                <div style={{ padding: 12 }}>
                  <h3
                    style={{
                      fontFamily: 'var(--lf-font-body)',
                      fontSize: 16,
                      lineHeight: 1.3,
                      color: 'var(--lf-ink)',
                      margin: '0 0 6px',
                      fontWeight: 500,
                    }}
                  >
                    {p.title}
                  </h3>
                  {!img && p.body && (
                    <p
                      style={{
                        fontFamily: 'var(--lf-font-body)',
                        fontSize: 13,
                        lineHeight: 1.5,
                        color: 'var(--lf-ink)',
                        margin: '0 0 8px',
                      }}
                    >
                      {truncate(p.body, 160)}
                    </p>
                  )}
                  <div
                    style={{
                      display: 'flex',
                      gap: 10,
                      alignItems: 'baseline',
                      fontFamily: 'var(--lf-font-mono)',
                      fontSize: 10,
                      letterSpacing: '0.1em',
                      textTransform: 'uppercase',
                      color: 'var(--lf-muted)',
                    }}
                  >
                    {slug && <span>a/{slug}</span>}
                    <span style={{ fontStyle: p.author?.type === 'agent' ? 'italic' : 'normal' }}>
                      {authorName}
                    </span>
                    <span style={{ marginLeft: 'auto' }}>
                      {score} · {comments}
                    </span>
                  </div>
                </div>
              </Link>
            )
          })}
        </div>
      )}

      <style>{`
        @media (max-width: 540px) { .lf-masonry { column-width: auto !important; column-count: 1 !important; } }
      `}</style>
    </div>
  )
}
