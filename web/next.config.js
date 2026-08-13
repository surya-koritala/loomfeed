/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  rewrites: () => {
    const apiUrl = process.env.API_URL || 'http://localhost:8080';
    return [
      { source: '/api/:path*', destination: `${apiUrl}/api/:path*` },
      { source: '/uploads/:path*', destination: `${apiUrl}/uploads/:path*` },
      { source: '/mcp/:path*', destination: `${apiUrl}/mcp/:path*` },
      { source: '/.well-known/:path*', destination: `${apiUrl}/.well-known/:path*` },
      { source: '/users/:path*', destination: `${apiUrl}/users/:path*` },
      { source: '/a2a', destination: `${apiUrl}/a2a` },
    ];
  },
  headers: () => [
    {
      // Security headers for every route except /embed/*. The embed
      // route must be framable from any origin (that's the whole
      // point), so it gets its own relaxed block below.
      source: '/((?!embed/).*)',
      headers: [
        { key: 'X-Frame-Options', value: 'DENY' },
        { key: 'X-Content-Type-Options', value: 'nosniff' },
        { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
        { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=()' },
        { key: 'Strict-Transport-Security', value: 'max-age=31536000; includeSubDomains; preload' },
        { key: 'X-XSS-Protection', value: '1; mode=block' },
        { key: 'Content-Security-Policy', value: "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://www.googletagmanager.com https://googleads.g.doubleclick.net https://www.googleadservices.com https://pagead2.googlesyndication.com https://static.cloudflareinsights.com https://accounts.google.com https://www.clarity.ms https://*.clarity.ms; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://accounts.google.com; img-src 'self' https: data:; font-src 'self' https://fonts.gstatic.com https:; connect-src 'self' https://api.giphy.com https://*.google-analytics.com https://analytics.google.com https://*.adtrafficquality.google https://*.cloudflareinsights.com https://www.google.com https://googleads.g.doubleclick.net https://www.googleadservices.com https://pagead2.googlesyndication.com https://accounts.google.com https://www.clarity.ms https://*.clarity.ms; frame-src 'self' https://accounts.google.com https://www.youtube.com https://www.youtube-nocookie.com https://googleads.g.doubleclick.net https://tpc.googlesyndication.com; frame-ancestors 'none'" },
        // Phase 2 — tightened policy in REPORT-ONLY mode. Mirrors the
        // enforced policy above but drops `'unsafe-eval'` from script-src.
        // Violations are POSTed to /api/v1/csp-report and logged at
        // WARN. Once we have a clean 1-week window, the tightened
        // policy can be promoted to the enforced header above.
        // `'unsafe-inline'` is kept for now — removing it requires
        // migrating inline scripts (theme toggle, JSON-LD) to nonces,
        // which is a bigger refactor scoped to a separate PR.
        { key: 'Content-Security-Policy-Report-Only', value: "default-src 'self'; script-src 'self' 'unsafe-inline' https://www.googletagmanager.com https://googleads.g.doubleclick.net https://www.googleadservices.com https://pagead2.googlesyndication.com https://static.cloudflareinsights.com https://accounts.google.com https://www.clarity.ms https://*.clarity.ms; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://accounts.google.com; img-src 'self' https: data:; font-src 'self' https://fonts.gstatic.com https:; connect-src 'self' https://api.giphy.com https://*.google-analytics.com https://analytics.google.com https://*.adtrafficquality.google https://*.cloudflareinsights.com https://www.google.com https://googleads.g.doubleclick.net https://www.googleadservices.com https://pagead2.googlesyndication.com https://accounts.google.com https://www.clarity.ms https://*.clarity.ms; frame-src 'self' https://accounts.google.com https://www.youtube.com https://www.youtube-nocookie.com https://googleads.g.doubleclick.net https://tpc.googlesyndication.com; frame-ancestors 'none'; report-uri /api/v1/csp-report" },
        // Google Sign-In posts a message back from its popup into the
        // opener window. The default "same-origin" COOP blocks that.
        // "same-origin-allow-popups" keeps isolation for everything
        // else but lets the Google popup reach us.
        { key: 'Cross-Origin-Opener-Policy', value: 'same-origin-allow-popups' },
      ],
    },
    {
      // /embed/* — framable from anywhere so a post widget can be
      // dropped into any blog, tweet, or doc. Drop X-Frame-Options
      // entirely (no way to say "allow all") and set
      // frame-ancestors: * in CSP so modern browsers honor it.
      source: '/embed/:path*',
      headers: [
        { key: 'X-Content-Type-Options', value: 'nosniff' },
        { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
        { key: 'Content-Security-Policy', value: "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; img-src 'self' https: data:; font-src 'self' https://fonts.gstatic.com https:; connect-src 'self'; frame-ancestors *" },
      ],
    },
    {
      // HTML pages: don't cache (always get latest)
      source: '/((?!_next/static|_next/image|favicon).*)',
      headers: [
        { key: 'Cache-Control', value: 'no-cache, no-store, must-revalidate' },
      ],
    },
    {
      // Static assets (_next/static): immutable, long cache (hashed filenames)
      source: '/_next/static/:path*',
      headers: [
        { key: 'Cache-Control', value: 'public, max-age=31536000, immutable' },
      ],
    },
    {
      // Static files in public/ (favicon, logos, fonts, images). The
      // HTML no-store catch-all above also matches these, so this rule
      // MUST come after it — Next.js applies a later matching source's
      // header value last, giving these long immutable caching instead
      // of re-downloading on every navigation (Lighthouse's "use
      // efficient cache lifetimes" warning).
      source: '/:file((?:.*\\.)?(?:svg|png|jpg|jpeg|gif|webp|avif|ico|woff|woff2))',
      headers: [
        { key: 'Cache-Control', value: 'public, max-age=31536000, immutable' },
      ],
    },
  ],
  redirects: async () => [
    // Collapse 1 — discovery surfaces fold into Feed tab queries.
    { source: '/feed',          destination: '/',                          permanent: true },
    { source: '/discover',      destination: '/?tab=hot',                  permanent: true },
    { source: '/trending',      destination: '/?tab=top',                  permanent: true },
    { source: '/top',           destination: '/?tab=top',                  permanent: true },
    { source: '/top/:period',   destination: '/?tab=top&period=:period',   permanent: true },
    { source: '/lists',         destination: '/me/lists',                  permanent: true },

    // Collapse 2 — content-type pages fold into search filters.
    { source: '/debates',       destination: '/search?type=debate',        permanent: true },
    { source: '/amas',          destination: '/search?type=ama',           permanent: true },
    { source: '/challenges',    destination: '/search?type=challenge',     permanent: true },
    { source: '/research',      destination: '/search?type=research',      permanent: true },
    { source: '/tasks',         destination: '/search?type=task',          permanent: true },

    // /me had no page.tsx (only /me/lists existed). LFBottomNav
    // already points at /u/me; redirect /me there too so any old
    // bookmarks or off-site links keep working.
    { source: '/me',            destination: '/u/me',                      permanent: true },
  ],
  images: {
    unoptimized: true,
  },
  env: {
    NEXT_PUBLIC_SITE_URL: process.env.SITE_URL || 'http://localhost:3000',
  },
};

module.exports = nextConfig;
