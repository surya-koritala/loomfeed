'use client'

import { useState, useEffect, useMemo } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import { mapCommunity } from '../api/mappers'
import type { CommunityView } from '../api/types'
import { LFCommunityCard } from '../components/lf'

// The eight discovery buckets, in display order. Keep in sync with
// migration 000068 and the create page's category dropdown. Anything
// outside this list lands in 'other'.
const CATEGORIES: { id: string; label: string }[] = [
  { id: '',          label: 'All' },
  { id: 'tech',      label: 'Tech' },
  { id: 'science',   label: 'Science' },
  { id: 'culture',   label: 'Culture' },
  { id: 'society',   label: 'Society' },
  { id: 'lifestyle', label: 'Lifestyle' },
  { id: 'mind',      label: 'Mind' },
  { id: 'business',  label: 'Business' },
  { id: 'meta',      label: 'Meta' },
]

type Tab = 'explore' | 'joined'
type SortMode = 'subscribers' | 'trending' | 'new' | 'alphabetical'

const SORT_OPTIONS: { value: SortMode; label: string }[] = [
  { value: 'subscribers',  label: 'Popular' },
  { value: 'trending',     label: 'Trending' },
  { value: 'new',          label: 'New' },
  { value: 'alphabetical', label: 'A–Z' },
]

// 14-day window for the "NEW" pill. After that the community
// has to stand on its own activity.
const NEW_BADGE_DAYS = 14

function isNewCommunity(c: CommunityView): boolean {
  if (!c.createdAt) return false
  const created = new Date(c.createdAt).getTime()
  if (Number.isNaN(created)) return false
  const ageDays = (Date.now() - created) / (1000 * 60 * 60 * 24)
  return ageDays <= NEW_BADGE_DAYS
}

const COMMUNITY_ACCENTS: Record<string, string> = {
  tech:      '#6366f1',
  science:   '#10b981',
  culture:   '#f59e0b',
  society:   '#ef4444',
  lifestyle: '#ec4899',
  mind:      '#8b5cf6',
  business:  '#14b8a6',
  meta:      '#71717a',
  other:     '#71717a',
}

export default function Discover() {
  const router = useRouter()

  const [allCommunities, setAllCommunities] = useState<CommunityView[]>([])
  const [trendingCommunities, setTrendingCommunities] = useState<CommunityView[]>([])
  const [newCommunities, setNewCommunities] = useState<CommunityView[]>([])
  const [loading, setLoading] = useState(true)

  const [search, setSearch] = useState('')
  const [tab, setTab] = useState<Tab>('explore')
  const [category, setCategory] = useState<string>('')
  const [sort, setSort] = useState<SortMode>('subscribers')

  const [subscribing, setSubscribing] = useState<string | null>(null)
  const [subscribed, setSubscribed] = useState<Set<string>>(new Set())

  // Load all three slices in parallel — main list, trending rail,
  // new rail. Each is a separate API call because the backend
  // returns different sort orders / filters per shape.
  useEffect(() => {
    setLoading(true)
    Promise.all([
      api.getCommunities({ limit: 500 }),
      api.getCommunities({ sort: 'trending', limit: 8 }),
      api.getCommunities({ sort: 'new', limit: 8 }),
    ])
      .then(async ([all, trending, fresh]) => {
        const allArr = Array.isArray(all) ? all : []
        const trendingArr = Array.isArray(trending) ? trending : []
        const freshArr = Array.isArray(fresh) ? fresh : []
        setAllCommunities(allArr.map(mapCommunity))
        setTrendingCommunities(trendingArr.map(mapCommunity))
        setNewCommunities(freshArr.map(mapCommunity))

        // Subscription status — only fetch once for the union of
        // visible communities. The previous code fired N parallel
        // requests on every page load, which got slow at 64
        // communities and would be unusable at 300+. The dedicated
        // /communities/subscriptions endpoint is one round trip.
        if (localStorage.getItem('token')) {
          try {
            const subs = await api.getSubscribedCommunities() as any[]
            const subSet = new Set<string>(
              (Array.isArray(subs) ? subs : []).map((s: any) => s.slug as string)
            )
            setSubscribed(subSet)
          } catch {}
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const filtered = useMemo(() => {
    const base = tab === 'joined'
      ? allCommunities.filter((c) => subscribed.has(c.slug))
      : allCommunities

    let res = base
    if (category) {
      res = res.filter((c) => c.category === category)
    }
    if (search.trim()) {
      const q = search.trim().toLowerCase()
      res = res.filter(
        (c) =>
          c.name.toLowerCase().includes(q) ||
          c.slug.toLowerCase().includes(q) ||
          (c.description ?? '').toLowerCase().includes(q),
      )
    }

    const sorted = [...res]
    switch (sort) {
      case 'trending':
        sorted.sort((a, b) => {
          const at = a.lastPostAt ? new Date(a.lastPostAt).getTime() : 0
          const bt = b.lastPostAt ? new Date(b.lastPostAt).getTime() : 0
          if (bt !== at) return bt - at
          return b.memberCount - a.memberCount
        })
        break
      case 'new':
        sorted.sort((a, b) => {
          const at = a.createdAt ? new Date(a.createdAt).getTime() : 0
          const bt = b.createdAt ? new Date(b.createdAt).getTime() : 0
          return bt - at
        })
        break
      case 'alphabetical':
        sorted.sort((a, b) => a.name.localeCompare(b.name))
        break
      default:
        sorted.sort((a, b) => b.memberCount - a.memberCount)
    }
    return sorted
  }, [allCommunities, subscribed, tab, category, search, sort])

  const handleSubscribe = async (slug: string) => {
    const token = localStorage.getItem('token')
    if (!token) {
      router.push('/login')
      return
    }
    setSubscribing(slug)
    try {
      const isJoined = subscribed.has(slug)
      const method = isJoined ? 'DELETE' : 'POST'
      await fetch(`/api/v1/communities/${slug}/subscribe`, {
        method,
        credentials: 'include',
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
      })
      setSubscribed((prev) => {
        const next = new Set(prev)
        if (isJoined) next.delete(slug)
        else next.add(slug)
        return next
      })
    } catch {
      // ignore
    } finally {
      setSubscribing(null)
    }
  }

  const totalMembers = allCommunities.reduce((acc, c) => acc + (c.memberCount ?? 0), 0)

  return (
    <div className="lf-discover">
      {/* Header — vertical stack on mobile per CLAUDE.md mobile-first
          rule. The previous flex-row with two CTAs collapsed badly at
          375px (button text wrapping mid-word). */}
      <header className="lf-disc-header">
        <div className="lf-disc-headcol">
          <h1 className="lf-text-display lf-disc-title">Communities</h1>
          <div className="lf-text-caption lf-disc-stats">
            {allCommunities.length} {allCommunities.length === 1 ? 'community' : 'communities'}
            {totalMembers > 0 && ` · ${totalMembers.toLocaleString()} members`}
          </div>
        </div>
        <div className="lf-disc-cta-row">
          <Link href="/communities/create" className="lf-disc-cta-primary">
            + New community
          </Link>
        </div>
      </header>

      {/* Search bar — full-width, prominent. Discovery starts here. */}
      <input
        id="community-search"
        type="text"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        placeholder="Search communities by name, slug, or topic…"
        className="lf-disc-search"
      />

      {/* Tabs — Joined / Explore. Hide Joined when not signed in.
          (subscribed.size === 0 covers both not-signed-in and
          signed-in-with-zero-subs.) */}
      <div className="lf-disc-tabs" role="tablist">
        <button
          role="tab"
          aria-selected={tab === 'explore'}
          onClick={() => setTab('explore')}
          className={`lf-disc-tab ${tab === 'explore' ? 'is-active' : ''}`}
        >
          Explore
        </button>
        <button
          role="tab"
          aria-selected={tab === 'joined'}
          onClick={() => setTab('joined')}
          className={`lf-disc-tab ${tab === 'joined' ? 'is-active' : ''}`}
          disabled={subscribed.size === 0}
          title={subscribed.size === 0 ? 'Join a community to see it here' : undefined}
        >
          Joined {subscribed.size > 0 && `(${subscribed.size})`}
        </button>
      </div>

      {/* Category chips — horizontal scroll on mobile so 9 chips
          don't wrap to four lines. */}
      <div className="lf-disc-cats">
        {CATEGORIES.map((c) => (
          <button
            key={c.id || 'all'}
            onClick={() => setCategory(c.id)}
            className={`lf-disc-cat ${category === c.id ? 'is-active' : ''}`}
          >
            {c.label}
          </button>
        ))}
      </div>

      {/* Sort row — compact pill group below chips. */}
      <div className="lf-disc-sort-row">
        <span className="lf-disc-sort-label">Sort</span>
        <div className="lf-disc-sort">
          {SORT_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              onClick={() => setSort(opt.value)}
              className={`lf-disc-sort-btn ${sort === opt.value ? 'is-active' : ''}`}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      {/* Trending + New rails — only show on Explore tab with no
          search/category active, so the user has a quick "what's
          happening right now" view above the full grid. */}
      {!loading && tab === 'explore' && !search && !category && trendingCommunities.length > 0 && (
        <>
          <Rail title="Trending now" subtitle="Communities with the most recent activity" communities={trendingCommunities.slice(0, 6)} subscribed={subscribed} onSubscribe={handleSubscribe} subscribing={subscribing} />
          {newCommunities.length > 0 && (
            <Rail title="New & growing" subtitle="Created in the last 30 days" communities={newCommunities.slice(0, 6)} subscribed={subscribed} onSubscribe={handleSubscribe} subscribing={subscribing} />
          )}
        </>
      )}

      {/* Loading skeletons — 6 boxes matching the grid. */}
      {loading && (
        <div className="lf-disc-grid">
          {[...Array(6)].map((_, i) => (
            <div key={i} className="lf-disc-skel" />
          ))}
        </div>
      )}

      {!loading && filtered.length === 0 && (
        <div className="lf-disc-empty">
          {tab === 'joined' ? (
            <>
              <p className="lf-disc-empty-msg">You haven&apos;t joined any communities yet.</p>
              <button onClick={() => setTab('explore')} className="lf-disc-empty-cta">
                Explore communities
              </button>
            </>
          ) : (
            <>
              <p className="lf-disc-empty-msg">
                {search ? `No communities match "${search}".` : 'No communities in this category yet.'}
              </p>
              <Link href="/communities/create" className="lf-disc-empty-cta">
                Create one
              </Link>
            </>
          )}
        </div>
      )}

      {!loading && filtered.length > 0 && (
        <>
          <div className="lf-disc-result-meta">
            {filtered.length} {filtered.length === 1 ? 'community' : 'communities'}
            {category && ` in ${CATEGORIES.find(c => c.id === category)?.label}`}
            {search && ` matching "${search}"`}
          </div>
          <div className="lf-disc-grid">
            {filtered.map((c) => {
              const accent = COMMUNITY_ACCENTS[c.category ?? 'other'] ?? COMMUNITY_ACCENTS.other
              const isSubscribed = subscribed.has(c.slug)
              const isNew = isNewCommunity(c)
              return (
                <div key={c.slug} className="lf-disc-card-wrap">
                  <LFCommunityCard
                    community={{
                      slug: c.slug,
                      name: c.name,
                      description: c.description,
                      memberCount: c.memberCount,
                      agentPolicy: c.agentPolicy,
                    }}
                    accent={accent}
                    subscribed={isSubscribed}
                    subscribePending={subscribing === c.slug}
                    onSubscribeToggle={() => handleSubscribe(c.slug)}
                  />
                  {isNew && <span className="lf-disc-newbadge">NEW</span>}
                </div>
              )
            })}
          </div>
        </>
      )}
    </div>
  )
}

// Trending / New rail — horizontal scroller on mobile, grid row
// on desktop. Cards are slimmer than the main grid so the rail
// reads as a "look here too" tease, not the main directory.
function Rail({
  title,
  subtitle,
  communities,
  subscribed,
  onSubscribe,
  subscribing,
}: {
  title: string
  subtitle?: string
  communities: CommunityView[]
  subscribed: Set<string>
  onSubscribe: (slug: string) => void
  subscribing: string | null
}) {
  if (communities.length === 0) return null
  return (
    <section className="lf-disc-rail">
      <div className="lf-disc-rail-head">
        <h2 className="lf-disc-rail-title">{title}</h2>
        {subtitle && <span className="lf-disc-rail-sub">{subtitle}</span>}
      </div>
      <div className="lf-disc-rail-scroll">
        {communities.map((c) => {
          const accent = COMMUNITY_ACCENTS[c.category ?? 'other'] ?? COMMUNITY_ACCENTS.other
          const isSubscribed = subscribed.has(c.slug)
          return (
            <div key={c.slug} className="lf-disc-rail-card">
              <LFCommunityCard
                community={{
                  slug: c.slug,
                  name: c.name,
                  description: c.description,
                  memberCount: c.memberCount,
                  agentPolicy: c.agentPolicy,
                }}
                accent={accent}
                subscribed={isSubscribed}
                subscribePending={subscribing === c.slug}
                onSubscribeToggle={() => onSubscribe(c.slug)}
              />
            </div>
          )
        })}
      </div>
    </section>
  )
}
