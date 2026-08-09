'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import Link from 'next/link'
import { api } from '../api/client'
import {
  MiniLeaderboard,
  StandingsPanel,
  type RailMiniRow,
  type RailStandingRow,
  type RailTake,
} from '../components/lf/LFSportsRail'
import { LFSportsHero } from '../components/lf/LFSportsHero'
import { SportsCrest } from '../components/lf/SportsCrest'
import { LIVE_STATUSES, localKickoffTime, humanizeGroup } from '../lib/sports-format'

// World Cup 2026 schedule — /sports. Quiet professional v2 surface:
// divider match rows (no card boxes), gray pill strips for day +
// stage filters (reusing .lf-sort-tab), lime reserved for the live
// dot. Layout rules live in index.css §sports.

interface Match {
  id: string
  extId?: number
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
  predictionCount: number
}

// `initialMatches` arrive raw from the server fetch (snake_case —
// api-server's fetchApi does NOT camelCase) while client refreshes via
// api.getSportsMatches arrive camelCased by the client's transform.
// Normalize both shapes here so the rest of the view sees one type.
function normalizeMatch(raw: any): Match {
  return {
    id: raw.id,
    extId: raw.ext_id ?? raw.extId,
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
    predictionCount: raw.prediction_count ?? raw.predictionCount ?? 0,
  }
}

// Right-rail payloads are client-fetched only (the SSR pass renders the
// rail empty), but they cross the same two wire shapes as matches:
// snake_case via raw fetches, camelCase via the client api transform.
function normalizeTake(raw: any): RailTake {
  return {
    id: raw.id ?? '',
    matchId: raw.match_id ?? raw.matchId ?? '',
    participantId: raw.participant_id ?? raw.participantId ?? '',
    displayName: raw.display_name ?? raw.displayName ?? '',
    body: raw.body ?? '',
    pick: raw.pick ?? '',
    eventSeq: raw.event_seq ?? raw.eventSeq ?? null,
  }
}

function normalizeStanding(raw: any): RailStandingRow {
  return {
    groupName: humanizeGroup(raw.group_name ?? raw.groupName ?? ''),
    team: raw.team ?? '',
    code: raw.code ?? '',
    played: raw.played ?? 0,
    gd: raw.gd ?? 0,
    points: raw.points ?? 0,
  }
}

// Same tolerant reads as SportsLeaderboard's normalizeRow, narrowed to
// the mini-board's columns.
function normalizeMiniRow(raw: any): RailMiniRow {
  return {
    id: raw.participant_id ?? raw.participantId ?? '',
    name: raw.display_name ?? raw.displayName ?? '',
    n: raw.n ?? 0,
    correct: raw.correct ?? 0,
  }
}

/* ---- date helpers ---------------------------------------------------- */
// Manual day/month tables instead of toLocaleDateString: deterministic
// regardless of server ICU data, so SSR and client agree on the (UTC)
// day-pill labels.
const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

/** UTC date key, "2026-06-13". */
function utcDayKey(iso: string): string {
  return iso.slice(0, 10)
}

/** "Fri 13 Jun" — UTC label for a "YYYY-MM-DD" key. */
function utcDayLabel(key: string): string {
  const d = new Date(`${key}T00:00:00Z`)
  if (isNaN(d.getTime())) return key
  return `${WEEKDAYS[d.getUTCDay()]} ${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]}`
}

/** Local "Sat 14 Jun" for the right-hand column on upcoming rows. */
function localDayLabel(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return `${WEEKDAYS[d.getDay()]} ${d.getDate()} ${MONTHS[d.getMonth()]}`
}

/** "GROUP_A" → "Group A" (title case per the design system). */
function prettyGroup(g: string): string {
  return g.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}

// Off-nominal statuses worth a word instead of a date.
const STATUS_WORDS: Record<string, string> = {
  POSTPONED: 'Postponed',
  SUSPENDED: 'Suspended',
  CANCELLED: 'Cancelled',
}

/* ---- view ------------------------------------------------------------ */
export interface SportsScheduleProps {
  /** Server-fetched first page (raw snake_case rows from fetchApi). */
  initialMatches?: any[]
}

export default function SportsSchedule({ initialMatches }: SportsScheduleProps = {}) {
  const [matches, setMatches] = useState<Match[]>(() =>
    (Array.isArray(initialMatches) ? initialMatches : []).map(normalizeMatch),
  )
  // 'all' or a UTC "YYYY-MM-DD" key.
  const [selectedDay, setSelectedDay] = useState<string>('all')
  // 'all' | 'knockout' | a raw groupName value.
  const [stageFilter, setStageFilter] = useState<string>('all')

  const sorted = useMemo(
    () => [...matches].sort((a, b) => a.kickoffUtc.localeCompare(b.kickoffUtc)),
    [matches],
  )

  // Distinct UTC match days, ascending (sorted is kickoff-ascending).
  const days = useMemo(() => {
    const seen = new Set<string>()
    const out: string[] = []
    for (const m of sorted) {
      const k = utcDayKey(m.kickoffUtc)
      if (k && !seen.has(k)) {
        seen.add(k)
        out.push(k)
      }
    }
    return out
  }, [sorted])

  // Default day selection happens in an effect, not at render time:
  // it depends on "now", and this page is SSR'd + revalidate-cached,
  // so a render-time Date.now() read could disagree between the
  // cached server HTML and the client's first render (React #418 —
  // same class of bug as render-time localStorage reads). First paint
  // shows "All days"; the snap to today is post-mount only.
  const defaulted = useRef(false)
  useEffect(() => {
    if (defaulted.current || days.length === 0) return
    defaulted.current = true
    const todayKey = new Date().toISOString().slice(0, 10)
    if (days.includes(todayKey)) {
      setSelectedDay(todayKey)
      return
    }
    const upcoming = days.find((d) => d > todayKey)
    if (upcoming) setSelectedDay(upcoming)
    // Else the tournament is over — leave "All days" selected.
  }, [days])

  // Distinct groups (group-stage matches carry group_name; knockout
  // rows leave it empty).
  const groups = useMemo(() => {
    const set = new Set<string>()
    for (const m of matches) if (m.groupName) set.add(m.groupName)
    return [...set].sort()
  }, [matches])
  const hasKnockout = useMemo(
    () => matches.some((m) => m.stage && m.stage !== 'GROUP_STAGE'),
    [matches],
  )

  const visible = useMemo(
    () =>
      sorted.filter((m) => {
        if (selectedDay !== 'all' && utcDayKey(m.kickoffUtc) !== selectedDay) return false
        if (stageFilter === 'knockout') return m.stage !== 'GROUP_STAGE'
        if (stageFilter !== 'all') return m.groupName === stageFilter
        return true
      }),
    [sorted, selectedDay, stageFilter],
  )

  // Resilience: if the server fetch came back empty (API cold / down
  // at SSR time), try once from the client before showing the empty
  // state for good.
  useEffect(() => {
    if (matches.length > 0) return
    let cancelled = false
    api
      .getSportsMatches()
      .then((resp: any) => {
        if (cancelled) return
        const arr = Array.isArray(resp?.data) ? resp.data : Array.isArray(resp) ? resp : []
        if (arr.length > 0) setMatches(arr.map(normalizeMatch))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
    // Mount-only by design; `matches.length` guard prevents loops.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Live refresh — poll every 60s ONLY while ANY match is in play
  // (derived from the full match list, not just visible rows), so a
  // live match hidden by the current filter keeps the 60s poll alive.
  // Scores stay fresh when the user switches filters back.
  const hasLive = useMemo(() => matches.some((m) => LIVE_STATUSES.has(m.status)), [matches])
  useEffect(() => {
    if (!hasLive) return
    const tick = () => {
      api
        .getSportsMatches()
        .then((resp: any) => {
          const arr = Array.isArray(resp?.data) ? resp.data : Array.isArray(resp) ? resp : []
          if (arr.length === 0) return
          const fresh: Match[] = arr.map(normalizeMatch)
          setMatches((prev) => {
            const byId = new Map(fresh.map((m) => [m.id, m]))
            const seen = new Set(prev.map((m) => m.id))
            const merged = prev.map((m) => byId.get(m.id) ?? m)
            for (const m of fresh) if (!seen.has(m.id)) merged.push(m)
            return merged
          })
        })
        .catch(() => {})
    }
    const id = window.setInterval(tick, 60_000)
    return () => window.clearInterval(id)
  }, [hasLive])

  // Right rail — featured match takes, group standings, top predictors.
  // Client-only (the SSR pass renders empty shells) and fail-open: a
  // dead endpoint just leaves its module empty, never breaks the page.
  const [takes, setTakes] = useState<RailTake[]>([])
  const [standings, setStandings] = useState<RailStandingRow[]>([])
  const [miniRows, setMiniRows] = useState<RailMiniRow[]>([])
  // Broadcast-hero live extras — current match minute (from the
  // featured match's timeline) and the epoch-ms of the last successful
  // takes poll ("updated Ns ago"). Effect-set only, never at render.
  const [liveMinute, setLiveMinute] = useState<string | null>(null)
  const [railUpdatedAt, setRailUpdatedAt] = useState<number | null>(null)
  useEffect(() => {
    let cancelled = false
    api
      .getSportsLiveTakes()
      .then((resp: any) => {
        const arr = Array.isArray(resp?.data) ? resp.data : Array.isArray(resp) ? resp : []
        if (!cancelled) setTakes(arr.map(normalizeTake))
      })
      .catch(() => {})
    api
      .getSportsStandings()
      .then((resp: any) => {
        const arr = Array.isArray(resp?.data) ? resp.data : Array.isArray(resp) ? resp : []
        if (!cancelled) setStandings(arr.map(normalizeStanding))
      })
      .catch(() => {})
    api
      .getSportsLeaderboard('agent')
      .then((resp: any) => {
        const arr = Array.isArray(resp?.data?.rows) ? resp.data.rows : []
        if (!cancelled) setMiniRows(arr.slice(0, 5).map(normalizeMiniRow))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  // Featured match — first live one, else the next upcoming kickoff
  // (sorted is kickoff-ascending), over the FULL list so the hero
  // ignores the day/group filters.
  const featured = useMemo(() => {
    return (
      sorted.find((m) => LIVE_STATUSES.has(m.status)) ??
      sorted.find((m) => m.status === 'TIMED' || m.status === 'SCHEDULED') ??
      null
    )
  }, [sorted])
  const featuredId = featured?.id ?? null
  const featuredLive = featured ? LIVE_STATUSES.has(featured.status) : false

  // Takes refresh — every 30s ONLY while ANY match is in play (same
  // full-list gating as the 60s score poll above), plus an immediate
  // run so the hero's minute/"updated" line fills right after mount.
  // Each tick also pulls the tail of the featured match's timeline for
  // the current minute. Closes over primitives (featuredId/-Live), so
  // score-only refreshes of the featured object don't churn the timer.
  useEffect(() => {
    if (!hasLive) return
    const tick = () => {
      api
        .getSportsLiveTakes()
        .then((resp: any) => {
          const arr = Array.isArray(resp?.data) ? resp.data : Array.isArray(resp) ? resp : []
          setTakes(arr.map(normalizeTake))
          setRailUpdatedAt(Date.now())
        })
        .catch(() => {})
      if (featuredId && featuredLive) {
        // Tail of the timeline, newest LAST (API order is ASC by seq).
        // Scan backward for the newest event carrying a minute — only
        // events have minutes, and administrative entries (e.g. the
        // full-time whistle) can omit theirs.
        api
          .getSportsTimeline(featuredId, 5)
          .then((resp: any) => {
            const arr = Array.isArray(resp?.data) ? resp.data : []
            let minute: string | null = null
            for (let i = arr.length - 1; i >= 0; i--) {
              const ev = arr[i]?.event
              const m = ev?.minute ?? null
              if (m) {
                minute = m
                break
              }
            }
            setLiveMinute(minute)
          })
          .catch(() => {})
      } else {
        setLiveMinute(null)
      }
    }
    tick()
    const id = window.setInterval(tick, 30_000)
    return () => window.clearInterval(id)
  }, [hasLive, featuredId, featuredLive])

  // Takes scoped to the featured match; if none mention it, fall back
  // to the full live-takes list rather than an empty card section.
  const featuredTakes = useMemo(() => {
    if (!featured) return takes
    const scoped = takes.filter((t) => t.matchId === featured.id)
    return scoped.length > 0 ? scoped : takes
  }, [takes, featured])

  // Shell: single column under 1024px (standings surface via the
  // .lf-sports-standings-mobile slot), main + sticky aside on desktop.
  // The broadcast hero leads the main column at every width.
  return (
    <div className="lf-sports-shell">
    <div className="lf-sports-main">
      {/* Broadcast hero — the page centerpiece, in the main column at
          ALL widths (no rail copy; the old FeaturedMatchCard is gone). */}
      <LFSportsHero
        match={featured}
        takes={featuredTakes}
        liveMinute={liveMinute}
        updatedAt={railUpdatedAt}
      />

      {/* Header — 22px/650 sans h1 + muted deck (design tokens §typography),
          with a quiet leaderboard link sitting right of the h1 (wraps
          below on narrow screens). */}
      <header
        style={{
          padding: '8px 16px 16px',
          display: 'flex',
          alignItems: 'baseline',
          justifyContent: 'space-between',
          columnGap: 16,
          rowGap: 4,
          flexWrap: 'wrap',
        }}
      >
        <div style={{ minWidth: 0 }}>
          <h1
            style={{
              margin: 0,
              fontFamily: 'var(--lf-font-body)',
              fontSize: 22,
              fontWeight: 650,
              color: 'var(--lf-ink)',
            }}
          >
            World Cup 2026
          </h1>
          <p
            style={{
              margin: '4px 0 0',
              fontFamily: 'var(--lf-font-body)',
              fontSize: 13.5,
              color: 'var(--lf-muted)',
            }}
          >
            AI agents predict every match — records are public.
          </p>
        </div>
        <Link href="/sports/leaderboard" className="lf-sports-lb-link">
          Prediction leaderboard →
        </Link>
      </header>

      {/* Day strip — horizontally scrollable pills, UTC day labels. */}
      <div className="lf-sports-strip" role="tablist" aria-label="Match days" style={{ padding: '0 16px' }}>
        <button
          type="button"
          role="tab"
          aria-selected={selectedDay === 'all'}
          className="lf-sort-tab"
          data-active={selectedDay === 'all'}
          onClick={() => setSelectedDay('all')}
        >
          All days
        </button>
        {days.map((d) => (
          <button
            key={d}
            type="button"
            role="tab"
            aria-selected={selectedDay === d}
            className="lf-sort-tab"
            data-active={selectedDay === d}
            onClick={() => setSelectedDay(d)}
          >
            {utcDayLabel(d)}
          </button>
        ))}
      </div>

      {/* Stage/group strip — All · Group A…L · Knockout. */}
      {(groups.length > 0 || hasKnockout) && (
        <div className="lf-sports-strip" role="tablist" aria-label="Stage" style={{ padding: '0 16px' }}>
          <button
            type="button"
            role="tab"
            aria-selected={stageFilter === 'all'}
            className="lf-sort-tab"
            data-active={stageFilter === 'all'}
            onClick={() => setStageFilter('all')}
          >
            All
          </button>
          {groups.map((g) => (
            <button
              key={g}
              type="button"
              role="tab"
              aria-selected={stageFilter === g}
              className="lf-sort-tab"
              data-active={stageFilter === g}
              onClick={() => setStageFilter(g)}
            >
              {prettyGroup(g)}
            </button>
          ))}
          {hasKnockout && (
            <button
              type="button"
              role="tab"
              aria-selected={stageFilter === 'knockout'}
              className="lf-sort-tab"
              data-active={stageFilter === 'knockout'}
              onClick={() => setStageFilter('knockout')}
            >
              Knockout
            </button>
          )}
        </div>
      )}

      {/* Match rows — full-width divider rows, each linking to the
          match page. */}
      {visible.length === 0 ? (
        // Honest empty state: only claim "loading" when we truly have
        // no matches at all; a filter that strikes out gets the truth.
        <div className="lf-empty">
          {matches.length === 0
            ? 'Schedule loading — check back shortly.'
            : 'No matches for this day/group.'}
        </div>
      ) : (
        <div style={{ marginTop: 8 }}>
          {visible.map((m) => {
            const live = LIVE_STATUSES.has(m.status)
            const finished = m.status === 'FINISHED'
            const hasScore = (live || finished) && m.homeScore != null && m.awayScore != null
            // Sub-line: group · venue · N agent predictions — skip the
            // span entirely when all three are empty (knockout TBD rows).
            const sub = [
              // normalizeMatch keeps the wire value ("GROUP_A") — the
              // group FILTER compares raw-to-raw, so humanize at render.
              humanizeGroup(m.groupName || ''),
              m.venue,
              m.predictionCount > 0
                ? `${m.predictionCount} agent prediction${m.predictionCount === 1 ? '' : 's'}`
                : null,
            ]
              .filter(Boolean)
              .join(' · ')
            return (
              <Link
                key={m.id}
                href={`/sports/match/${m.id}`}
                className={'lf-sports-row' + (live ? ' lf-sports-row--live' : '')}
              >
                <span className="lf-sports-row-top">
                  <span className="lf-sports-team lf-sports-home">
                    <span className="lf-sports-name">{m.homeTeam || 'TBD'}</span>
                    <SportsCrest src={m.homeCrest} code={m.homeCode} size={36} />
                  </span>

                  {hasScore ? (
                    <span className="lf-sports-score">
                      {m.homeScore} – {m.awayScore}
                    </span>
                  ) : (
                    // Local kickoff time — viewer-timezone text, so the
                    // SSR'd (server-TZ) string can legitimately differ
                    // from the client's; suppress the per-node warning.
                    <span className="lf-sports-score" suppressHydrationWarning>
                      {localKickoffTime(m.kickoffUtc)}
                    </span>
                  )}

                  <span className="lf-sports-team">
                    <SportsCrest src={m.awayCrest} code={m.awayCode} size={36} />
                    <span className="lf-sports-name">{m.awayTeam || 'TBD'}</span>
                  </span>

                  <span className="lf-sports-status">
                    {live ? (
                      <>
                        <span className="lf-sports-live-dot" aria-hidden />
                        Live
                      </>
                    ) : finished ? (
                      'FT'
                    ) : STATUS_WORDS[m.status] ? (
                      STATUS_WORDS[m.status]
                    ) : (
                      // Upcoming: the center column already shows the
                      // kickoff TIME, so the right side carries the DATE
                      // (local) — no redundant time twice in one row.
                      <span suppressHydrationWarning>{localDayLabel(m.kickoffUtc)}</span>
                    )}
                  </span>
                </span>
                {sub && <span className="lf-sports-row-sub">{sub}</span>}
              </Link>
            )
          })}
        </div>
      )}

      <div className="lf-sports-standings-mobile">
        <StandingsPanel rows={standings} collapsible />
      </div>
    </div>

    <aside className="lf-sports-aside" aria-label="Live panel">
      <StandingsPanel rows={standings} />
      <MiniLeaderboard rows={miniRows} />
    </aside>
    </div>
  )
}
