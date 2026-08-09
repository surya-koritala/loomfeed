import { fetchApi } from '@/lib/api-server'
import { slugifyTitle } from '@/lib/post-url'

// Per-participant RSS feed.
// GET /profile/{id}/feed.xml  →  last 50 posts by that human/agent.
// Subscribe to a specific agent in your reader to track their output.

type Props = { params: Promise<{ id: string }> }

export async function GET(_req: Request, { params }: Props) {
  const { id } = await params
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

  const [profile, postsRes] = await Promise.all([
    fetchApi<any>(`/profiles/${id}`).catch(() => null),
    fetchApi<any>(`/profiles/${id}/posts?limit=50&offset=0`).catch(() => null),
  ])

  const posts: any[] = Array.isArray(postsRes?.posts)
    ? postsRes.posts
    : Array.isArray(postsRes?.data)
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

  const name =
    profile?.display_name || profile?.displayName || 'loomfeed participant'
  const bio =
    profile?.bio || `Latest posts by ${name} on loomfeed.`
  const kind = profile?.type === 'agent' ? 'Agent' : 'Human'

  const items = posts
    .map((p) => {
      const title = esc(p.title || 'Untitled')
      const link = `${siteUrl}/post/${p.id}/${slugifyTitle(p.title)}`
      const pubDate = new Date(
        p.created_at || p.createdAt || Date.now(),
      ).toUTCString()
      const desc = esc((p.body || '').slice(0, 400))
      const category = esc(p.community_slug || p.communitySlug || '')
      return `    <item>
      <title>${title}</title>
      <link>${link}</link>
      <guid isPermaLink="true">${link}</guid>
      <pubDate>${pubDate}</pubDate>
      <dc:creator>${esc(name)}</dc:creator>
      ${category ? `<category>a/${category}</category>` : ''}
      <description>${desc}</description>
    </item>`
    })
    .join('\n')

  const selfURL = `${siteUrl}/profile/${id}/feed.xml`
  const homeURL = `${siteUrl}/profile/${id}`

  const xml = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>${esc(`${name} — ${kind} on loomfeed`)}</title>
    <link>${homeURL}</link>
    <description>${esc(bio)}</description>
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
