import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { slugifyTitle } from './lib/post-url'

// Edge middleware for /post/{uuid}[/slug] — owns BOTH the soft-404
// case (deleted/unknown post → real HTTP 404) AND the slug-canonical
// redirect (slugless or wrong slug → 308 to canonical).
//
// Why all of this lives in middleware instead of the page Server
// Component:
//
//   We tried both notFound() and redirect() inside the page
//   Server Component. In production, neither set the response status
//   reliably — bad-UUID pages came back 200 with the metadata
//   fallback, and pages whose URL slug was non-canonical didn't fire
//   the redirect at all. Best guess at root cause: ISR/standalone
//   build interaction with Server Component throws. We don't need to
//   fix that — middleware runs at the edge before rendering and
//   NextResponse.redirect / NextResponse.rewrite return real HTTP
//   responses with whatever status we set.
//
// Soft-404 matters because Google de-indexes "200 with empty
// content" pages aggressively (we just lived through it). Slug
// canonicalization matters because every truncated/old/wrong slug
// in the wild needs to consolidate to one URL or Google splits the
// ranking signal across variants.
//
// Slug source of truth: slugifyTitle from lib/post-url. The page,
// the sitemap, and the RSS feed all use this same function. The
// previous middleware had its own naive .slice(0, 80) which could
// cut a word in the middle (e.g. "...reaching-350-u"), producing a
// URL that mismatched the page's canonical slug — Google then saw
// a redirect into a page whose <link rel="canonical"> pointed at a
// different URL. Sharing the function eliminates that drift.

const POST_RE =
  /^\/post\/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(?:\/([^/]+))?\/?$/

export async function middleware(request: NextRequest) {
  const m = request.nextUrl.pathname.match(POST_RE)
  if (!m) return NextResponse.next()

  const id = m[1].toLowerCase()
  const slug = m[2]
  const apiUrl = process.env.API_URL || 'http://localhost:8080'

  let post: { title?: string } | null = null
  let apiStatus = 0
  try {
    const res = await fetch(`${apiUrl}/api/v1/posts/${id}`, {
      next: { revalidate: 60 },
      signal: AbortSignal.timeout(2000),
    })
    apiStatus = res.status
    if (res.ok) post = (await res.json()) as { title?: string }
  } catch {
    // Network/timeout/parse error — fall through. We let the page
    // handle this case; best to show stale-but-served content than
    // 404 a real post just because the API hiccuped.
    return NextResponse.next()
  }

  // Definitive 404 from the API → return a real HTTP 404. This is
  // the path Googlebot follows when re-crawling deleted/unknown
  // posts; we MUST give it 404, not 200, or those URLs persist in
  // the index as soft-404s.
  if (apiStatus === 404 || (apiStatus >= 200 && apiStatus < 300 && !post)) {
    return new NextResponse(NOT_FOUND_HTML, {
      status: 404,
      headers: { 'content-type': 'text/html; charset=utf-8' },
    })
  }

  // API returned non-2xx, non-404 (5xx, etc) — let the page render
  // what it can. We don't want to mask outages as 404.
  if (!post) return NextResponse.next()

  const canonicalSlug = slugifyTitle(post.title)
  if (slug !== canonicalSlug) {
    const url = request.nextUrl.clone()
    url.pathname = `/post/${id}/${canonicalSlug}`
    return NextResponse.redirect(url, 308)
  }

  return NextResponse.next()
}

// Inline 404 page. Crawlers only need the status; humans see a
// minimal but readable page that points back home. We don't try to
// reuse the global not-found.tsx because middleware can't render
// React components — and rewriting to a route to render it has
// proven flaky on this stack (same reason we're in middleware in
// the first place).
const NOT_FOUND_HTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>Post not found | loomfeed</title>
<style>
  body { font-family: ui-sans-serif, system-ui, sans-serif; background: #f4f2eb; color: #18181b; margin: 0; padding: 80px 24px; text-align: center; }
  h1 { font-size: 48px; font-weight: 800; letter-spacing: -0.04em; margin: 0 0 8px; }
  p { font-size: 18px; color: #71717a; max-width: 560px; margin: 0 auto 32px; }
  a { display: inline-block; padding: 10px 24px; border-radius: 8px; background: #18181b; color: #fff; font-weight: 600; text-decoration: none; }
</style>
</head>
<body>
<h1>404</h1>
<p>This post doesn't exist. It may have been deleted, or the link may be wrong.</p>
<a href="/">Go home</a>
</body>
</html>`

export const config = {
  matcher: ['/post/:id', '/post/:id/:slug'],
}
