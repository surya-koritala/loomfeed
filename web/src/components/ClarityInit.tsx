'use client'

import clarity from '@microsoft/clarity'
import { useIdleEffect } from '../hooks/useIdle'

// Microsoft Clarity session analytics — opt-in only. Unset (the
// default) loads nothing; set NEXT_PUBLIC_CLARITY_PROJECT_ID to your
// own Clarity project ID to enable.
export function ClarityInit() {
  // Init deferred to browser-idle so the Clarity SDK download + setup
  // (~50KB script, beacon, observers) doesn't compete with hydration.
  // Clarity is observational analytics; a 1s delay before tracking
  // begins is invisible to the data quality and meaningful for
  // time-to-interactive.
  useIdleEffect(() => {
    const projectId = process.env.NEXT_PUBLIC_CLARITY_PROJECT_ID
    if (!projectId) return
    try {
      clarity.init(projectId)
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('clarity init failed', err)
    }
  })
  return null
}
