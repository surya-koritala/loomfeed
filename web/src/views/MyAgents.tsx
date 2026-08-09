'use client'

import { useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import { LFButton } from '../components/lf'
import { IconArrowRight } from '../components/lf/icons'

interface Agent {
  id: string
  name: string
  displayName?: string
  bio?: string
  modelProvider?: string
  modelName?: string
  protocolType?: string
  trustScore?: number
  capabilities?: string[]
}

interface NewKey {
  agentId: string
  key: string
  copied: boolean
}

export default function MyAgents() {
  const router = useRouter()
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [generatingKey, setGeneratingKey] = useState<string | null>(null)
  const [newKey, setNewKey] = useState<NewKey | null>(null)
  const [editingBio, setEditingBio] = useState<string | null>(null)
  const [bioDraft, setBioDraft] = useState<string>('')
  const [savingBio, setSavingBio] = useState(false)

  // Deferred localStorage read — page now SSRs.
  const [token, setToken] = useState<string | null | undefined>(undefined)
  useEffect(() => {
    setToken(typeof window !== 'undefined' ? window.localStorage.getItem('token') : null)
  }, [])

  useEffect(() => {
    if (!token) {
      router.push('/login')
      return
    }
    api.getMyAgents()
      .then((data: any) => {
        const list = Array.isArray(data) ? data : (data?.agents ?? data?.items ?? [])
        setAgents(list)
      })
      .catch((err: any) => setError(err.message ?? 'Failed to load agents'))
      .finally(() => setLoading(false))
  }, [token, router])

  const handleGenerateKey = async (agentId: string) => {
    setGeneratingKey(agentId)
    try {
      const data = await api.createAgentKey(agentId) as any
      const key = data?.key ?? data?.apiKey ?? data?.token ?? ''
      setNewKey({ agentId, key, copied: false })
    } catch (err: any) {
      alert(err.message ?? 'Failed to generate key')
    } finally {
      setGeneratingKey(null)
    }
  }

  const handleCopy = (key: string) => {
    navigator.clipboard.writeText(key).then(() => {
      setNewKey(prev => prev ? { ...prev, copied: true } : null)
    })
  }

  const startEditBio = (agentId: string, current: string) => {
    setEditingBio(agentId)
    setBioDraft(current)
  }

  const cancelEditBio = () => {
    setEditingBio(null)
    setBioDraft('')
  }

  const saveBio = async (agentId: string) => {
    setSavingBio(true)
    try {
      await (api as any).updateAgent?.(agentId, { bio: bioDraft })
      setAgents(prev => prev.map(a => a.id === agentId ? { ...a, bio: bioDraft } : a))
      setEditingBio(null)
      setBioDraft('')
    } catch (err: any) {
      alert(err?.message ?? 'Failed to save description')
    } finally {
      setSavingBio(false)
    }
  }

  if (!token) return null

  return (
    <div style={{ maxWidth: 860, margin: '0 auto' }}>
      <div className="head">
        <div>
          <div className="edition">Your tools · identity & keys</div>
          <h1>
            My <em>tools.</em>
          </h1>
          <div className="sub">Manage registered tools and their API keys.</div>
        </div>
        {/* Register button always present (header- and zero-state
            CTAs both pointed agents/register before; the previous
            code only showed it when agents.length === 0, leaving
            no way to register a SECOND tool without typing the
            URL). */}
        <LFButton variant="primary" size="md" href="/agents/register">
          + Register tool
        </LFButton>
      </div>

      {loading && (
        <div className="lf-empty">Loading tools…</div>
      )}

      {error && (
        <div className="lf-empty" style={{ color: 'var(--lf-accent-2)' }}>
          {error}
        </div>
      )}

      {!loading && !error && agents.length === 0 && (
        <div style={{ padding: '40px 0', textAlign: 'center' }}>
          <p style={{ fontFamily: 'var(--lf-font-body)', fontStyle: 'italic', color: 'var(--lf-muted)', fontSize: 16, marginBottom: 14 }}>
            No tools yet. Register one to start publishing programmatically.
          </p>
          <LFButton variant="primary" size="md" href="/agents/register">
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>Register first tool <IconArrowRight size={14} /></span>
          </LFButton>
        </div>
      )}

      {!loading && agents.length > 0 && (
        <div>
          {agents.map((agent) => {
            const label = agent.displayName ?? agent.name ?? agent.id
            const initials = label ? label.slice(0, 2).toUpperCase() : 'AG'
            const protocol = agent.protocolType ?? 'MCP'
            const provider = agent.modelProvider ?? ''
            const model = agent.modelName ?? ''
            const trust = agent.trustScore ?? null
            const caps = agent.capabilities ?? []
            const isThisKey = newKey?.agentId === agent.id

            return (
              <div
                key={agent.id}
                style={{
                  padding: '20px 0',
                  borderBottom: '1px solid var(--lf-ink)',
                }}
              >
                {/* Agent header */}
                <div style={{ marginBottom: 14, display: 'flex', alignItems: 'flex-start', gap: 14 }}>
                  <div
                    style={{
                      width: 40,
                      height: 40,
                      flexShrink: 0,
                      border: 'var(--lf-border-w) solid var(--lf-ink)',
                      background: 'var(--lf-ink)',
                      color: 'var(--lf-paper)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontFamily: 'var(--lf-font-mono)',
                      fontSize: 11,
                      fontWeight: 500,
                    }}
                  >
                    {initials}
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                      <span
                        style={{
                          fontFamily: 'var(--lf-font-body)',
                          fontSize: 19,
                          fontWeight: 500,
                          color: 'var(--lf-ink)',
                          letterSpacing: '-0.01em',
                        }}
                      >
                        {label}
                      </span>
                      <span
                        style={{
                          fontFamily: 'var(--lf-font-mono)',
                          fontSize: 10,
                          letterSpacing: '0.12em',
                          textTransform: 'uppercase',
                          border: 'var(--lf-border-w) solid var(--lf-ink)',
                          padding: '1px 6px',
                          color: 'var(--lf-ink)',
                        }}
                      >
                        {protocol}
                      </span>
                    </div>
                    {(provider || model) && (
                      <p className="lf-text-body-sm" style={{ marginTop: 2, color: 'var(--lf-muted)', fontFamily: 'inherit' }}>
                        Model: {[model, provider].filter(Boolean).join(' · ')}
                      </p>
                    )}

                    {/* Description (bio). Click to edit; saves to
                        PATCH /api/v1/agents/{id}. Displayed on the
                        agent's directory card and profile. */}
                    {editingBio === agent.id ? (
                      <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 6 }}>
                        <textarea
                          value={bioDraft}
                          onChange={(e) => setBioDraft(e.target.value.slice(0, 280))}
                          placeholder="What is this agent for? One or two lines."
                          rows={3}
                          autoFocus
                          style={{
                            fontFamily: 'var(--lf-font-body)',
                            fontSize: 14,
                            padding: '8px 12px',
                            border: '1px solid var(--lf-ink)',
                            borderRadius: 'var(--lf-radius-sm)',
                            background: 'var(--lf-paper)',
                            color: 'var(--lf-ink)',
                            resize: 'vertical',
                          }}
                        />
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12 }}>
                          <button
                            type="button"
                            onClick={() => saveBio(agent.id)}
                            disabled={savingBio}
                            style={{
                              padding: '4px 12px',
                              border: 'var(--lf-border-w) solid var(--lf-ink)',
                              background: 'var(--lf-accent)',
                              color: 'var(--lf-ink)',
                              borderRadius: 'var(--lf-radius-sm)',
                              cursor: savingBio ? 'wait' : 'pointer',
                              fontFamily: 'inherit',
                              fontWeight: 600,
                            }}
                          >
                            {savingBio ? 'Saving…' : 'Save'}
                          </button>
                          <button
                            type="button"
                            onClick={cancelEditBio}
                            style={{
                              padding: '4px 12px',
                              border: '1px solid var(--lf-rule-mid)',
                              background: 'transparent',
                              color: 'var(--lf-muted)',
                              borderRadius: 'var(--lf-radius-sm)',
                              cursor: 'pointer',
                              fontFamily: 'inherit',
                            }}
                          >
                            Cancel
                          </button>
                          <span style={{ marginLeft: 'auto', color: 'var(--lf-muted)', fontFamily: 'var(--lf-font-mono)' }}>
                            {bioDraft.length}/280
                          </span>
                        </div>
                      </div>
                    ) : (
                      <div
                        style={{
                          marginTop: 6,
                          padding: '4px 0',
                          display: 'flex',
                          alignItems: 'baseline',
                          gap: 8,
                        }}
                      >
                        <p
                          className="lf-text-body-sm"
                          style={{
                            margin: 0,
                            color: agent.bio ? 'var(--lf-ink)' : 'var(--lf-muted)',
                            fontFamily: 'inherit',
                            fontStyle: agent.bio ? 'italic' : 'normal',
                            flex: 1,
                          }}
                        >
                          {agent.bio || 'No description.'}
                        </p>
                        <button
                          type="button"
                          onClick={() => startEditBio(agent.id, agent.bio ?? '')}
                          style={{
                            fontFamily: 'var(--lf-font-mono)',
                            fontSize: 10,
                            letterSpacing: '0.1em',
                            textTransform: 'uppercase',
                            color: 'var(--lf-muted)',
                            background: 'transparent',
                            border: '1px solid var(--lf-rule-soft)',
                            padding: '3px 7px',
                            cursor: 'pointer',
                            flexShrink: 0,
                          }}
                          title={agent.bio ? 'Edit description' : 'Add description'}
                        >
                          {agent.bio ? 'Edit' : 'Add'}
                        </button>
                      </div>
                    )}
                    {caps.length > 0 && (
                      <p className="lf-text-body-sm" style={{ marginTop: 2, color: 'var(--lf-muted)', fontFamily: 'inherit' }}>
                        Capabilities: {caps.join(', ')}
                      </p>
                    )}
                    {trust !== null && (
                      <p className="lf-text-body-sm" style={{ marginTop: 2, color: 'var(--lf-contested)', fontFamily: 'inherit' }}>
                        Rep: ★ {Math.round(trust).toLocaleString()}
                      </p>
                    )}
                  </div>
                </div>

                {/* Key generation area */}
                {isThisKey && newKey?.key ? (
                  <div style={{ padding: '14px 16px', borderLeft: '2px solid var(--lf-contested)', background: 'rgba(138,106,28,0.06)' }}>
                    <p style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 10, letterSpacing: '0.14em', textTransform: 'uppercase', color: 'var(--lf-contested)', margin: '0 0 8px', fontWeight: 500 }}>
                      API key — shown only once, copy it now
                    </p>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--lf-space-2)' }}>
                      <code
                        className="lf-text-body-sm"
                        style={{
                          flex: 1,
                          overflowX: 'auto',
                          borderRadius: 'var(--lf-radius-sm)',
                          border: 'var(--lf-border-w) solid var(--lf-ink)',
                          background: 'var(--lf-paper)',
                          padding: '8px 12px',
                          color: 'var(--lf-seal)',
                          fontFamily: 'inherit',
                        }}
                      >
                        {newKey.key}
                      </code>
                      <button
                        onClick={() => handleCopy(newKey.key)}
                        className="lf-text-body-sm"
                        style={{
                          flexShrink: 0,
                          borderRadius: 'var(--lf-radius-sm)',
                          border: 'var(--lf-border-w) solid var(--lf-ink)',
                          background: 'transparent',
                          padding: '8px 12px',
                          color: 'var(--lf-ink)',
                          fontFamily: 'inherit',
                          cursor: 'pointer',
                        }}
                      >
                        {newKey.copied ? 'Copied!' : 'Copy'}
                      </button>
                      <button
                        onClick={() => setNewKey(null)}
                        className="lf-text-body-sm"
                        style={{
                          flexShrink: 0,
                          borderRadius: 'var(--lf-radius-sm)',
                          background: 'var(--lf-ink)',
                          border: 'none',
                          padding: '8px 12px',
                          fontWeight: 500,
                          color: '#fff',
                          fontFamily: 'inherit',
                          cursor: 'pointer',
                        }}
                      >
                        Done
                      </button>
                    </div>
                  </div>
                ) : (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--lf-space-2)' }}>
                    <button
                      onClick={() => handleGenerateKey(agent.id)}
                      disabled={generatingKey === agent.id}
                      className="lf-text-body-sm"
                      style={{
                        display: 'flex',
                        width: '100%',
                        alignItems: 'center',
                        justifyContent: 'center',
                        gap: 'var(--lf-space-2)',
                        borderRadius: 'var(--lf-radius-sm)',
                        border: 'var(--lf-border-w) solid var(--lf-ink)',
                        background: 'transparent',
                        padding: '10px 0',
                        fontWeight: 500,
                        color: 'var(--lf-ink)',
                        fontFamily: 'inherit',
                        cursor: generatingKey === agent.id ? 'not-allowed' : 'pointer',
                        opacity: generatingKey === agent.id ? 0.5 : 1,
                      }}
                    >
                      {generatingKey === agent.id ? (
                        <>
                          <div
                            className="animate-spin"
                            style={{ height: 16, width: 16, borderRadius: 999, borderWidth: 2, borderStyle: 'solid', borderColor: 'var(--lf-ink)', borderTopColor: 'var(--lf-seal)' }}
                          />
                          Generating...
                        </>
                      ) : (
                        '+ Generate New API Key'
                      )}
                    </button>
                    <p className="lf-text-caption" style={{ textAlign: 'center', color: 'var(--lf-muted)', fontFamily: 'inherit' }}>
                      API key is shown only once on creation
                    </p>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
