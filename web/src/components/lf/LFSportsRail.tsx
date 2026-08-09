'use client'

import Link from 'next/link'

// Sports right-rail modules — /sports (SportsSchedule.tsx). Purely
// presentational: the view normalizes all wire data (snake/camel) and
// passes it in; nothing here fetches or normalizes. Quiet professional
// v2: 1px --lf-rule-soft dividers, tabular numerals, lime strictly for
// the live dot. Layout rules live in index.css §sports.
// (The featured-match card moved to LFSportsHero — the broadcast hero
// in the main column; the rail keeps standings + the mini-board.)

export interface RailTake {
  id: string
  matchId: string
  participantId: string
  displayName: string
  body: string
  pick: string
  eventSeq: number | null
}

export interface RailStandingRow {
  groupName: string
  team: string
  code: string
  played: number
  gd: number
  points: number
}

export interface RailMiniRow {
  id: string
  name: string
  n: number
  correct: number
}

/* ---- group standings -------------------------------------------------- */
function StandingsTable({ rows }: { rows: RailStandingRow[] }) {
  return (
    <table className="lf-sprail-table">
      <thead>
        <tr>
          <th scope="col" className="lf-sprail-th--team">Team</th>
          <th scope="col">P</th>
          <th scope="col">GD</th>
          <th scope="col">PTS</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={`${r.groupName}-${r.team}`}>
            <td className="lf-sprail-td--team">{r.team}</td>
            <td>{r.played}</td>
            <td>{r.gd > 0 ? `+${r.gd}` : r.gd}</td>
            <td className="lf-sprail-td--pts">{r.points}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

export function StandingsPanel({
  rows,
  collapsible,
}: {
  rows: RailStandingRow[]
  collapsible?: boolean
}) {
  if (rows.length === 0) return null
  // Group by groupName, preserving wire order within and across groups.
  const order: string[] = []
  const byGroup = new Map<string, RailStandingRow[]>()
  for (const r of rows) {
    const g = r.groupName || 'Standings'
    if (!byGroup.has(g)) {
      byGroup.set(g, [])
      order.push(g)
    }
    byGroup.get(g)!.push(r)
  }
  return (
    <section className="lf-sprail-standings" aria-label="Group standings">
      <h2 className="lf-sprail-h">Standings</h2>
      {order.map((g, i) =>
        collapsible ? (
          <details key={g} className="lf-sprail-group" open={i === 0}>
            <summary className="lf-sprail-group-name">{g}</summary>
            <StandingsTable rows={byGroup.get(g)!} />
          </details>
        ) : (
          <div key={g} className="lf-sprail-group">
            <h3 className="lf-sprail-group-name">{g}</h3>
            <StandingsTable rows={byGroup.get(g)!} />
          </div>
        ),
      )}
    </section>
  )
}

/* ---- top-predictors mini-board ---------------------------------------- */
export function MiniLeaderboard({ rows }: { rows: RailMiniRow[] }) {
  if (rows.length === 0) return null
  return (
    <section className="lf-sprail-mini" aria-label="Top predictors">
      <h2 className="lf-sprail-h">Top predictors</h2>
      {rows.slice(0, 5).map((r, i) => (
        <div key={r.id || i} className="lf-sprail-mini-row">
          <span className="lf-sprail-mini-rank">{String(i + 1).padStart(2, '0')}</span>
          <span className="lf-sprail-mini-name">{r.name || 'Agent'}</span>
          <span className="lf-sprail-mini-record">
            {r.correct}/{r.n}
          </span>
        </div>
      ))}
      <Link href="/sports/leaderboard" className="lf-sprail-mini-more">
        Full leaderboard →
      </Link>
    </section>
  )
}
