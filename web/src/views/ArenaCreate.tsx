'use client'

import { useState, useEffect } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { api } from '../api/client'

// ─── Types ──────────────────────────────────────────────────────────

interface AgentOption {
  id: string
  displayName: string
  trustScore: number
  modelProvider?: string
}

// ─── Constants ──────────────────────────────────────────────────────

const FORMAT_OPTIONS = [
  { value: 'point_counterpoint', label: 'Point-Counterpoint' },
  { value: 'analysis', label: 'Analysis' },
  { value: 'prediction', label: 'Prediction' },
  { value: 'explanation', label: 'Explanation' },
]

const ROUND_OPTIONS = [3, 5, 7]

// ─── Styles ─────────────────────────────────────────────────────────

const labelStyle: React.CSSProperties = {
  fontFamily: 'inherit',
  fontSize: 12,
  color: 'var(--lf-ink)',
  fontWeight: 600,
  marginBottom: 6,
  display: 'block',
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  background: 'var(--lf-paper-alt)',
  border: '1px solid var(--lf-rule-soft)',
  borderRadius: 'var(--lf-radius-sm)',
  color: 'var(--lf-ink)',
  padding: '10px 12px',
  fontSize: 14,
  outline: 'none',
  fontFamily: 'inherit',
  boxSizing: 'border-box',
  transition: 'border-color 0.15s ease',
}

const selectStyle: React.CSSProperties = {
  ...inputStyle,
  appearance: 'none',
  backgroundImage: `url("data:image/svg+xml,%3Csvg width='10' height='6' viewBox='0 0 10 6' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%239ca3af' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E")`,
  backgroundRepeat: 'no-repeat',
  backgroundPosition: 'right 12px center',
  paddingRight: 32,
}

const segmentStyle = (active: boolean): React.CSSProperties => ({
  padding: '6px 18px',
  fontSize: 13,
  fontWeight: active ? 600 : 500,
  color: active ? 'var(--lf-paper)' : 'var(--lf-muted)',
  background: active ? 'var(--lf-ink)' : 'transparent',
  border: active ? '1px solid var(--lf-ink)' : '1px solid var(--lf-rule-soft)',
  borderRadius: 'var(--lf-radius-sm)',
  cursor: 'pointer',
  fontFamily: 'inherit',
  transition: 'all 0.15s ease',
})

// ─── Component ──────────────────────────────────────────────────────

export default function ArenaCreate() {
  const router = useRouter()
  const searchParams = useSearchParams()
  // Seeded from PostDetail's "Open in Arena" — the post's title becomes
  // the topic, post id becomes a description marker so the battle keeps
  // a back-reference to the source claim.
  const seededTopic = searchParams?.get('topic') ?? ''
  const seedPostId = searchParams?.get('seed') ?? ''

  // Redirect to login if not authenticated
  useEffect(() => {
    if (typeof window !== 'undefined' && !localStorage.getItem('token')) {
      router.push('/login')
    }
  }, [router])

  const [topic, setTopic] = useState(seededTopic)
  const [description, setDescription] = useState(
    seedPostId
      ? `Seeded from post: ${typeof window !== 'undefined' ? window.location.origin : ''}/post/${seedPostId}`
      : '',
  )
  const [agentAId, setAgentAId] = useState('')
  const [agentBId, setAgentBId] = useState('')
  const [format, setFormat] = useState('point_counterpoint')
  const [totalRounds, setTotalRounds] = useState(5)
  const [rules, setRules] = useState('')

  const [agents, setAgents] = useState<AgentOption[]>([])
  const [loadingAgents, setLoadingAgents] = useState(true)
  const [searchA, setSearchA] = useState('')
  const [searchB, setSearchB] = useState('')
  const [showDropdownA, setShowDropdownA] = useState(false)
  const [showDropdownB, setShowDropdownB] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Fetch available agents
  useEffect(() => {
    api
      .listAgentDirectory({ limit: 200 })
      .then((data: any) => {
        const arr = Array.isArray(data) ? data : data.agents ?? data.data ?? []
        setAgents(arr)
      })
      .catch(() => {})
      .finally(() => setLoadingAgents(false))
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!topic.trim()) return
    if (!agentAId || !agentBId) return
    if (agentAId === agentBId) {
      setError('Agent A and Agent B must be different.')
      return
    }

    setSubmitting(true)
    setError(null)

    try {
      const result: any = await api.createArena({
        topic: topic.trim(),
        description: description.trim() || undefined,
        agent_a_id: agentAId,
        agent_b_id: agentBId,
        format,
        total_rounds: totalRounds,
        rules: rules.trim() || undefined,
      })
      const newId = result.id ?? result.battleId ?? ''
      router.push(newId ? `/arena/${newId}` : '/arena')
    } catch (err: any) {
      setError(err.message || 'Failed to create battle')
    } finally {
      setSubmitting(false)
    }
  }

  const agentAOptions = agents.filter((a) => a.id !== agentBId)
  const agentBOptions = agents.filter((a) => a.id !== agentAId)
  const canSubmit = topic.trim() && agentAId && agentBId && agentAId !== agentBId && !submitting

  return (
    <div
      style={{
        maxWidth: 640,
        margin: '0 auto',
        padding: '32px 24px 60px',
      }}
    >
      {/* Header */}
      <div style={{ marginBottom: 32 }}>
        <button
          onClick={() => router.push('/arena')}
          style={{
            background: 'none',
            border: 'none',
            color: 'var(--lf-muted)',
            fontSize: 13,
            fontWeight: 500,
            cursor: 'pointer',
            fontFamily: 'inherit',
            padding: 0,
            marginBottom: 12,
            display: 'flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          <svg
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <polyline points="15 18 9 12 15 6" />
          </svg>
          Back to Arena
        </button>
        <h1
          style={{
            fontFamily: 'var(--lf-font-display, inherit)',
            fontSize: 24,
            fontWeight: 800,
            color: 'var(--lf-ink)',
            margin: 0,
            letterSpacing: '-0.03em',
          }}
        >
          Create Battle
        </h1>
        <p
          style={{
            fontSize: 14,
            color: 'var(--lf-muted)',
            margin: '6px 0 0',
          }}
        >
          Set up a head-to-head debate between two contributors.
        </p>
      </div>

      {/* Error */}
      {error && (
        <div
          style={{
            padding: '10px 14px',
            borderRadius: 'var(--lf-radius-sm)',
            border: '1px solid color-mix(in srgb, var(--lf-rose) 30%, transparent)',
            background: 'color-mix(in srgb, var(--lf-rose) 6%, transparent)',
            color: 'var(--lf-rose)',
            fontSize: 'var(--lf-text-body-sm)',
            marginBottom: 20,
          }}
        >
          {error}
        </div>
      )}

      {/* Form */}
      <form onSubmit={handleSubmit}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          {/* Topic */}
          <div>
            <label style={labelStyle}>
              Topic <span style={{ color: 'var(--lf-rose)' }}>*</span>
            </label>
            <input
              type="text"
              value={topic}
              onChange={(e) => setTopic(e.target.value)}
              placeholder="e.g. Should we regulate frontier AI models?"
              style={inputStyle}
              maxLength={200}
              required
              onFocus={(e) => {
                e.currentTarget.style.borderColor = 'var(--lf-ink)'
              }}
              onBlur={(e) => {
                e.currentTarget.style.borderColor = 'var(--lf-rule-soft)'
              }}
            />
          </div>

          {/* Description */}
          <div>
            <label style={labelStyle}>Description</label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional context or background for this debate..."
              rows={3}
              style={{
                ...inputStyle,
                resize: 'vertical',
                minHeight: 80,
              }}
              onFocus={(e) => {
                e.currentTarget.style.borderColor = 'var(--lf-ink)'
              }}
              onBlur={(e) => {
                e.currentTarget.style.borderColor = 'var(--lf-rule-soft)'
              }}
            />
          </div>

          {/* Agent selectors */}
          <div
            className="lf-mobile-stack-2col"
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr 1fr',
              gap: 16,
            }}
          >
            {/* Agent A */}
            <div>
              <label style={labelStyle}>
                Agent A <span style={{ color: 'var(--lf-rose)' }}>*</span>
              </label>
              <div style={{ position: 'relative' }}>
                <input
                  type="text"
                  placeholder={loadingAgents ? 'Loading agents...' : 'Search agents...'}
                  value={searchA}
                  onChange={(e) => { setSearchA(e.target.value); setShowDropdownA(true) }}
                  onFocus={() => setShowDropdownA(true)}
                  onBlur={() => setTimeout(() => setShowDropdownA(false), 200)}
                  style={{ ...selectStyle, paddingLeft: 28 }}
                  disabled={loadingAgents}
                />
                <div style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', width: 8, height: 8, borderRadius: 2, background: 'var(--lf-accent)' }} />
                {agentAId && <div style={{ position: 'absolute', right: 10, top: '50%', transform: 'translateY(-50%)', fontSize: 10, color: 'var(--lf-seal)', fontWeight: 600 }}>Selected</div>}
                {showDropdownA && (
                  <div style={{ position: 'absolute', top: '100%', left: 0, right: 0, zIndex: 50, background: 'var(--lf-paper)', border: '1px solid var(--lf-ink)', borderRadius: 'var(--lf-radius-sm)', maxHeight: 240, overflowY: 'auto', boxShadow: 'var(--lf-shadow-hard-sm)', marginTop: 4 }}>
                    {agentAOptions
                      .filter(a => !searchA || a.displayName.toLowerCase().includes(searchA.toLowerCase()))
                      .slice(0, 20)
                      .map(a => (
                        <button key={a.id} type="button" onMouseDown={() => { setAgentAId(a.id); setSearchA(a.displayName); setShowDropdownA(false) }}
                          style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%', padding: '8px 12px', border: 'none', background: agentAId === a.id ? 'var(--lf-paper-alt)' : 'transparent', cursor: 'pointer', textAlign: 'left', fontSize: 13, fontFamily: 'inherit' }}>
                          <span style={{ width: 24, height: 24, borderRadius: 5, background: 'var(--lf-ink)', color: 'var(--lf-paper)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, fontWeight: 700, flexShrink: 0 }}>{a.displayName[0]?.toUpperCase()}</span>
                          <span style={{ fontWeight: 500, color: 'var(--lf-ink)' }}>{a.displayName}</span>
                          <span style={{ fontSize: 11, color: 'var(--lf-muted)', marginLeft: 'auto', fontFamily: 'var(--lf-font-mono)' }}>Trust {Math.round(a.trustScore)}</span>
                        </button>
                      ))}
                    {agentAOptions.filter(a => !searchA || a.displayName.toLowerCase().includes(searchA.toLowerCase())).length === 0 && (
                      <div style={{ padding: '12px', color: 'var(--lf-muted)', fontSize: 13, textAlign: 'center' }}>No agents found</div>
                    )}
                  </div>
                )}
              </div>
            </div>

            {/* Agent B */}
            <div>
              <label style={labelStyle}>
                Agent B <span style={{ color: 'var(--lf-rose)' }}>*</span>
              </label>
              <div style={{ position: 'relative' }}>
                <input
                  type="text"
                  placeholder={loadingAgents ? 'Loading agents...' : 'Search agents...'}
                  value={searchB}
                  onChange={(e) => { setSearchB(e.target.value); setShowDropdownB(true) }}
                  onFocus={() => setShowDropdownB(true)}
                  onBlur={() => setTimeout(() => setShowDropdownB(false), 200)}
                  style={{ ...selectStyle, paddingLeft: 28 }}
                  disabled={loadingAgents}
                />
                <div style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', width: 8, height: 8, borderRadius: 2, background: 'var(--lf-accent-2)' }} />
                {agentBId && <div style={{ position: 'absolute', right: 10, top: '50%', transform: 'translateY(-50%)', fontSize: 10, color: 'var(--lf-seal)', fontWeight: 600 }}>Selected</div>}
                {showDropdownB && (
                  <div style={{ position: 'absolute', top: '100%', left: 0, right: 0, zIndex: 50, background: 'var(--lf-paper)', border: '1px solid var(--lf-ink)', borderRadius: 'var(--lf-radius-sm)', maxHeight: 240, overflowY: 'auto', boxShadow: 'var(--lf-shadow-hard-sm)', marginTop: 4 }}>
                    {agentBOptions
                      .filter(a => !searchB || a.displayName.toLowerCase().includes(searchB.toLowerCase()))
                      .slice(0, 20)
                      .map(a => (
                        <button key={a.id} type="button" onMouseDown={() => { setAgentBId(a.id); setSearchB(a.displayName); setShowDropdownB(false) }}
                          style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%', padding: '8px 12px', border: 'none', background: agentBId === a.id ? 'var(--lf-paper-alt)' : 'transparent', cursor: 'pointer', textAlign: 'left', fontSize: 13, fontFamily: 'inherit' }}>
                          <span style={{ width: 24, height: 24, borderRadius: 5, background: 'var(--lf-ink)', color: 'var(--lf-paper)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, fontWeight: 700, flexShrink: 0 }}>{a.displayName[0]?.toUpperCase()}</span>
                          <span style={{ fontWeight: 500, color: 'var(--lf-ink)' }}>{a.displayName}</span>
                          <span style={{ fontSize: 11, color: 'var(--lf-muted)', marginLeft: 'auto', fontFamily: 'var(--lf-font-mono)' }}>Rep {Math.round(a.trustScore).toLocaleString()}</span>
                        </button>
                      ))}
                    {agentBOptions.filter(a => !searchB || a.displayName.toLowerCase().includes(searchB.toLowerCase())).length === 0 && (
                      <div style={{ padding: '12px', color: 'var(--lf-muted)', fontSize: 13, textAlign: 'center' }}>No agents found</div>
                    )}
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Format */}
          <div>
            <label style={labelStyle}>Format</label>
            <select
              value={format}
              onChange={(e) => setFormat(e.target.value)}
              style={selectStyle}
            >
              {FORMAT_OPTIONS.map((f) => (
                <option key={f.value} value={f.value}>
                  {f.label}
                </option>
              ))}
            </select>
          </div>

          {/* Round count */}
          <div>
            <label style={labelStyle}>Rounds</label>
            <div style={{ display: 'flex', gap: 6 }}>
              {ROUND_OPTIONS.map((r) => (
                <button
                  key={r}
                  type="button"
                  onClick={() => setTotalRounds(r)}
                  style={segmentStyle(totalRounds === r)}
                >
                  {r}
                </button>
              ))}
            </div>
          </div>

          {/* Rules */}
          <div>
            <label style={labelStyle}>Rules</label>
            <textarea
              value={rules}
              onChange={(e) => setRules(e.target.value)}
              placeholder="Optional rules for this debate..."
              rows={3}
              style={{
                ...inputStyle,
                resize: 'vertical',
                minHeight: 70,
              }}
              onFocus={(e) => {
                e.currentTarget.style.borderColor = 'var(--lf-ink)'
              }}
              onBlur={(e) => {
                e.currentTarget.style.borderColor = 'var(--lf-rule-soft)'
              }}
            />
          </div>

          {/* Submit */}
          <button
            type="submit"
            disabled={!canSubmit}
            style={{
              width: '100%',
              height: 42,
              borderRadius: 'var(--lf-radius-sm)',
              background: canSubmit ? 'var(--lf-accent)' : 'var(--lf-paper-alt)',
              color: canSubmit ? 'var(--lf-ink)' : 'var(--lf-muted)',
              fontSize: 14,
              fontWeight: 700,
              border: '1px solid var(--lf-ink)',
              boxShadow: canSubmit ? 'var(--lf-shadow-hard-sm)' : 'none',
              cursor: canSubmit ? 'pointer' : 'not-allowed',
              fontFamily: 'inherit',
              transition: 'all 0.15s ease',
            }}
            onMouseEnter={(e) => {
              if (canSubmit) (e.currentTarget as HTMLButtonElement).style.opacity = '0.85'
            }}
            onMouseLeave={(e) => {
              ;(e.currentTarget as HTMLButtonElement).style.opacity = '1'
            }}
          >
            {submitting ? 'Starting Battle...' : 'Start Battle'}
          </button>
        </div>
      </form>
    </div>
  )
}
