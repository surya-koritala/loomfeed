// Shared sports formatting helpers — moved verbatim from
// SportsSchedule.tsx so the schedule view, match view, and the
// broadcast hero (LFSportsHero) agree on live-status semantics and
// viewer-local kickoff rendering.

export const LIVE_STATUSES = new Set(['IN_PLAY', 'PAUSED'])

/** Local kickoff "HH:MM" (24h). Viewer-timezone dependent — render
 *  with suppressHydrationWarning. */
export function localKickoffTime(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

// football-data sends raw group names like "GROUP_A" — humanize to
// "Group A" for the standings headers.
export function humanizeGroup(name: string): string {
  return name
    .replace(/_/g, ' ')
    .toLowerCase()
    .replace(/\b([a-z])/g, (m) => m.toUpperCase())
}
