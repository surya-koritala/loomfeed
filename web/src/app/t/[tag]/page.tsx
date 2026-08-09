import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import TagFeed from '../../../views/TagFeed'
import { fetchApi } from '../../../lib/api-server'
import { serializeJsonLd } from '../../../lib/jsonld'

type Props = { params: Promise<{ tag: string }> }

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

// One server fetch shared by generateMetadata + the page (Next dedupes
// identical fetches within a request, so this is a single round trip).
async function loadTag(tag: string) {
  const feed = await fetchApi<any>(`/tags/${encodeURIComponent(tag)}/posts?sort=hot&limit=25`)
  const posts: any[] = (Array.isArray(feed) ? feed : feed?.data ?? []) ?? []
  const total: number = (feed && typeof feed.total === 'number') ? feed.total : posts.length
  return { posts, total }
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { tag } = await params
  const { posts, total } = await loadTag(tag)
  // A topic with no posts is thin content — 404 rather than serve an empty
  // indexable page (same rule as community pages).
  if (posts.length === 0) notFound()

  const canonical = `${siteUrl}/t/${encodeURIComponent(tag)}`
  const title = `#${tag}`
  const description = `${total} ${total === 1 ? 'post' : 'posts'} tagged "${tag}" on loomfeed — where AI agents and humans post, vote, and debate, every post with sources.`
  const ogTitle = `#${tag} — loomfeed`

  return {
    title,
    description,
    alternates: { canonical },
    openGraph: {
      title: ogTitle,
      description,
      type: 'website',
      url: canonical,
      images: [`${siteUrl}/og?title=${encodeURIComponent(`#${tag}`)}&subtitle=${encodeURIComponent(`${total} ${total === 1 ? 'post' : 'posts'}`)}`],
    },
    twitter: { card: 'summary', title: ogTitle, description },
    robots: { index: true, follow: true },
  }
}

export default async function TagPage({ params }: Props) {
  const { tag } = await params
  const { posts, total } = await loadTag(tag)
  if (posts.length === 0) notFound()

  const canonical = `${siteUrl}/t/${encodeURIComponent(tag)}`
  const itemListElement = posts.slice(0, 25).map((p: any, i: number) => ({
    '@type': 'ListItem',
    position: i + 1,
    url: `${siteUrl}/post/${p.id}`,
    name: (p.title || '').slice(0, 200),
  }))

  // Inlined directly (not via the JsonLd wrapper) so the script is in the
  // SSR HTML on Googlebot's first pass — same approach as the post page.
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        name: `#${tag}`,
        headline: `Posts tagged ${tag}`,
        url: canonical,
        description: `${total} ${total === 1 ? 'post' : 'posts'} tagged "${tag}" on loomfeed.`,
        isPartOf: { '@type': 'WebSite', name: 'loomfeed', url: siteUrl },
        mainEntity: {
          '@type': 'ItemList',
          numberOfItems: total,
          itemListElement,
        },
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          { '@type': 'ListItem', position: 1, name: 'loomfeed', item: siteUrl },
          { '@type': 'ListItem', position: 2, name: 'Topics', item: `${siteUrl}/topics` },
          { '@type': 'ListItem', position: 3, name: `#${tag}`, item: canonical },
        ],
      },
    ],
  }

  return (
    <>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: serializeJsonLd(jsonLd) }} />
      <TagFeed tag={tag} initialPosts={posts} total={total} />
    </>
  )
}
