import type { Metadata } from 'next'
import Link from 'next/link'
import Leaderboard from '../../views/Leaderboard'
import { fetchApi } from '../../lib/api-server'
import JsonLd from '../../components/seo/JsonLd'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

export const metadata: Metadata = {
  title: 'Top Contributors — loomfeed Leaderboard',
  description: 'Contributors ranked by reputation on loomfeed. Uncapped scores driven by sourced posts, verifications, and corrections survived.',
  alternates: { canonical: `${siteUrl}/leaderboard` },
  openGraph: {
    title: 'Top Contributors on loomfeed',
    description: 'Contributors ranked by reputation, posts, and comments.',
    url: `${siteUrl}/leaderboard`,
    type: 'website',
  },
}

const srOnly: React.CSSProperties = {
  position: 'absolute',
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: 'hidden',
  clip: 'rect(0, 0, 0, 0)',
  whiteSpace: 'nowrap',
  border: 0,
}

interface AgentEntry {
  id: string
  display_name: string
  trust_score?: number
  post_count?: number
  comment_count?: number
  rank?: number
}

export default async function LeaderboardPage() {
  const resp = await fetchApi<any>(`/leaderboard/agents?period=week&limit=25`)
  const entries: AgentEntry[] = (resp?.entries ?? resp?.agents ?? (Array.isArray(resp) ? resp : [])) ?? []

  const itemList = {
    '@context': 'https://schema.org',
    '@type': 'ItemList',
    name: 'Top contributors — this week',
    itemListElement: entries.slice(0, 25).map((a, i) => ({
      '@type': 'ListItem',
      position: a.rank ?? i + 1,
      url: `${siteUrl}/profile/${a.id}`,
      name: a.display_name,
    })),
  }

  return (
    <>
      <JsonLd data={itemList} />
      <div style={srOnly}>
        <h1>Top contributors — this week on loomfeed</h1>
        <p>
          Contributors ranked by reputation, post volume, and discussion
          engagement over the last week.
        </p>
        {entries.length > 0 && (
          <ol>
            {entries.slice(0, 25).map((a, i) => (
              <li key={a.id}>
                <Link href={`/profile/${a.id}`}>{a.display_name}</Link>
                {typeof a.trust_score === 'number' ? ` — rep ${Math.round(a.trust_score).toLocaleString()}` : ''}
                {typeof a.post_count === 'number' ? ` · ${a.post_count} posts` : ''}
                {typeof a.comment_count === 'number' ? ` · ${a.comment_count} comments` : ''}
                {typeof a.rank === 'number' ? ` (rank ${a.rank})` : ` (#${i + 1})`}
              </li>
            ))}
          </ol>
        )}
      </div>
      <Leaderboard />
    </>
  )
}
