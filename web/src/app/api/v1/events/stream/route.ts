import { NextRequest } from 'next/server'

export const runtime = 'nodejs'
export const dynamic = 'force-dynamic'
export const fetchCache = 'force-no-store'

// Dedicated SSE proxy route — bypasses Next.js rewrite proxy which kills
// long-lived connections. Streams events from the Go API to the client.
//
// Auth: forwards (in priority order) the lf_access cookie, the
// Authorization header, or the legacy ?token= query param. The frontend
// EventSource (providers.tsx) deliberately doesn't put the token in
// the URL anymore — relies on the cookie attaching automatically.
// Previously this route required ?token= and returned 401 for every
// cookie-authed request, which is what surfaced as the 401 the user
// reported. Backend (handlers/events.go) accepts all three sources.
export async function GET(req: NextRequest) {
  const queryToken = req.nextUrl.searchParams.get('token') ?? ''
  const cookieToken = req.cookies.get('lf_access')?.value ?? ''
  const authHeader = req.headers.get('authorization') ?? ''
  const headerToken = authHeader.startsWith('Bearer ')
    ? authHeader.slice('Bearer '.length)
    : ''
  const token = cookieToken || headerToken || queryToken
  if (!token) {
    return new Response(JSON.stringify({ error: 'unauthorized' }), {
      status: 401,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  const apiUrl = process.env.API_URL || 'http://localhost:8080'
  const upstream = `${apiUrl}/api/v1/events/stream`

  let upstreamRes: Response
  try {
    upstreamRes = await fetch(upstream, {
      headers: {
        Accept: 'text/event-stream',
        'Cache-Control': 'no-cache',
        // Forward the resolved token as a Bearer header — works no
        // matter which of the three sources it came from. Avoids
        // putting the token in the URL (which would log it in
        // intermediary access logs upstream of the Go backend).
        Authorization: `Bearer ${token}`,
      },
      signal: req.signal,
      // @ts-expect-error — Node/undici extension, disables response buffering
      duplex: 'half',
    })
  } catch {
    return new Response(JSON.stringify({ error: 'upstream unavailable' }), {
      status: 502,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  // Non-2xx: read body eagerly and return a concrete response so CF/edge
  // proxies don't see a chunked stream they can't interpret.
  if (!upstreamRes.ok) {
    const text = await upstreamRes.text().catch(() => '')
    return new Response(text || JSON.stringify({ error: 'upstream error' }), {
      status: upstreamRes.status,
      headers: {
        'Content-Type': upstreamRes.headers.get('content-type') || 'application/json',
      },
    })
  }

  // Success: stream upstream SSE body through with a TransformStream to
  // ensure chunks flush individually. Prefix with a 2KB padding comment so
  // proxies (Cloudflare, etc.) that buffer the first response chunk release
  // headers to the client immediately.
  const encoder = new TextEncoder()
  const padding = ':' + ' '.repeat(2048) + '\n\n'

  const body = upstreamRes.body
  if (!body) {
    return new Response(JSON.stringify({ error: 'upstream empty' }), {
      status: 502,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  const stream = new ReadableStream<Uint8Array>({
    async start(controller) {
      controller.enqueue(encoder.encode(padding))
      const reader = body.getReader()
      try {
        for (;;) {
          const { done, value } = await reader.read()
          if (done) break
          if (value) controller.enqueue(value)
        }
      } catch {
        // upstream dropped — close the stream
      } finally {
        try { controller.close() } catch {}
        try { reader.releaseLock() } catch {}
      }
    },
    cancel() {
      try { body.cancel() } catch {}
    },
  })

  return new Response(stream, {
    status: 200,
    headers: {
      'Content-Type': 'text/event-stream; charset=utf-8',
      'Cache-Control': 'no-cache, no-transform',
      Connection: 'keep-alive',
      'X-Accel-Buffering': 'no',
    },
  })
}
