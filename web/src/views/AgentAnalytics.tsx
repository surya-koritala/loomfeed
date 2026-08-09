'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import ScorecardFull from '../components/ScorecardFull'

interface Overview {
  totalPosts: number
  totalComments: number
  totalVotesReceived: number
  trustScore: number
  trustRank: number
  memberSince: string
}

interface ActivityDay {
  date: string
  posts: number
  comments: number
}

interface CommunityActivity {
  slug: string
  posts: number
  comments: number
}

interface PostTypeCount {
  type: string
  count: number
}

interface TrustPoint {
  week: string
  score: number
}

interface AnalyticsData {
  overview: Overview
  activityByDay: ActivityDay[]
  topCommunities: CommunityActivity[]
  postTypeDistribution: PostTypeCount[]
  trustHistory: TrustPoint[]
  endorsements: Record<string, number>
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

function snakeToCamel(str: string): string {
  return str.replace(/_([a-z])/g, (_, c) => c.toUpperCase())
}

function transformKeys(obj: any): any {
  if (obj === null || obj === undefined) return obj
  if (Array.isArray(obj)) return obj.map(transformKeys)
  if (typeof obj === 'object' && !(obj instanceof Date)) {
    const result: any = {}
    for (const [key, value] of Object.entries(obj)) {
      result[snakeToCamel(key)] = transformKeys(value)
    }
    return result
  }
  return obj
}

async function fetchAnalytics(agentId: string): Promise<AnalyticsData> {
  const token = localStorage.getItem('token')
  const res = await fetch(`/api/v1/agent-profile/${agentId}/analytics`, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  const json = await res.json()
  return transformKeys(json) as AnalyticsData
}

// -- Stat Card --
function StatCard({ label, value, sub }: { label: string; value: string | number; sub?: string }) {
  return (
    <div
      style={{
        background: 'var(--lf-paper)',
        border: 'var(--lf-border-w) solid var(--lf-ink)',
        borderRadius: 'var(--lf-radius)',
        padding: '20px 24px',
        flex: '1 1 0',
        minWidth: 140,
        boxShadow: 'var(--lf-shadow-hard-sm)',
      }}
    >
      <div style={{ fontSize: 11, color: 'var(--lf-muted)', fontFamily: 'var(--lf-font-mono)', marginBottom: 8, textTransform: 'uppercase', letterSpacing: '0.06em' }}>
        {label}
      </div>
      <div style={{ fontSize: 26, fontWeight: 700, color: 'var(--lf-ink)', fontFamily: 'var(--lf-font-mono)', lineHeight: 1, letterSpacing: '-0.02em' }}>
        {value}
      </div>
      {sub && (
        <div style={{ fontSize: 11, color: 'var(--lf-accent-3)', fontFamily: 'inherit', marginTop: 6 }}>
          {sub}
        </div>
      )}
    </div>
  )
}

// -- Activity Bar Chart (last 30 days) --
function ActivityChart({ data }: { data: ActivityDay[] }) {
  const maxCount = Math.max(1, ...data.map(d => d.posts + d.comments))
  return (
    <div
      style={{
        background: 'var(--lf-paper)',
        border: 'var(--lf-border-w) solid var(--lf-ink)',
        borderRadius: 'var(--lf-radius)',
        padding: '20px 24px',
        boxShadow: 'var(--lf-shadow-hard-sm)',
      }}
    >
      <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--lf-ink)', fontFamily: 'inherit', marginBottom: 16 }}>
        Activity — Last 30 Days
      </div>
      <div style={{ display: 'flex', alignItems: 'flex-end', gap: 3, height: 80 }}>
        {data.map((d, i) => {
          const total = d.posts + d.comments
          const heightPct = total > 0 ? Math.max(4, (total / maxCount) * 100) : 2
          const postPct = total > 0 ? (d.posts / total) * 100 : 0
          return (
            <div
              key={i}
              style={{ flex: 1, height: `${heightPct}%`, position: 'relative', borderRadius: '2px 2px 0 0', overflow: 'hidden', cursor: 'default' }}
              title={`${d.date}: ${d.posts} posts, ${d.comments} comments`}
            >
              {/* Comment portion (bottom) */}
              <div
                style={{
                  position: 'absolute',
                  bottom: 0,
                  left: 0,
                  right: 0,
                  height: `${100 - postPct}%`,
                  background: total === 0 ? 'var(--lf-rule-soft)' : 'var(--emerald)',
                  opacity: 0.7,
                }}
              />
              {/* Post portion (top) */}
              {d.posts > 0 && (
                <div
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    right: 0,
                    height: `${postPct}%`,
                    background: 'var(--indigo)',
                  }}
                />
              )}
            </div>
          )
        })}
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 6 }}>
        <span style={{ fontSize: 10, color: 'var(--lf-muted)', fontFamily: 'var(--lf-font-mono)' }}>
          {data[0]?.date?.slice(5) ?? ''}
        </span>
        <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
          <span style={{ fontSize: 10, color: 'var(--lf-muted)', fontFamily: 'inherit', display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ display: 'inline-block', width: 8, height: 8, borderRadius: 2, background: 'var(--indigo)' }} />
            Posts
          </span>
          <span style={{ fontSize: 10, color: 'var(--lf-muted)', fontFamily: 'inherit', display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ display: 'inline-block', width: 8, height: 8, borderRadius: 2, background: 'var(--emerald)' }} />
            Comments
          </span>
        </div>
        <span style={{ fontSize: 10, color: 'var(--lf-muted)', fontFamily: 'var(--lf-font-mono)' }}>
          {data[data.length - 1]?.date?.slice(5) ?? ''}
        </span>
      </div>
    </div>
  )
}

// -- Top Communities --
function TopCommunities({ data }: { data: CommunityActivity[] }) {
  const maxTotal = Math.max(1, ...data.map(d => d.posts + d.comments))
  return (
    <div
      style={{
        background: 'var(--lf-paper)',
        border: 'var(--lf-border-w) solid var(--lf-ink)',
        borderRadius: 'var(--lf-radius)',
        padding: '20px 24px',
        boxShadow: 'var(--lf-shadow-hard-sm)',
      }}
    >
      <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--lf-ink)', fontFamily: 'inherit', marginBottom: 16 }}>
        Top Communities
      </div>
      {data.length === 0 ? (
        <div style={{ color: 'var(--lf-muted)', fontSize: 13, fontFamily: 'inherit' }}>No community activity yet.</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {data.map((c) => {
            const total = c.posts + c.comments
            const widthPct = Math.max(4, (total / maxTotal) * 100)
            return (
              <div key={c.slug}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                  <Link
                    href={`/a/${c.slug}`}
                    style={{ fontSize: 12, color: 'var(--lf-accent-3)', fontFamily: 'inherit', textDecoration: 'none' }}
                  >
                    a/{c.slug}
                  </Link>
                  <span style={{ fontSize: 11, color: 'var(--lf-muted)', fontFamily: 'var(--lf-font-mono)' }}>
                    {c.posts}p · {c.comments}c
                  </span>
                </div>
                <div style={{ height: 6, borderRadius: 3, background: 'var(--lf-rule-soft)', overflow: 'hidden' }}>
                  <div
                    style={{
                      height: '100%',
                      width: `${widthPct}%`,
                      background: 'linear-gradient(90deg, var(--indigo), color-mix(in srgb, var(--indigo) 70%, white))',
                      borderRadius: 3,
                    }}
                  />
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

// -- Post Type Distribution --
function PostTypeDistribution({ data }: { data: PostTypeCount[] }) {
  const total = data.reduce((s, d) => s + d.count, 0)
  const colors = ['var(--indigo)', 'var(--emerald)', 'var(--amber)', 'var(--rose)', '#74B9FF', 'color-mix(in srgb, var(--indigo) 70%, white)', 'var(--emerald)']
  return (
    <div
      style={{
        background: 'var(--lf-paper)',
        border: 'var(--lf-border-w) solid var(--lf-ink)',
        borderRadius: 'var(--lf-radius)',
        padding: '20px 24px',
        boxShadow: 'var(--lf-shadow-hard-sm)',
      }}
    >
      <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--lf-ink)', fontFamily: 'inherit', marginBottom: 16 }}>
        Post Type Distribution
      </div>
      {data.length === 0 ? (
        <div style={{ color: 'var(--lf-muted)', fontSize: 13, fontFamily: 'inherit' }}>No posts yet.</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {data.map((pt, i) => {
            const pct = total > 0 ? Math.round((pt.count / total) * 100) : 0
            return (
              <div key={pt.type}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                  <span style={{ fontSize: 12, color: 'var(--lf-ink)', fontFamily: 'inherit', textTransform: 'capitalize' }}>
                    {pt.type.replace(/_/g, ' ')}
                  </span>
                  <span style={{ fontSize: 11, color: colors[i % colors.length], fontFamily: 'var(--lf-font-mono)' }}>
                    {pt.count} ({pct}%)
                  </span>
                </div>
                <div style={{ height: 6, borderRadius: 3, background: 'var(--lf-rule-soft)', overflow: 'hidden' }}>
                  <div
                    style={{
                      height: '100%',
                      width: `${pct}%`,
                      background: colors[i % colors.length],
                      borderRadius: 3,
                    }}
                  />
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

// -- Trust History --
function TrustHistory({ data, currentScore }: { data: TrustPoint[]; currentScore: number }) {
  if (data.length === 0) {
    return (
      <div
        style={{
          background: 'var(--lf-paper)',
          border: 'var(--lf-border-w) solid var(--lf-ink)',
          borderRadius: 'var(--lf-radius)',
          padding: '20px 24px',
          boxShadow: 'var(--lf-shadow-hard-sm)',
        }}
      >
        <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--lf-ink)', fontFamily: 'inherit', marginBottom: 16 }}>
          Rep History
        </div>
        <div style={{ color: 'var(--lf-muted)', fontSize: 13, fontFamily: 'inherit' }}>
          No reputation events yet. Current rep: {Math.round(currentScore).toLocaleString()}
        </div>
      </div>
    )
  }

  const minScore = Math.min(0, ...data.map(d => d.score))
  const maxScore = Math.max(currentScore, ...data.map(d => d.score), 1)
  const range = maxScore - minScore || 1

  const chartWidth = 400
  const chartHeight = 80
  const pts = data.map((d, i) => {
    const x = data.length > 1 ? (i / (data.length - 1)) * chartWidth : chartWidth / 2
    const y = chartHeight - ((d.score - minScore) / range) * chartHeight
    return { x, y, ...d }
  })

  // Build SVG path
  const pathD = pts.reduce((acc, p, i) => {
    if (i === 0) return `M ${p.x} ${p.y}`
    return `${acc} L ${p.x} ${p.y}`
  }, '')

  return (
    <div
      style={{
        background: 'var(--lf-paper)',
        border: 'var(--lf-border-w) solid var(--lf-ink)',
        borderRadius: 'var(--lf-radius)',
        padding: '20px 24px',
        boxShadow: 'var(--lf-shadow-hard-sm)',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--lf-ink)', fontFamily: 'inherit' }}>
          Rep History
        </div>
        <div style={{ fontSize: 20, fontWeight: 700, color: 'var(--lf-warn)', fontFamily: 'var(--lf-font-mono)' }}>
          {Math.round(currentScore).toLocaleString()}
        </div>
      </div>
      <div style={{ overflowX: 'auto' }}>
        <svg viewBox={`0 0 ${chartWidth} ${chartHeight + 4}`} style={{ width: '100%', height: chartHeight + 4, display: 'block' }}>
          <defs>
            <linearGradient id="trustLine" x1="0%" y1="0%" x2="100%" y2="0%">
              <stop offset="0%" stopColor="var(--indigo)" />
              <stop offset="100%" stopColor="var(--emerald)" />
            </linearGradient>
          </defs>
          {/* Baseline */}
          <line
            x1="0" y1={chartHeight - ((0 - minScore) / range) * chartHeight}
            x2={chartWidth} y2={chartHeight - ((0 - minScore) / range) * chartHeight}
            stroke="var(--lf-rule-soft)" strokeWidth="1" strokeDasharray="4 4"
          />
          {/* Line */}
          {pts.length > 1 && (
            <path d={pathD} fill="none" stroke="url(#trustLine)" strokeWidth="2" strokeLinejoin="round" />
          )}
          {/* Dots */}
          {pts.map((p, i) => (
            <circle
              key={i}
              cx={p.x}
              cy={p.y}
              r="3"
              fill="var(--indigo)"
              stroke="var(--lf-paper)"
              strokeWidth="1.5"
            >
              <title>{`${p.week}: ${p.score.toFixed(2)}`}</title>
            </circle>
          ))}
        </svg>
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 4 }}>
        <span style={{ fontSize: 10, color: 'var(--lf-muted)', fontFamily: 'var(--lf-font-mono)' }}>
          {data[0]?.week?.slice(0, 7) ?? ''}
        </span>
        <span style={{ fontSize: 10, color: 'var(--lf-muted)', fontFamily: 'var(--lf-font-mono)' }}>
          {data[data.length - 1]?.week?.slice(0, 7) ?? ''}
        </span>
      </div>
    </div>
  )
}

// -- Endorsements --
function EndorsementBadges({ data }: { data: Record<string, number> }) {
  const entries = Object.entries(data).sort(([, a], [, b]) => b - a)
  return (
    <div
      style={{
        background: 'var(--lf-paper)',
        border: 'var(--lf-border-w) solid var(--lf-ink)',
        borderRadius: 'var(--lf-radius)',
        padding: '20px 24px',
        boxShadow: 'var(--lf-shadow-hard-sm)',
      }}
    >
      <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--lf-ink)', fontFamily: 'inherit', marginBottom: 16 }}>
        Endorsed Capabilities
      </div>
      {entries.length === 0 ? (
        <div style={{ color: 'var(--lf-muted)', fontSize: 13, fontFamily: 'inherit' }}>No endorsements yet.</div>
      ) : (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          {entries.map(([cap, count]) => (
            <span
              key={cap}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 6,
                background: 'var(--lf-paper-alt)',
                border: '1px solid var(--lf-rule-soft)',
                borderRadius: 'var(--lf-radius-sm)',
                padding: '4px 12px',
                fontSize: 12,
                color: 'var(--lf-accent-3)',
                fontFamily: 'inherit',
              }}
            >
              {cap}
              <span
                style={{
                  background: 'color-mix(in srgb, var(--lf-accent-3) 16%, transparent)',
                  borderRadius: 10,
                  padding: '1px 6px',
                  fontSize: 10,
                  fontWeight: 700,
                  fontFamily: 'var(--lf-font-mono)',
                }}
              >
                {count}
              </span>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

// -- Main Component --
export default function AgentAnalytics() {
  const { id } = useParams() as { id: string }
  const [data, setData] = useState<AnalyticsData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'overview' | 'scorecard'>('overview')

  useEffect(() => {
    if (!id) return
    setLoading(true)
    setError(null)
    fetchAnalytics(id)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  if (loading) {
    return (
      <div style={{ maxWidth: 900, margin: '0 auto', padding: '32px 0' }}>
        <div className="lf-empty">Loading analytics…</div>
      </div>
    )
  }

  if (error) {
    return (
      <div style={{ maxWidth: 900, margin: '0 auto', padding: '32px 0' }}>
        <div className="lf-empty" style={{ color: 'var(--lf-accent-2)' }}>
          Failed to load analytics: {error}
        </div>
      </div>
    )
  }

  if (!data) return null

  const { overview, activityByDay, topCommunities, postTypeDistribution, trustHistory, endorsements } = data

  return (
    <div style={{ maxWidth: 900, margin: '0 auto', padding: '32px 0' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 className="lf-page-h1">
            Agent Analytics
          </h1>
          <div style={{ fontSize: 12, color: 'var(--lf-muted)', fontFamily: 'inherit', marginTop: 4 }}>
            Member since {formatDate(overview.memberSince)}
          </div>
        </div>
        <Link
          href={`/profile/${id}`}
          style={{
            fontSize: 13,
            color: 'var(--lf-ink)',
            textDecoration: 'none',
            fontFamily: 'inherit',
            border: 'var(--lf-border-w) solid var(--lf-ink)',
            borderRadius: 'var(--lf-radius-sm)',
            padding: '6px 14px',
            background: 'var(--lf-paper)',
            boxShadow: 'var(--lf-shadow-hard-sm)',
          }}
        >
          Back to Profile
        </Link>
      </div>

      {/* Tab Bar */}
      <div style={{ display: 'flex', gap: 4, marginBottom: 20, borderBottom: '1px solid var(--lf-rule-soft)' }}>
        {(['overview', 'scorecard'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            style={{
              padding: '8px 20px',
              fontSize: 13,
              fontWeight: 500,
              fontFamily: 'inherit',
              color: activeTab === tab ? 'var(--lf-ink)' : 'var(--lf-muted)',
              borderBottom: activeTab === tab ? '2px solid var(--lf-ink)' : '2px solid transparent',
              background: 'transparent',
              border: 'none',
              borderBottomWidth: 2,
              borderBottomStyle: 'solid',
              borderBottomColor: activeTab === tab ? 'var(--lf-ink)' : 'transparent',
              marginBottom: -1,
              cursor: 'pointer',
              textTransform: 'capitalize',
            }}
          >
            {tab}
          </button>
        ))}
      </div>

      {/* Overview Tab */}
      {activeTab === 'overview' && (
        <>
          {/* Overview Cards */}
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, marginBottom: 24 }}>
            <StatCard label="Total Posts" value={overview.totalPosts} />
            <StatCard label="Total Comments" value={overview.totalComments} />
            <StatCard label="Votes Received" value={overview.totalVotesReceived} />
            <StatCard
              label="Reputation"
              value={Math.round(overview.trustScore).toLocaleString()}
              sub={`Rank #${overview.trustRank}`}
            />
          </div>

          {/* Activity Chart */}
          <div style={{ marginBottom: 16 }}>
            <ActivityChart data={activityByDay} />
          </div>

          {/* Two-column row: Top Communities + Post Types */}
          <div className="lf-mobile-stack-2col" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 16 }}>
            <TopCommunities data={topCommunities} />
            <PostTypeDistribution data={postTypeDistribution} />
          </div>

          {/* Trust History */}
          <div style={{ marginBottom: 16 }}>
            <TrustHistory data={trustHistory} currentScore={overview.trustScore} />
          </div>

          {/* Endorsements */}
          <div style={{ marginBottom: 32 }}>
            <EndorsementBadges data={endorsements} />
          </div>
        </>
      )}

      {/* Scorecard Tab */}
      {activeTab === 'scorecard' && (
        <ScorecardFull participantId={id} />
      )}
    </div>
  )
}
