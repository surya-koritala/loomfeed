'use client'

import { useState, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import { mapPost } from '../api/mappers'
import { useToast } from '../components/ToastProvider'
import type { PostView } from '../api/types'
import { LFPostCard } from '../components/lf'

// Topic landing page — every post carrying <tag>. The first page is
// server-rendered (initialPosts) so crawlers see real content; humans get
// client-side "load more" + optimistic voting. Vote handling mirrors Home.

interface TagFeedProps {
  tag: string
  initialPosts: unknown[]
  total?: number
}

const PAGE = 25

export default function TagFeed({ tag, initialPosts, total }: TagFeedProps) {
  const router = useRouter()
  const { addToast } = useToast()
  const [posts, setPosts] = useState<PostView[]>(() =>
    Array.isArray(initialPosts) ? initialPosts.map((p) => mapPost(p as any)) : [],
  )
  const [loading, setLoading] = useState(false)
  const [done, setDone] = useState((initialPosts?.length ?? 0) < PAGE)

  const loadMore = useCallback(async () => {
    if (loading || done) return
    setLoading(true)
    try {
      const resp: any = await api.getTagFeed(tag, 'new', PAGE, posts.length)
      const arr = (Array.isArray(resp) ? resp : resp?.data ?? []) ?? []
      const mapped = arr.map((p: any) => mapPost(p))
      setPosts((prev) => {
        const seen = new Set(prev.map((p) => p.id))
        return [...prev, ...mapped.filter((p: PostView) => !seen.has(p.id))]
      })
      if (mapped.length < PAGE) setDone(true)
    } catch {
      setDone(true)
    } finally {
      setLoading(false)
    }
  }, [loading, done, tag, posts.length])

  const handleVote = useCallback(
    async (postId: string, direction: 'up' | 'down') => {
      const token = localStorage.getItem('token')
      if (!token) {
        addToast('Login required to vote', 'info')
        router.push('/login')
        return
      }
      setPosts((prev) =>
        prev.map((p) => {
          if (p.id !== postId) return p
          const prevVote = (p.userVote ?? null) as 'up' | 'down' | null
          const same = prevVote === direction
          const nextVote = same ? null : direction
          const prevDelta = prevVote === 'up' ? 1 : prevVote === 'down' ? -1 : 0
          const nextDelta = nextVote === 'up' ? 1 : nextVote === 'down' ? -1 : 0
          return { ...p, score: p.score - prevDelta + nextDelta, userVote: nextVote }
        }),
      )
      try {
        await api.vote({ target_id: postId, target_type: 'post', direction })
      } catch {
        // leave the optimistic flip in place; the next refresh reconciles
      }
    },
    [addToast, router],
  )

  return (
    <div className="lf-narrow" style={{ padding: '24px 16px 96px' }}>
      <header style={{ marginBottom: 18 }}>
        <div
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 'var(--lf-text-label)',
            fontWeight: 600,
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
            marginBottom: 6,
          }}
        >
          Topic
        </div>
        <h1 className="lf-page-h1">#{tag}</h1>
        <p style={{ marginTop: 8, color: 'var(--lf-muted)', fontSize: 'var(--lf-text-body)' }}>
          {typeof total === 'number' ? `${total} ${total === 1 ? 'post' : 'posts'} tagged ` : 'Posts tagged '}
          <strong style={{ color: 'var(--lf-ink)' }}>{tag}</strong>.
        </p>
      </header>

      {posts.length === 0 ? (
        <div className="lf-empty">No posts tagged {tag} yet.</div>
      ) : (
        <>
          {posts.map((p) => (
            <div key={p.id} style={{ marginBottom: 8 }}>
              <LFPostCard post={p} onVote={handleVote} />
            </div>
          ))}
          {!done && (
            <button
              type="button"
              className="pill-btn"
              onClick={loadMore}
              disabled={loading}
              style={{ margin: '16px auto 0', display: 'block' }}
            >
              {loading ? 'Loading…' : 'Load more'}
            </button>
          )}
        </>
      )}
    </div>
  )
}
