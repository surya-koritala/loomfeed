import { fetchApi } from '@/lib/api-server'
import { slugifyTitle } from '@/lib/post-url'

export async function GET() {
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

  let posts: any[] = []
  try {
    const result = await fetchApi<any>('/posts?sort=new&limit=50')
    if (result && Array.isArray(result.data)) {
      posts = result.data
    } else if (Array.isArray(result)) {
      posts = result
    }
  } catch {
    // fallback to empty
  }

  const escapeXml = (s: string) =>
    s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&apos;')

  const items = posts.map((p) => {
    const title = escapeXml(p.title || 'Untitled')
    const link = `${siteUrl}/post/${p.id}/${slugifyTitle(p.title)}`
    const author = escapeXml(p.author?.display_name || p.author?.displayName || 'Unknown')
    const pubDate = new Date(p.created_at || p.createdAt || Date.now()).toUTCString()
    const desc = escapeXml((p.body || '').slice(0, 300))
    return `    <item>
      <title>${title}</title>
      <link>${link}</link>
      <guid isPermaLink="true">${link}</guid>
      <pubDate>${pubDate}</pubDate>
      <dc:creator>${author}</dc:creator>
      <description>${desc}</description>
    </item>`
  }).join('\n')

  const xml = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>loomfeed — Posts that come with sources</title>
    <link>${siteUrl}</link>
    <description>Latest posts from loomfeed — topical communities for discussion that cites its sources.</description>
    <language>en-us</language>
    <lastBuildDate>${new Date().toUTCString()}</lastBuildDate>
    <atom:link href="${siteUrl}/feed.xml" rel="self" type="application/rss+xml"/>
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
