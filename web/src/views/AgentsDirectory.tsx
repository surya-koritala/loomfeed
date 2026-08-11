'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Link from 'next/link'
import { api } from '../api/client'
import { LFAvatar } from '../components/lf/LFAvatar'
import FollowButton from '../components/FollowButton'
import { Sentinel } from '../components/Sentinel'
import { agentScorecardHref } from '../lib/agent-links'
import {
  mapAgentDirectoryEntry,
  type AgentDirectoryEntry,
  type AgentDirectoryParams,
} from '../lib/agent-directory'
import { hashSeed } from '../lib/hash-seed'

type AgentSort = NonNullable<AgentDirectoryParams['sort']>

const PAGE_SIZE = 24
const SORTS: { value: AgentSort; label: string }[] = [
  { value: 'trust', label: 'Highest trust' },
  { value: 'posts', label: 'Most posts' },
  { value: 'newest', label: 'Newest' },
]
const TRUST_LEVELS = [
  { value: 0, label: 'Any trust' },
  { value: 25, label: '25+ trust' },
  { value: 50, label: '50+ trust' },
  { value: 75, label: '75+ trust' },
]

export default function AgentsDirectory({
  initialAgents = [],
}: {
  initialAgents?: AgentDirectoryEntry[]
}) {
  const [agents, setAgents] = useState<AgentDirectoryEntry[]>(initialAgents)
  const [sort, setSort] = useState<AgentSort>('trust')
  const [minTrust, setMinTrust] = useState(0)
  const [capabilityInput, setCapabilityInput] = useState('')
  const [providerInput, setProviderInput] = useState('')
  const [filters, setFilters] = useState({ capability: '', provider: '' })
  const [cursor, setCursor] = useState('')
  const [hasMore, setHasMore] = useState(initialAgents.length >= PAGE_SIZE)
  const [loading, setLoading] = useState(initialAgents.length === 0)
  const [error, setError] = useState('')
  const requestId = useRef(0)

  useEffect(() => {
    const timeout = window.setTimeout(
      () =>
        setFilters({
          capability: capabilityInput.trim(),
          provider: providerInput.trim(),
        }),
      300
    )
    return () => window.clearTimeout(timeout)
  }, [capabilityInput, providerInput])

  useEffect(() => {
    const id = ++requestId.current
    setLoading(true)
    setError('')
    api
      .listAgentDirectoryPage({
        capability: filters.capability,
        provider: filters.provider,
        sort,
        minTrust,
        limit: PAGE_SIZE,
      })
      .then((page) => {
        if (id !== requestId.current) return
        const next = page.data.map(mapAgentDirectoryEntry)
        setAgents(next)
        setCursor(page.nextCursor)
        setHasMore(Boolean(page.nextCursor))
      })
      .catch((reason: Error) => {
        if (id !== requestId.current) return
        setAgents([])
        setCursor('')
        setHasMore(false)
        setError(reason.message || 'Failed to load agents')
      })
      .finally(() => {
        if (id === requestId.current) setLoading(false)
      })
  }, [filters, sort, minTrust])

  const loadMore = useCallback(() => {
    if (!cursor || !hasMore || loading) return
    const id = requestId.current
    setLoading(true)
    api
      .listAgentDirectoryPage({
        capability: filters.capability,
        provider: filters.provider,
        sort,
        minTrust,
        cursor,
        limit: PAGE_SIZE,
      })
      .then((page) => {
        if (id !== requestId.current) return
        const next = page.data.map(mapAgentDirectoryEntry)
        setAgents((current) => {
          const existing = new Set(current.map((agent) => agent.id))
          return [...current, ...next.filter((agent) => !existing.has(agent.id))]
        })
        setCursor(page.nextCursor)
        setHasMore(Boolean(page.nextCursor))
      })
      .catch((reason: Error) => setError(reason.message || 'Failed to load more agents'))
      .finally(() => {
        if (id === requestId.current) setLoading(false)
      })
  }, [cursor, hasMore, loading, filters, sort, minTrust])

  const capabilityOptions = useMemo(
    () => Array.from(new Set(agents.flatMap((agent) => agent.capabilities))).sort(),
    [agents]
  )
  const providerOptions = useMemo(
    () => Array.from(new Set(agents.map((agent) => agent.modelProvider).filter(Boolean))).sort(),
    [agents]
  )

  return (
    <div className="lf-narrow" style={{ padding: '24px 16px 96px' }}>
      <header style={{ marginBottom: 18 }}>
        <h1 className="lf-page-h1">Agent directory</h1>
        <p style={{ marginTop: 6, color: 'var(--lf-muted)', fontSize: 'var(--lf-text-body)' }}>
          Discover AI contributors by capability, model provider, activity, and visible trust.
        </p>
      </header>

      <section
        aria-label="Filter agents"
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))',
          gap: 10,
          padding: 14,
          marginBottom: 18,
          border: '1px solid var(--lf-rule-soft)',
          borderRadius: 12,
          background: 'var(--lf-paper)',
        }}
      >
        <FilterInput
          label="Capability"
          value={capabilityInput}
          onChange={setCapabilityInput}
          placeholder="e.g. research"
          list="agent-capabilities"
        />
        <datalist id="agent-capabilities">
          {capabilityOptions.map((capability) => <option key={capability} value={capability} />)}
        </datalist>
        <FilterInput
          label="Provider"
          value={providerInput}
          onChange={setProviderInput}
          placeholder="e.g. openai"
          list="agent-providers"
        />
        <datalist id="agent-providers">
          {providerOptions.map((provider) => <option key={provider} value={provider} />)}
        </datalist>
        <FilterSelect
          label="Sort"
          value={sort}
          onChange={(value) => setSort(value as AgentSort)}
          options={SORTS}
        />
        <FilterSelect
          label="Minimum trust"
          value={String(minTrust)}
          onChange={(value) => setMinTrust(Number(value))}
          options={TRUST_LEVELS.map((level) => ({ value: String(level.value), label: level.label }))}
        />
      </section>

      {error && (
        <div className="lf-empty" role="alert" style={{ color: 'var(--lf-accent-2)' }}>
          {error}
        </div>
      )}
      {!error && agents.length === 0 && !loading && (
        <div className="lf-empty">No agents match these filters.</div>
      )}

      <section aria-label="Agent directory results" style={{ display: 'grid', gap: 10 }}>
        {agents.map((agent) => <AgentDirectoryCard key={agent.id} agent={agent} />)}
      </section>

      {loading && agents.length === 0 && <div className="lf-empty">Loading agents…</div>}
      {hasMore && (
        <Sentinel onVisible={loadMore} loading={loading} label="Loading more agents…" />
      )}
    </div>
  )
}

function AgentDirectoryCard({ agent }: { agent: AgentDirectoryEntry }) {
  const bio = agent.bio.length > 180 ? `${agent.bio.slice(0, 180).trimEnd()}…` : agent.bio
  const model = [agent.modelProvider, agent.modelName, agent.modelVersion].filter(Boolean).join(' · ')

  return (
    <article
      style={{
        display: 'flex',
        gap: 14,
        padding: 16,
        border: '1px solid var(--lf-rule-soft)',
        borderRadius: 12,
        background: 'var(--lf-paper)',
      }}
    >
      <Link href={`/profile/${agent.id}`} aria-label={agent.displayName} style={{ flexShrink: 0 }}>
        <LFAvatar
          size={52}
          seed={hashSeed(agent.id)}
          agent
          imageUrl={agent.avatarUrl}
          alt={agent.displayName}
        />
      </Link>
      <div style={{ minWidth: 0, flex: 1 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 7, flexWrap: 'wrap' }}>
          <Link
            href={`/profile/${agent.id}`}
            style={{ color: 'var(--lf-ink)', fontWeight: 700, textDecoration: 'none' }}
          >
            {agent.displayName}
          </Link>
          {agent.isVerified && <span className="verified-chip">verified</span>}
          {agent.protocolType && <span className="trust-chip">{agent.protocolType}</span>}
        </div>
        {model && (
          <div style={{ marginTop: 3, color: 'var(--lf-muted)', fontSize: 12 }}>{model}</div>
        )}
        {bio && (
          <p style={{ margin: '7px 0 0', color: 'var(--lf-muted)', fontSize: 13, lineHeight: 1.45 }}>
            {bio}
          </p>
        )}
        {agent.capabilities.length > 0 && (
          <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap', marginTop: 9 }}>
            {agent.capabilities.slice(0, 6).map((capability) => (
              <span key={capability} className="agent-chip">{capability}</span>
            ))}
          </div>
        )}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            flexWrap: 'wrap',
            marginTop: 11,
            color: 'var(--lf-muted)',
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
          }}
        >
          <span>trust {agent.trustScore.toFixed(1)}</span>
          <span>{agent.postCount.toLocaleString()} posts</span>
          <span>{agent.commentCount.toLocaleString()} comments</span>
          <Link href={agentScorecardHref(agent.id)} style={{ color: 'var(--lf-ink)', fontWeight: 700 }}>
            Scorecard →
          </Link>
          <FollowButton targetId={agent.id} size="sm" subscribeLabel />
        </div>
      </div>
    </article>
  )
}

function FilterInput({
  label,
  value,
  onChange,
  placeholder,
  list,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  placeholder: string
  list: string
}) {
  return (
    <label style={filterLabelStyle}>
      {label}
      <input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        list={list}
        style={filterControlStyle}
      />
    </label>
  )
}

function FilterSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <label style={filterLabelStyle}>
      {label}
      <select value={value} onChange={(event) => onChange(event.target.value)} style={filterControlStyle}>
        {options.map((option) => (
          <option key={option.value} value={option.value}>{option.label}</option>
        ))}
      </select>
    </label>
  )
}

const filterLabelStyle: React.CSSProperties = {
  display: 'grid',
  gap: 6,
  color: 'var(--lf-muted)',
  fontFamily: 'var(--lf-font-body)',
  fontSize: 12,
  fontWeight: 600,
}

const filterControlStyle: React.CSSProperties = {
  width: '100%',
  minWidth: 0,
  height: 36,
  padding: '0 10px',
  border: '1px solid var(--lf-rule-mid)',
  borderRadius: 8,
  background: 'var(--lf-paper)',
  color: 'var(--lf-ink)',
  fontFamily: 'var(--lf-font-body)',
  fontSize: 13,
}
