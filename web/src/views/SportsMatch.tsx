'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import Link from 'next/link'
import { api } from '../api/client'
import { useToast } from '../components/ToastProvider'
import { LFAvatar } from '../components/lf/LFAvatar'
import { SportsCrest } from '../components/lf/SportsCrest'
import { IconBall, IconCardSwatch, IconSubArrows, IconWhistle } from '../components/lf/icons'
import { hashSeed } from '../lib/hash-seed'
import { LIVE_STATUSES, localKickoffTime, humanizeGroup } from '../lib/sports-format'

// Match detail — /sports/match/[id]. Quiet professional v2 surface:
// score header (crests + big tabular score), the human pick widget,
// the community split bars, and the agent predictions list with
// public track records. Layout rules live in index.css §sports.
//
// Conventions follow SportsSchedule.tsx: snake/camel-tolerant
// normalizers (server fetchApi sends snake_case, client api camelCases),
// Crest fallback to the 3-letter code, lime strictly for the live dot,
// and seal green only for "Called it".

interface Match {
  id: string
  competition: string
  stage: string
  groupName: string
  homeTeam: string
  homeCode: string
  homeCrest: string
  awayTeam: string
  awayCode: string
  awayCrest: string
  kickoffUtc: string
  status: string
  homeScore: number | null
  awayScore: number | null
  venue: string
}

function normalizeMatch(raw: any): Match {
  return {
    id: raw.id,
    competition: raw.competition ?? '',
    stage: raw.stage ?? '',
    groupName: raw.group_name ?? raw.groupName ?? '',
    homeTeam: raw.home_team ?? raw.homeTeam ?? '',
    homeCode: raw.home_code ?? raw.homeCode ?? '',
    homeCrest: raw.home_crest ?? raw.homeCrest ?? '',
    awayTeam: raw.away_team ?? raw.awayTeam ?? '',
    awayCode: raw.away_code ?? raw.awayCode ?? '',
    awayCrest: raw.away_crest ?? raw.awayCrest ?? '',
    kickoffUtc: raw.kickoff_utc ?? raw.kickoffUtc ?? '',
    status: raw.status ?? 'SCHEDULED',
    homeScore: raw.home_score ?? raw.homeScore ?? null,
    awayScore: raw.away_score ?? raw.awayScore ?? null,
    venue: raw.venue ?? '',
  }
}

interface Prediction {
  id: string
  participantId: string
  predictorKind: string
  displayName: string
  homeProb: number | null
  drawProb: number | null
  awayProb: number | null
  pick: string
  reasoning: string
  outcome: string | null
  statsN: number
  statsCorrect: number
  statsBrier: number | null
}

function normalizePrediction(raw: any): Prediction {
  return {
    id: raw.id ?? '',
    participantId: raw.participant_id ?? raw.participantId ?? '',
    predictorKind: raw.predictor_kind ?? raw.predictorKind ?? '',
    displayName: raw.display_name ?? raw.displayName ?? '',
    homeProb: raw.home_prob ?? raw.homeProb ?? null,
    drawProb: raw.draw_prob ?? raw.drawProb ?? null,
    awayProb: raw.away_prob ?? raw.awayProb ?? null,
    pick: raw.pick ?? '',
    reasoning: raw.reasoning ?? '',
    outcome: raw.outcome ?? null,
    statsN: raw.stats_n ?? raw.statsN ?? 0,
    statsCorrect: raw.stats_correct ?? raw.statsCorrect ?? 0,
    statsBrier: raw.stats_avg_brier ?? raw.statsAvgBrier ?? null,
  }
}

// Aggregates wire shape (repository.PredictionAggregates): per-pick
// counts `home`/`draw`/`away`, `total`, optional `avg_probs` (average
// agent probabilities), and `viewer` — the caller's own prediction
// when the request was authed, else null.
interface Aggregates {
  home: number
  draw: number
  away: number
  total: number
  avgProbs: { home: number | null; draw: number | null; away: number | null } | null
  viewer: Prediction | null
}

function normalizeAggregates(raw: any): Aggregates {
  const probs = raw?.avg_probs ?? raw?.avgProbs ?? null
  return {
    home: raw?.home ?? 0,
    draw: raw?.draw ?? 0,
    away: raw?.away ?? 0,
    total: raw?.total ?? 0,
    avgProbs: probs
      ? { home: probs.home ?? null, draw: probs.draw ?? null, away: probs.away ?? null }
      : null,
    viewer: raw?.viewer ? normalizePrediction(raw.viewer) : null,
  }
}

// Timeline wire shape — /sports/matches/:id/timeline returns merged
// ESPN play-by-play events and agent takes ASC by seq. Server fetches
// send snake_case; the client api wrapper camelCases — normalize both.
interface TimelineEvent {
  seq: number
  minute: string
  kind: string
  side: string
  player: string
  body: string
}

interface TimelineTake {
  id: string
  participantId: string
  eventSeq: number
  body: string
  createdAt: string
  displayName: string
  pick: string
  outcome: string | null
}

interface TimelineItem {
  kind: string // 'event' | 'take'
  event: TimelineEvent | null
  take: TimelineTake | null
}

function normalizeTimelineItem(raw: any): TimelineItem {
  const ev = raw?.event ?? null
  const tk = raw?.take ?? null
  return {
    kind: raw?.kind ?? (tk ? 'take' : 'event'),
    event: ev
      ? {
          seq: ev.seq ?? 0,
          minute: ev.minute ?? '',
          kind: ev.kind ?? '',
          side: ev.side ?? '',
          player: ev.player ?? '',
          body: ev.body ?? '',
        }
      : null,
    take: tk
      ? {
          id: tk.id ?? '',
          participantId: tk.participant_id ?? tk.participantId ?? '',
          eventSeq: tk.event_seq ?? tk.eventSeq ?? 0,
          body: tk.body ?? '',
          createdAt: tk.created_at ?? tk.createdAt ?? '',
          displayName: tk.display_name ?? tk.displayName ?? '',
          pick: tk.pick ?? '',
          outcome: tk.outcome ?? null,
        }
      : null,
  }
}

// Lineups wire shape — /sports/matches/:id/lineups returns
// {home, away} (raw ESPN-derived JSON, single-word keys so snake/camel
// agree) or null when no lineups have been ingested.
interface LineupPlayer {
  name: string
  jersey: string
  pos: string
}

interface LineupSide {
  team: string
  formation: string
  starters: LineupPlayer[]
  bench: LineupPlayer[]
}

interface Lineups {
  home: LineupSide | null
  away: LineupSide | null
}

function normalizeLineupSide(raw: any): LineupSide | null {
  if (!raw) return null
  const players = (arr: any): LineupPlayer[] =>
    (Array.isArray(arr) ? arr : []).map((p: any) => ({
      name: p?.name ?? '',
      jersey: p?.jersey != null ? String(p.jersey) : '',
      pos: p?.pos ?? '',
    }))
  return {
    team: raw.team ?? '',
    formation: raw.formation ?? '',
    starters: players(raw.starters),
    bench: players(raw.bench),
  }
}

/* ---- date helpers (same tables as SportsSchedule) -------------------- */
const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

/** Local "Sat 14 Jun" — viewer-timezone, render with suppressHydrationWarning. */
function localDayLabel(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return `${WEEKDAYS[d.getDay()]} ${d.getDate()} ${MONTHS[d.getMonth()]}`
}

// Statuses that still allow predictions (kickoff time pending).
const PRE_MATCH_STATUSES = new Set(['SCHEDULED', 'TIMED'])
const STATUS_WORDS: Record<string, string> = {
  POSTPONED: 'Postponed',
  SUSPENDED: 'Suspended',
  CANCELLED: 'Cancelled',
}

/* ---- timeline kind icons ---------------------------------------------- */
// Glyph for the minute gutter — broadcast pass. Goals get the lime
// disc, cards the swatch (tomato when the commentary says red), subs
// the swap arrows, half/full time the whistle; plain play rows none.
function iconForKind(ev: TimelineEvent) {
  switch (ev.kind) {
    case 'goal':
      return (
        <span className="lf-tl-ico lf-tl-ico-goal">
          <IconBall size={12} />
        </span>
      )
    case 'card':
      return (
        <span
          className={
            'lf-tl-ico lf-tl-ico-card' +
            (/red card/i.test(ev.body) ? ' lf-tl-ico-card--red' : '')
          }
        >
          <IconCardSwatch size={14} />
        </span>
      )
    case 'sub':
      return (
        <span className="lf-tl-ico">
          <IconSubArrows size={14} />
        </span>
      )
    case 'ht':
    case 'ft':
      return (
        <span className="lf-tl-ico">
          <IconWhistle size={14} />
        </span>
      )
    default:
      return null
  }
}

/* ---- view ------------------------------------------------------------- */
type PickKey = 'home' | 'draw' | 'away'
const PICKS: PickKey[] = ['home', 'draw', 'away']

export interface SportsMatchProps {
  /** Server-fetched match (raw snake_case from fetchApi). */
  initialMatch: any
  /** Server-fetched prediction aggregates (raw snake_case, anon — no viewer). */
  initialAggregates?: any
}

export default function SportsMatch({ initialMatch, initialAggregates }: SportsMatchProps) {
  const { addToast } = useToast()

  const [match, setMatch] = useState<Match>(() => normalizeMatch(initialMatch ?? {}))
  const [agg, setAgg] = useState<Aggregates>(() => normalizeAggregates(initialAggregates))

  // Hydration-safe logged-in flag (same pattern as Home.tsx): SSR and
  // the first client render show the anon markup; authed sessions
  // re-render after mount. Auth-dependent markup gates ONLY on this.
  const [isAuthed, setIsAuthed] = useState(false)
  useEffect(() => { setIsAuthed(!!localStorage.getItem('token')) }, [])

  // Kickoff lock — also effect-set, never computed at render time: it
  // reads Date.now(), which would diverge between the revalidate-cached
  // server HTML and the client's first render. First paint assumes
  // "not locked"; the effect corrects immediately after mount and a
  // timer flips it the moment kickoff passes.
  const [locked, setLocked] = useState(false)
  useEffect(() => {
    const started = !PRE_MATCH_STATUSES.has(match.status)
    const kickoffMs = new Date(match.kickoffUtc).getTime()
    const update = () =>
      setLocked(started || (!isNaN(kickoffMs) && Date.now() >= kickoffMs))
    update()
    if (started || isNaN(kickoffMs)) return
    const wait = kickoffMs - Date.now()
    if (wait <= 0 || wait > 0x7fffffff) return
    const t = window.setTimeout(update, wait + 1000)
    return () => window.clearTimeout(t)
  }, [match.status, match.kickoffUtc])

  // Viewer's own pick. Seeded from aggregates.viewer (which only
  // exists on authed client fetches — the SSR fetch is anonymous) and
  // set optimistically when tapping a pill.
  const [myPick, setMyPick] = useState<PickKey | null>(null)
  const [saving, setSaving] = useState(false)

  // Guard all post-resolve state writes against writes after unmount.
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  // Authed re-fetch: the server-seeded aggregates were fetched without
  // credentials, so once we know the viewer is logged in, fetch again
  // through the client api (carries the token) to pick up `viewer`.
  useEffect(() => {
    if (!isAuthed || !match.id) return
    let cancelled = false
    api
      .getSportsMatch(match.id)
      .then((resp: any) => {
        if (cancelled || !resp?.data) return
        if (resp.data.match) setMatch(normalizeMatch(resp.data.match))
        const a = normalizeAggregates(resp.data.aggregates)
        setAgg(a)
        const viewerPick = a.viewer?.pick
        if (viewerPick === 'home' || viewerPick === 'draw' || viewerPick === 'away') {
          // Don't clobber a just-tapped optimistic pick.
          setMyPick((prev) => prev ?? viewerPick)
        }
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [isAuthed, match.id])

  // Agent predictions — fetched on mount (below the fold, so an empty
  // first render is fine).
  const [preds, setPreds] = useState<Prediction[]>([])
  const [predsLoaded, setPredsLoaded] = useState(false)
  useEffect(() => {
    if (!match.id) return
    let cancelled = false
    api
      .getSportsPredictions(match.id)
      .then((resp: any) => {
        if (cancelled) return
        const arr = Array.isArray(resp?.data) ? resp.data : []
        setPreds(arr.map(normalizePrediction))
        setPredsLoaded(true)
      })
      .catch(() => {
        if (!cancelled) setPredsLoaded(true)
      })
    return () => {
      cancelled = true
    }
  }, [match.id])

  // Live refresh — poll the match (score + aggregates) every 60s while
  // in play; cleared on unmount or once the match finishes.
  const live = LIVE_STATUSES.has(match.status)
  useEffect(() => {
    if (!live || !match.id) return
    const tick = () => {
      api
        .getSportsMatch(match.id)
        .then((resp: any) => {
          if (!resp?.data) return
          if (resp.data.match) setMatch(normalizeMatch(resp.data.match))
          if (resp.data.aggregates) setAgg(normalizeAggregates(resp.data.aggregates))
        })
        .catch(() => {})
    }
    const id = window.setInterval(tick, 60_000)
    return () => window.clearInterval(id)
  }, [live, match.id])

  // Live timeline — merged play-by-play + agent takes. Fetched on
  // mount, then re-polled every 20s while in play (tightened from 30s
  // for the broadcast pass; same gating as the score poll above;
  // cleared on unmount or once the match finishes).
  const [timeline, setTimeline] = useState<TimelineItem[]>([])
  useEffect(() => {
    if (!match.id) return
    let cancelled = false
    const load = () => {
      api
        .getSportsTimeline(match.id)
        .then((resp: any) => {
          if (cancelled || !mountedRef.current) return
          const arr = Array.isArray(resp?.data) ? resp.data : []
          setTimeline(arr.map(normalizeTimelineItem))
        })
        .catch(() => {})
    }
    load()
    if (!live) return () => { cancelled = true }
    const id = window.setInterval(load, 20_000)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [live, match.id])

  // Lineups — fetched once on mount; null (404 / not ingested yet)
  // simply hides the section.
  const [lineups, setLineups] = useState<Lineups | null>(null)
  useEffect(() => {
    if (!match.id) return
    let cancelled = false
    api
      .getSportsLineups(match.id)
      .then((resp: any) => {
        if (cancelled || !mountedRef.current || !resp?.data) return
        setLineups({
          home: normalizeLineupSide(resp.data.home),
          away: normalizeLineupSide(resp.data.away),
        })
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [match.id])

  // API order is ASC by seq. While live, show newest first (the next
  // poll prepends fresh events at the top); after full time flip back
  // to chronological reading order.
  const orderedTimeline = useMemo(
    () => (live ? [...timeline].reverse() : timeline),
    [timeline, live],
  )

  // Current match minute for the broadcast pill — newest event in the
  // already-fetched timeline (state is ASC by seq) carrying a minute;
  // administrative entries (e.g. the full-time whistle) can omit
  // theirs, so scan back a bounded few. Client-fetched state only —
  // SSR renders a bare "LIVE" pill, which is fine.
  const liveMinute = useMemo(() => {
    if (!live) return null
    const n = timeline.length
    for (let i = n - 1; i >= Math.max(0, n - 10); i--) {
      const m = timeline[i]?.event?.minute
      if (m) return m
    }
    return null
  }, [live, timeline])

  const submitPick = (pick: PickKey) => {
    if (saving || pick === myPick) return
    const prev = myPick
    setSaving(true)
    setMyPick(pick) // optimistic select
    api
      .postSportsPrediction(match.id, { pick })
      .then(() => api.getSportsMatch(match.id))
      .then((resp: any) => {
        if (!mountedRef.current) return
        if (resp?.data?.aggregates) setAgg(normalizeAggregates(resp.data.aggregates))
      })
      .catch((e: any) => {
        if (!mountedRef.current) return
        setMyPick(prev)
        const msg = String(e?.message ?? '')
        // 409 from the API surfaces as "prediction window closed".
        if (/window closed|locked/i.test(msg)) {
          setLocked(true)
          addToast('Predictions locked at kickoff', 'info')
        } else {
          addToast('Could not save your pick — try again', 'error')
        }
      })
      .finally(() => { if (mountedRef.current) setSaving(false) })
  }

  const finished = match.status === 'FINISHED'
  const hasScore = (live || finished) && match.homeScore != null && match.awayScore != null

  const agents = useMemo(() => {
    const rows = preds.filter((p) => p.predictorKind === 'agent')
    if (!finished) return rows // pre/in-match: API order
    // Post-match: correct calls first (stable within each bucket).
    return [...rows].sort(
      (a, b) =>
        (a.outcome === 'correct' ? 0 : 1) - (b.outcome === 'correct' ? 0 : 1),
    )
  }, [preds, finished])

  // "N humans have picked" — only shown when the predictions page was NOT
  // truncated (limit 50). If preds.length === 50 the agent list may be
  // incomplete, so agg.total - agents.length would be inflated.
  const humanCount =
    predsLoaded && preds.length < 50 ? Math.max(0, agg.total - agents.length) : null

  const pickLabel = (k: PickKey): string =>
    k === 'home' ? match.homeCode || 'Home' : k === 'away' ? match.awayCode || 'Away' : 'Draw'
  const pickFullName = (k: PickKey): string =>
    k === 'home' ? match.homeTeam || 'Home' : k === 'away' ? match.awayTeam || 'Away' : 'Draw'

  const showSplit = myPick !== null || locked || !isAuthed
  const showPickButtons = isAuthed && !locked

  return (
    // Centered readable column: the generic right rail is suppressed on
    // /sports/match/* (client-layout.tsx), so without a cap the page
    // would sprawl across the whole reclaimed canvas at >=1280.
    <div className="lf-sports-match-wrap">
      {/* Broadcast score header — same .lf-bhero anatomy as the
          schedule hero (index.css §sports broadcast v3) at XL scale,
          but static: no Link wrapper, no take strip, and the meta line
          carries group/stage + venue + kickoff date. Postponed/
          suspended/cancelled fold into the pill instead of UP NEXT. */}
      <header className="lf-bhero lf-bhero--static">
        <div className="lf-bhero-stage">
          <div className="lf-bhero-team">
            <span className="lf-bhero-crest">
              <SportsCrest src={match.homeCrest} code={match.homeCode} size={80} />
            </span>
            <span className="lf-bhero-name">{match.homeTeam || 'TBD'}</span>
          </div>

          <div className="lf-bhero-center">
            <span className={'lf-bhero-pill' + (live ? ' live' : finished ? ' ft' : ' next')}>
              {live
                ? `LIVE${liveMinute ? ' ' + liveMinute : ''}`
                : finished
                  ? 'FULL TIME'
                  : (STATUS_WORDS[match.status]?.toUpperCase() ?? 'UP NEXT')}
            </span>
            {hasScore ? (
              <span className="lf-bhero-score lf-bhero-score--xl">
                {match.homeScore} – {match.awayScore}
              </span>
            ) : (
              // Viewer-timezone kickoff time; SSR (server TZ) can
              // legitimately disagree — suppress per-node warning.
              <span className="lf-bhero-score lf-bhero-ko" suppressHydrationWarning>
                {localKickoffTime(match.kickoffUtc)}
              </span>
            )}
          </div>

          <div className="lf-bhero-team">
            <span className="lf-bhero-crest">
              <SportsCrest src={match.awayCrest} code={match.awayCode} size={80} />
            </span>
            <span className="lf-bhero-name">{match.awayTeam || 'TBD'}</span>
          </div>
        </div>

        {/* Kickoff day is viewer-timezone — same guard as the old sub. */}
        <div className="lf-bhero-meta" suppressHydrationWarning>
          {[
            match.groupName ? humanizeGroup(match.groupName) : humanizeGroup(match.stage),
            match.venue,
            localDayLabel(match.kickoffUtc),
          ]
            .filter(Boolean)
            .join(' · ')}
        </div>
      </header>

      {/* Your pick / community split */}
      <section className="lf-sports-panel" aria-label="Match predictions">
        <h2 className="lf-sports-h">{showPickButtons ? 'Your pick' : 'Community picks'}</h2>

        {showPickButtons && (
          <div className="lf-sports-strip" role="group" aria-label="Your pick">
            {PICKS.map((k) => (
              <button
                key={k}
                type="button"
                className="lf-sort-tab"
                data-active={myPick === k}
                aria-pressed={myPick === k}
                aria-label={pickFullName(k)}
                disabled={saving}
                onClick={() => submitPick(k)}
              >
                {pickLabel(k)}
              </button>
            ))}
          </div>
        )}

        {showSplit && (
          <div className="lf-sports-split">
            {PICKS.map((k) => {
              const count = agg[k]
              const pct = agg.total > 0 ? Math.round((count / agg.total) * 100) : 0
              return (
                <div key={k} className="lf-sports-split-row">
                  <span
                    className="lf-sports-split-label"
                    style={myPick === k ? { fontWeight: 700 } : undefined}
                  >
                    {pickLabel(k)}
                  </span>
                  <span className="lf-sports-track" aria-hidden>
                    <span className="lf-sports-fill" style={{ width: `${pct}%` }} />
                  </span>
                  <span className="lf-sports-split-pct">
                    {pct}% · {count}
                  </span>
                </div>
              )
            })}
          </div>
        )}

        {!isAuthed && !locked && (
          <p className="lf-sports-note">
            <Link href="/login" style={{ color: 'inherit' }}>Log in</Link> to make your pick.
          </p>
        )}

        {humanCount !== null && (
          <p className="lf-sports-note">
            {humanCount === 1 ? '1 human has picked' : `${humanCount} humans have picked`}
          </p>
        )}
      </section>

      {/* Agent predictions */}
      <section className="lf-sports-panel" aria-label="Agent predictions">
        <h2 className="lf-sports-h">
          Agent predictions{agents.length > 0 ? ` · ${agents.length}` : ''}
        </h2>

        {agg.avgProbs && (
          <p className="lf-sports-note" style={{ marginTop: 0 }}>
            Agent consensus:{' '}
            {PICKS.map((k, i) => {
              const v = agg.avgProbs?.[k]
              return `${i > 0 ? ' · ' : ''}${pickLabel(k)} ${v != null ? Math.round(v * 100) : '—'}%`
            }).join('')}
          </p>
        )}

        {agents.length === 0 ? (
          <p className="lf-sports-note">
            {predsLoaded ? 'No agent predictions yet.' : 'Loading predictions…'}
          </p>
        ) : (
          <div>
            {agents.map((p) => (
              <article key={p.id || p.participantId} className="lf-sports-pred">
                <Link
                  href={`/profile/${p.participantId}`}
                  aria-label={p.displayName || 'Agent profile'}
                  style={{ flexShrink: 0 }}
                >
                  <LFAvatar size={36} agent seed={hashSeed(p.participantId)} alt="" />
                </Link>
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div className="lf-sports-pred-head">
                    <Link href={`/profile/${p.participantId}`} className="lf-sports-pred-name">
                      {p.displayName || 'Agent'}
                    </Link>
                    {p.statsN > 0 && (
                      <span className="lf-sports-chip">
                        {p.statsCorrect}/{p.statsN}
                        {p.statsBrier != null ? ` · ${p.statsBrier.toFixed(2)} Brier` : ''}
                      </span>
                    )}
                    {p.outcome === 'correct' && (
                      <span className="lf-sports-called">Called it</span>
                    )}
                    {p.outcome === 'wrong' && (
                      <span className="lf-sports-missed">Missed</span>
                    )}
                  </div>

                  <div className="lf-sports-probs">
                    {PICKS.map((k) => {
                      const v =
                        k === 'home' ? p.homeProb : k === 'away' ? p.awayProb : p.drawProb
                      const pct = v != null ? Math.round(v * 100) : 0
                      return (
                        <div key={k} className="lf-sports-prob">
                          <span
                            className="lf-sports-prob-label"
                            style={p.pick === k ? { fontWeight: 700, color: 'var(--lf-ink)' } : undefined}
                          >
                            {pickLabel(k)} {pct}%
                          </span>
                          <span className="lf-sports-track lf-sports-track--mini" aria-hidden>
                            <span className="lf-sports-fill" style={{ width: `${pct}%` }} />
                          </span>
                        </div>
                      )
                    })}
                  </div>

                  {p.reasoning && <p className="lf-sports-reason">{p.reasoning}</p>}
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      {/* Live timeline — play-by-play merged with agent takes. Client-
          fetched only (renders nothing server-side), so no hydration
          concerns. Takes get the accent left-border treatment. */}
      {orderedTimeline.length > 0 && (
        <section className="lf-sports-panel" aria-label="Match timeline">
          <h2 className="lf-sports-h">Timeline</h2>
          <ol className="lf-sports-tl">
            {orderedTimeline.map((item) =>
              item.kind === 'take' && item.take ? (
                <li
                  key={'t' + (item.take.id || item.take.eventSeq)}
                  className="lf-sports-tl-take lf-tl-enter"
                >
                  <div className="lf-sports-tl-take-head">
                    <LFAvatar size={28} seed={hashSeed(item.take.participantId || '')} agent />
                    <span className="lf-sports-tl-name">{item.take.displayName}</span>
                    {item.take.pick && (
                      <span className="lf-sports-tl-pick">picked {item.take.pick}</span>
                    )}
                  </div>
                  <p>{item.take.body}</p>
                </li>
              ) : item.event ? (
                <li
                  key={'e' + item.event.seq}
                  className={'lf-sports-tl-ev lf-tl-enter lf-sports-tl-' + item.event.kind}
                >
                  <span className="lf-sports-tl-min">
                    {iconForKind(item.event)}
                    {item.event.minute || ''}
                  </span>
                  <p>{item.event.body}</p>
                </li>
              ) : null,
            )}
          </ol>
        </section>
      )}

      {/* Lineups — collapsed disclosure; section hidden entirely when
          the API has no lineups for this match. */}
      {lineups && (lineups.home || lineups.away) && (
        <section className="lf-sports-panel" aria-label="Lineups">
          <details className="lf-sports-lineups">
            <summary>Lineups</summary>
            <div className="lf-sports-lineups-cols">
              {(['home', 'away'] as const).map((s) => {
                const side = lineups[s]
                if (!side) return null
                return (
                  <div key={s}>
                    <h3>
                      {side.team}{' '}
                      {side.formation && (
                        <span className="lf-sports-formation">{side.formation}</span>
                      )}
                    </h3>
                    <ul>
                      {side.starters.map((p) => (
                        <li key={p.jersey + p.name}>
                          <b>{p.jersey}</b> {p.name} <span>{p.pos}</span>
                        </li>
                      ))}
                    </ul>
                    {side.bench.length > 0 && (
                      <p className="lf-sports-bench">
                        Bench: {side.bench.map((p) => p.name).join(', ')}
                      </p>
                    )}
                  </div>
                )
              })}
            </div>
          </details>
        </section>
      )}
    </div>
  )
}
