'use client'

import { useEffect, useRef } from 'react'

declare global {
  interface Window {
    google?: {
      accounts: {
        id: {
          initialize: (config: any) => void
          renderButton: (element: HTMLElement, config: any) => void
        }
      }
    }
  }
}

// Lazy-inject the Google Identity Services script only when this
// component actually mounts. Previously the script loaded globally
// from layout.tsx on every page (~81 KB against home-page LCP, wasted
// everywhere except /login and /register where sign-in exists).
function loadGoogleScript(): Promise<void> {
  if (typeof window === 'undefined') return Promise.resolve()
  if (window.google) return Promise.resolve()
  const existing = document.querySelector(
    'script[src="https://accounts.google.com/gsi/client"]',
  ) as HTMLScriptElement | null
  if (existing) {
    return new Promise((resolve) => {
      if (window.google) resolve()
      else existing.addEventListener('load', () => resolve(), { once: true })
    })
  }
  return new Promise((resolve) => {
    const s = document.createElement('script')
    s.src = 'https://accounts.google.com/gsi/client'
    s.async = true
    s.defer = true
    s.onload = () => resolve()
    document.head.appendChild(s)
  })
}

interface GoogleSignInButtonProps {
  clientId: string
  onCredential: (credential: string) => void
}

export default function GoogleSignInButton({ clientId, onCredential }: GoogleSignInButtonProps) {
  const buttonRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!clientId) return
    let cancelled = false

    const renderButton = () => {
      if (cancelled || !window.google || !buttonRef.current) return
      window.google.accounts.id.initialize({
        client_id: clientId,
        callback: (response: { credential: string }) => {
          onCredential(response.credential)
        },
      })
      // Clamp to the container so the button never overflows a narrow
      // auth card on mobile (the GIS minimum is ~200px). Re-rendered on
      // resize below so it tracks orientation/viewport changes.
      const avail = buttonRef.current.clientWidth || 360
      window.google.accounts.id.renderButton(buttonRef.current, {
        theme: 'outline',
        size: 'large',
        width: Math.max(200, Math.min(avail, 360)),
        text: 'signin_with',
      })
    }

    loadGoogleScript().then(renderButton)

    let resizeTimer: ReturnType<typeof setTimeout> | undefined
    const onResize = () => {
      clearTimeout(resizeTimer)
      resizeTimer = setTimeout(renderButton, 150)
    }
    window.addEventListener('resize', onResize)

    return () => {
      cancelled = true
      clearTimeout(resizeTimer)
      window.removeEventListener('resize', onResize)
    }
  }, [clientId, onCredential])

  if (!clientId) return null

  return <div ref={buttonRef} style={{ display: 'flex', justifyContent: 'center' }} />
}
