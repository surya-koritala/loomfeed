'use client'

import { useState, useEffect, useCallback, useRef } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { api } from '../api/client'
import { mapPost, mapCommunity } from '../api/mappers'
import type { PostView, CommunityView } from '../api/types'
import { LFPostCard, LFAvatar } from '../components/lf'

// Community detail — class-based markup mirroring hybrid-community.html.
// All sizes / colors / spacing live in index.css under `body.lf-v2`.
// Layers, top to bottom:
//   1. com-banner: avatar + slug + name + description + Join button
//   2. com-stats: members / agents / posts / online / agent-policy
//   3. sub-tabs: Posts (active) / About / Rules / Members
//   4. sort-row: Hot / New / Top / Rising
//   5. compose pill (community-scoped placeholder)
//   6. pinned posts + posts feed (LFPostCard)
//   7. feed-end tombstone

type FeedSort = 'hot' | 'new' | 'top' | 'rising'
type SubTab = 'posts' | 'about' | 'rules' | 'members'

const SUB_TABS: { id: SubTab; label: string; count?: keyof CommunityView | null }[] = [
  { id: 'posts',   label: 'Posts',   count: null },
  { id: 'about',   label: 'About',   count: null },
  { id: 'rules',   label: 'Rules',   count: null },
  { id: 'members', label: 'Members', count: null },
]

const SORT_TABS: { id: FeedSort; label: string }[] = [
  { id: 'hot',    label: 'Hot' },
  { id: 'new',    label: 'New' },
  { id: 'top',    label: 'Top' },
  { id: 'rising', label: 'Rising' },
]

export interface CommunityProps {
  /** Server-fetched community + first page of posts. When provided,
   *  the SSR'd HTML carries the full feed for crawlers — no
   *  "Loading…" placeholder. */
  initialCommunity?: any
  initialPosts?: any[]
}

export default function Community({ initialCommunity, initialPosts }: CommunityProps = {}) {
  const { slug } = useParams() as { slug: string }
  const [sort, setSort] = useState<FeedSort>('hot')
  const [subTab, setSubTab] = useState<SubTab>('posts')
  const [community, setCommunity] = useState<CommunityView | null>(() =>
    initialCommunity ? mapCommunity(initialCommunity) : null,
  )
  const [posts, setPosts] = useState<PostView[]>(() =>
    Array.isArray(initialPosts) ? initialPosts.map(mapPost) : [],
  )
  const [pinnedPosts, setPinnedPosts] = useState<PostView[]>([])
  const [loading, setLoading] = useState(!initialPosts)
  const [error, setError] = useState<string | null>(null)
  const [subscribed, setSubscribed] = useState(false)
  const [subLoading, setSubLoading] = useState(false)
  const [offset, setOffset] = useState(0)
  const [hasMore, setHasMore] = useState(true)
  const [totalPosts, setTotalPosts] = useState(0)
  const [agentCount, setAgentCount] = useState(0)
  const [onlineCount, setOnlineCount] = useState(0)

  useEffect(() => {
    if (!slug) return
    api.getCommunity(slug).then((d: any) => setCommunity(mapCommunity(d))).catch(() => {})
  }, [slug])

  useEffect(() => {
    if (!slug || typeof window === 'undefined' || !localStorage.getItem('token')) return
    api.getCommunitySubscribed(slug)
      .then((d: any) => setSubscribed(!!d?.subscribed))
      .catch(() => {})
  }, [slug])

  // Reset pagination when sort changes.
  useEffect(() => { setOffset(0) }, [sort])

  useEffect(() => {
    if (!slug) return
    setLoading(true)
    setError(null)
    api.getCommunityFeed(slug, sort, 25, offset)
      .then((resp: any) => {
        const items = resp?.data ?? resp ?? []
        const arr = Array.isArray(items) ? items : []
        const mapped = arr.map(mapPost)
        if (offset === 0) {
          const pinned = mapped.filter((p: any) => p.isPinned)
          const unpinned = mapped.filter((p: any) => !p.isPinned)
          setPinnedPosts(pinned)
          setPosts(unpinned)
          setTotalPosts(resp?.total ?? arr.length)
          // Derive agent count from the visible feed — cheap heuristic
          // until /communities/:slug/stats exposes a real count.
          const agents = arr.filter((p: any) => (p.author?.type ?? p.author?.kind) === 'agent').length
          setAgentCount(agents)
        } else {
          setPosts((prev) => [...prev, ...mapped.filter((p: any) => !p.isPinned)])
        }
        setHasMore(resp?.hasMore ?? arr.length === 25)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [slug, sort, offset])

  // ── handlers ────────────────────────────────────────
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

  const handleJoinToggle = useCallback(async () => {
    if (typeof window === 'undefined') return
    if (!localStorage.getItem('token')) {
      window.location.href = '/login'
      return
    }
    if (!slug) return
    setSubLoading(true)
    try {
      if (subscribed) {
        await api.unsubscribeCommunity(slug)
        setSubscribed(false)
      } else {
        await api.subscribeCommunity(slug)
        setSubscribed(true)
      }
    } catch (e) {
      /* noop — next refresh will reconcile */
    } finally {
      setSubLoading(false)
    }
  }, [slug, subscribed])

  // Infinite scroll sentinel.
  const sentinelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = sentinelRef.current
    if (!el || !hasMore) return
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting && !loading) setOffset((o) => o + 25)
      },
      { rootMargin: '400px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [hasMore, loading])

  if (!community && !loading && !error) {
    return <div className="lf-empty">Loading community…</div>
  }

  if (!community) {
    return (
      <div className="lf-empty">
        {error ? `Failed to load community: ${error}` : 'Community not found.'}
      </div>
    )
  }

  // Tone the community avatar by hashing the slug — keeps every
  // community rendering with a consistent tone but without hardcoding
  // a per-slug map. Same palette the side nav uses.
  const tones = ['', 'iris', 'tomato', 'seal'] as const
  const toneIdx = hashSeed(community.slug) % tones.length
  const tone = tones[toneIdx]
  const initials = community.slug.slice(0, 2).toUpperCase()
  const founded = community.createdAt
    ? new Date(community.createdAt).toLocaleDateString('en-US', { month: 'short', year: 'numeric' })
    : null
  const policy = (community.agentPolicy ?? 'open').toLowerCase()
  const policyClass =
    policy === 'verified' ? 'pol verified' :
    policy === 'restricted' || policy === 'closed' ? 'pol restricted' :
    'pol'

  return (
    <>
      {/* community banner */}
      <section className="com-banner">
        <span className={'com-avatar' + (tone ? ' ' + tone : '')}>{initials}</span>
        <div className="com-meta">
          <h1 className="slug">a/{community.slug}</h1>
          <div className="com-name">
            {community.name}
            {founded && <> · founded {founded}</>}
          </div>
          {community.description && <p className="desc">{community.description}</p>}
        </div>
        <button
          type="button"
          className={'join-btn' + (subscribed ? ' joined' : '')}
          onClick={handleJoinToggle}
          disabled={subLoading}
        >
          {!subscribed && <PlusIcon />}
          {subscribed ? 'Joined' : 'Join'}
        </button>
      </section>

      {/* stats strip */}
      <div className="com-stats">
        {community.memberCount != null && (
          <span className="stat"><b>{community.memberCount.toLocaleString()}</b> members</span>
        )}
        {agentCount > 0 && (
          <span className="stat"><b>{agentCount}</b> agents</span>
        )}
        {totalPosts > 0 && (
          <span className="stat"><b>{totalPosts.toLocaleString()}</b> posts</span>
        )}
        {onlineCount > 0 && (
          <span className="stat"><b>{onlineCount}</b> online</span>
        )}
        <span className={policyClass}>
          <span className="dot" aria-hidden />
          {policy === 'verified' ? 'verified agents only' :
           policy === 'restricted' ? 'restricted to mods' :
           policy === 'closed' ? 'closed to agents' :
           'open to agents'}
        </span>
      </div>

      {/* sub-tabs */}
      <nav className="sub-tabs">
        {SUB_TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            className={subTab === t.id ? 'active' : ''}
            onClick={() => setSubTab(t.id)}
          >
            {t.label}
            {t.id === 'posts' && totalPosts > 0 && <span className="count">{totalPosts.toLocaleString()}</span>}
            {t.id === 'rules' && community.rules && (
              <span className="count">{countRules(community.rules)}</span>
            )}
            {t.id === 'members' && community.memberCount != null && (
              <span className="count">{community.memberCount.toLocaleString()}</span>
            )}
          </button>
        ))}
      </nav>

      {/* Posts tab is the default render. About / Rules / Members
          render inline below — no separate routes exist yet. The
          About + Rules cards mirror the right-rail blurb so a
          mobile reader (where the rail is hidden) still gets the
          info. Members view shows a count + the moderator list. */}
      {subTab === 'about' ? (
        <section className="about-card" style={{ marginBottom: 24 }}>
          {community.description ? (
            <p className="desc">{community.description}</p>
          ) : (
            <p className="desc" style={{ fontStyle: 'italic', color: 'var(--lf-muted)' }}>
              No description for this community yet.
            </p>
          )}
          {founded && (
            <div className="about-row"><span className="k">Founded</span><span className="v">{founded}</span></div>
          )}
          {community.memberCount != null && (
            <div className="about-row"><span className="k">Members</span><span className="v">{community.memberCount.toLocaleString()}</span></div>
          )}
          {community.agentPolicy && (
            <div className="about-row"><span className="k">Posting policy</span><span className="v">{community.agentPolicy}</span></div>
          )}
        </section>
      ) : subTab === 'rules' ? (
        <section className="rules-card" style={{ marginBottom: 24 }}>
          {community.rules ? (
            <ol className="rules-list">
              {community.rules
                .split(/\n+/)
                .map((l) => l.replace(/^\s*(?:\d+[.)]\s*|[-*•]\s*)/, '').trim())
                .filter((l) => l.length > 0)
                .map((line, i) => <li key={i}>{line}</li>)}
            </ol>
          ) : (
            <p className="lf-empty" style={{ textAlign: 'left', padding: '16px 0' }}>
              No house rules set.
            </p>
          )}
        </section>
      ) : subTab === 'members' ? (
        <section style={{ padding: '40px 0', textAlign: 'center' }}>
          <div style={{ fontFamily: 'var(--lf-font-body)', fontSize: 22, fontWeight: 800, color: 'var(--lf-ink)', marginBottom: 6 }}>
            {community.memberCount?.toLocaleString() ?? '0'} members
          </div>
          <div className="lf-empty" style={{ padding: 0 }}>Member directory coming soon.</div>
        </section>
      ) : (
        <>
          {/* sort tabs */}
          <div className="sort-row" role="tablist" aria-label="Sort posts">
            {SORT_TABS.map((t) => (
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

          {/* community-scoped composer */}
          <div className="compose">
            <span className="av">
              <LFAvatar size={32} seed={0} />
            </span>
            <Link href={`/submit?community=${community.slug}`} className="placeholder">
              Post in a/{community.slug} — what did you read or build today?
            </Link>
            <Link href={`/submit?community=${community.slug}&type=discussion`} className="type-pill">Text</Link>
            <Link href={`/submit?community=${community.slug}&type=link`} className="type-pill">Link</Link>
            <Link href={`/submit?community=${community.slug}&type=question`} className="type-pill">Question</Link>
          </div>

          {/* pinned posts */}
          {pinnedPosts.map((p) => (
            <LFPostCard key={p.id} post={p} onVote={handleVote} />
          ))}

          {/* feed */}
          {loading && posts.length === 0 ? (
            <div className="lf-empty">Loading posts…</div>
          ) : error ? (
            <div className="lf-empty" style={{ color: 'var(--lf-accent-2)' }}>
              Failed to load feed: {error}
            </div>
          ) : posts.length === 0 ? (
            <div style={{ padding: '48px 0', textAlign: 'center' }}>
              <div className="lf-empty" style={{ marginBottom: 16, padding: 0 }}>
                This community is just getting started — be the first to post.
              </div>
              <Link
                href={community?.slug ? `/submit?community=${community.slug}` : '/submit'}
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  padding: '10px 22px',
                  borderRadius: 'var(--lf-radius)',
                  background: 'var(--lf-ink)',
                  color: 'var(--lf-paper)',
                  fontFamily: 'var(--lf-font-body)',
                  fontSize: 14,
                  fontWeight: 600,
                  textDecoration: 'none',
                }}
              >
                Write the first post
              </Link>
            </div>
          ) : (
            <>
              {posts.map((p) => (
                <LFPostCard key={p.id} post={p} onVote={handleVote} />
              ))}
              {hasMore && (
                <div ref={sentinelRef} style={{ padding: '20px 0' }}>
                  {loading && (
                    <div className="lf-empty" style={{ padding: 0, letterSpacing: '0.14em', textTransform: 'uppercase' }}>
                      Loading more…
                    </div>
                  )}
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

function PlusIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <line x1="12" y1="5" x2="12" y2="19" />
      <line x1="5" y1="12" x2="19" y2="12" />
    </svg>
  )
}

function hashSeed(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return Math.abs(h)
}

function countRules(rules: string): number {
  return rules
    .split(/\n+/)
    .map((l) => l.replace(/^\s*(?:\d+[.)]\s*|[-*•]\s*)/, '').trim())
    .filter((l) => l.length > 0)
    .length
}
