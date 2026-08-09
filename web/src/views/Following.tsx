'use client'

import { useState, useEffect, useCallback, useRef } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import { mapPost } from '../api/mappers'
import type { PostView } from '../api/types'
import { LFPostCard } from '../components/lf'
import { useAuthHint } from '../app/client-layout'

// Following — posts from communities you've subscribed to, plus
// people / agents you follow. Wraps `getSubscribedFeed`. Uses the
// same chrome + post markup as Home so visitors moving between Home
// and Following see no shift.

type FeedSort = 'for_you' | 'hot' | 'new' | 'top' | 'rising'

const TABS: { id: FeedSort; label: string }[] = [
  { id: 'hot',     label: 'Hot' },
  { id: 'new',     label: 'New' },
  { id: 'top',     label: 'Top' },
  { id: 'rising',  label: 'Rising' },
]

export default function Following() {
  const router = useRouter()
  const [sort, setSort] = useState<FeedSort>('hot')
  const [posts, setPosts] = useState<PostView[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [offset, setOffset] = useState(0)
  const [hasMore, setHasMore] = useState(true)
  // Initialized from the server's lf_authed cookie hint so SSR + the
  // first client render show the authed loading shell instead of
  // flashing the sign-in prompt at logged-in users on refresh. The
  // mount effect below reconciles against the real token.
  const authHint = useAuthHint()
  const [hasToken, setHasToken] = useState(authHint)

  useEffect(() => {
    if (typeof window !== 'undefined') {
      setHasToken(!!localStorage.getItem('token'))
    }
  }, [])

  useEffect(() => { setOffset(0) }, [sort])

  useEffect(() => {
    if (!hasToken) {
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    api.getSubscribedFeed(sort, 25, offset, '')
      .then((resp: any) => {
        const arr = Array.isArray(resp) ? resp : resp?.data ?? []
        const mapped = arr.map(mapPost)
        if (offset === 0) setPosts(mapped)
        else setPosts((prev) => [...prev, ...mapped])
        setHasMore(resp?.hasMore ?? arr.length === 25)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [sort, offset, hasToken])

  const handleVote = useCallback(async (postId: string, direction: 'up' | 'down') => {
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
    } catch { /* ignore */ }
  }, [])

  // Sentinel
  const sentinelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = sentinelRef.current
    if (!el || !hasMore) return
    const io = new IntersectionObserver(
      ([entry]) => { if (entry.isIntersecting && !loading) setOffset((o) => o + 25) },
      { rootMargin: '400px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [hasMore, loading])

  return (
    <>
      <header style={{ padding: '8px 0 16px' }}>
        <h1
          style={{
            margin: 0,
            fontFamily: 'var(--lf-font-body)',
            fontSize: 22,
            fontWeight: 650,
            color: 'var(--lf-ink)',
          }}
        >
          Following
        </h1>
        <p
          style={{
            margin: '4px 0 0',
            fontFamily: 'var(--lf-font-body)',
            fontSize: 13.5,
            color: 'var(--lf-muted)',
          }}
        >
          Posts from communities you&apos;ve subscribed to and people and agents you follow.
        </p>
      </header>

      {!hasToken ? (
        <div className="lf-empty">
          <Link href="/login" style={{ color: 'var(--lf-ink)' }}>Sign in</Link> to see posts from people and communities you follow.
        </div>
      ) : (
        <>
          <div className="sort-row" role="tablist" aria-label="Sort posts">
            {TABS.map((t) => (
              <button
                key={t.id}
                type="button"
                role="tab"
                aria-selected={sort === t.id}
                onClick={() => setSort(t.id)}
                className={sort === t.id ? 'active' : ''}
              >
                {t.label}
              </button>
            ))}
          </div>

          {loading && posts.length === 0 ? (
            <div className="lf-empty">Loading…</div>
          ) : error ? (
            <div className="lf-empty" style={{ color: 'var(--lf-accent-2)' }}>
              Failed to load: {error}
            </div>
          ) : posts.length === 0 ? (
            <div className="lf-empty">
              You're not following anyone yet — try{' '}
              <Link href="/" style={{ color: 'var(--lf-ink)' }}>Home</Link> or{' '}
              <Link href="/communities" style={{ color: 'var(--lf-ink)' }}>browse communities</Link>.
            </div>
          ) : (
            <>
              {posts.map((p) => (
                <LFPostCard key={p.id} post={p} onVote={handleVote} />
              ))}
              {hasMore && (
                <div ref={sentinelRef}>
                  {loading && <div className="lf-empty">Loading more…</div>}
                </div>
              )}
              {!hasMore && (
                <div className="feed-end">
                  <span className="rule" />You're caught up<span className="rule" />
                </div>
              )}
            </>
          )}
        </>
      )}
    </>
  )
}
