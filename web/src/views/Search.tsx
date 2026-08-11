'use client'

import React, { useState, useEffect } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { api } from '../api/client'
import { mapPost } from '../api/mappers'
import type { PostView } from '../api/types'
import { LFSearchInput, LFFilterChips, LFPostCard } from '../components/lf'

function FilterRowLabel({ children }: { children: React.ReactNode }) {
  return (
    <span
      style={{
        font: '700 10.5px var(--lf-font-mono)',
        letterSpacing: '0.12em',
        textTransform: 'uppercase',
        color: 'var(--lf-muted)',
        minWidth: 44,
      }}
    >
      {children}
    </span>
  )
}

function FilterChipRow({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  options: ReadonlyArray<{ key: string; label: string }>
}) {
  return (
    <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
      <FilterRowLabel>{label}</FilterRowLabel>
      <LFFilterChips
        mode="single"
        value={value}
        onChange={onChange}
        options={options}
      />
    </div>
  )
}

function RelevanceBar({ score }: { score: number }) {
  const pct = Math.round(score * 100)
  return (
    <div className="lf-relevance" style={{ marginBottom: 4 }}>
      <div className="lf-relevance-bar" style={{ maxWidth: 80 }}>
        <span style={{ width: `${pct}%` }} />
      </div>
      <span>{pct}%</span>
    </div>
  )
}

export default function Search() {
  const searchParams = useSearchParams()
  const query = searchParams.get('q') ?? ''
  const [inputValue, setInputValue] = useState(query)
  useEffect(() => {
    setInputValue(query)
  }, [query])
  const router = useRouter()
  const submitSearch = (q: string) => {
    if (!q.trim()) return
    const params = new URLSearchParams(searchParams.toString())
    params.set('q', q)
    router.push(`/search?${params.toString()}`)
  }

  const [posts, setPosts] = useState<PostView[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [nextCursor, setNextCursor] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [searchMode, setSearchMode] = useState<'hybrid' | 'text'>('hybrid')
  const [community, setCommunity] = useState(searchParams.get('community') ?? '')
  const [authorType, setAuthorType] = useState(searchParams.get('author_type') ?? '')
  const [postType, setPostType] = useState(searchParams.get('post_type') ?? '')
  const [period, setPeriod] = useState(searchParams.get('period') ?? '')
  const [filtersOpen, setFiltersOpen] = useState(false)
  const activeFilterCount = [period, authorType, postType, community].filter(Boolean).length

  // Filter changes update the URL so a filtered search is
  // shareable / bookmarkable. Skip the no-op write when nothing
  // changed (avoids a router.replace per render).
  useEffect(() => {
    if (!query) return
    const params = new URLSearchParams(searchParams.toString())
    const setOrDelete = (k: string, v: string) => {
      if (v) params.set(k, v)
      else params.delete(k)
    }
    setOrDelete('community', community)
    setOrDelete('author_type', authorType)
    setOrDelete('post_type', postType)
    setOrDelete('period', period)
    const next = params.toString()
    if (next !== searchParams.toString()) {
      router.replace(`/search?${next}`, { scroll: false })
    }
  }, [community, authorType, postType, period, query, router, searchParams])

  useEffect(() => {
    if (!query) return

    setLoading(true)
    setError(null)
    api
      .search(query, 25, 0, searchMode, {
        community: community || undefined,
        authorType: authorType || undefined,
        postType: postType || undefined,
        period: period || undefined,
      })
      .then((resp: any) => {
        const items = resp.data ?? []
        const arr = Array.isArray(items) ? items : []
		setPosts(arr.map(mapPost))
		setTotal(resp.total ?? arr.length)
		setNextCursor(resp.nextCursor ?? '')
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [query, searchMode, community, authorType, postType, period])

  const loadMore = async () => {
    if (!nextCursor || loadingMore) return
    setLoadingMore(true)
    setError(null)
    try {
      const resp: any = await api.search(query, 25, 0, searchMode, {
        community: community || undefined,
        authorType: authorType || undefined,
        postType: postType || undefined,
        period: period || undefined,
      }, nextCursor)
      const arr = Array.isArray(resp?.data) ? resp.data.map(mapPost) : []
      setPosts((current) => {
        const seen = new Set(current.map((post) => post.id))
        return [...current, ...arr.filter((post: PostView) => !seen.has(post.id))]
      })
      setNextCursor(resp?.nextCursor ?? '')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Search failed')
    } finally {
      setLoadingMore(false)
    }
  }

  const handleVote = async (postId: string, direction: 'up' | 'down') => {
    try {
      await api.vote({ target_id: postId, target_type: 'post', direction })
      setPosts((prev) =>
        prev.map((p) => {
          if (p.id !== postId) return p
          const undo = direction === p.userVote
          return {
            ...p,
            score: undo
              ? p.score + (direction === 'up' ? -1 : 1)
              : p.score + (direction === 'up' ? 1 : -1),
            userVote: undo ? null : direction,
          }
        })
      )
    } catch {
      // ignore vote errors
    }
  }

  return (
    <div>
      <div>
      {/* LF masthead + search input */}
      <div style={{ marginBottom: 20 }}>
        <h1
          style={{
            fontFamily: 'var(--lf-font-display)',
            fontWeight: 800,
            fontSize: 32,
            letterSpacing: '-0.03em',
            color: 'var(--lf-ink)',
            margin: '0 0 16px',
          }}
        >
          Search
        </h1>
        <LFSearchInput
          value={inputValue}
          onChange={setInputValue}
          onSubmit={submitSearch}
          placeholder="Search posts, contributors, communities…"
          autoFocus={!query}
        />
      </div>

      {query && (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16, marginTop: 16, marginBottom: 16, flexWrap: 'wrap' }}>
          <LFFilterChips
            mode="single"
            value={searchMode}
            onChange={(k) => setSearchMode(k as 'hybrid' | 'text')}
            options={[
              { key: 'hybrid', label: 'Hybrid' },
              { key: 'text', label: 'Text' },
            ]}
          />
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 11,
              color: 'var(--lf-muted)',
              letterSpacing: '0.06em',
              textTransform: 'uppercase',
            }}
          >
            {loading
              ? 'Searching…'
              : `${total} result${total !== 1 ? 's' : ''} · ${searchMode === 'hybrid' ? 'ranked by reciprocal rank fusion' : 'text match'}`}
          </div>
        </div>
      )}

      {/* Mobile-only disclosure for the filter rows below — hidden on
          desktop via .lf-search-filters-toggle. */}
      {query && (
        <button
          type="button"
          className="lf-search-filters-toggle"
          aria-expanded={filtersOpen}
          onClick={() => setFiltersOpen((o) => !o)}
          style={{
            font: '600 11.5px var(--lf-font-mono)',
            padding: '8px 12px',
            minHeight: 36,
            borderRadius: 999,
            border: '1px solid var(--lf-rule-mid)',
            background: filtersOpen ? 'var(--lf-gray-50)' : 'transparent',
            color: 'var(--lf-muted)',
            letterSpacing: '0.06em',
            textTransform: 'uppercase',
            cursor: 'pointer',
            marginBottom: 12,
          }}
        >
          Filters{activeFilterCount > 0 ? ` · ${activeFilterCount}` : ''}
        </button>
      )}

      {/* Filter chips. Toggling a chip writes "" → state, which the
          URL-sync effect picks up and propagates. A small "any"
          chip per row is the explicit clear; the trailing "Clear
          all" only appears when something is set. */}
      {query && (
        <div className={'lf-search-filters' + (filtersOpen ? ' open' : '')}>
          <FilterChipRow
            label="When"
            value={period}
            onChange={setPeriod}
            options={[
              { key: '', label: 'Any time' },
              { key: 'day', label: '24h' },
              { key: 'week', label: 'Week' },
              { key: 'month', label: 'Month' },
              { key: 'year', label: 'Year' },
            ]}
          />
          <FilterChipRow
            label="Author"
            value={authorType}
            onChange={setAuthorType}
            options={[
              { key: '', label: 'Anyone' },
              { key: 'agent', label: 'Programmatic' },
              { key: 'human', label: 'People' },
            ]}
          />
          <FilterChipRow
            label="Type"
            value={postType}
            onChange={setPostType}
            options={[
              { key: '', label: 'All' },
              { key: 'text', label: 'Text' },
              { key: 'link', label: 'Link' },
              { key: 'synthesis', label: 'Synthesis' },
              { key: 'debate', label: 'Debate' },
              { key: 'question', label: 'Question' },
              { key: 'code_review', label: 'Code review' },
            ]}
          />
          <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
            <FilterRowLabel>In</FilterRowLabel>
            <input
              type="text"
              value={community}
              onChange={e => setCommunity(e.target.value)}
              placeholder="community slug…"
              style={{
                font: '500 13px var(--lf-font-body)',
                padding: '8px 14px', minHeight: 36,
                borderRadius: 999,
                border: '1px solid var(--lf-rule-mid)',
                background: 'var(--lf-paper)',
                color: 'var(--lf-ink)',
                width: 180,
              }}
            />
            {activeFilterCount > 0 && (
              <button
                type="button"
                onClick={() => { setPeriod(''); setAuthorType(''); setPostType(''); setCommunity('') }}
                style={{
                  font: '600 11.5px var(--lf-font-mono)',
                  padding: '8px 12px', minHeight: 36,
                  borderRadius: 999,
                  border: '1px solid var(--lf-rule-mid)',
                  background: 'transparent',
                  color: 'var(--lf-muted)',
                  letterSpacing: '0.06em',
                  textTransform: 'uppercase',
                  cursor: 'pointer',
                }}
              >
                Clear all
              </button>
            )}
          </div>
        </div>
      )}

      {/* Loading skeleton */}
      {loading && (
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          {[...Array(5)].map((_, i) => (
            <div key={i} style={{ padding: '20px 0', borderBottom: 'var(--lf-border-w) solid var(--lf-rule-soft)' }}>
              <div style={{ display: 'flex', gap: 6, marginBottom: 10 }}>
                <div className="skeleton" style={{ width: 80, height: 12 }} />
                <div className="skeleton skeleton-avatar" />
                <div className="skeleton" style={{ width: 60, height: 12 }} />
              </div>
              <div className="skeleton skeleton-title" />
              <div className="skeleton skeleton-text" style={{ width: '85%' }} />
            </div>
          ))}
        </div>
      )}

      {/* Error */}
      {error && (
        <div className="lf-empty" style={{ color: 'var(--lf-accent-2)' }}>
          Search failed: {error}
        </div>
      )}

      {/* Empty state */}
      {!loading && !error && query && posts.length === 0 && (
        <div className="lf-empty">
          No results found for &ldquo;{query}&rdquo;. Try a different search term.
        </div>
      )}

      {/* Results */}
      {!loading &&
        posts.map((post) => (
          <div key={post.id}>
            {post.relevanceScore != null && post.relevanceScore > 0 && (
              <RelevanceBar score={post.relevanceScore} />
            )}
            <LFPostCard post={post} onVote={(_id, dir) => handleVote(post.id, dir)} />
          </div>
        ))}
      {!loading && nextCursor && (
        <div style={{ display: 'flex', justifyContent: 'center', padding: '24px 0' }}>
          <button type="button" className="lf-btn lf-btn-secondary" disabled={loadingMore} onClick={loadMore}>
            {loadingMore ? 'Loading…' : 'Load more results'}
          </button>
        </div>
      )}
      </div>

    </div>
  )
}
