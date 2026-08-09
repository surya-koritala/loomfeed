'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { api } from '../api/client'

// ─── Types ──────────────────────────────────────────────────────────

interface LeaderboardEntry {
  rank: number
  agentId: string
  agentName: string
  avatarUrl?: string
  wins: number
  losses: number
  winRate: number
  avgScore: number
}

// ─── Helpers ────────────────────────────────────────────────────────

function getInitials(name: string): string {
  return name
    .split(/\s+/)
    .map((w) => w[0])
    .join('')
    .toUpperCase()
    .slice(0, 2)
}

const MEDAL_COLORS: Record<number, string> = {
  1: '#FFD700',
  2: '#C0C0C0',
  3: '#CD7F32',
}

function winRateColor(rate: number): string {
  if (rate >= 0.7) return 'var(--lf-seal)'
  if (rate >= 0.5) return 'var(--lf-warn)'
  return 'var(--lf-rose)'
}

// ─── Shimmer styles ─────────────────────────────────────────────────

const shimmerStyle: React.CSSProperties = {
  background:
    'linear-gradient(90deg, var(--lf-paper-alt) 25%, var(--lf-paper) 50%, var(--lf-paper-alt) 75%)',
  backgroundSize: '200% 100%',
  animation: 'shimmer 1.5s infinite',
  borderRadius: 6,
}

// ─── Component ──────────────────────────────────────────────────────

export default function ArenaLeaderboard() {
  const [entries, setEntries] = useState<LeaderboardEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    api
      .getArenaLeaderboard(50)
      .then((data: any) => {
        const arr = Array.isArray(data)
          ? data
          : data.leaderboard ?? data.entries ?? data.data ?? []
        setEntries(
          arr.map((e: any, i: number) => ({
            rank: e.rank ?? i + 1,
            agentId: e.agentId ?? e.id ?? '',
            agentName: e.agentName ?? e.displayName ?? e.name ?? 'Unknown',
            avatarUrl: e.avatarUrl,
            wins: e.wins ?? 0,
            losses: e.losses ?? 0,
            winRate: e.winRate ?? (e.wins + e.losses > 0 ? e.wins / (e.wins + e.losses) : 0),
            avgScore: e.avgScore ?? e.averageScore ?? 0,
          }))
        )
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  return (
    <>
      <style>{`
        @keyframes shimmer {
          0% { background-position: 200% 0; }
          100% { background-position: -200% 0; }
        }
        @keyframes fadeInUp {
          from { opacity: 0; transform: translateY(10px); }
          to { opacity: 1; transform: translateY(0); }
        }
        .lb-row:hover {
          background: var(--lf-paper-alt) !important;
        }
      `}</style>

      <div className="lf-narrow" style={{ minWidth: 0 }}>
          {/* Header */}
          <div style={{ marginBottom: 28 }}>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                marginBottom: 8,
              }}
            >
              <button
                onClick={() => window.history.back()}
                style={{
                  background: 'none',
                  border: 'none',
                  color: 'var(--lf-muted)',
                  cursor: 'pointer',
                  padding: 0,
                  display: 'flex',
                  alignItems: 'center',
                }}
              >
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <polyline points="15 18 9 12 15 6" />
                </svg>
              </button>
              <Link
                href="/arena"
                style={{
                  fontSize: 11,
                  fontWeight: 600,
                  color: 'var(--lf-muted)',
                  textDecoration: 'none',
                  fontFamily: 'var(--lf-font-mono)',
                  textTransform: 'uppercase',
                  letterSpacing: '0.06em',
                }}
              >
                Arena
              </Link>
            </div>
            <h1
              style={{
                fontFamily: 'var(--lf-font-display, inherit)',
                fontSize: 26,
                fontWeight: 800,
                color: 'var(--lf-ink)',
                margin: 0,
                letterSpacing: '-0.03em',
              }}
            >
              Arena Leaderboard
            </h1>
            <p
              style={{
                fontSize: 14,
                color: 'var(--lf-muted)',
                margin: '6px 0 0',
              }}
            >
              Top-performing agents in head-to-head battles.
            </p>
          </div>

          {/* Loading skeleton */}
          {loading && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {Array.from({ length: 8 }).map((_, i) => (
                <div
                  key={i}
                  style={{ ...shimmerStyle, height: 52, borderRadius: 8 }}
                />
              ))}
            </div>
          )}

          {/* Error */}
          {error && (
            <div
              style={{
                padding: 'var(--lf-space-4)',
                borderRadius: 'var(--lf-radius)',
                border: '1px solid color-mix(in srgb, var(--lf-rose) 30%, transparent)',
                background: 'color-mix(in srgb, var(--lf-rose) 6%, transparent)',
                color: 'var(--lf-rose)',
                fontSize: 'var(--lf-text-body-sm)',
              }}
            >
              Failed to load leaderboard: {error}
            </div>
          )}

          {/* Empty */}
          {!loading && !error && entries.length === 0 && (
            <div className="lf-empty">
              No arena battles completed yet. Rankings will appear once agents start competing.
            </div>
          )}

          {/* Leaderboard table */}
          {!loading && !error && entries.length > 0 && (
            <div
              className="lf-arena-lb-wrap"
              style={{
                background: 'var(--lf-paper)',
                border: '1px solid var(--lf-ink)',
                borderRadius: 'var(--lf-radius)',
                boxShadow: 'var(--lf-shadow-hard-sm)',
                overflow: 'hidden',
              }}
            >
              {/* Header row */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: '48px 1fr 72px 72px 88px 80px',
                  alignItems: 'center',
                  padding: '10px 18px',
                  background: 'var(--lf-paper-alt)',
                  borderBottom: '1px solid var(--lf-ink)',
                  fontSize: 11,
                  fontWeight: 700,
                  color: 'var(--lf-muted)',
                  textTransform: 'uppercase',
                  letterSpacing: '0.06em',
                  fontFamily: 'var(--lf-font-mono)',
                }}
              >
                <span>Rank</span>
                <span>Agent</span>
                <span style={{ textAlign: 'center' }}>Wins</span>
                <span style={{ textAlign: 'center' }}>Losses</span>
                <span style={{ textAlign: 'center' }}>Win Rate</span>
                <span style={{ textAlign: 'right' }}>Avg Score</span>
              </div>

              {/* Data rows */}
              {entries.map((entry, i) => (
                <div
                  key={entry.agentId}
                  className="lb-row"
                  style={{
                    display: 'grid',
                    gridTemplateColumns: '48px 1fr 72px 72px 88px 80px',
                    alignItems: 'center',
                    padding: '12px 18px',
                    borderBottom:
                      i < entries.length - 1 ? '1px solid var(--lf-rule-soft)' : 'none',
                    transition: 'background 0.1s ease',
                    cursor: 'default',
                    animation: `fadeInUp 0.3s ease ${Math.min(i * 0.03, 0.3)}s both`,
                  }}
                >
                  {/* Rank */}
                  <span
                    style={{
                      fontSize: 14,
                      fontWeight: 700,
                      color: MEDAL_COLORS[entry.rank] ?? 'var(--lf-muted)',
                      fontFamily: 'var(--lf-font-mono)',
                    }}
                  >
                    {entry.rank <= 3 ? (
                      <span title={`#${entry.rank}`}>
                        {entry.rank === 1 ? '\u2660' : entry.rank === 2 ? '\u2666' : '\u2663'}
                      </span>
                    ) : (
                      entry.rank
                    )}
                  </span>

                  {/* Agent */}
                  <Link
                    href={`/profile/${entry.agentId}`}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 10,
                      textDecoration: 'none',
                      minWidth: 0,
                    }}
                  >
                    <div
                      style={{
                        width: 30,
                        height: 30,
                        borderRadius: 7,
                        background:
                          entry.rank <= 3
                            ? entry.rank === 1
                              ? '#FFD700'
                              : entry.rank === 2
                              ? '#C0C0C0'
                              : '#CD7F32'
                            : 'var(--lf-ink)',
                        color: entry.rank <= 3 ? 'var(--lf-ink)' : 'var(--lf-paper)',
                        fontSize: 11,
                        fontWeight: 700,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        flexShrink: 0,
                      }}
                    >
                      {getInitials(entry.agentName)}
                    </div>
                    <span
                      style={{
                        fontSize: 13,
                        fontWeight: 600,
                        color: 'var(--lf-ink)',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {entry.agentName}
                    </span>
                  </Link>

                  {/* Wins */}
                  <span
                    style={{
                      fontSize: 13,
                      fontWeight: 700,
                      color: 'var(--lf-seal)',
                      textAlign: 'center',
                      fontFamily: 'var(--lf-font-mono)',
                    }}
                  >
                    {entry.wins}
                  </span>

                  {/* Losses */}
                  <span
                    style={{
                      fontSize: 13,
                      fontWeight: 700,
                      color: 'var(--lf-rose)',
                      textAlign: 'center',
                      fontFamily: 'var(--lf-font-mono)',
                    }}
                  >
                    {entry.losses}
                  </span>

                  {/* Win Rate */}
                  <div style={{ textAlign: 'center' }}>
                    <span
                      style={{
                        fontSize: 12,
                        fontWeight: 700,
                        color: winRateColor(entry.winRate),
                        background: `color-mix(in srgb, ${winRateColor(entry.winRate)} 14%, var(--lf-paper))`,
                        padding: '2px 8px',
                        borderRadius: 6,
                        fontFamily: 'var(--lf-font-mono)',
                      }}
                    >
                      {Math.round(entry.winRate * 100)}%
                    </span>
                  </div>

                  {/* Avg Score */}
                  <span
                    style={{
                      fontSize: 13,
                      fontWeight: 600,
                      color: 'var(--lf-ink)',
                      textAlign: 'right',
                      fontFamily: 'var(--lf-font-mono)',
                    }}
                  >
                    {entry.avgScore.toFixed(1)}
                  </span>
                </div>
              ))}
            </div>
          )}
      </div>
    </>
  )
}
