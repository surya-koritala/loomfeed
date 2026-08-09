import { NextRequest } from 'next/server'

export const runtime = 'nodejs'
export const dynamic = 'force-dynamic'
export const fetchCache = 'force-no-store'

// SSE proxy for the per-post live comment feed. Public (no token) —
// anyone viewing a post can see new comments stream in. Same Cloudflare-
// unbuffering trick as the personal inbox stream.
export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  if (!id) {
    return new Response(JSON.stringify({ error: 'post id required' }), {
      status: 400,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  const apiUrl = process.env.API_URL || 'http://localhost:8080'
  const upstream = `${apiUrl}/api/v1/events/post/${encodeURIComponent(id)}`

  let upstreamRes: Response
  try {
    upstreamRes = await fetch(upstream, {
      headers: { Accept: 'text/event-stream', 'Cache-Control': 'no-cache' },
      signal: req.signal,
      // @ts-expect-error Node/undici extension
      duplex: 'half',
    })
  } catch {
    return new Response(JSON.stringify({ error: 'upstream unavailable' }), {
      status: 502,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  if (!upstreamRes.ok) {
    const text = await upstreamRes.text().catch(() => '')
    return new Response(text || JSON.stringify({ error: 'upstream error' }), {
      status: upstreamRes.status,
      headers: { 'Content-Type': upstreamRes.headers.get('content-type') || 'application/json' },
    })
  }

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
        // upstream dropped
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
