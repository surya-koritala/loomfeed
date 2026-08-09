'use client'

import { useEffect, useState } from 'react'
import { api } from '../api/client'

/**
 * Browser push-notification opt-in.
 *
 * States:
 *   - not supported: browser lacks Notification / PushManager / SW.
 *   - disabled by server: VAPID keys not configured → no-op button.
 *   - permission default: "Enable push" button triggers the flow.
 *   - permission granted + subscribed: "On — click to disable" button.
 *   - permission denied: instructions to unblock in browser settings.
 */

function urlB64ToBuffer(base64String: string): ArrayBuffer {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4)
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(base64)
  const buf = new ArrayBuffer(raw.length)
  const view = new Uint8Array(buf)
  for (let i = 0; i < raw.length; i++) view[i] = raw.charCodeAt(i)
  return buf
}

export default function PushToggle() {
  const [supported, setSupported] = useState<boolean | null>(null)
  const [serverEnabled, setServerEnabled] = useState(false)
  const [publicKey, setPublicKey] = useState('')
  const [subscribed, setSubscribed] = useState(false)
  const [permission, setPermission] = useState<NotificationPermission>('default')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    // Feature detection
    const ok =
      typeof window !== 'undefined' &&
      'serviceWorker' in navigator &&
      'PushManager' in window &&
      'Notification' in window
    setSupported(ok)
    if (!ok) return

    setPermission(Notification.permission)

    // Fetch server key + enabled flag
    api
      .getPushPublicKey()
      .then((d: any) => {
        setServerEnabled(!!d?.enabled)
        setPublicKey(d?.public_key || '')
      })
      .catch(() => setServerEnabled(false))

    // See if we already have an active subscription
    navigator.serviceWorker.ready
      .then((reg) => reg.pushManager.getSubscription())
      .then((sub) => setSubscribed(!!sub))
      .catch(() => setSubscribed(false))
  }, [])

  const subscribe = async () => {
    if (!publicKey) return
    setBusy(true)
    setError(null)
    try {
      const perm = await Notification.requestPermission()
      setPermission(perm)
      if (perm !== 'granted') return
      const reg = await navigator.serviceWorker.ready
      const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlB64ToBuffer(publicKey),
      })
      const json = sub.toJSON() as { endpoint: string; keys: { p256dh: string; auth: string } }
      await api.pushSubscribe({
        endpoint: json.endpoint,
        keys: { p256dh: json.keys.p256dh, auth: json.keys.auth },
      })
      setSubscribed(true)
    } catch (e: any) {
      setError(e?.message ?? 'Failed to enable push notifications')
    } finally {
      setBusy(false)
    }
  }

  const unsubscribe = async () => {
    setBusy(true)
    setError(null)
    try {
      const reg = await navigator.serviceWorker.ready
      const sub = await reg.pushManager.getSubscription()
      if (sub) {
        await sub.unsubscribe()
        await api.pushUnsubscribe(sub.endpoint).catch(() => {})
      }
      setSubscribed(false)
    } catch (e: any) {
      setError(e?.message ?? 'Failed to disable push notifications')
    } finally {
      setBusy(false)
    }
  }

  if (supported === null) return null // loading

  const labelStyle: React.CSSProperties = {
    fontFamily: 'var(--lf-font-mono)',
    fontSize: 10,
    letterSpacing: '0.12em',
    textTransform: 'uppercase',
  }

  const button = (label: string, action: () => void, variant: 'on' | 'off') => (
    <button
      onClick={action}
      disabled={busy}
      style={{
        padding: '6px 12px',
        background: variant === 'on' ? 'var(--lf-ink)' : 'var(--lf-paper)',
        color: variant === 'on' ? 'var(--lf-paper)' : 'var(--lf-ink)',
        border: '1px solid var(--lf-ink)',
        cursor: busy ? 'wait' : 'pointer',
        opacity: busy ? 0.6 : 1,
        ...labelStyle,
      }}
    >
      {label}
    </button>
  )

  return (
    <div className="flex items-start justify-between gap-16 py-2">
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium" style={{ color: 'var(--lf-ink)' }}>
          Browser push
        </span>
        <span className="text-xs" style={{ color: 'var(--lf-muted)' }}>
          {!supported
            ? "Your browser doesn't support web push. Try Chrome, Firefox, or Edge."
            : !serverEnabled
            ? 'Push isn\u2019t configured on this server yet — the toggle is inert.'
            : permission === 'denied'
            ? 'Blocked in this browser. Unblock notifications in your site settings, then try again.'
            : subscribed
            ? 'You\u2019ll see a notification in this browser when someone mentions or replies to you.'
            : 'Get a silent tap on this browser when someone mentions or replies to you.'}
        </span>
        {error && (
          <span className="text-xs" style={{ color: 'var(--lf-rose)', marginTop: 4 }}>
            {error}
          </span>
        )}
      </div>
      <div style={{ flexShrink: 0 }}>
        {!supported || !serverEnabled || permission === 'denied' ? (
          <span style={{ ...labelStyle, color: 'var(--lf-muted)' }}>Unavailable</span>
        ) : subscribed ? (
          button('On — click to disable', unsubscribe, 'on')
        ) : (
          button('Enable push', subscribe, 'off')
        )}
      </div>
    </div>
  )
}
