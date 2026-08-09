import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import Community from '../../../views/Community'
import { fetchApi } from '../../../lib/api-server'
import JsonLd from '../../../components/seo/JsonLd'

type Props = { params: Promise<{ slug: string }> }

function formatCount(n: number): string {
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
  return String(n)
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params
  const community = await fetchApi<any>(`/communities/${slug}`)
  if (!community) notFound()

  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'
  const name = community.name || slug
  const baseDesc = (community.description || '').slice(0, 120)
  const memberCount = community.subscriber_count ?? community.subscriberCount ?? 0

  // The /communities/{slug} endpoint doesn't include post_count, so we
  // probe the feed directly to decide whether the community is truly
  // empty. Next.js dedupes this fetch with the one in the page body
  // below, so it's a single round trip per request.
  const feedProbe = await fetchApi<any>(`/communities/${slug}/feed?sort=hot&limit=1`)
  const probePosts =
    (Array.isArray(feedProbe) ? feedProbe : feedProbe?.data ?? []) ?? []
  const hasAnyPosts = probePosts.length > 0

  // Build a rich description: "description . X members"
  const parts = [baseDesc]
  if (memberCount > 0) parts.push(`${formatCount(memberCount)} members`)
  const desc = parts.filter(Boolean).join(' \u00B7 ')

  const ogTitle = `a/${slug} \u2014 ${name}`
  const ogDesc = baseDesc || `Join the ${name} community on loomfeed`

  const meta: Metadata = {
    title: `a/${slug}`,
    description: desc,
    alternates: {
      canonical: `${siteUrl}/a/${slug}`,
      types: {
        'application/rss+xml': `${siteUrl}/a/${slug}/feed.xml`,
      },
    },
    openGraph: {
      title: ogTitle,
      description: ogDesc,
      type: 'website',
      url: `${siteUrl}/a/${slug}`,
      images: [
        `${siteUrl}/og?title=${encodeURIComponent(name)}&subtitle=${encodeURIComponent(`${formatCount(memberCount)} members`)}`,
      ],
    },
    twitter: {
      card: 'summary',
      title: ogTitle,
      description: ogDesc,
    },
  }

  // Only noindex when the community is genuinely empty — serving an
  // empty community to Google trains its Soft 404 classifier against
  // us. We check the feed itself (not the community object, which
  // doesn't expose a post_count field) so a brand-new but populated
  // community stays indexable.
  if (!hasAnyPosts) {
    meta.robots = { index: false, follow: true }
  }

  return meta
}

export default async function CommunityPage({ params }: Props) {
  const { slug } = await params
  const community = await fetchApi<any>(`/communities/${slug}`)
  if (!community) notFound()
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

  // Fetch the first page of posts server-side so the initial HTML has
  // real, crawler-visible content. This is the load-bearing piece of
  // the Soft-404 fix — previously the HTML was 18 chars of chrome.
  const feed = community
    ? await fetchApi<any>(`/communities/${slug}/feed?sort=hot&limit=25`)
    : null
  const posts: Array<{ id: string; title: string; body?: string; author?: any; vote_score?: number; comment_count?: number }> =
    (Array.isArray(feed) ? feed : feed?.data ?? []) ?? []

  const jsonLd = community ? {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    name: community.name || slug,
    url: `${siteUrl}/a/${slug}`,
    description: (community.description || '').slice(0, 200),
    memberOf: {
      '@type': 'WebSite',
      name: 'loomfeed',
      url: siteUrl,
    },
  } : null

  return (
    <>
      {jsonLd && (
        <JsonLd data={jsonLd} />
      )}

      <Community initialCommunity={community} initialPosts={posts} />
    </>
  )
}
