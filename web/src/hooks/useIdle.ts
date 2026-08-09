'use client'

import { useEffect } from 'react'

// Defer a callback until the browser is idle — the work won't run
// during the critical hydration path where it would compete with
// React reconciliation + first paint. Falls back to setTimeout when
// requestIdleCallback isn't available (Safari < 16.4).
//
// Used to delay non-critical fetches that the right rail, sidebar
// and home hero LFLiveSignal kick off on mount. Those calls don't
// affect above-the-fold content but were piling up at hydration
// time, slowing time-to-interactive.
//
// Returns nothing; the cleanup cancels the scheduled callback if
// the component unmounts (or deps change) before it fires.

export function useIdleEffect(
  effect: () => void | (() => void),
  deps: React.DependencyList = [],
  /** Hard timeout fallback even if the browser never idles. */
  timeoutMs: number = 1500,
) {
  useEffect(() => {
    if (typeof window === 'undefined') return
    let cleanup: void | (() => void)
    let cancelled = false

    const run = () => {
      if (cancelled) return
      cleanup = effect()
    }

    let idleHandle: number | undefined
    let timeoutHandle: number | undefined

    if (typeof (window as any).requestIdleCallback === 'function') {
      idleHandle = (window as any).requestIdleCallback(run, { timeout: timeoutMs })
    } else {
      // Safari < 16.4 etc. — defer one tick + a short delay.
      timeoutHandle = window.setTimeout(run, 200) as unknown as number
    }

    return () => {
      cancelled = true
      if (idleHandle !== undefined && typeof (window as any).cancelIdleCallback === 'function') {
        ;(window as any).cancelIdleCallback(idleHandle)
      }
      if (timeoutHandle !== undefined) window.clearTimeout(timeoutHandle)
      if (typeof cleanup === 'function') cleanup()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)
}
