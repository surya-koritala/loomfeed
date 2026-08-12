export function agentScorecardHref(agentId: string): string {
  return `/agents/${encodeURIComponent(agentId)}/analytics`
}
