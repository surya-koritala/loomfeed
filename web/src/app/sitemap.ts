import type { MetadataRoute } from 'next'
import { fetchApi } from '@/lib/api-server'
import { slugifyTitle } from '@/lib/post-url'

// Next.js sitemap route — serves the sitemap index at /sitemap.xml
// and the actual URL lists at /sitemap/{id}.xml.
//
// Sitemap layout (sub-sitemap id → contents)
//   0 → static pages + communities + profiles (well under 50k each)
//   1..N → posts, chunked at POSTS_PER_SHARD per shard
//
// Why split: the sitemaps.org spec caps a single sitemap file at
// 50k URLs. loomfeed crossed 50k posts in early May 2026; without
// shards Google sees only the most recent 50k posts (newest-first
// truncation), and older content stops being submitted.

export const revalidate = 1800 // regenerate every 30 min

// Per-shard cap. Set well below the 50k spec ceiling so we have
// headroom and shards stay reasonably sized for Google's fetch budget.
const POSTS_PER_SHARD = 25000

type SitemapEntry = { id: string; slug?: string; title?: string; updated_at: string }
type PostsCount = { total: number }

const siteUrl = () => process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

function toISO(s: string | undefined | null, fallback?: Date): Date {
  if (!s) return fallback ?? new Date()
  const d = new Date(s)
  return isNaN(d.getTime()) ? fallback ?? new Date() : d
}

// generateSitemaps tells Next.js how many sub-sitemap files exist.
// The shard count is computed at build/revalidate time from the live
// post count; new shards appear automatically as the corpus grows.
export async function generateSitemaps() {
  let postShards = 1
  try {
    const c = await fetchApi<PostsCount>('/sitemap/posts/count')
    if (c && typeof c.total === 'number' && c.total > 0) {
      postShards = Math.max(1, Math.ceil(c.total / POSTS_PER_SHARD))
    }
  } catch {
    // Degrade gracefully — at minimum we still serve sub-sitemap 0
    // (static + communities + profiles) and one post shard.
  }
  // Shard 0 is the static + communities + profiles bundle.
  // Shards 1..postShards are post chunks.
  const total = 1 + postShards
  return Array.from({ length: total }, (_, i) => ({ id: i }))
}

export default async function sitemap({ id }: { id: number }): Promise<MetadataRoute.Sitemap> {
  // Next.js delivers the id parsed from the URL as a STRING at runtime
  // despite the declared number type. Strict `id === 0` never matched "0",
  // so shard 0 served an empty urlset in production — static pages,
  // communities, tag hubs, and profiles were not being submitted to Google.
  const shardId = Number(id)
  if (shardId === 0) {
    return await staticAndDirectorySitemap()
  }
  // Post shards: id 1 → offset 0, id 2 → offset POSTS_PER_SHARD, etc.
  const offset = (shardId - 1) * POSTS_PER_SHARD
  return await postsShard(offset)
}

async function staticAndDirectorySitemap(): Promise<MetadataRoute.Sitemap> {
  const base = siteUrl()
  const now = new Date()

  const staticPages: MetadataRoute.Sitemap = [
    '',
    '/communities',
    '/topics',
    '/leaderboard',
    '/arena',
    '/sports',
    '/sports/leaderboard',
    '/shorts',
    '/visual',
    // '/search' is intentionally omitted — it's noindex (see
    // app/search/page.tsx), and listing a noindex URL in the sitemap
    // sends Google contradictory signals ("index this" vs the meta tag).
    '/connect',
    '/about',
    '/policy',
    '/privacy',
    '/terms',
  ].map((path) => ({
    url: `${base}${path}`,
    lastModified: now,
    changeFrequency: 'daily' as const,
    priority: path === '' ? 1.0 : 0.8,
  }))

  let communityPages: MetadataRoute.Sitemap = []
  try {
    const communities = await fetchApi<SitemapEntry[]>('/sitemap/communities')
    if (Array.isArray(communities)) {
      communityPages = communities.map((c) => ({
        url: `${base}/a/${c.slug}`,
        lastModified: toISO(c.updated_at, now),
        changeFrequency: 'daily' as const,
        priority: 0.9,
      }))
    }
  } catch {
    // Degrade gracefully — a missing page of the sitemap is better
    // than a 500 that Google treats as sitemap poisoning.
  }

  let profilePages: MetadataRoute.Sitemap = []
  try {
    const profiles = await fetchApi<SitemapEntry[]>('/sitemap/profiles')
    if (Array.isArray(profiles)) {
      profilePages = profiles.map((p) => ({
        url: `${base}/profile/${p.id}`,
        lastModified: toISO(p.updated_at, now),
        changeFrequency: 'weekly' as const,
        priority: 0.6,
      }))
    }
  } catch {
    // pass
  }

  let tagPages: MetadataRoute.Sitemap = []
  try {
    // /sitemap/tags only returns tags with >=2 posts (single-post topic
    // pages are thin content), so every URL here is a substantial page.
    const tags = await fetchApi<Array<{ tag: string; count: number; updated_at: string }>>(
      '/sitemap/tags?limit=5000'
    )
    if (Array.isArray(tags)) {
      tagPages = tags.map((t) => ({
        url: `${base}/t/${encodeURIComponent(t.tag)}`,
        lastModified: toISO(t.updated_at, now),
        changeFrequency: 'daily' as const,
        priority: 0.7,
      }))
    }
  } catch {
    // pass
  }

  return [...staticPages, ...communityPages, ...tagPages, ...profilePages]
}

async function postsShard(offset: number): Promise<MetadataRoute.Sitemap> {
  const base = siteUrl()
  const now = new Date()
  try {
    const posts = await fetchApi<SitemapEntry[]>(
      `/sitemap/posts?offset=${offset}&limit=${POSTS_PER_SHARD}`
    )
    if (!Array.isArray(posts)) return []
    return posts.map((p) => ({
      // Slug-suffixed canonical URL — keywords in the path are a
      // direct ranking signal and improve SERP click-through.
      url: `${base}/post/${p.id}/${slugifyTitle(p.title)}`,
      lastModified: toISO(p.updated_at, now),
      changeFrequency: 'weekly' as const,
      priority: 0.7,
    }))
  } catch {
    return []
  }
}
