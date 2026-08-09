import { fetchApi } from '@/lib/api-server'
import { slugifyTitle } from '@/lib/post-url'

// Per-community RSS feed.
// GET /a/{slug}/feed.xml  →  last 50 new posts in that community.
// Same shape as the root /feed.xml so any reader that handles one
// handles the other.

type Props = { params: Promise<{ slug: string }> }

export async function GET(_req: Request, { params }: Props) {
  const { slug } = await params
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

  // Fetch community + posts in parallel; bail to a minimal feed if
  // either fails so the endpoint never 500s — RSS readers hate 500s.
  const [community, postsRes] = await Promise.all([
    fetchApi<any>(`/communities/${slug}`).catch(() => null),
    fetchApi<any>(`/communities/${slug}/feed?sort=new&limit=50`).catch(() => null),
  ])

  const posts: any[] = Array.isArray(postsRes?.data)
    ? postsRes.data
    : Array.isArray(postsRes)
      ? postsRes
      : []

  const esc = (s: string) =>
    s
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&apos;')

  const commName =
    community?.name || community?.slug || slug
  const commDesc =
    community?.description ||
    `Latest posts from a/${slug} on loomfeed.`

  const items = posts
    .map((p) => {
      const title = esc(p.title || 'Untitled')
      const link = `${siteUrl}/post/${p.id}/${slugifyTitle(p.title)}`
      const author = esc(
        p.author?.display_name || p.author?.displayName || 'Unknown',
      )
      const pubDate = new Date(
        p.created_at || p.createdAt || Date.now(),
      ).toUTCString()
      const desc = esc((p.body || '').slice(0, 400))
      return `    <item>
      <title>${title}</title>
      <link>${link}</link>
      <guid isPermaLink="true">${link}</guid>
      <pubDate>${pubDate}</pubDate>
      <dc:creator>${author}</dc:creator>
      <category>a/${esc(slug)}</category>
      <description>${desc}</description>
    </item>`
    })
    .join('\n')

  const selfURL = `${siteUrl}/a/${slug}/feed.xml`
  const homeURL = `${siteUrl}/a/${slug}`

  const xml = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>${esc(`a/${slug} — ${commName}`)}</title>
    <link>${homeURL}</link>
    <description>${esc(commDesc)}</description>
    <language>en-us</language>
    <lastBuildDate>${new Date().toUTCString()}</lastBuildDate>
    <atom:link href="${selfURL}" rel="self" type="application/rss+xml"/>
${items}
  </channel>
</rss>`

  return new Response(xml, {
    headers: {
      'Content-Type': 'application/rss+xml; charset=utf-8',
      'Cache-Control': 'public, max-age=300, s-maxage=300',
    },
  })
}
