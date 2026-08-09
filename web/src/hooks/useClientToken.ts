'use client'

import { useEffect, useState } from 'react'

/**
 * Returns the auth token from localStorage, deferred to client-only.
 *
 * Background: `localStorage` doesn't exist in Node, so reading it at
 * the top of a component's render body (`const t = localStorage.getItem('token')`)
 * crashes SSR. Most loomfeed pages that need a token to load also need
 * to render server-side so crawlers see the page chrome — this hook
 * defers the read to a useEffect tick so the first render returns
 * null safely.
 *
 * Returns:
 *   - undefined on the very first render (server + first client paint
 *     before useEffect runs) — distinguishes "we haven't checked yet"
 *     from "checked, no token". Lets pages render a skeleton state
 *     instead of an early redirect-to-login on SSR.
 *   - string when a token is present
 *   - null when checked + nothing stored
 */
export function useClientToken(): string | null | undefined {
  const [token, setToken] = useState<string | null | undefined>(undefined)
  useEffect(() => {
    try {
      setToken(localStorage.getItem('token'))
    } catch {
      setToken(null)
    }
  }, [])
  return token
}

/**
 * Same shape as useClientToken but for the userId key.
 */
export function useClientUserId(): string | null | undefined {
  const [uid, setUid] = useState<string | null | undefined>(undefined)
  useEffect(() => {
    try {
      setUid(localStorage.getItem('userId'))
    } catch {
      setUid(null)
    }
  }, [])
  return uid
}
