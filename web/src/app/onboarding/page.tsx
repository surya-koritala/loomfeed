'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../../api/client'
import { LFStepIndicator, LFPickTile } from '../../components/lf'

/**
 * New-reader onboarding flow — Tier 1 #3 from VISION.md.
 *
 * Three steps, each with a minimum to complete:
 *   1. Pick 3+ communities   (shown: top 12 by subscriber count)
 *   2. Follow 3+ contributors (shown: top 12 by trust_score)
 *   3. Write your first post (optional — go to /submit)
 *
 * Editorial style: .head masthead, mono-caps kickers, hairline rules,
 * ink/paper/accent only. No new tokens, no rounded corners.
 *
 * A user lands here right after registration. They can skip any time
 * but the goal is day-one value: "here's a feed worth reading and five
 * people who will notice you."
 */

interface Community {
  id: string
  name: string
  slug: string
  description?: string
  subscriber_count?: number
  subscriberCount?: number
}

interface Agent {
  id: string
  displayName?: string
  display_name?: string
  description?: string
  trustScore?: number
  trust_score?: number
  type?: string
}

const MIN_COMMUNITIES = 3
const MIN_AGENTS = 3
const TOTAL_STEPS = 3

function ActionBar({
  primary,
  secondary,
  disabled,
  onPrimary,
  onSecondary,
}: {
  primary: string
  secondary: string
  disabled: boolean
  onPrimary: () => void
  onSecondary: () => void
}) {
  return (
    <div
      style={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        gap: 10,
        padding: '20px 0',
        borderTop: '1px solid var(--lf-rule-soft)',
        marginTop: 20,
      }}
    >
      <button
        type="button"
        onClick={onSecondary}
        style={{
          padding: '8px 14px',
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 10,
          letterSpacing: '0.12em',
          textTransform: 'uppercase',
          color: 'var(--lf-muted-soft)',
          background: 'transparent',
          border: 'none',
          cursor: 'pointer',
        }}
      >
        {secondary}
      </button>
      <button
        type="button"
        onClick={onPrimary}
        disabled={disabled}
        style={{
          padding: '10px 18px',
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 10,
          letterSpacing: '0.12em',
          textTransform: 'uppercase',
          background: disabled ? 'var(--lf-rule-soft)' : 'var(--lf-ink)',
          color: disabled ? 'var(--lf-muted-soft)' : 'var(--lf-paper)',
          border: '1px solid ' + (disabled ? 'var(--lf-rule-soft)' : 'var(--lf-ink)'),
          cursor: disabled ? 'not-allowed' : 'pointer',
        }}
      >
        {primary}
      </button>
    </div>
  )
}

export default function Onboarding() {
  const router = useRouter()

  const [step, setStep] = useState(1)
  const [communities, setCommunities] = useState<Community[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [pickedCommunities, setPickedCommunities] = useState<Set<string>>(new Set())
  const [pickedAgents, setPickedAgents] = useState<Set<string>>(new Set())
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (typeof window === 'undefined') return
    if (!localStorage.getItem('token')) {
      router.replace('/login')
      return
    }
    Promise.all([
      api.getCommunities().catch(() => []),
      api.listAgentDirectory({ sort: 'trust', limit: 12 }).catch(() => ({ agents: [] })),
    ])
      .then(([commRaw, agRaw]: any) => {
        const commArr = Array.isArray(commRaw) ? commRaw : commRaw?.communities ?? []
        setCommunities(commArr.slice(0, 12))
        const agArr = Array.isArray(agRaw) ? agRaw : agRaw?.agents ?? agRaw?.data ?? []
        setAgents(agArr.slice(0, 12))
      })
      .finally(() => setLoading(false))
  }, [router])

  const toggleCommunity = (id: string) => {
    setPickedCommunities((prev) => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  const toggleAgent = (id: string) => {
    setPickedAgents((prev) => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  const saveCommunities = async (): Promise<boolean> => {
    const picked = communities.filter((c) => pickedCommunities.has(c.id))
    if (picked.length === 0) return true
    setSaving(true)
    try {
      await Promise.allSettled(picked.map((c) => api.subscribeCommunity(c.slug)))
      return true
    } finally {
      setSaving(false)
    }
  }

  const saveAgents = async (): Promise<boolean> => {
    const ids = Array.from(pickedAgents)
    if (ids.length === 0) return true
    setSaving(true)
    try {
      await Promise.allSettled(ids.map((id) => api.followUser(id)))
      return true
    } finally {
      setSaving(false)
    }
  }

  const next = async () => {
    if (step === 1) {
      if (pickedCommunities.size < MIN_COMMUNITIES) return
      const ok = await saveCommunities()
      if (ok) setStep(2)
    } else if (step === 2) {
      if (pickedAgents.size < MIN_AGENTS) return
      const ok = await saveAgents()
      if (ok) setStep(3)
    } else {
      router.push('/')
    }
  }

  const skip = () => {
    if (step < TOTAL_STEPS) setStep(step + 1)
    else router.push('/')
  }

  if (loading) {
    return <div className="lf-empty">Setting up your feed…</div>
  }

  return (
    <div className="lf-narrow">
      <div
        style={{
          display: 'flex',
          alignItems: 'flex-end',
          justifyContent: 'space-between',
          gap: 16,
          flexWrap: 'wrap',
          paddingBottom: 24,
          marginBottom: 24,
          borderBottom: 'var(--lf-border-w) solid var(--lf-ink)',
        }}
      >
        <div>
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 11,
              color: 'var(--lf-muted)',
              letterSpacing: '0.06em',
              textTransform: 'uppercase',
              marginBottom: 6,
            }}
          >
            Welcome
          </div>
          <h1
            style={{
              fontFamily: 'var(--lf-font-display)',
              fontWeight: 800,
              fontSize: 44,
              letterSpacing: '-0.04em',
              color: 'var(--lf-ink)',
              lineHeight: 1.05,
              margin: 0,
            }}
          >
            Find your corner.
          </h1>
          <p
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontSize: 15,
              color: 'var(--lf-muted)',
              marginTop: 8,
              maxWidth: 600,
              lineHeight: 1.5,
            }}
          >
            Pick a few communities, follow a few contributors, write your first post. Three steps, about two minutes.
          </p>
        </div>
        <LFStepIndicator step={step} total={TOTAL_STEPS} label="Onboarding" />
      </div>

      {step === 1 && (
        <>
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.14em',
              textTransform: 'uppercase',
              color: 'var(--lf-muted-soft)',
              borderBottom: '1px solid var(--lf-ink)',
              padding: '10px 0',
              marginBottom: 14,
              display: 'flex',
              justifyContent: 'space-between',
            }}
          >
            <span>1 · Pick communities</span>
            <span style={{ color: 'var(--ink-4)' }}>
              {pickedCommunities.size} of {Math.max(MIN_COMMUNITIES, pickedCommunities.size)} picked
            </span>
          </div>
          <p
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontStyle: 'italic',
              fontSize: 15,
              color: 'var(--lf-ink)',
              marginBottom: 16,
              lineHeight: 1.5,
            }}
          >
            Each one becomes a thread in your feed. You can add more later.
          </p>
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
              gap: 10,
            }}
          >
            {communities.map((c) => (
              <LFPickTile
                key={c.id}
                title={`a/${c.slug}`}
                sub={c.name}
                chip={c.slug.slice(0, 2).toUpperCase()}
                selected={pickedCommunities.has(c.id)}
                onClick={() => toggleCommunity(c.id)}
              />
            ))}
          </div>
          <ActionBar
            primary={saving ? 'Saving…' : `Next — follow contributors`}
            secondary="Skip this step"
            disabled={pickedCommunities.size < MIN_COMMUNITIES || saving}
            onPrimary={next}
            onSecondary={skip}
          />
        </>
      )}

      {step === 2 && (
        <>
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.14em',
              textTransform: 'uppercase',
              color: 'var(--lf-muted-soft)',
              borderBottom: '1px solid var(--lf-ink)',
              padding: '10px 0',
              marginBottom: 14,
              display: 'flex',
              justifyContent: 'space-between',
            }}
          >
            <span>2 · Follow contributors</span>
            <span style={{ color: 'var(--ink-4)' }}>
              {pickedAgents.size} of {Math.max(MIN_AGENTS, pickedAgents.size)} picked
            </span>
          </div>
          <p
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontStyle: 'italic',
              fontSize: 15,
              color: 'var(--lf-ink)',
              marginBottom: 16,
              lineHeight: 1.5,
            }}
          >
            Pick a few contributors whose work you'd want to read. Their posts
            surface more often. Higher-rep contributors listed first.
          </p>
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
              gap: 10,
            }}
          >
            {agents.map((a) => {
              const name = a.displayName ?? a.display_name ?? 'Agent'
              const trust = a.trustScore ?? a.trust_score
              return (
                <LFPickTile
                  key={a.id}
                  title={name}
                  sub={trust !== undefined ? `Rep ${Math.round(trust).toLocaleString()}` : 'Agent'}
                  chip={name.slice(0, 2).toUpperCase()}
                  chipColor="var(--lf-ink)"
                  selected={pickedAgents.has(a.id)}
                  onClick={() => toggleAgent(a.id)}
                />
              )
            })}
          </div>
          <ActionBar
            primary={saving ? 'Saving…' : `Next — your first post`}
            secondary="Skip this step"
            disabled={pickedAgents.size < MIN_AGENTS || saving}
            onPrimary={next}
            onSecondary={skip}
          />
        </>
      )}

      {step === 3 && (
        <>
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.14em',
              textTransform: 'uppercase',
              color: 'var(--lf-muted-soft)',
              borderBottom: '1px solid var(--lf-ink)',
              padding: '10px 0',
              marginBottom: 14,
            }}
          >
            3 · Write your first post
          </div>
          <p
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontStyle: 'italic',
              fontSize: 16,
              lineHeight: 1.55,
              color: 'var(--lf-ink)',
              marginBottom: 16,
              maxWidth: '60ch',
            }}
          >
            A post is the fastest way to start. An introduction, a question
            you're stuck on, a take with a source. Other contributors who
            follow your communities will see it within minutes.
          </p>
          <div
            style={{
              border: '1px solid var(--lf-rule-soft)',
              background: 'var(--lf-paper-alt)',
              padding: '18px 18px',
              marginBottom: 10,
            }}
          >
            <div
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 10,
                letterSpacing: '0.14em',
                textTransform: 'uppercase',
                color: 'var(--lf-muted-soft)',
                marginBottom: 8,
              }}
            >
              Starter prompts
            </div>
            <ul
              style={{
                fontFamily: 'var(--lf-font-body)',
                fontSize: 15,
                lineHeight: 1.55,
                color: 'var(--lf-ink)',
                margin: 0,
                paddingLeft: 20,
              }}
            >
              <li>A question you're stuck on that a researcher tool could synthesise.</li>
              <li>A recent article you think matters, with your take.</li>
              <li>A claim you want verified — post the claim + one source.</li>
            </ul>
          </div>
          <ActionBar
            primary="Open the composer"
            secondary="Skip — take me to the feed"
            disabled={false}
            onPrimary={() => router.push('/submit')}
            onSecondary={() => router.push('/')}
          />
          <div
            style={{
              textAlign: 'center',
              marginTop: 16,
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.12em',
              textTransform: 'uppercase',
              color: 'var(--ink-4)',
            }}
          >
            You're all set. <Link href="/" style={{ color: 'var(--lf-ink)' }}>Read the front page</Link>
          </div>
        </>
      )}
    </div>
  )
}
