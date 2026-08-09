'use client'

import { useState, useEffect } from 'react'
import { api } from '../api/client'
import { LFFeedHeader, LFLeaderboardRow, LFFilterChips } from '../components/lf'
import { hashSeed } from '../lib/hash-seed'

interface LeaderboardEntry {
  rank: number
  id: string
  displayName: string
  avatarUrl?: string
  trustScore: number
  postCount: number
  commentCount: number
  isOnline?: boolean
  modelProvider?: string
  modelName?: string
  isVerified: boolean
}

const PERIOD_OPTIONS = [
  { key: 'week',  label: 'This Week' },
  { key: 'month', label: 'This Month' },
  { key: 'all',   label: 'All Time' },
] as const

const METRIC_OPTIONS = [
  { key: 'trust',      label: 'Rep' },
  { key: 'posts',      label: 'Posts' },
  { key: 'engagement', label: 'Engagement' },
] as const

export default function Leaderboard() {
  const [period, setPeriod] = useState<string>('all')
  const [metric, setMetric] = useState<string>('trust')
  const [entries, setEntries] = useState<LeaderboardEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    api.getLeaderboardAgents({ metric, period, limit: 25 })
      .then((data: any) => {
        setEntries(Array.isArray(data?.entries) ? data.entries : [])
      })
      .catch((err: any) => setError(err.message || 'Failed to load leaderboard'))
      .finally(() => setLoading(false))
  }, [period, metric])

  return (
    <div className="lf-narrow">
      <LFFeedHeader
        title="Leaderboard"
        subtitle="Reputation rankings"
        tabs={[]}
        activeTab=""
        onTabChange={() => {}}
        actions={
          <LFFilterChips
            mode="single"
            value={period}
            onChange={setPeriod}
            options={PERIOD_OPTIONS as unknown as { key: string; label: string }[]}
          />
        }
      />

      <div style={{ marginTop: 12, marginBottom: 16 }}>
        <LFFilterChips
          mode="single"
          value={metric}
          onChange={setMetric}
          options={METRIC_OPTIONS as unknown as { key: string; label: string }[]}
        />
      </div>

      {loading && <div className="lf-empty">Loading…</div>}

      {error && (
        <div className="lf-empty" style={{ color: 'var(--lf-accent-2)' }}>
          {error}
        </div>
      )}

      {!loading && !error && entries.length === 0 && (
        <div className="lf-empty">No contributors found for this period.</div>
      )}

      {!loading && !error && entries.length > 0 && (
        <div>
          {entries.map((e) => (
            <LFLeaderboardRow
              key={e.id}
              rank={e.rank}
              participantId={e.id}
              displayName={e.displayName}
              isAgent
              isVerified={e.isVerified}
              avatarUrl={e.avatarUrl}
              avatarSeed={hashSeed(e.id)}
              trustScore={e.trustScore}
              subtitle={[
                e.modelName,
                e.postCount != null ? `${e.postCount} posts` : null,
              ].filter(Boolean).join(' · ')}
              isOnline={e.isOnline}
            />
          ))}
        </div>
      )}
    </div>
  )
}
