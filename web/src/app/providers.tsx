'use client'

import { useEffect } from 'react'
import ToastProvider from '../components/ToastProvider'

// Kept for backwards compatibility — always returns light
export function useTheme() {
  return { theme: 'light' as const, toggleTheme: () => {} }
}

export default function Providers({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) return

    // Decode token to check expiry
    let expiresAt = 0
    try {
      const payload = JSON.parse(atob(token.split('.')[1]))
      expiresAt = (payload.exp || 0) * 1000
      if (expiresAt < Date.now()) return
    } catch {
      return
    }

    // Close SSE 30 seconds before token expires to avoid 401
    const timeUntilExpiry = expiresAt - Date.now() - 30000
    if (timeUntilExpiry < 10000) return

    let es: EventSource | null = null
    let reconnectAttempts = 0
    const maxReconnectAttempts = 5

    // Fire a window event whenever a notification-producing SSE event
    // arrives. The Nav badge + anything else that cares about "new
    // notification" can listen without each subscriber re-opening the
    // EventSource (browsers cap concurrent SSE streams per origin).
    const emit = (kind: string, ev: MessageEvent) => {
      let data: unknown = undefined
      try { data = ev.data ? JSON.parse(ev.data) : undefined } catch {}
      window.dispatchEvent(
        new CustomEvent('loomfeed:notification', { detail: { kind, data } })
      )
    }

    function connect() {
      // Same-origin EventSource attaches the lf_access cookie automatically.
      // Backend (handlers/events.go) reads it as the preferred auth source.
      // The ?token= URL form was removed because it leaks the JWT into
      // Referer headers, browser history, and any intermediary access log.
      es = new EventSource('/api/v1/events/stream')
      es.addEventListener('comment.created', (ev) => emit('comment.created', ev as MessageEvent))
      es.addEventListener('mention', (ev) => emit('mention', ev as MessageEvent))
      es.addEventListener('vote.received', (ev) => emit('vote.received', ev as MessageEvent))
      es.addEventListener('connected', () => { reconnectAttempts = 0 })
      es.onerror = () => {
        es?.close()
        // Exponential backoff reconnection (1s, 2s, 4s, 8s, 16s then stop)
        if (reconnectAttempts < maxReconnectAttempts) {
          const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), 16000)
          reconnectAttempts++
          setTimeout(connect, delay)
        }
      }
    }

    connect()

    const timer = setTimeout(() => { es?.close() }, timeUntilExpiry)

    return () => {
      clearTimeout(timer)
      reconnectAttempts = maxReconnectAttempts // prevent reconnection on cleanup
      es?.close()
    }
  }, [])

  return (
    <ToastProvider>
      {children}
    </ToastProvider>
  )
}
