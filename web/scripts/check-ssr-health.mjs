#!/usr/bin/env node
// SSR content health check.
//
// Starts the production server (`next start`) against a stubbed API,
// curls a set of contract-bearing routes, and asserts each rendered
// HTML still carries the crawler-visible content we depend on for
// SEO + first-impression. The class of bug this catches: a client
// component wrapping content with a hydration gate (`mounted`,
// top-level `localStorage`, `useSearchParams()` without server
// fall-through, etc.) that quietly guts SSR.
//
// Failure mode: process exits non-zero with the broken contracts
// listed. Wire into CI so a regression blocks merge.
//
// Expected environment:
//   - The web app has already been built (`npm run build`).
//   - API_URL is set to a reachable backend OR omitted, in which case
//     the stub server below answers any /api/v1/* call with empty
//     arrays so feed/profile SSR doesn't 500.

import { readFileSync } from 'node:fs'
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import http from 'node:http'

const PORT = process.env.SSR_HEALTH_PORT ? Number(process.env.SSR_HEALTH_PORT) : 3456
const STUB_PORT = PORT + 1
const STUB_URL = `http://127.0.0.1:${STUB_PORT}`
const BASE = `http://127.0.0.1:${PORT}`

// Each contract = "this route must contain this substring or pattern"
// in its rendered HTML. Pages not listed here are not checked. Add a
// page when it joins the SEO-critical set; remove when deprecated.
const CONTRACTS = [
  {
    route: '/',
    label: 'Home (/)',
    musts: [
      { kind: 'h1', desc: 'visible <h1> tag in HTML' },
      { kind: 'main-min', desc: '<main> content > 400 chars', min: 400 },
      { kind: 'text', desc: 'hero tagline', needle: 'does the research' },
    ],
  },
  {
    route: '/about',
    label: 'About (/about)',
    musts: [
      { kind: 'h1', desc: 'visible <h1> tag in HTML' },
      { kind: 'text', desc: '"Posts that come with sources" headline', needle: 'Posts that come with sources' },
    ],
  },
  {
    route: '/leaderboard',
    label: 'Leaderboard (/leaderboard)',
    musts: [{ kind: 'h1', desc: 'visible <h1> tag' }],
  },
  {
    route: '/connect',
    label: 'Connect (/connect)',
    musts: [
      { kind: 'h1', desc: 'visible <h1> tag' },
      { kind: 'main-min', desc: '<main> content > 500 chars', min: 500 },
    ],
  },
]

function checkContract(html, must) {
  switch (must.kind) {
    case 'h1':
      return html.includes('<h1')
    case 'text':
      return html.includes(must.needle)
    case 'main-min': {
      const m = html.match(/<main[^>]*>([\s\S]*?)<\/main>/)
      return (m ? m[1].length : 0) >= must.min
    }
    case 'json-ld':
      return html.includes(must.needle) && html.includes('application/ld+json')
    default:
      return false
  }
}

function fetchHtml(path) {
  return new Promise((resolve, reject) => {
    http
      .get(BASE + path, { headers: { 'User-Agent': 'ssr-health-check' } }, (res) => {
        let body = ''
        res.on('data', (c) => (body += c))
        res.on('end', () => resolve({ status: res.statusCode, html: body }))
      })
      .on('error', reject)
  })
}

// Stub backend — answers `GET /api/v1/*` with empty-but-valid shapes
// so server-side fetches in page.tsx don't 500 during prerender. This
// keeps the test self-contained; the contract is "did the page
// render its shell content?", not "did the backend return real
// data?". Real backend health is its own check.
function startStub() {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      res.setHeader('content-type', 'application/json')
      // Most list endpoints return { data: [...] }; some return [...]
      // directly. Empty data shape works for both.
      res.end(JSON.stringify({ data: [] }))
    })
    server.listen(STUB_PORT, '127.0.0.1', () => resolve(server))
  })
}

async function waitForServer(maxMs = 60_000) {
  const start = Date.now()
  while (Date.now() - start < maxMs) {
    try {
      const r = await fetchHtml('/')
      if (r.status && r.status < 500) return
    } catch {}
    await sleep(500)
  }
  throw new Error(`server never came up on ${BASE}`)
}

async function main() {
  console.log('SSR health check: starting stub backend + next start...')
  const stub = await startStub()

  const server = spawn('node', ['node_modules/next/dist/bin/next', 'start', '-p', String(PORT)], {
    env: {
      ...process.env,
      API_URL: STUB_URL,
      NEXT_PUBLIC_API_URL: STUB_URL,
      NODE_ENV: 'production',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  const serverLogs = []
  server.stdout.on('data', (c) => serverLogs.push(c.toString()))
  server.stderr.on('data', (c) => serverLogs.push(c.toString()))

  let exitCode = 0
  try {
    await waitForServer()

    const failures = []
    const summary = []
    for (const c of CONTRACTS) {
      let html
      try {
        const r = await fetchHtml(c.route)
        if (r.status >= 400) {
          failures.push(`[${c.label}] HTTP ${r.status}`)
          summary.push(`  ✗ ${c.label} (HTTP ${r.status})`)
          continue
        }
        html = r.html
      } catch (e) {
        failures.push(`[${c.label}] fetch error: ${e.message}`)
        summary.push(`  ✗ ${c.label} (fetch error)`)
        continue
      }
      const pageFails = c.musts.filter((m) => !checkContract(html, m)).map((m) => m.desc)
      if (pageFails.length > 0) {
        failures.push(`[${c.label}] missing: ${pageFails.join(', ')}`)
      }
      summary.push(`  ${pageFails.length === 0 ? '✓' : '✗'} ${c.label}`)
    }

    console.log('SSR content health check:')
    summary.forEach((s) => console.log(s))
    console.log('')

    if (failures.length > 0) {
      console.error('FAIL — SSR contract broken:')
      failures.forEach((f) => console.error('  ' + f))
      console.error('')
      console.error('Crawlers (Google, social previews) will see the broken HTML, not the')
      console.error('hydrated one. Usual cause: a client component wrapping content with a')
      console.error('mounted-gate, a top-level localStorage / window read that throws on the')
      console.error('server, or `useSearchParams()` in a page rendered through a `<Suspense')
      console.error('fallback={null}>` boundary. See PR #113 and #134 for prior fixes.')
      exitCode = 1
    } else {
      console.log('All SSR contracts intact.')
    }
  } catch (e) {
    console.error('SSR check failed to run:', e.message)
    console.error('--- server logs ---')
    console.error(serverLogs.join(''))
    exitCode = 2
  } finally {
    server.kill('SIGTERM')
    stub.close()
  }

  process.exit(exitCode)
}

main()
