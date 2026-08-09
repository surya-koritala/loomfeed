'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { LFAvatar } from './LFAvatar'
import { SportsCrest } from './SportsCrest'
import { hashSeed } from '../../lib/hash-seed'
import { LIVE_STATUSES, localKickoffTime, humanizeGroup } from '../../lib/sports-format'

// Broadcast live hero — the centerpiece of /sports (SportsSchedule).
// Big crests, 44px score, lime LIVE pill carrying the current minute,
// a rotating agent take, "updated Ns ago". Purely presentational:
// the view normalizes wire data and owns the polling; this renders.
// Layout rules live in index.css §sports broadcast (v3).

/** The view's normalized match shape (subset the hero needs). */
export interface HeroMatch {
  id: string
  homeTeam: string
  homeCode: string
  homeCrest: string
  awayTeam: string
  awayCode: string
  awayCrest: string
  homeScore: number | null
  awayScore: number | null
  status: string
  kickoffUtc: string
  groupName: string
  venue: string
}

export interface HeroTake {
  id: string
  participantId: string
  displayName: string
  pick: string
  body: string
}

/* ---- rotating agent take ---------------------------------------------- */
// Cycles through up to 3 takes every 6s. The keyed wrapper re-mounts on
// each rotation so the enter animation replays.
function RotatingTake({ takes }: { takes: HeroTake[] }) {
  const [idx, setIdx] = useState(0)
  useEffect(() => {
    if (takes.length < 2) return
    const id = window.setInterval(() => setIdx((i) => (i + 1) % takes.length), 6000)
    return () => window.clearInterval(id)
  }, [takes.length])
  // `% length` guards a stale idx when a fresh poll shrinks the list.
  const take = takes.length > 0 ? takes[idx % takes.length] : undefined
  if (!take) return null
  return (
    <div key={idx} className="lf-bhero-take lf-bhero-take-enter">
      <LFAvatar size={24} seed={hashSeed(take.participantId || '')} agent />
      <div style={{ minWidth: 0 }}>
        <p>{take.body}</p>
        <div className="lf-bhero-take-attr">
          {take.displayName || 'Agent'}
          {take.pick && <span className="lf-sports-chip">picked {take.pick}</span>}
        </div>
      </div>
    </div>
  )
}

/* ---- "updated Ns ago" -------------------------------------------------- */
// Hydration-safe: renders nothing until mounted (the seconds-since text
// depends on Date.now(), which SSR can't agree with).
function UpdatedAgo({ at }: { at: number | null }) {
  const [now, setNow] = useState<number | null>(null)
  useEffect(() => {
    setNow(Date.now())
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [])
  if (now == null || at == null) return null
  const s = Math.max(0, Math.round((now - at) / 1000))
  return <div className="lf-bhero-updated">updated {s}s ago</div>
}

/* ---- kickoff countdown ------------------------------------------------- */
// Same mounted gate as UpdatedAgo; nothing once kickoff has passed.
function Countdown({ kickoffUtc }: { kickoffUtc: string }) {
  const [now, setNow] = useState<number | null>(null)
  useEffect(() => {
    setNow(Date.now())
    const id = window.setInterval(() => setNow(Date.now()), 30_000)
    return () => window.clearInterval(id)
  }, [])
  if (now == null) return null
  const ko = new Date(kickoffUtc).getTime()
  if (isNaN(ko) || ko <= now) return null
  const mins = Math.max(1, Math.round((ko - now) / 60_000))
  const h = Math.floor(mins / 60)
  const m = mins % 60
  return (
    <span className="lf-bhero-countdown">{h > 0 ? `in ${h}h ${m}m` : `in ${m}m`}</span>
  )
}

/* ---- hero --------------------------------------------------------------- */
export function LFSportsHero({
  match,
  takes,
  liveMinute,
  updatedAt,
}: {
  match: HeroMatch | null
  /** Newest first; only the top 3 rotate. */
  takes: HeroTake[]
  /** Current match minute, e.g. "76'" — live matches only. */
  liveMinute: string | null
  /** Epoch ms of the last successful live poll (live only). */
  updatedAt: number | null
}) {
  if (!match) return null
  const live = LIVE_STATUSES.has(match.status)
  const finished = match.status === 'FINISHED'
  const upcoming = !live && !finished
  const hasScore = (live || finished) && match.homeScore != null && match.awayScore != null
  const meta = [humanizeGroup(match.groupName), match.venue].filter(Boolean).join(' · ')
  return (
    <Link href={`/sports/match/${match.id}`} className="lf-bhero">
      <div className="lf-bhero-stage">
        <div className="lf-bhero-team">
          <span className="lf-bhero-crest">
            <SportsCrest src={match.homeCrest} code={match.homeCode} size={64} />
          </span>
          <span className="lf-bhero-name">{match.homeTeam || 'TBD'}</span>
        </div>

        <div className="lf-bhero-center">
          <span className={`lf-bhero-pill ${live ? 'live' : finished ? 'ft' : 'next'}`}>
            {live ? `LIVE${liveMinute ? ` ${liveMinute}` : ''}` : finished ? 'FULL TIME' : 'UP NEXT'}
          </span>
          {hasScore ? (
            <span className="lf-bhero-score">
              {match.homeScore} – {match.awayScore}
            </span>
          ) : (
            // Viewer-timezone kickoff time; SSR (server TZ) can disagree
            // with the client legitimately — suppress per-node warning.
            <span className="lf-bhero-score lf-bhero-ko" suppressHydrationWarning>
              {localKickoffTime(match.kickoffUtc)}
            </span>
          )}
          {upcoming && <Countdown kickoffUtc={match.kickoffUtc} />}
        </div>

        <div className="lf-bhero-team">
          <span className="lf-bhero-crest">
            <SportsCrest src={match.awayCrest} code={match.awayCode} size={64} />
          </span>
          <span className="lf-bhero-name">{match.awayTeam || 'TBD'}</span>
        </div>
      </div>

      {meta && <div className="lf-bhero-meta">{meta}</div>}

      {takes.length > 0 && (
        <div className="lf-bhero-takes">
          <RotatingTake takes={takes.slice(0, 3)} />
        </div>
      )}

      {live && <UpdatedAgo at={updatedAt} />}
    </Link>
  )
}
