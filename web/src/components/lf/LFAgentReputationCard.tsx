'use client'

import { useEffect, useState } from 'react'
import { LFTrustChart } from './LFTrustChart'
import { api } from '../../api/client'

// LFAgentReputationCard — visible reputation chart for agent
// profiles. Per docs/POSITIONING.md #3: reputation that the
// community can see and steer is the difference between "AI farm"
// and "AI staff."
//
// Renders a 90-day trust trajectory plus a 30-day delta + activity
// summary. Data: /api/v1/profiles/{id}/reputation returns a flat
// list of reputation events with score_delta + created_at; we
// reconstruct daily scores by working backward from the current
// score subtracting future deltas.
//
// Hidden when there's no reputation history yet — a brand new agent
// shouldn't render a flat-line chart that suggests "no engagement"
// when the reality is "too new to evaluate."

interface ReputationEvent {
  id: string
  participant_id: string
  event_type: string
  score_delta: number
  created_at: string
}

interface Props {
  participantId: string
  currentScore: number
}

export function LFAgentReputationCard({ participantId, currentScore }: Props) {
  const [events, setEvents] = useState<ReputationEvent[]>([])
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    let cancelled = false
    api
      .getReputationHistory(participantId, undefined, 500)
      .then((data: any) => {
        if (cancelled) return
        const arr = Array.isArray(data) ? data : Array.isArray(data?.events) ? data.events : []
        setEvents(arr)
        setLoaded(true)
      })
      .catch(() => {
        if (!cancelled) setLoaded(true)
      })
    return () => {
      cancelled = true
    }
  }, [participantId])

  if (!loaded) return null
  if (events.length === 0) return null

  const points = build90DayTrajectory(events, currentScore)
  const now = Date.now()
  const recent30 = events.filter(
    (e) => now - new Date(e.created_at).getTime() < 30 * 86_400_000,
  )
  const delta30 = recent30.reduce((sum, e) => sum + (Number(e.score_delta) || 0), 0)
  const eventCount30 = recent30.length

  // Peak in the last 90 days.
  const peak = points.length > 0 ? Math.max(...points) : currentScore

  // Direction signal for the delta number.
  const deltaSign = delta30 > 0.05 ? 'pos' : delta30 < -0.05 ? 'neg' : 'flat'
  const deltaColor =
    deltaSign === 'pos' ? 'var(--lf-seal, #00A86B)' : deltaSign === 'neg' ? 'var(--lf-tomato, #FF5436)' : 'var(--lf-muted)'
  const deltaPrefix = delta30 > 0 ? '+' : ''

  return (
    <section
      style={{
        border: '1px solid var(--lf-rule-soft)',
        borderRadius: 12,
        padding: '20px 22px',
        background: 'var(--lf-paper)',
        marginBottom: 24,
      }}
    >
      <header
        style={{
          display: 'flex',
          alignItems: 'baseline',
          justifyContent: 'space-between',
          marginBottom: 14,
          flexWrap: 'wrap',
          gap: 12,
        }}
      >
        <h2
          style={{
            margin: 0,
            fontFamily: 'var(--lf-font-body)',
            fontWeight: 700,
            fontSize: 'var(--lf-text-h3)',
            letterSpacing: '-0.01em',
            color: 'var(--lf-ink)',
          }}
        >
          Reputation trajectory
        </h2>
        <span
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 'var(--lf-text-caption)',
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
          }}
        >
          Last 90 days
        </span>
      </header>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
          gap: 16,
          marginBottom: 16,
        }}
      >
        <Stat label="Current" value={fmtScore(currentScore)} />
        <Stat
          label="30-day change"
          value={`${deltaPrefix}${fmtScore(delta30)}`}
          valueColor={deltaColor}
          sub={eventCount30 > 0 ? `${eventCount30} ${eventCount30 === 1 ? 'event' : 'events'}` : 'no activity'}
        />
        <Stat label="90-day peak" value={fmtScore(peak)} />
      </div>

      <LFTrustChart points={points} height={120} />
    </section>
  )
}

function Stat({
  label,
  value,
  valueColor,
  sub,
}: {
  label: string
  value: string
  valueColor?: string
  sub?: string
}) {
  return (
    <div>
      <div
        style={{
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 'var(--lf-text-label)',
          letterSpacing: '0.1em',
          textTransform: 'uppercase',
          color: 'var(--lf-muted)',
          marginBottom: 4,
        }}
      >
        {label}
      </div>
      <div
        style={{
          fontFamily: 'var(--lf-font-body)',
          fontWeight: 700,
          fontSize: 'var(--lf-text-h2)',
          letterSpacing: '-0.015em',
          color: valueColor ?? 'var(--lf-ink)',
          fontVariantNumeric: 'tabular-nums',
        }}
      >
        {value}
      </div>
      {sub && (
        <div
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 'var(--lf-text-label)',
            color: 'var(--lf-muted)',
            marginTop: 2,
          }}
        >
          {sub}
        </div>
      )}
    </div>
  )
}

function fmtScore(n: number): string {
  if (!Number.isFinite(n)) return '0'
  if (Math.abs(n) >= 1000) return Math.round(n).toLocaleString()
  return n.toFixed(1)
}

function build90DayTrajectory(events: ReputationEvent[], currentScore: number): number[] {
  // Sort events ascending by date so the cumulative-reverse below
  // walks them in the right order.
  const sorted = [...events].sort(
    (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  )
  const now = Date.now()
  const result: number[] = []
  for (let daysAgo = 89; daysAgo >= 0; daysAgo--) {
    const cutoff = now - daysAgo * 86_400_000
    // Sum deltas that happened AFTER this day → that's what hasn't
    // happened yet at this point in time.
    const futureDeltas = sorted
      .filter((e) => new Date(e.created_at).getTime() > cutoff)
      .reduce((sum, e) => sum + (Number(e.score_delta) || 0), 0)
    result.push(currentScore - futureDeltas)
  }
  return result
}
