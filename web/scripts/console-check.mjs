// Capture console errors + pageerrors on the home page, anon and authed.
// Companion to ui-shots.mjs: shots catch layout regressions, this catches
// runtime ones (hydration mismatches like React #418 only fire for authed
// sessions, which a human spot-check in a logged-out browser never sees).
//
// Usage:
//   echo "<jwt>" > /tmp/smoke-token; echo "<user-id>" > /tmp/smoke-uid
//   LD_LIBRARY_PATH=/tmp/pw-libs/usr/lib/x86_64-linux-gnu \
//     CHECK_URL=http://localhost:3471 node scripts/console-check.mjs
// CHECK_URL defaults to prod (https://www.loomfeed.com).
import { chromium } from 'playwright'
import { readFileSync } from 'node:fs'

const BASE = process.env.CHECK_URL || 'https://www.loomfeed.com'
const token = readFileSync('/tmp/smoke-token', 'utf8').trim()
const uid = readFileSync('/tmp/smoke-uid', 'utf8').trim()

const browser = await chromium.launch()

async function run(label, authed) {
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } })
  if (authed) {
    await ctx.addInitScript(([t, u]) => {
      localStorage.setItem('token', t)
      localStorage.setItem('userId', u)
      localStorage.setItem('onboarding_complete', '1')
    }, [token, uid])
  }
  const page = await ctx.newPage()
  const errors = []
  page.on('console', m => { if (m.type() === 'error') errors.push(m.text().slice(0, 300)) })
  page.on('pageerror', e => errors.push('PAGEERROR: ' + String(e).slice(0, 300)))
  await page.goto(BASE + '/', { waitUntil: 'networkidle', timeout: 45000 }).catch(e => errors.push('NAV: ' + e.message))
  await page.waitForTimeout(3000)
  console.log(`\n=== ${label}: ${errors.length} console error(s) ===`)
  for (const e of errors) console.log('  -', e)
  await ctx.close()
}

await run('ANON', false)
await run('AUTHED', true)
await browser.close()
