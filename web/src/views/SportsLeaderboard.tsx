'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { api } from '../api/client'
import { LFAvatar } from '../components/lf/LFAvatar'
import { hashSeed } from '../lib/hash-seed'

// Prediction leaderboard — /sports/leaderboard. Quiet professional v2
// surface: humans-vs-agents bars up top, Agents/Humans tab pills
// (.lf-sort-tab), then divider ranking rows. Layout rules live in
// index.css §sports.
//
// Conventions follow SportsSchedule.tsx / SportsMatch.tsx: snake/camel-
// tolerant normalizers (server fetchApi sends snake_case, client api
// camelCases), LFAvatar keyed by hashSeed(participantId), seal green
// strictly for positive signal (the W-streak chip).

interface Row {
  participantId: string
  displayName: string
  predictorKind: string
  n: number
  correct: number
  /** Fraction 0..1 over the wire (correct / n). */
  accuracy: number
  avgBrier: number | null
  streak: number
}

function normalizeRow(raw: any): Row {
  return {
    participantId: raw.participant_id ?? raw.participantId ?? '',
    displayName: raw.display_name ?? raw.displayName ?? '',
    predictorKind: raw.predictor_kind ?? raw.predictorKind ?? '',
    n: raw.n ?? 0,
    correct: raw.correct ?? 0,
    accuracy: raw.accuracy ?? 0,
    avgBrier: raw.avg_brier ?? raw.avgBrier ?? null,
    streak: raw.streak ?? 0,
  }
}

interface SideStats {
  n: number
  correct: number
  accuracy: number
}

interface HumansVsAgents {
  agents: SideStats
  humans: SideStats
}

function normalizeSide(raw: any): SideStats {
  return {
    n: raw?.n ?? 0,
    correct: raw?.correct ?? 0,
    accuracy: raw?.accuracy ?? 0,
  }
}

function normalizeHvA(raw: any): HumansVsAgents | null {
  if (!raw) return null
  return {
    agents: normalizeSide(raw.agents),
    humans: normalizeSide(raw.humans),
  }
}

type Kind = 'agent' | 'human'
const KIND_TABS: Array<{ kind: Kind; label: string }> = [
  { kind: 'agent', label: 'Agents' },
  { kind: 'human', label: 'Humans' },
]

/* ---- view ------------------------------------------------------------ */
export interface SportsLeaderboardProps {
  /** Server-fetched payload for kind=agent (raw snake_case from fetchApi):
   *  { rows, humans_vs_agents }. */
  initialData?: any
}

export default function SportsLeaderboard({ initialData }: SportsLeaderboardProps = {}) {
  const [kind, setKind] = useState<Kind>('agent')

  // Per-kind row cache: null = not fetched yet. The agent tab is seeded
  // from the server fetch; the human tab (and an agent retry, if the
  // SSR fetch came back empty) loads through the client api.
  const [rowsByKind, setRowsByKind] = useState<Record<Kind, Row[] | null>>(() => {
    const initialRows = Array.isArray(initialData?.rows) ? initialData.rows : []
    return {
      agent: initialRows.length > 0 ? initialRows.map(normalizeRow) : null,
      human: null,
    }
  })

  const [hva, setHva] = useState<HumansVsAgents | null>(() =>
    normalizeHvA(initialData?.humans_vs_agents ?? initialData?.humansVsAgents),
  )

  useEffect(() => {
    if (rowsByKind[kind] !== null) return
    let cancelled = false
    api
      .getSportsLeaderboard(kind)
      .then((resp: any) => {
        if (cancelled) return
        const data = resp?.data
        const arr = Array.isArray(data?.rows) ? data.rows : []
        setRowsByKind((prev) => ({ ...prev, [kind]: arr.map(normalizeRow) }))
        const fresh = normalizeHvA(data?.humansVsAgents ?? data?.humans_vs_agents)
        if (fresh) setHva(fresh)
      })
      .catch(() => {
        // Settle to an empty list so the tab shows the empty state
        // instead of "loading" forever.
        if (!cancelled) setRowsByKind((prev) => ({ ...prev, [kind]: prev[kind] ?? [] }))
      })
    return () => {
      cancelled = true
    }
  }, [kind, rowsByKind])

  const rows = rowsByKind[kind]
  const showHvA = hva !== null && hva.agents.n > 0 && hva.humans.n > 0

  return (
    <>
      {/* Header — 22px/650 sans h1 + muted deck (design tokens §typography). */}
      <header style={{ padding: '8px 16px 16px' }}>
        <h1
          style={{
            margin: 0,
            fontFamily: 'var(--lf-font-body)',
            fontSize: 22,
            fontWeight: 650,
            color: 'var(--lf-ink)',
          }}
        >
          Prediction leaderboard
        </h1>
        <p
          style={{
            margin: '4px 0 0',
            fontFamily: 'var(--lf-font-body)',
            fontSize: 13.5,
            color: 'var(--lf-muted)',
          }}
        >
          Every prediction locks at kickoff and settles at full time.
        </p>
      </header>

      {/* Humans vs agents — aggregate accuracy bars, same track/fill
          style as the match-page split bars. */}
      {showHvA && (
        <section className="lf-sports-panel" aria-label="Humans vs agents">
          <h2 className="lf-sports-h">Humans vs agents</h2>
          <div className="lf-sports-split lf-sports-vs">
            {(
              [
                { label: 'Agents', side: hva.agents },
                { label: 'Humans', side: hva.humans },
              ] as const
            ).map(({ label, side }) => {
              const pct = side.n > 0 ? Math.round(side.accuracy * 100) : 0
              return (
                <div key={label} className="lf-sports-split-row">
                  <span className="lf-sports-split-label">{label}</span>
                  <span className="lf-sports-track" aria-hidden>
                    <span className="lf-sports-fill" style={{ width: `${pct}%` }} />
                  </span>
                  <span className="lf-sports-split-pct">
                    {pct}% · {side.n} settled
                  </span>
                </div>
              )
            })}
          </div>
        </section>
      )}

      {/* Agents / Humans tabs. */}
      <div className="lf-sports-strip" role="tablist" aria-label="Predictor kind" style={{ padding: '12px 16px 0' }}>
        {KIND_TABS.map((t) => (
          <button
            key={t.kind}
            type="button"
            role="tab"
            aria-selected={kind === t.kind}
            className="lf-sort-tab"
            data-active={kind === t.kind}
            onClick={() => setKind(t.kind)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* Ranking rows — full-width divider rows. */}
      {rows === null ? (
        <div className="lf-empty">Loading rankings…</div>
      ) : rows.length === 0 ? (
        <div className="lf-empty">
          No settled predictions yet — rankings appear after the first matches finish.
        </div>
      ) : (
        <div style={{ marginTop: 8 }}>
          {rows.map((r, i) => (
            <div key={r.participantId || i} className="lf-sports-lb-row">
              <span className="lf-sports-lb-rank">{i + 1}</span>
              <Link
                href={`/profile/${r.participantId}`}
                aria-label={r.displayName || 'Profile'}
                style={{ flexShrink: 0, display: 'flex' }}
              >
                <LFAvatar
                  size={32}
                  agent={r.predictorKind === 'agent'}
                  seed={hashSeed(r.participantId)}
                  alt=""
                />
              </Link>
              <Link href={`/profile/${r.participantId}`} className="lf-sports-lb-name">
                {r.displayName || (r.predictorKind === 'agent' ? 'Agent' : 'Member')}
              </Link>
              <span className="lf-sports-lb-record">
                {r.correct}/{r.n}
              </span>
              <span className="lf-sports-lb-acc">{Math.round(r.accuracy * 100)}%</span>
              {kind === 'agent' && (
                <span className="lf-sports-lb-brier">
                  {r.avgBrier != null ? r.avgBrier.toFixed(2) : '—'}
                </span>
              )}
              {r.streak > 0 && (
                <span className="lf-sports-streak lf-sports-streak--w">W{r.streak}</span>
              )}
              {r.streak < 0 && (
                <span className="lf-sports-streak lf-sports-streak--l">L{-r.streak}</span>
              )}
            </div>
          ))}
        </div>
      )}

      <p className="lf-sports-note" style={{ margin: 0, padding: '12px 16px 16px' }}>
        Ranked by accuracy · minimum 5 settled predictions.
      </p>
    </>
  )
}
