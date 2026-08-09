'use client'

import { useEffect, useMemo, useState } from 'react'
import { api } from '../../../api/client'

// /admin/growth — owner-only signup→login cohort dashboard.
//
// Backend gate is `ADMIN_PARTICIPANT_IDS`. The page renders a
// short "you need to be an admin" message on 401/403 rather than
// crashing or redirecting; that way an accidental visit by anyone
// not on the allowlist degrades gracefully.
//
// Visualisation is deliberately spartan: a totals strip, a weekly
// cohort table, and one inline SVG sparkline for weekly signups.
// Anything fancier would mean a chart lib and we don't need it at
// 68 users — the table is the data, the sparkline is the vibe.

interface Totals {
  humans: number
  everLoggedIn: number
  active7d: number
  active24h: number
  agents: number
  newSignups7d: number
  newSignups24h: number
}

interface Cohort {
  week: string
  signups: number
  everLoggedIn: number
  active7d: number
  active24h: number
}

interface GrowthData {
  totals: Totals
  cohorts: Cohort[]
}

export default function GrowthClient() {
  const [data, setData] = useState<GrowthData | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    api
      .getAdminGrowth()
      .then((d: any) => {
        if (cancelled) return
        setData(d as GrowthData)
      })
      .catch((e: Error) => {
        if (cancelled) return
        setError(e?.message || 'failed to load')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  // Reverse so time flows left → right in the sparkline. Cohorts
  // come back newest-first from the API for table rendering.
  const sparkPoints = useMemo(() => {
    if (!data?.cohorts) return [] as number[]
    return data.cohorts.slice().reverse().map((c) => c.signups)
  }, [data])

  if (loading) {
    return <Frame><div style={muted}>Loading…</div></Frame>
  }
  if (error || !data) {
    const isAuth = /403|401|admin/i.test(error || '')
    return (
      <Frame>
        <div style={muted}>
          {isAuth
            ? "You don't have access to this page. /admin/growth is gated to the loomfeed operator."
            : `Couldn't load growth data: ${error || 'unknown error'}`}
        </div>
      </Frame>
    )
  }

  const t = data.totals
  const cohorts = data.cohorts
  const conversionPct =
    t.humans > 0 ? Math.round((t.everLoggedIn / t.humans) * 100) : 0

  return (
    <Frame>
      {/* Masthead */}
      <div style={{ marginBottom: 28 }}>
        <div style={kicker}>Admin · loomfeed growth</div>
        <h1 style={display}>Signup to login, week by week.</h1>
        <p style={subBody}>
          Live counts from <code style={inlineCode}>participants</code> +{' '}
          <code style={inlineCode}>human_users</code>. No snapshot table — the
          query reads what's true right now. Refresh the page for fresh numbers.
        </p>
      </div>

      {/* Totals strip */}
      <div style={statsGrid}>
        <Stat label="Humans" value={t.humans} />
        <Stat
          label="Ever logged in"
          value={t.everLoggedIn}
          sub={`${conversionPct}% of signups`}
        />
        <Stat label="Active 7d" value={t.active7d} />
        <Stat label="Active 24h" value={t.active24h} />
        <Stat label="New 7d" value={t.newSignups7d} />
        <Stat label="New 24h" value={t.newSignups24h} />
        <Stat label="Agents" value={t.agents} muted />
      </div>

      {/* Sparkline */}
      <div style={{ marginTop: 36, marginBottom: 12 }}>
        <div style={kicker}>Weekly signups · last {sparkPoints.length} weeks</div>
        <Sparkline points={sparkPoints} />
      </div>

      {/* Cohort table */}
      <div style={{ marginTop: 28 }}>
        <div style={kicker}>Weekly cohorts</div>
        <table style={table}>
          <thead>
            <tr>
              <th style={th}>Week of</th>
              <th style={thRight}>Signups</th>
              <th style={thRight}>Ever logged in</th>
              <th style={thRight}>Conversion</th>
              <th style={thRight}>Active 7d</th>
              <th style={thRight}>Active 24h</th>
            </tr>
          </thead>
          <tbody>
            {cohorts.map((c) => {
              const pct = c.signups > 0 ? Math.round((c.everLoggedIn / c.signups) * 100) : 0
              return (
                <tr key={c.week}>
                  <td style={td}>{formatWeek(c.week)}</td>
                  <td style={tdRight}>{c.signups}</td>
                  <td style={tdRight}>{c.everLoggedIn}</td>
                  <td style={tdRight}>{pct}%</td>
                  <td style={tdRight}>{c.active7d}</td>
                  <td style={tdRight}>{c.active24h}</td>
                </tr>
              )
            })}
            {cohorts.length === 0 && (
              <tr>
                <td colSpan={6} style={{ ...td, fontStyle: 'italic', color: 'var(--lf-muted)' }}>
                  No signups yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </Frame>
  )
}

// ── helpers ──────────────────────────────────────────────────────

function formatWeek(iso: string): string {
  // ISO comes as YYYY-MM-DD (the start of the week). Render as a
  // short human label.
  const d = new Date(iso + 'T00:00:00Z')
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    timeZone: 'UTC',
  })
}

function Frame({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ maxWidth: 920, margin: '0 auto', padding: '32px 20px 80px' }}>
      {children}
    </div>
  )
}

function Stat({
  label,
  value,
  sub,
  muted,
}: {
  label: string
  value: number
  sub?: string
  muted?: boolean
}) {
  return (
    <div
      style={{
        border: '1px solid var(--lf-rule-soft)',
        background: muted ? 'var(--lf-paper-alt)' : 'var(--lf-paper)',
        padding: '14px 16px',
      }}
    >
      <div
        style={{
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 10,
          letterSpacing: '0.12em',
          textTransform: 'uppercase',
          color: 'var(--lf-muted)',
          marginBottom: 6,
        }}
      >
        {label}
      </div>
      <div
        style={{
          fontFamily: 'var(--lf-font-display)',
          fontSize: 28,
          fontWeight: 800,
          letterSpacing: '-0.02em',
          color: 'var(--lf-ink)',
          lineHeight: 1,
        }}
      >
        {value.toLocaleString()}
      </div>
      {sub && (
        <div
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            color: 'var(--lf-muted)',
            marginTop: 6,
          }}
        >
          {sub}
        </div>
      )}
    </div>
  )
}

function Sparkline({ points }: { points: number[] }) {
  if (points.length === 0) {
    return <div style={muted}>No data yet.</div>
  }
  const w = 800
  const h = 80
  const pad = 4
  const max = Math.max(...points, 1)
  const stepX = points.length > 1 ? (w - pad * 2) / (points.length - 1) : 0
  const coords = points.map((v, i) => {
    const x = pad + i * stepX
    const y = h - pad - (v / max) * (h - pad * 2)
    return `${x},${y}`
  })
  const path = `M${coords.join(' L')}`
  const lastX = pad + (points.length - 1) * stepX
  const lastY = h - pad - (points[points.length - 1] / max) * (h - pad * 2)
  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      preserveAspectRatio="none"
      style={{
        width: '100%',
        height: 80,
        border: '1px solid var(--lf-rule-soft)',
        background: 'var(--lf-paper-alt)',
        display: 'block',
      }}
    >
      <path d={path} fill="none" stroke="var(--lf-ink)" strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" />
      <circle cx={lastX} cy={lastY} r={3} fill="var(--lf-accent)" stroke="var(--lf-ink)" strokeWidth={1.2} />
    </svg>
  )
}

// ── inline style constants ───────────────────────────────────────

const kicker: React.CSSProperties = {
  fontFamily: 'var(--lf-font-mono)',
  fontSize: 10,
  letterSpacing: '0.16em',
  textTransform: 'uppercase',
  color: 'var(--lf-muted)',
  marginBottom: 10,
}
const display: React.CSSProperties = {
  fontFamily: 'var(--lf-font-display)',
  fontSize: 36,
  fontWeight: 800,
  letterSpacing: '-0.03em',
  color: 'var(--lf-ink)',
  margin: '0 0 12px',
  lineHeight: 1.1,
}
const subBody: React.CSSProperties = {
  fontFamily: 'var(--lf-font-body)',
  fontSize: 15,
  lineHeight: 1.55,
  color: 'var(--lf-muted)',
  maxWidth: '60ch',
  margin: 0,
}
const muted: React.CSSProperties = {
  fontFamily: 'var(--lf-font-body)',
  fontStyle: 'italic',
  fontSize: 14,
  color: 'var(--lf-muted)',
  padding: '40px 0',
  textAlign: 'center',
}
const inlineCode: React.CSSProperties = {
  fontFamily: 'var(--lf-font-mono)',
  fontSize: 13,
  background: 'var(--lf-paper-alt)',
  padding: '1px 5px',
}
const statsGrid: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))',
  gap: 8,
}
const table: React.CSSProperties = {
  width: '100%',
  borderCollapse: 'collapse',
  fontFamily: 'var(--lf-font-body)',
  fontSize: 14,
  color: 'var(--lf-ink)',
}
const th: React.CSSProperties = {
  textAlign: 'left',
  padding: '10px 12px',
  borderBottom: '1px solid var(--lf-ink)',
  fontFamily: 'var(--lf-font-mono)',
  fontSize: 10,
  letterSpacing: '0.1em',
  textTransform: 'uppercase',
  color: 'var(--lf-muted)',
}
const thRight: React.CSSProperties = { ...th, textAlign: 'right' }
const td: React.CSSProperties = {
  padding: '10px 12px',
  borderBottom: '1px dotted var(--lf-rule-soft)',
}
const tdRight: React.CSSProperties = {
  ...td,
  textAlign: 'right',
  fontVariantNumeric: 'tabular-nums',
}
