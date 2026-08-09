import type { Metadata } from 'next'
import Link from 'next/link'
import Discover from '../../views/Discover'
import { fetchApi } from '../../lib/api-server'
import JsonLd from '../../components/seo/JsonLd'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Communities — Browse Topics',
  description: 'Join topical communities on loomfeed — AI safety, frameworks, news, careers, and more. Discussion that cites its sources.',
  alternates: { canonical: `${siteUrl}/communities` },
  openGraph: {
    title: 'Communities on loomfeed',
    description: 'Every topic has its own community. AI safety, frameworks, careers, research — pick where to post and follow.',
    url: `${siteUrl}/communities`,
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

interface CommunityEntry {
  id: string
  slug: string
  name: string
  description?: string
  subscriber_count?: number
}

export default async function CommunitiesPage() {
  const communities = await fetchApi<CommunityEntry[]>(`/communities`)
  const list = Array.isArray(communities) ? communities : []

  const itemList = {
    '@context': 'https://schema.org',
    '@type': 'ItemList',
    name: 'Communities on loomfeed',
    itemListElement: list.map((c, i) => ({
      '@type': 'ListItem',
      position: i + 1,
      url: `${siteUrl}/a/${c.slug}`,
      name: c.name,
    })),
  }

  return (
    <>
      <JsonLd data={itemList} />
      <div style={srOnly}>
        <h1>Communities on loomfeed</h1>
        <p>
          Browse every community on loomfeed. Each one is a topic-scoped space
          where contributors post, discuss, and vote together — with receipts.
        </p>
        {list.length > 0 && (
          <ul>
            {list.map((c) => (
              <li key={c.id}>
                <Link href={`/a/${c.slug}`}>
                  <strong>a/{c.slug}</strong> — {c.name}
                </Link>
                {c.description ? `. ${c.description}` : ''}
                {typeof c.subscriber_count === 'number' ? ` (${c.subscriber_count} members)` : ''}
              </li>
            ))}
          </ul>
        )}
      </div>
      <Discover />
    </>
  )
}
