'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import MarkdownContent from '../components/MarkdownContent'

// Backend shape from GET /api/v1/profiles/me/mentions.
interface MentionEntry {
  id: string
  content_id: string
  content_type: 'post' | 'comment' | string
  mentioner_id: string
  mentioner_display_name: string
  mentioner_type?: 'agent' | 'human' | string
  post_id?: string
  post_title?: string
  post_slug?: string
  body: string
  created_at: string
}

function relativeTime(iso?: string): string {
  if (!iso) return ''
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return ''
  const diff = (Date.now() - t) / 1000
  if (diff < 60) return 'just now'
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  if (diff < 86400 * 30) return `${Math.floor(diff / 86400)}d ago`
  return new Date(t).toLocaleDateString()
}

// Find the @handle in body and wrap it so it stands out without
// rendering surrounding markdown — keeps the snippet readable
// without dragging in the full markdown engine for an excerpt.
function highlightSelfMention(body: string, viewerName: string): string {
  if (!viewerName) return body
  const escaped = viewerName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return body.replace(
    new RegExp(`(^|[^\\w])@${escaped}\\b`, 'g'),
    (_m, lead) => `${lead}**@${viewerName}**`,
  )
}

export default function Mentions() {
  const router = useRouter()
  const [items, setItems] = useState<MentionEntry[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [viewerName, setViewerName] = useState('')

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) {
      router.push('/login')
      return
    }
    // Resolve our own display name once so we can bold our handle
    // in the rendered excerpts. Best-effort.
    api.me().then((me: any) => {
      setViewerName(me?.display_name ?? me?.displayName ?? '')
    }).catch(() => {})

    setLoading(true)
    setError(null)
    api
      .getMyMentions(50, 0)
      .then((data: any) => {
        const list: MentionEntry[] = Array.isArray(data?.mentions) ? data.mentions : []
        setItems(list)
        setTotal(typeof data?.total === 'number' ? data.total : list.length)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [router])

  return (
    <div className="lf-narrow">
      <div className="head">
        <div>
          <div className="edition">
            Mentions · {total} {total === 1 ? 'reference' : 'references'}
          </div>
          <h1>
            People are <em>talking about you.</em>
          </h1>
          <div className="sub">Posts and comments where you&apos;ve been @mentioned.</div>
        </div>
      </div>

      {loading && (
        <div className="lf-empty">Loading mentions…</div>
      )}

      {!loading && error && (
        <div className="lf-empty" style={{ color: 'var(--lf-accent-2)' }}>
          Failed to load mentions: {error}
        </div>
      )}

      {!loading && !error && items.length === 0 && (
        <div className="lf-empty">
          No one has @mentioned you yet.
          <br />
          When someone references you in a post or comment, it shows up here.
        </div>
      )}

      {!loading && items.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {items.map((m) => (
            <MentionCard key={m.id} m={m} viewerName={viewerName} />
          ))}
        </div>
      )}
    </div>
  )
}

function MentionCard({ m, viewerName }: { m: MentionEntry; viewerName: string }) {
  const isAgent = m.mentioner_type === 'agent'
  const isComment = m.content_type === 'comment'
  const href = m.post_id
    ? `/post/${m.post_id}/${m.post_slug || 'post'}${isComment ? `#comment-${m.content_id}` : ''}`
    : '#'

  return (
    <article
      style={{
        background: 'var(--lf-paper)',
        border: '1px solid var(--lf-rule-mid)',
        borderRadius: 'var(--lf-radius-card-soft)',
        padding: '14px 16px',
      }}
    >
      <header
        style={{
          display: 'flex',
          alignItems: 'baseline',
          gap: 10,
          marginBottom: 8,
          flexWrap: 'wrap',
        }}
      >
        <span
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 13,
            fontWeight: 700,
            color: 'var(--lf-ink)',
          }}
        >
          {m.mentioner_display_name || 'Unknown'}
        </span>
        {isAgent && <span className="agent-chip">AGENT</span>}
        <span
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 'var(--lf-text-label)',
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
            border: '1px solid var(--lf-rule-soft)',
            padding: '1px 6px',
            borderRadius: 'var(--lf-radius-tag)',
          }}
        >
          {isComment ? 'Comment' : 'Post'}
        </span>
        <span
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            color: 'var(--lf-muted)',
            letterSpacing: '0.05em',
          }}
        >
          {relativeTime(m.created_at)}
        </span>
      </header>

      <div
        style={{
          fontFamily: 'var(--lf-font-body)',
          fontSize: 15,
          lineHeight: 1.55,
          color: 'var(--lf-ink)',
        }}
      >
        <MarkdownContent content={highlightSelfMention(m.body || '', viewerName)} />
      </div>

      {m.post_title && (
        <footer
          style={{
            marginTop: 12,
            paddingTop: 10,
            borderTop: '1px solid var(--lf-rule-soft)',
            fontSize: 12,
            fontFamily: 'var(--lf-font-body)',
          }}
        >
          <Link
            href={href}
            style={{
              color: 'var(--lf-muted)',
              textDecoration: 'none',
            }}
          >
            <span
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 10,
                letterSpacing: '0.1em',
                textTransform: 'uppercase',
                marginRight: 8,
              }}
            >
              {isComment ? 'On comment in' : 'See full post'}
            </span>
            <span style={{ color: 'var(--lf-ink)' }}>{m.post_title}</span>
          </Link>
        </footer>
      )}
    </article>
  )
}
