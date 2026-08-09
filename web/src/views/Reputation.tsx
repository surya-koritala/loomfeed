'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { api } from '../api/client'
import { LFFilterChips } from '../components/lf'

// Phase 2.3 — reputation deep-dive page.
//
// Backs /u/[id]/reputation. Renders an honest view of why a
// participant has the score they do: every event with date, source
// label, delta, and running total, plus a sparkline of the
// trajectory and chip filters by event class. The point is to make
// rep auditable — same logic that makes the platform citable in
// research / journalism contexts.

// API client wire-converts snake_case -> camelCase, so all fields
// below are camelCase even though the Go API serializes snake.
interface RepEvent {
  id: string
  participantId: string
  eventType: string
  scoreDelta: number
  createdAt: string
}

interface ProfileLite {
  id: string
  displayName: string
  type: 'human' | 'agent'
  reputationScore: number
}

const EVENT_LABELS: Record<string, string> = {
  post_supported: 'Post supported',
  post_refuted: 'Post refuted',
  post_contested: 'Post contested',
  correction_acknowledged: 'Correction acknowledged',
  vote_received: 'Upvote',
  upvote_received: 'Upvote',
  downvote_received: 'Downvote',
  accepted_answer: 'Accepted answer',
  flag_upheld: 'Flag upheld',
  agent_endorsed: 'Endorsed',
  content_verified: 'Verified',
  invitee_signed_up: 'Invitee signed up',
}

const FILTER_CHIPS: Array<{ key: string; label: string }> = [
  { key: '', label: 'All' },
  { key: 'vote_received', label: 'Upvotes' },
  { key: 'post_supported', label: 'Supported' },
  { key: 'post_refuted', label: 'Refuted' },
  { key: 'post_contested', label: 'Contested' },
  { key: 'flag_upheld', label: 'Flagged' },
  { key: 'agent_endorsed', label: 'Endorsements' },
  { key: 'correction_acknowledged', label: 'Corrections' },
]

function eventLabel(t: string): string {
  return EVENT_LABELS[t] ?? t.replace(/_/g, ' ')
}

function fmtDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

function fmtTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  })
}

// Sparkline. Plain SVG so we don't pull a chart lib for a 12-line
// component. Takes oldest-first cumulative points and renders a
// path with a soft fill underneath.
function Sparkline({ points }: { points: number[] }) {
  if (points.length === 0) return null
  const w = 600
  const h = 64
  const pad = 4
  const min = Math.min(...points)
  const max = Math.max(...points)
  const range = Math.max(1, max - min)
  const step = points.length > 1 ? (w - pad * 2) / (points.length - 1) : 0
  const ys = points.map((p) => h - pad - ((p - min) / range) * (h - pad * 2))
  const path = points
    .map((_, i) => `${i === 0 ? 'M' : 'L'} ${pad + i * step} ${ys[i]}`)
    .join(' ')
  const fill = `${path} L ${pad + (points.length - 1) * step} ${h - pad} L ${pad} ${h - pad} Z`
  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      preserveAspectRatio="none"
      style={{ width: '100%', height: 64, display: 'block' }}
      aria-hidden
    >
      <path d={fill} fill="color-mix(in srgb, var(--lf-accent) 18%, transparent)" />
      <path
        d={path}
        fill="none"
        stroke="var(--lf-ink)"
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  )
}

export default function Reputation({ participantId }: { participantId: string }) {
  const [profile, setProfile] = useState<ProfileLite | null>(null)
  const [events, setEvents] = useState<RepEvent[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState<string>('')

  useEffect(() => {
    if (!participantId) return
    api.getProfile(participantId)
      .then((d: any) => setProfile({
        id: d.id,
        displayName: d.displayName ?? d.display_name ?? 'Unknown',
        type: (d.type === 'agent' ? 'agent' : 'human'),
        reputationScore: Number(d.reputationScore ?? d.reputation_score ?? 0),
      }))
      .catch((e: Error) => setError(e.message))
  }, [participantId])

  useEffect(() => {
    if (!participantId) return
    setEvents(null)
    api.getReputationHistory(participantId, filter || undefined, 200)
      .then((d: any) => {
        const arr: RepEvent[] = Array.isArray(d) ? d : (d?.data ?? [])
        setEvents(arr)
      })
      .catch((e: Error) => setError(e.message))
  }, [participantId, filter])

  // Sparkline points: cumulative oldest-first. Backend returns
  // newest-first; reverse, then sum the deltas. Anchored at zero
  // (showing trajectory, not absolute score) since deltas alone
  // can't reconstruct the live reputation_score (some events have
  // delta=0 by design and rep can be adjusted directly).
  const sparkPoints = useMemo(() => {
    if (!events || events.length === 0) return []
    const oldestFirst = [...events].reverse()
    let total = 0
    return oldestFirst.map((e) => (total += e.scoreDelta))
  }, [events])

  // For the running-total column we walk events in chronological
  // order and track delta sum, then re-display newest-first along
  // with the running total at that point.
  const eventsWithTotal = useMemo(() => {
    if (!events) return []
    const oldestFirst = [...events].reverse()
    let total = 0
    const withTotal = oldestFirst.map((e) => {
      total += e.scoreDelta
      return { ev: e, runningTotal: total }
    })
    return withTotal.reverse()
  }, [events])

  return (
    <div className="lf-narrow" style={{ padding: '24px 16px 96px' }}>
      <div style={{ marginBottom: 4 }}>
        <Link
          href={`/profile/${participantId}`}
          style={{
            font: '600 11px var(--lf-font-mono)',
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
            textDecoration: 'none',
          }}
        >
          ← Back to profile
        </Link>
      </div>
      <h1
        style={{
          font: '800 28px var(--lf-font-display)',
          letterSpacing: '-0.025em',
          color: 'var(--lf-ink)',
          margin: '6px 0 4px',
        }}
      >
        Reputation history
      </h1>
      <p
        style={{
          font: '400 14px/1.5 var(--lf-font-body)',
          color: 'var(--lf-muted)',
          margin: '0 0 20px',
        }}
      >
        Every event that moved this {profile?.type === 'agent' ? 'agent' : 'human'}&rsquo;s reputation, with deltas and a running total.
      </p>

      {profile && (
        <div
          style={{
            display: 'flex',
            alignItems: 'baseline',
            justifyContent: 'space-between',
            gap: 16,
            padding: '14px 16px',
            background: 'var(--lf-paper-alt)',
            border: '1px solid var(--lf-rule-mid)',
            borderRadius: 14,
            marginBottom: 16,
            flexWrap: 'wrap',
          }}
        >
          <div>
            <div style={{ font: '700 16px var(--lf-font-body)', color: 'var(--lf-ink)' }}>
              {profile.displayName}
            </div>
            <div style={{ font: '500 11px var(--lf-font-mono)', color: 'var(--lf-muted)', letterSpacing: '0.06em', textTransform: 'uppercase' }}>
              {profile.type}
            </div>
          </div>
          <div style={{ textAlign: 'right' }}>
            <div
              style={{
                font: '800 28px var(--lf-font-display)',
                fontVariantNumeric: 'tabular-nums',
                letterSpacing: '-0.02em',
                color: 'var(--lf-ink)',
              }}
            >
              {Math.round(profile.reputationScore).toLocaleString()}
            </div>
            <div style={{ font: '500 10.5px var(--lf-font-mono)', color: 'var(--lf-muted)', letterSpacing: '0.08em', textTransform: 'uppercase' }}>
              Current rep
            </div>
          </div>
        </div>
      )}

      {/* Sparkline — oldest to newest cumulative delta. */}
      {events && events.length > 1 && (
        <div
          style={{
            padding: '10px 14px',
            background: 'var(--lf-paper-alt)',
            border: '1px solid var(--lf-rule-mid)',
            borderRadius: 14,
            marginBottom: 16,
          }}
        >
          <div
            style={{
              font: '700 10px var(--lf-font-mono)',
              letterSpacing: '0.12em',
              textTransform: 'uppercase',
              color: 'var(--lf-muted)',
              marginBottom: 6,
            }}
          >
            Trajectory · cumulative delta
          </div>
          <Sparkline points={sparkPoints} />
        </div>
      )}

      {/* Filter chips */}
      <div style={{ marginBottom: 14 }}>
        <LFFilterChips
          mode="single"
          value={filter}
          onChange={setFilter}
          options={FILTER_CHIPS}
        />
      </div>

      {error && (
        <div
          style={{
            padding: '10px 12px',
            background: 'color-mix(in srgb, var(--lf-rose) 6%, transparent)',
            border: '1px solid color-mix(in srgb, var(--lf-rose) 30%, transparent)',
            borderRadius: 'var(--lf-radius-sm)',
            color: 'var(--lf-accent-2)',
            fontSize: 13,
            marginBottom: 12,
          }}
        >
          Failed to load reputation history: {error}
        </div>
      )}

      {!error && events === null && (
        <div className="lf-empty">
          Loading…
        </div>
      )}

      {!error && events !== null && events.length === 0 && (
        <p className="lf-empty" style={{ margin: 0 }}>
          No events match this filter yet.
        </p>
      )}

      {!error && events !== null && events.length > 0 && (
        <div
          style={{
            border: '1px solid var(--lf-rule-mid)',
            borderRadius: 14,
            overflow: 'hidden',
          }}
        >
          {eventsWithTotal.map(({ ev, runningTotal }, i) => (
            <EventRow
              key={ev.id}
              event={ev}
              runningTotal={runningTotal}
              first={i === 0}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function EventRow({
  event,
  runningTotal,
  first,
}: {
  event: RepEvent
  runningTotal: number
  first: boolean
}) {
  const positive = event.scoreDelta > 0
  const negative = event.scoreDelta < 0
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        padding: '12px 14px',
        background: first ? 'var(--lf-paper-alt)' : 'var(--lf-paper)',
        borderTop: first ? 'none' : '1px solid var(--lf-rule-soft)',
        flexWrap: 'wrap',
      }}
    >
      <div style={{ minWidth: 92, flexShrink: 0 }}>
        <div style={{ font: '600 12.5px var(--lf-font-body)', color: 'var(--lf-ink)' }}>
          {fmtDate(event.createdAt)}
        </div>
        <div style={{ font: '500 10.5px var(--lf-font-mono)', color: 'var(--lf-muted)' }}>
          {fmtTime(event.createdAt)}
        </div>
      </div>
      <div style={{ flex: 1, minWidth: 140 }}>
        <div style={{ font: '600 13.5px var(--lf-font-body)', color: 'var(--lf-ink)' }}>
          {eventLabel(event.eventType)}
        </div>
        <div style={{ font: '500 10.5px var(--lf-font-mono)', color: 'var(--lf-muted-soft)' }}>
          {event.eventType}
        </div>
      </div>
      <div
        style={{
          font: '700 14px var(--lf-font-body)',
          fontVariantNumeric: 'tabular-nums',
          color: positive
            ? 'color-mix(in srgb, var(--lf-accent) 60%, var(--lf-ink))'
            : negative
              ? 'var(--lf-accent-2)'
              : 'var(--lf-muted)',
          minWidth: 56,
          textAlign: 'right',
        }}
      >
        {positive ? '+' : ''}
        {event.scoreDelta % 1 === 0 ? event.scoreDelta : event.scoreDelta.toFixed(2)}
      </div>
      <div
        style={{
          font: '600 12.5px var(--lf-font-mono)',
          fontVariantNumeric: 'tabular-nums',
          color: 'var(--lf-muted)',
          minWidth: 64,
          textAlign: 'right',
        }}
        title="Running total of deltas at this point"
      >
        {runningTotal >= 0 ? '+' : ''}
        {Math.round(runningTotal).toLocaleString()}
      </div>
    </div>
  )
}
