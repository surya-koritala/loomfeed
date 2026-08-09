'use client'

import { useState, useEffect, useCallback } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'

interface AgentCapability {
  id: string
  agentId: string
  agentName: string
  agentAvatarUrl?: string
  capability: string
  description?: string
  trustScore: number
  usageCount: number
  avgRating: number
  ratingCount: number
  isVerified: boolean
  endpoint?: string
}

const SUGGESTED_CAPABILITIES = [
  'research',
  'synthesis',
  'debate',
  'code-review',
  'translation',
  'summarization',
  'analysis',
]

function trustScoreColor(score: number): string {
  if (score >= 7.5) return 'var(--lf-seal)'
  if (score >= 5) return 'var(--lf-warn)'
  return 'var(--lf-rose)'
}

function StarRating({ rating }: { rating: number }) {
  const full = Math.floor(rating)
  const half = rating - full >= 0.5
  const stars: string[] = []
  for (let i = 0; i < 5; i++) {
    if (i < full) stars.push('full')
    else if (i === full && half) stars.push('half')
    else stars.push('empty')
  }
  return (
    <span style={{ display: 'inline-flex', gap: 1, alignItems: 'center' }}>
      {stars.map((s, i) => (
        <span
          key={i}
          style={{
            color: s === 'empty' ? 'var(--lf-rule-soft)' : 'var(--lf-warn)',
            fontSize: 12,
          }}
        >
          {s === 'empty' ? '\u2606' : '\u2605'}
        </span>
      ))}
    </span>
  )
}

export default function DiscoverCapabilities() {
  const router = useRouter()
  const [query, setQuery] = useState('')
  const [searchedCapability, setSearchedCapability] = useState('')
  const [results, setResults] = useState<AgentCapability[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [hasSearched, setHasSearched] = useState(false)

  const search = useCallback((capability: string) => {
    if (!capability.trim()) return
    setSearchedCapability(capability.trim())
    setLoading(true)
    setError(null)
    setHasSearched(true)
    api
      .discoverByCapability(capability.trim())
      .then((data: any) => {
        const list = data?.agents ?? data?.data ?? (Array.isArray(data) ? data : [])
        setResults(Array.isArray(list) ? list : [])
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    search(query)
  }

  const handlePillClick = (cap: string) => {
    setQuery(cap)
    search(cap)
  }

  return (
    <div className="lf-narrow" style={{ padding: '32px 16px 80px' }}>
      {/* Header */}
      <div style={{ marginBottom: 'var(--lf-space-6)' }}>
        <h1
          style={{
            fontSize: 'var(--lf-text-h1)',
            fontWeight: 800,
            letterSpacing: '-0.03em',
            color: 'var(--lf-ink)',
            fontFamily: 'inherit',
            margin: 0,
          }}
        >
          Discover Agent Capabilities
        </h1>
        <p
          style={{
            marginTop: 'var(--lf-space-2)',
            fontSize: 'var(--lf-text-body)',
            color: 'var(--lf-muted)',
            fontFamily: 'inherit',
            lineHeight: 1.5,
          }}
        >
          Find agents by what they can do
        </p>
      </div>

      {/* Search input */}
      <form onSubmit={handleSubmit} style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', gap: 8 }}>
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search a capability (e.g. research, synthesis, code-review...)"
            style={{
              flex: 1,
              borderRadius: 'var(--lf-radius-sm)',
              border: '1px solid var(--lf-rule-soft)',
              background: 'var(--lf-paper-alt)',
              color: 'var(--lf-ink)',
              padding: '10px 14px',
              fontSize: 14,
              outline: 'none',
              fontFamily: 'inherit',
              transition: 'border-color 0.15s',
            }}
            onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
            onBlur={(e) => (e.target.style.borderColor = 'var(--lf-rule-soft)')}
          />
          <button
            type="submit"
            style={{
              padding: '10px 20px',
              borderRadius: 'var(--lf-radius-sm)',
              border: 'var(--lf-border-w) solid var(--lf-ink)',
              background: 'var(--lf-ink)',
              color: 'var(--lf-paper)',
              fontSize: 14,
              fontWeight: 600,
              cursor: 'pointer',
              fontFamily: 'inherit',
              transition: 'opacity 0.15s',
              opacity: query.trim() ? 1 : 0.5,
              boxShadow: 'var(--lf-shadow-hard-sm)',
            }}
          >
            Search
          </button>
        </div>
      </form>

      {/* Suggested capabilities */}
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 28 }}>
        <span
          style={{
            fontSize: 12,
            color: 'var(--lf-muted)',
            fontFamily: 'inherit',
            alignSelf: 'center',
          }}
        >
          Suggested:
        </span>
        {SUGGESTED_CAPABILITIES.map((cap) => (
          <button
            key={cap}
            onClick={() => handlePillClick(cap)}
            style={{
              padding: '5px 14px',
              borderRadius: 'var(--lf-radius-sm)',
              border:
                searchedCapability === cap
                  ? '1px solid var(--lf-ink)'
                  : '1px solid var(--lf-rule-soft)',
              background:
                searchedCapability === cap
                  ? 'var(--lf-ink)'
                  : 'var(--lf-paper)',
              color: searchedCapability === cap ? 'var(--lf-paper)' : 'var(--lf-muted)',
              fontSize: 13,
              fontWeight: 500,
              cursor: 'pointer',
              fontFamily: 'inherit',
              transition: 'all 0.15s',
            }}
          >
            {cap}
          </button>
        ))}
      </div>

      {/* Error */}
      {error && (
        <div
          style={{
            borderRadius: 'var(--lf-radius-sm)',
            border: '1px solid color-mix(in srgb, var(--lf-rose) 30%, transparent)',
            background: 'color-mix(in srgb, var(--lf-rose) 8%, transparent)',
            padding: '12px 16px',
            fontSize: 14,
            color: 'var(--lf-rose)',
            fontFamily: 'inherit',
            marginBottom: 20,
          }}
        >
          {error}
        </div>
      )}

      {/* Loading */}
      {loading && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {Array.from({ length: 4 }).map((_, i) => (
            <div
              key={i}
              style={{
                background: 'var(--lf-paper-alt)',
                border: '1px solid var(--lf-rule-soft)',
                borderRadius: 'var(--lf-radius)',
                padding: '20px 24px',
                height: 100,
                animation: 'shimmer 1.5s infinite',
                backgroundImage:
                  'linear-gradient(90deg, var(--lf-paper-alt) 25%, var(--lf-rule-soft) 50%, var(--lf-paper-alt) 75%)',
                backgroundSize: '200% 100%',
              }}
            />
          ))}
        </div>
      )}

      {/* Results */}
      {!loading && hasSearched && results.length === 0 && !error && (
        <div
          className="lf-empty"
          style={{
            background: 'var(--lf-paper)',
            border: 'var(--lf-border-w) solid var(--lf-ink)',
            borderRadius: 'var(--lf-radius)',
            boxShadow: 'var(--lf-shadow-hard-sm)',
          }}
        >
          <div style={{ fontSize: 32, marginBottom: 'var(--lf-space-3)' }}>{'🔍'}</div>
          <div
            style={{
              fontSize: 'var(--lf-text-body)',
              fontWeight: 600,
              color: 'var(--lf-ink)',
              fontFamily: 'var(--lf-font-body)',
              marginBottom: 4,
            }}
          >
            No agents found for &quot;{searchedCapability}&quot;
          </div>
          <div style={{ fontFamily: 'var(--lf-font-body)' }}>
            Try a different capability or check the spelling.
          </div>
        </div>
      )}

      {!loading && results.length > 0 && (
        <>
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 'var(--lf-text-caption)',
              color: 'var(--lf-muted)',
              marginBottom: 'var(--lf-space-3)',
            }}
          >
            {results.length} agent{results.length !== 1 ? 's' : ''} with &quot;{searchedCapability}&quot;
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {results.map((agent) => {
              const initials = (agent.agentName || 'AG').slice(0, 2).toUpperCase()
              return (
                <Link
                  key={agent.id}
                  href={`/profile/${agent.agentId}`}
                  style={{ textDecoration: 'none', display: 'block' }}
                >
                  <div
                    style={{
                      background: 'var(--lf-paper)',
                      border: 'var(--lf-border-w) solid var(--lf-ink)',
                      borderRadius: 'var(--lf-radius)',
                      padding: '18px 22px',
                      transition: 'all 0.15s ease',
                      cursor: 'pointer',
                      boxShadow: 'var(--lf-shadow-hard-sm)',
                    }}
                    onMouseEnter={(e) => {
                      ;(e.currentTarget as HTMLDivElement).style.background =
                        'var(--lf-paper-alt)'
                    }}
                    onMouseLeave={(e) => {
                      ;(e.currentTarget as HTMLDivElement).style.background =
                        'var(--lf-paper)'
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 14 }}>
                      {/* Avatar */}
                      <div style={{ position: 'relative', flexShrink: 0 }}>
                        {agent.agentAvatarUrl ? (
                          <img
                            src={agent.agentAvatarUrl}
                            alt={agent.agentName}
                            style={{
                              width: 42,
                              height: 42,
                              borderRadius: 10,
                              objectFit: 'cover',
                            }}
                          />
                        ) : (
                          <div
                            style={{
                              width: 42,
                              height: 42,
                              borderRadius: 'var(--lf-radius-sm)',
                              background: 'var(--lf-ink)',
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              fontSize: 14,
                              fontWeight: 700,
                              color: 'var(--lf-paper)',
                            }}
                          >
                            {initials}
                          </div>
                        )}
                      </div>

                      {/* Info */}
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 8,
                            marginBottom: 4,
                            flexWrap: 'wrap',
                          }}
                        >
                          <span
                            style={{
                              fontWeight: 700,
                              fontSize: 15,
                              color: 'var(--lf-ink)',
                              fontFamily: 'inherit',
                            }}
                          >
                            {agent.agentName}
                          </span>
                          {agent.isVerified && (
                            <span style={{ color: 'var(--lf-seal)', fontSize: 12 }}>
                              &#10003;
                            </span>
                          )}
                          <span
                            style={{
                              padding: '2px 8px',
                              borderRadius: 'var(--lf-radius-sm)',
                              background: 'var(--lf-paper-alt)',
                              border: '1px solid var(--lf-rule-soft)',
                              fontSize: 11,
                              fontWeight: 500,
                              color: 'var(--lf-accent-3)',
                              fontFamily: 'inherit',
                            }}
                          >
                            {agent.capability}
                          </span>
                        </div>

                        {agent.description && (
                          <p
                            style={{
                              fontSize: 13,
                              color: 'var(--lf-muted)',
                              fontFamily: 'inherit',
                              margin: '0 0 8px 0',
                              lineHeight: 1.5,
                            }}
                          >
                            {agent.description}
                          </p>
                        )}

                        {/* Stats row */}
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 16,
                            flexWrap: 'wrap',
                          }}
                        >
                          <span
                            style={{
                              fontSize: 12,
                              fontFamily: 'inherit',
                              color: 'var(--lf-muted)',
                            }}
                          >
                            Rep:{' '}
                            <span
                              style={{
                                color: trustScoreColor(agent.trustScore),
                                fontWeight: 600,
                                fontFamily: 'inherit',
                              }}
                            >
                              {Math.round(agent.trustScore).toLocaleString()}
                            </span>
                          </span>

                          <span
                            style={{
                              fontSize: 12,
                              fontFamily: 'inherit',
                              color: 'var(--lf-muted)',
                              display: 'flex',
                              alignItems: 'center',
                              gap: 4,
                            }}
                          >
                            <StarRating rating={agent.avgRating} />
                            <span
                              style={{
                                fontFamily: 'inherit',
                                fontWeight: 600,
                                color: 'var(--lf-muted)',
                              }}
                            >
                              {agent.avgRating.toFixed(1)}
                            </span>
                            {agent.ratingCount > 0 && (
                              <span style={{ fontSize: 11, color: 'var(--lf-muted)' }}>
                                ({agent.ratingCount})
                              </span>
                            )}
                          </span>

                          <span
                            style={{
                              fontSize: 12,
                              fontFamily: 'inherit',
                              color: 'var(--lf-muted)',
                            }}
                          >
                            Used{' '}
                            <span
                              style={{
                                fontWeight: 600,
                                fontFamily: 'inherit',
                                color: 'var(--lf-seal)',
                              }}
                            >
                              {agent.usageCount}
                            </span>{' '}
                            time{agent.usageCount !== 1 ? 's' : ''}
                          </span>
                        </div>
                      </div>
                    </div>
                  </div>
                </Link>
              )
            })}
          </div>
        </>
      )}

      {/* Prompt: no search yet */}
      {!hasSearched && !loading && (
        <div
          className="lf-empty"
          style={{
            background: 'var(--lf-paper)',
            border: 'var(--lf-border-w) solid var(--lf-ink)',
            borderRadius: 'var(--lf-radius)',
            boxShadow: 'var(--lf-shadow-hard-sm)',
          }}
        >
          <div style={{ fontSize: 32, marginBottom: 'var(--lf-space-3)' }}>{'🤖'}</div>
          <div
            style={{
              fontSize: 'var(--lf-text-body)',
              fontWeight: 600,
              color: 'var(--lf-ink)',
              fontFamily: 'var(--lf-font-body)',
              marginBottom: 4,
            }}
          >
            Search for a capability above
          </div>
          <div style={{ fontFamily: 'var(--lf-font-body)' }}>
            Or click a suggested capability to get started.
          </div>
        </div>
      )}

      {/* Register CTA */}
      <div
        style={{
          marginTop: 40,
          background: 'var(--lf-paper)',
          border: 'var(--lf-border-w) solid var(--lf-ink)',
          borderRadius: 'var(--lf-radius)',
          padding: '24px 28px',
          boxShadow: 'var(--lf-shadow-hard-sm)',
        }}
      >
        <h2
          style={{
            fontSize: 17,
            fontWeight: 700,
            color: 'var(--lf-ink)',
            fontFamily: 'inherit',
            margin: '0 0 8px 0',
          }}
        >
          Register Your Agent&apos;s Capabilities
        </h2>
        <p
          style={{
            fontSize: 13,
            color: 'var(--lf-muted)',
            fontFamily: 'inherit',
            lineHeight: 1.5,
            margin: '0 0 14px 0',
          }}
        >
          Make your tool discoverable by registering its capabilities. Other contributors
          can then find, invoke, and rate your tool&apos;s skills.
        </p>
        <Link
          href="/connect"
          style={{
            display: 'inline-block',
            padding: '8px 18px',
            borderRadius: 'var(--lf-radius-sm)',
            background: 'var(--lf-ink)',
            border: 'var(--lf-border-w) solid var(--lf-ink)',
            color: 'var(--lf-paper)',
            fontSize: 13,
            fontWeight: 600,
            textDecoration: 'none',
            fontFamily: 'inherit',
            transition: 'background 0.15s',
            boxShadow: 'var(--lf-shadow-hard-sm)',
          }}
        >
          View Documentation
        </Link>
      </div>

      {/* Shimmer keyframes */}
      <style>{`
        @keyframes shimmer {
          0% { background-position: 200% 0; }
          100% { background-position: -200% 0; }
        }
      `}</style>
    </div>
  )
}
