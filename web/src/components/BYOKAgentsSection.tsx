'use client'

import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import { isBYOKAvailable } from '../lib/byok-availability'

interface BYOKAgent {
  id: string
  displayName: string
  provider: string
  model: string
  personaPrompt: string
  enabled: boolean
  createdAt: string
}

const providerOptions = [
  { value: 'openai', label: 'OpenAI', defaultModel: 'gpt-4o-mini', keyHint: 'sk-...' },
  { value: 'anthropic', label: 'Anthropic', defaultModel: 'claude-haiku-4-5-20251001', keyHint: 'sk-ant-...' },
  { value: 'google', label: 'Google (Gemini)', defaultModel: 'gemini-2.0-flash', keyHint: 'AIza...' },
]

export default function BYOKAgentsSection() {
  const [agents, setAgents] = useState<BYOKAgent[]>([])
  const [loading, setLoading] = useState(true)
  const [byokEnabled, setBYOKEnabled] = useState<boolean | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [displayName, setDisplayName] = useState('')
  const [provider, setProvider] = useState('openai')
  const [model, setModel] = useState('gpt-4o-mini')
  const [apiKey, setApiKey] = useState('')
  const [personaPrompt, setPersonaPrompt] = useState('')
  const [bio, setBio] = useState('')

  const load = useCallback(() => {
    setLoading(true)
    api
      .listBYOKAgents()
      .then((res: any) => setAgents(res?.agents || []))
      .catch(() => setAgents([]))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    api
      .getConfig()
      .then((config) => {
        const enabled = isBYOKAvailable(config)
        setBYOKEnabled(enabled)
        if (enabled) {
          load()
        } else {
          setLoading(false)
        }
      })
      .catch(() => {
        setBYOKEnabled(false)
        setLoading(false)
      })
  }, [load])

  const onProviderChange = (v: string) => {
    setProvider(v)
    const opt = providerOptions.find((p) => p.value === v)
    if (opt) setModel(opt.defaultModel)
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (!displayName.trim() || !apiKey.trim()) {
      setError('Name and API key are required')
      return
    }
    setSaving(true)
    try {
      await api.createBYOKAgent({
        display_name: displayName.trim(),
        provider,
        model: model.trim(),
        api_key: apiKey.trim(),
        persona_prompt: personaPrompt.trim(),
        bio: bio.trim(),
      })
      setDisplayName('')
      setApiKey('')
      setPersonaPrompt('')
      setBio('')
      setShowForm(false)
      load()
    } catch (e: any) {
      setError(e?.message || 'Failed to create agent')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (id: string, name: string) => {
    if (!confirm(`Delete agent "${name}"? This cannot be undone.`)) return
    try {
      await api.deleteBYOKAgent(id)
      load()
    } catch {
      setError('Failed to delete agent')
    }
  }

  const keyHint = providerOptions.find((p) => p.value === provider)?.keyHint ?? ''

  return (
    <div style={{ padding: '20px 0', borderBottom: '1px solid var(--lf-rule-soft)' }}>
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h2 className="lf-text-h3" style={{ fontWeight: 600, color: 'var(--lf-ink)' }}>
            Your AI Agents
          </h2>
          <p className="lf-text-caption" style={{ marginTop: 4, color: 'var(--lf-muted)' }}>
            Bring your own OpenAI, Anthropic, or Google key. We encrypt it and run the agent for you.
          </p>
        </div>
        {byokEnabled === true && !showForm && (
          <button
            type="button"
            onClick={() => setShowForm(true)}
            className="rounded-lg px-3 py-1.5 text-sm font-medium"
            style={{ background: 'var(--lf-accent)', color: 'var(--lf-ink)', border: '1px solid var(--lf-ink)', cursor: 'pointer' }}
          >
            + New agent
          </button>
        )}
      </div>

      {loading || byokEnabled === null ? (
        <div className="lf-text-body-sm py-4 text-center" style={{ color: 'var(--lf-muted)' }}>
          Loading…
        </div>
      ) : byokEnabled === false ? (
        <div
          className="lf-text-body-sm rounded-lg p-4"
          style={{ background: 'var(--lf-paper-alt)', color: 'var(--lf-muted)' }}
        >
          BYOK agents are not configured on this server. Ask the operator to
          enable BYOK and provide a valid encryption key before adding an agent.
        </div>
      ) : agents.length === 0 && !showForm ? (
        <div
          className="lf-text-body-sm rounded-lg p-4 text-center"
          style={{ background: 'var(--lf-paper-alt)', color: 'var(--lf-muted)' }}
        >
          No BYOK agents yet. Click <strong>+ New agent</strong> to create one.
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          {agents.map((a) => (
            <div
              key={a.id}
              className="flex items-center justify-between rounded-lg border p-3"
              style={{ borderColor: 'var(--lf-rule-soft)', background: 'var(--lf-paper-alt)' }}
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-semibold text-sm" style={{ color: 'var(--lf-ink)' }}>
                    {a.displayName}
                  </span>
                  <span
                    className="lf-text-micro rounded px-1.5 py-0.5"
                    style={{ background: 'var(--lf-paper-alt)', color: 'var(--lf-muted)', border: '1px solid var(--lf-rule-soft)' }}
                  >
                    {a.provider}
                  </span>
                  {!a.enabled && (
                    <span
                      className="lf-text-micro rounded px-1.5 py-0.5"
                      style={{ background: 'var(--lf-paper-alt)', color: 'var(--lf-accent-2)', border: '1px solid var(--lf-rule-soft)' }}
                    >
                      disabled
                    </span>
                  )}
                </div>
                <div className="lf-text-caption" style={{ marginTop: 2, color: 'var(--lf-muted)' }}>
                  {a.model}
                </div>
              </div>
              <button
                type="button"
                onClick={() => remove(a.id, a.displayName)}
                className="lf-text-caption"
                style={{
                  background: 'transparent',
                  border: '1px solid var(--lf-rule-soft)',
                  color: 'var(--lf-muted)',
                  borderRadius: 6,
                  padding: '4px 10px',
                  cursor: 'pointer',
                }}
              >
                Delete
              </button>
            </div>
          ))}
        </div>
      )}

      {showForm && (
        <form
          onSubmit={submit}
          className="mt-4 flex flex-col gap-3 rounded-lg border p-4"
          style={{ borderColor: 'var(--lf-rule-soft)', background: 'var(--lf-paper-alt)' }}
        >
          <div className="flex flex-col gap-1.5">
            <label className="lf-text-caption" style={{ fontWeight: 500, color: 'var(--lf-muted)' }}>
              Agent name
            </label>
            <input
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="e.g. Research Partner"
              className="rounded-lg px-3 py-2 text-sm"
              style={{ border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper)', color: 'var(--lf-ink)' }}
              required
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-1.5">
              <label className="lf-text-caption" style={{ fontWeight: 500, color: 'var(--lf-muted)' }}>
                Provider
              </label>
              <select
                value={provider}
                onChange={(e) => onProviderChange(e.target.value)}
                className="rounded-lg px-3 py-2 text-sm"
                style={{ border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper)', color: 'var(--lf-ink)' }}
              >
                {providerOptions.map((p) => (
                  <option key={p.value} value={p.value}>
                    {p.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex flex-col gap-1.5">
              <label className="lf-text-caption" style={{ fontWeight: 500, color: 'var(--lf-muted)' }}>
                Model
              </label>
              <input
                type="text"
                value={model}
                onChange={(e) => setModel(e.target.value)}
                className="rounded-lg px-3 py-2 text-sm"
                style={{ border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper)', color: 'var(--lf-ink)' }}
                required
              />
            </div>
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="lf-text-caption" style={{ fontWeight: 500, color: 'var(--lf-muted)' }}>
              API key <span style={{ color: 'var(--lf-muted)' }}>(encrypted at rest)</span>
            </label>
            <input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={keyHint}
              className="rounded-lg px-3 py-2 text-sm"
              style={{ border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper)', color: 'var(--lf-ink)', fontFamily: 'var(--lf-font-mono)' }}
              required
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="lf-text-caption" style={{ fontWeight: 500, color: 'var(--lf-muted)' }}>
              Persona / system prompt
            </label>
            <textarea
              value={personaPrompt}
              onChange={(e) => setPersonaPrompt(e.target.value)}
              placeholder="You are a thoughtful skeptic. Push back where claims are weak. Cite specific sources when relevant."
              rows={4}
              className="rounded-lg px-3 py-2 text-sm"
              style={{ border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper)', color: 'var(--lf-ink)', fontFamily: 'inherit' }}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="lf-text-caption" style={{ fontWeight: 500, color: 'var(--lf-muted)' }}>
              Bio (public)
            </label>
            <input
              type="text"
              value={bio}
              onChange={(e) => setBio(e.target.value)}
              placeholder="Short description others will see on the tool's profile"
              className="rounded-lg px-3 py-2 text-sm"
              style={{ border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper)', color: 'var(--lf-ink)' }}
            />
          </div>

          {error && <div className="lf-text-body-sm" style={{ color: 'var(--lf-accent-2)' }}>{error}</div>}

          <div className="flex gap-2">
            <button
              type="submit"
              disabled={saving}
              className="rounded-lg px-4 py-2 text-sm font-medium"
              style={{ background: 'var(--lf-accent)', color: 'var(--lf-ink)', border: '1px solid var(--lf-ink)', cursor: saving ? 'wait' : 'pointer', opacity: saving ? 0.7 : 1 }}
            >
              {saving ? 'Creating…' : 'Create agent'}
            </button>
            <button
              type="button"
              onClick={() => {
                setShowForm(false)
                setError(null)
              }}
              disabled={saving}
              className="rounded-lg px-4 py-2 text-sm"
              style={{ background: 'var(--lf-paper)', color: 'var(--lf-ink)', border: '1px solid var(--lf-rule-soft)', cursor: 'pointer' }}
            >
              Cancel
            </button>
          </div>
        </form>
      )}
    </div>
  )
}
