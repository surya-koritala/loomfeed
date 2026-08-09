import { getTweet } from 'react-tweet/api'

// Server-side tweet fetch endpoint. The `<Tweet>` client component in
// EmbedRenderer hits this instead of going directly to Twitter's
// syndication API, which is flaky from browsers (rate limits, CORS,
// ad blockers, the token endpoint sometimes 403s).
//
// IMPORTANT: this route lives at /tweet-embed/[id], NOT /api/tweet/[id],
// because the production reverse proxy routes every /api/* path to
// the Go backend — a Next.js route under /api/ would be unreachable
// (verified: /api/tweet/X returns Go's plain-text 404 in prod). Keep
// this prefix in sync with EmbedRenderer's apiUrl prop.
//
// Routing the fetch through our backend gives us:
//   - reliable fetch (Node runtime, no browser quirks)
//   - one cached call per tweet via Next's `cache: 'force-cache'`
//     under the hood in getTweet (and the s-maxage header below)
//   - a single place to add a fallback / log failures
//
// react-tweet's <Tweet id apiUrl="/tweet-embed/{id}"> consumes the
// { data: Tweet } shape returned here.

export const runtime = 'nodejs'

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id } = await params

  if (!/^\d+$/.test(id)) {
    return Response.json({ error: 'invalid tweet id' }, { status: 400 })
  }

  try {
    const tweet = await getTweet(id)
    if (!tweet) {
      return Response.json({ data: null }, {
        status: 404,
        headers: { 'Cache-Control': 'public, s-maxage=300, stale-while-revalidate=600' },
      })
    }
    return Response.json({ data: tweet }, {
      headers: { 'Cache-Control': 'public, s-maxage=3600, stale-while-revalidate=86400' },
    })
  } catch (err) {
    const message = err instanceof Error ? err.message : 'unknown error'
    return Response.json({ error: message }, {
      status: 500,
      headers: { 'Cache-Control': 'no-store' },
    })
  }
}
