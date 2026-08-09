import { describe, it, expect, vi } from 'vitest'
import sitemap from './sitemap'

// No backend in unit tests — fetchApi resolves null (its real no-backend
// behavior) so directory fetches degrade and only unconditional URLs remain.
vi.mock('@/lib/api-server', () => ({ fetchApi: vi.fn(async () => null) }))

// At runtime Next.js delivers the generateSitemaps id to sitemap({ id })
// as a STRING. Production regression: `id === 0` failed for "0", so shard 0
// fell through to the posts branch with offset ("0" - 1) * 25000 = -25000
// and served an empty <urlset> — static pages, communities, tag hubs, and
// profiles were never submitted to Google. (Shards "1".."3" worked only
// because "1" - 1 coerces to 0 numerically.)
//
// fetchApi returns null when no backend is reachable (as in this test env),
// so the directory fetches degrade gracefully and the static pages — which
// are unconditional — must still be present.
describe('sitemap shard routing', () => {
  it('serves the static/directory bundle when id arrives as the string "0"', async () => {
    const entries = await sitemap({ id: '0' as unknown as number })
    const urls = entries.map((e) => e.url)
    expect(urls).toContain('https://www.loomfeed.com')
    expect(urls).toContain('https://www.loomfeed.com/communities')
    expect(urls).toContain('https://www.loomfeed.com/topics')
  })

  it('serves the static/directory bundle for numeric id 0', async () => {
    const entries = await sitemap({ id: 0 })
    expect(entries.map((e) => e.url)).toContain('https://www.loomfeed.com')
  })
})
