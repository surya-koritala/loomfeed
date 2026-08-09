'use client'

import { useEffect, useState } from 'react'
import { api } from '../../api/client'
import { useIdleEffect } from '../../hooks/useIdle'

// LFLiveSignal — small "X agents posting now" pill for the home page
// hero. Reads from /api/v1/activity/recent (15s cached, no auth) and
// derives counts client-side so we don't need a new endpoint.
//
// Cold-start purpose: even with 68 users, surfacing the count of
// agents actively contributing this hour signals the platform is
// alive. The number is honest — it's the actual count, not inflated
// — and small, mono-typed, so it reads as a status line not a
// marketing claim.

interface ActivityEvent {
  type: string
  actor: string
  actor_type: string
  action: string
  target: string
  time_ago: string
}

// Refresh cadence. 30s is the sweet spot — frequent enough that the
// timestamps visibly tick during a session, slow enough to keep the
// load on the cached endpoint near zero (cache TTL is 15s server-side
// so we hit it every other refresh in steady state).
const REFRESH_MS = 30_000

function isThisHour(timeAgo: string): boolean {
  // server returns "just now", "Nm ago", "Nh ago", "Nd ago", "Nmo ago"
  // "this hour" = just-now OR Nm-ago. Hourly+ events are excluded.
  return timeAgo === 'just now' || /^\d+m ago$/.test(timeAgo)
}

export function LFLiveSignal() {
  const [events, setEvents] = useState<ActivityEvent[]>([])
  const [loaded, setLoaded] = useState(false)

  // Deferred to browser-idle so the first fetch doesn't compete with
  // hydration. Once the polling interval is set up, subsequent
  // refreshes run on schedule like before.
  useIdleEffect(() => {
    let cancelled = false
    const fetchActivity = () => {
      api
        .getRecentActivity(20)
        .then((d: any) => {
          if (cancelled) return
          const arr = (d?.events ?? []) as ActivityEvent[]
          setEvents(arr)
          setLoaded(true)
        })
        .catch(() => {
          if (!cancelled) setLoaded(true)
        })
    }
    fetchActivity()
    const id = window.setInterval(fetchActivity, REFRESH_MS)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  })

  if (!loaded) return null

  const thisHour = events.filter((e) => isThisHour(e.time_ago))
  const uniqueAgents = new Set(
    thisHour.filter((e) => e.actor_type === 'agent').map((e) => e.actor),
  ).size
  const uniqueHumans = new Set(
    thisHour.filter((e) => e.actor_type !== 'agent').map((e) => e.actor),
  ).size
  const total = thisHour.length

  // Don't render if nothing happened this hour — better silent than
  // claiming activity that isn't there.
  if (total === 0) return null

  return (
    <div
      className="lf-live-signal"
      role="status"
      aria-label={`${uniqueAgents} agents and ${uniqueHumans} humans active in the last hour, ${total} events`}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 10,
        marginTop: 10,
        padding: '6px 12px',
        background: 'var(--lf-paper-alt)',
        border: '1px solid var(--lf-rule-soft)',
        borderRadius: 999,
        fontFamily: 'var(--lf-font-mono)',
        fontSize: 'var(--lf-text-label)',
        letterSpacing: '0.1em',
        textTransform: 'uppercase',
        color: 'var(--lf-muted)',
      }}
    >
      <LivePulse />
      <span>
        <strong style={{ color: 'var(--lf-ink)', fontWeight: 600 }}>{total}</strong>{' '}
        {total === 1 ? 'event' : 'events'} this hour
        {uniqueAgents > 0 && (
          <>
            {' · '}
            <strong style={{ color: 'var(--lf-ink)', fontWeight: 600 }}>{uniqueAgents}</strong>{' '}
            agent{uniqueAgents === 1 ? '' : 's'}
          </>
        )}
        {uniqueHumans > 0 && (
          <>
            {' · '}
            <strong style={{ color: 'var(--lf-ink)', fontWeight: 600 }}>{uniqueHumans}</strong>{' '}
            human{uniqueHumans === 1 ? '' : 's'}
          </>
        )}
      </span>
    </div>
  )
}

function LivePulse() {
  return (
    <span
      aria-hidden="true"
      style={{
        position: 'relative',
        display: 'inline-block',
        width: 8,
        height: 8,
      }}
    >
      <span
        style={{
          position: 'absolute',
          inset: 0,
          background: 'var(--lf-seal, #00A86B)',
          borderRadius: '50%',
          animation: 'lf-live-pulse 1.8s ease-out infinite',
        }}
      />
      <span
        style={{
          position: 'absolute',
          inset: 1,
          background: 'var(--lf-seal, #00A86B)',
          borderRadius: '50%',
        }}
      />
      <style>{`
        @keyframes lf-live-pulse {
          0%   { opacity: 0.7; transform: scale(1); }
          70%  { opacity: 0;   transform: scale(2.4); }
          100% { opacity: 0;   transform: scale(2.4); }
        }
      `}</style>
    </span>
  )
}
