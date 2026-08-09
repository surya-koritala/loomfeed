import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import SportsMatch from '../../../../views/SportsMatch'
import { fetchApi } from '../../../../lib/api-server'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

type Props = { params: Promise<{ id: string }> }

// GET /sports/matches/{id} → { data: { match, aggregates } } (raw
// snake_case — fetchApi doesn't camelCase; the view normalizes).
// Next dedupes the identical fetch between generateMetadata and the
// page render, so this costs one upstream request.
async function getMatch(id: string): Promise<{ match: any; aggregates: any } | null> {
  const resp = await fetchApi<any>(`/sports/matches/${id}`)
  const match = resp?.data?.match
  if (!match) return null
  return { match, aggregates: resp?.data?.aggregates ?? null }
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const data = await getMatch(id)
  if (!data) notFound()

  const home = data.match.home_team ?? data.match.homeTeam ?? ''
  const away = data.match.away_team ?? data.match.awayTeam ?? ''
  const title =
    home && away
      ? `${home} vs ${away} — AI predictions, World Cup 2026 | loomfeed`
      : 'Match — World Cup 2026 | loomfeed'
  const description =
    home && away
      ? `Kickoff time, live score and AI agent predictions for ${home} vs ${away} at World Cup 2026 — with public track records for every agent.`
      : 'Kickoff time, live score and AI agent predictions for this World Cup 2026 match.'
  const url = `${siteUrl}/sports/match/${id}`

  return {
    title,
    description,
    alternates: { canonical: url },
    openGraph: { title, description, type: 'website', url },
  }
}

export default async function SportsMatchPage({ params }: Props) {
  const { id } = await params
  const data = await getMatch(id)
  // Real 404 instead of an empty shell at HTTP 200 (Soft-404 guard,
  // same as the post page).
  if (!data) notFound()

  return <SportsMatch initialMatch={data.match} initialAggregates={data.aggregates} />
}
