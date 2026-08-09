import { fetchApi } from '@/lib/api-server'

// Sitemap index route. Next.js's `generateSitemaps()` in
// `app/sitemap.ts` produces /sitemap/{id}.xml sub-sitemaps but does
// NOT auto-create a /sitemap.xml index pointing at them. Without
// this route, /sitemap.xml 404s — which is what robots.txt advertises
// to Google and what Search Console has already cached.
//
// Output is a sitemapindex XML referencing each /sitemap/{id}.xml in
// turn. Shard count mirrors the logic in app/sitemap.ts:
//   shard 0 = static + communities + profiles
//   shards 1..N = posts in chunks of POSTS_PER_SHARD
//
// Cached for 30 min to match the Next sitemap revalidate window.

// Keep in lockstep with app/sitemap.ts. If we ever change the post
// shard size, update both.
const POSTS_PER_SHARD = 25000

export const revalidate = 1800

type PostsCount = { total: number }

export async function GET() {
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'
  const lastmod = new Date().toISOString()

  let postShards = 1
  try {
    const c = await fetchApi<PostsCount>('/sitemap/posts/count')
    if (c && typeof c.total === 'number' && c.total > 0) {
      postShards = Math.max(1, Math.ceil(c.total / POSTS_PER_SHARD))
    }
  } catch {
    // Degrade gracefully — emit a valid index with just shard 0 and
    // one post shard. Better than a 404 for the index URL.
  }

  const totalShards = 1 + postShards
  const entries: string[] = []
  for (let i = 0; i < totalShards; i++) {
    entries.push(
      `  <sitemap>\n` +
        `    <loc>${siteUrl}/sitemap/${i}.xml</loc>\n` +
        `    <lastmod>${lastmod}</lastmod>\n` +
        `  </sitemap>`
    )
  }

  const xml =
    `<?xml version="1.0" encoding="UTF-8"?>\n` +
    `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n` +
    entries.join('\n') +
    `\n</sitemapindex>\n`

  return new Response(xml, {
    headers: {
      'Content-Type': 'application/xml; charset=utf-8',
      // Match the 30-min Next revalidate. Independent CDN cache hint
      // so an out-of-band fetch (Googlebot, curl) gets a fresh
      // response when one is available.
      'Cache-Control': 'public, max-age=1800, s-maxage=1800',
    },
  })
}
