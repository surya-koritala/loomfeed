// Loomfeed service worker.
//
// Goals (in order):
//  1. Installable PWA (enables "Add to Home Screen" + standalone mode).
//  2. Offline-safe shell: GET requests for navigations fall back to a
//     tiny cached page when the network is down. API calls pass through
//     — stale feed data is worse than a clear error.
//  3. Push notifications: receive + display, and route the click back
//     into the app. Runs even when no tab is open.
//
// Bump CACHE_VERSION whenever the offline shell changes; the activate
// event purges every older cache so clients converge without needing
// the user to hard-refresh.

const CACHE_VERSION = 'loomfeed-v2'
const OFFLINE_URL = '/offline.html'
const PRECACHE_URLS = [
  OFFLINE_URL,
  '/favicon.svg',
]

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_VERSION).then((cache) => cache.addAll(PRECACHE_URLS)).then(() => self.skipWaiting()),
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE_VERSION).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  )
})

// Navigation fallback: if the user is offline and requests an HTML page,
// serve the minimal offline shell. API and static asset GETs pass
// through so normal error handling applies.
self.addEventListener('fetch', (event) => {
  const req = event.request
  if (req.method !== 'GET') return

  const accept = req.headers.get('accept') || ''
  const isNavigation = req.mode === 'navigate' || accept.includes('text/html')
  if (!isNavigation) return

  event.respondWith(
    fetch(req).catch(() => caches.match(OFFLINE_URL).then((hit) => hit || new Response('Offline', { status: 503 }))),
  )
})

// Push: message shape we send from the server is JSON:
//   { title, body, url?, tag? }
// If the payload fails to parse we still notify so the event isn't
// dropped silently.
self.addEventListener('push', (event) => {
  let payload = { title: 'Loomfeed', body: 'You have a new notification.' }
  try {
    if (event.data) payload = { ...payload, ...event.data.json() }
  } catch (_) {
    // Fall back to defaults; don't throw — throwing kills the event.
  }
  const { title, body, url, tag } = payload
  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      icon: '/favicon.svg',
      badge: '/favicon.svg',
      tag: tag || 'loomfeed',
      data: { url: url || '/notifications' },
    }),
  )
})

// Clicking a notification: focus an existing tab if one is already on
// the URL; otherwise open a new one.
self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const targetUrl = (event.notification.data && event.notification.data.url) || '/notifications'
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((wins) => {
      for (const w of wins) {
        try {
          const u = new URL(w.url)
          if (u.pathname + u.search === targetUrl) {
            return w.focus()
          }
        } catch (_) {
          // skip malformed
        }
      }
      return self.clients.openWindow(targetUrl)
    }),
  )
})
