// ui-shots.mjs — captures the standard 8 verification screenshots (home+detail ×
// authed/anon × desktop/mobile) against a local stack. Usage:
//   SHOT_TOKEN=<jwt> SHOT_UID=<participant-id> SHOT_DIR=/tmp/out \
//   LD_LIBRARY_PATH=/tmp/pw-libs/usr/lib/x86_64-linux-gnu node scripts/ui-shots.mjs
// (see DESIGN_TOKENS.md / docs verification discipline)
import { chromium } from 'playwright'
const TOKEN = process.env.SHOT_TOKEN, UID = process.env.SHOT_UID
const OUT = process.env.SHOT_DIR || '/tmp/ui-wave1/before'
const browser = await chromium.launch()
async function shoot(authed, name, viewport) {
  const ctx = await browser.newContext({ viewport, reducedMotion: 'no-preference' })
  if (authed) {
    await ctx.addInitScript(([t, u]) => {
      localStorage.setItem('token', t); localStorage.setItem('userId', u)
      localStorage.setItem('onboarding_complete', '1')
    }, [TOKEN, UID])
  } else {
    await ctx.addInitScript(() => localStorage.setItem('onboarding_complete', '1'))
  }
  const page = await ctx.newPage()
  await page.goto('http://localhost:3471/', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(2500)
  await page.screenshot({ path: `${OUT}/home-${name}.png`, fullPage: false })
  const href = await page.locator('a[href^="/post/"]').first().getAttribute('href').catch(() => null)
  if (href) {
    await page.goto('http://localhost:3471' + href, { waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(2500)
    await page.screenshot({ path: `${OUT}/detail-${name}.png`, fullPage: false })
  }
  await ctx.close()
}
await shoot(true,  'authed-desktop', { width: 1280, height: 900 })
await shoot(true,  'authed-mobile',  { width: 390,  height: 844 })
await shoot(false, 'anon-desktop',   { width: 1280, height: 900 })
await shoot(false, 'anon-mobile',    { width: 390,  height: 844 })
await browser.close()
console.log('shots done ->', OUT)
