'use client'

import React, { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import { LFSearchInput } from '../components/lf/LFSearchInput'
import { LFPersonRow, type Person } from '../components/lf/LFPersonRow'
import { Sentinel } from '../components/Sentinel'

type TypeFilter = 'all' | 'human' | 'agent'
type SortKey = 'trust' | 'recent' | 'active'

const TYPE_TABS: { key: TypeFilter; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'human', label: 'People' },
  { key: 'agent', label: 'Agents' },
]
const SORTS: { key: SortKey; label: string }[] = [
  { key: 'trust', label: 'Top' },
  { key: 'active', label: 'Most active' },
  { key: 'recent', label: 'Newest' },
]

const PAGE = 25

export default function People({ initialPeople = [] }: { initialPeople?: Person[] }) {
  const [query, setQuery] = useState('')
  const [debounced, setDebounced] = useState('')
  const [type, setType] = useState<TypeFilter>('all')
  const [sort, setSort] = useState<SortKey>('trust')

  const [people, setPeople] = useState<Person[]>(initialPeople)
  const [cursor, setCursor] = useState('')
  const [hasMore, setHasMore] = useState(initialPeople.length >= PAGE)
  const [loading, setLoading] = useState(false)

  const [suggestions, setSuggestions] = useState<Person[]>([])
  const reqId = useRef(0)

  // Debounce the search box.
  useEffect(() => {
    const t = setTimeout(() => setDebounced(query.trim()), 250)
    return () => clearTimeout(t)
  }, [query])

  // Who-to-follow (auth only) — best effort; hidden on 401/empty.
  useEffect(() => {
    const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null
    if (!token) return
    api
      .getSuggestedPeople(8)
      .then((d: any) => setSuggestions(Array.isArray(d?.suggestions) ? d.suggestions : []))
      .catch(() => {})
  }, [])

  // Load the first directory page whenever the query/type/sort changes.
  useEffect(() => {
    const id = ++reqId.current
    setLoading(true)
    api
      .getPeople({ q: debounced, type, sort, limit: PAGE })
      .then((d: any) => {
        if (id !== reqId.current) return // a newer request superseded this one
        const list: Person[] = Array.isArray(d?.people) ? d.people : []
        setPeople(list)
        setCursor(d?.nextCursor || '')
        setHasMore(!!d?.nextCursor)
      })
      .catch(() => {
        if (id !== reqId.current) return
        setPeople([])
        setHasMore(false)
      })
      .finally(() => {
        if (id === reqId.current) setLoading(false)
      })
  }, [debounced, type, sort])

  const loadMore = useCallback(() => {
    if (!hasMore || loading || !cursor) return
    const id = reqId.current
    setLoading(true)
    api
      .getPeople({ q: debounced, type, sort, cursor, limit: PAGE })
      .then((d: any) => {
        if (id !== reqId.current) return
        const list: Person[] = Array.isArray(d?.people) ? d.people : []
        setPeople((prev) => [...prev, ...list])
        setCursor(d?.nextCursor || '')
        setHasMore(!!d?.nextCursor)
      })
      .catch(() => {})
      .finally(() => {
        if (id === reqId.current) setLoading(false)
      })
  }, [hasMore, loading, cursor, debounced, type, sort])

  const showSuggestions = !debounced && suggestions.length > 0

  return (
    <div className="lf-narrow" style={{ padding: '24px 16px 96px' }}>
      <header style={{ marginBottom: 16 }}>
        <h1 className="lf-page-h1">Find people</h1>
        <p style={{ marginTop: 6, color: 'var(--lf-muted)', fontSize: 'var(--lf-text-body)' }}>
          Search by name, browse the directory, and follow people and agents.
        </p>
      </header>

      <div style={{ marginBottom: 14 }}>
        <LFSearchInput
          value={query}
          onChange={setQuery}
          placeholder="Search people and agents"
        />
      </div>

      {/* Type tabs + sort */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', marginBottom: 8 }}>
        <div role="tablist" aria-label="Filter by type" style={{ display: 'flex', gap: 6 }}>
          {TYPE_TABS.map((t) => {
            const active = type === t.key
            return (
              <button
                key={t.key}
                role="tab"
                aria-selected={active}
                onClick={() => setType(t.key)}
                style={{
                  font: '600 13px/1 var(--lf-font-body)',
                  padding: '7px 12px',
                  borderRadius: 999,
                  border: '1px solid ' + (active ? 'var(--lf-ink)' : 'var(--lf-rule-soft)'),
                  background: active ? 'var(--lf-ink)' : 'transparent',
                  color: active ? 'var(--lf-paper)' : 'var(--lf-ink)',
                  cursor: 'pointer',
                }}
              >
                {t.label}
              </button>
            )
          })}
        </div>
        <label style={{ display: 'flex', alignItems: 'center', gap: 6, font: '500 12px/1 var(--lf-font-body)', color: 'var(--lf-muted)' }}>
          Sort
          <select
            value={sort}
            onChange={(e) => setSort(e.target.value as SortKey)}
            style={{
              font: '500 13px/1 var(--lf-font-body)',
              padding: '6px 8px',
              borderRadius: 8,
              border: '1px solid var(--lf-rule-soft)',
              background: 'var(--lf-paper)',
              color: 'var(--lf-ink)',
            }}
          >
            {SORTS.map((s) => (
              <option key={s.key} value={s.key}>{s.label}</option>
            ))}
          </select>
        </label>
      </div>

      {showSuggestions && (
        <section aria-label="Suggested for you" style={{ marginTop: 16, marginBottom: 8 }}>
          <h2 style={{ font: '700 12px/1 var(--lf-font-mono)', letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--lf-muted)', marginBottom: 6 }}>
            Suggested for you
          </h2>
          {suggestions.map((p) => (
            <LFPersonRow key={'sg-' + p.id} person={p} />
          ))}
        </section>
      )}

      <section aria-label="People directory" style={{ marginTop: showSuggestions ? 18 : 8 }}>
        {showSuggestions && (
          <h2 style={{ font: '700 12px/1 var(--lf-font-mono)', letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--lf-muted)', marginBottom: 6 }}>
            Directory
          </h2>
        )}
        {people.length === 0 && !loading && (
          <div style={{ padding: '32px 0', textAlign: 'center', color: 'var(--lf-muted)', fontSize: 'var(--lf-text-body)' }}>
            {debounced ? `No one matches “${debounced}”.` : 'No people to show yet.'}
          </div>
        )}
        {people.map((p) => (
          <LFPersonRow key={p.id} person={p} />
        ))}
        {hasMore && <Sentinel onVisible={loadMore} loading={loading} label="Loading more people…" />}
      </section>
    </div>
  )
}
