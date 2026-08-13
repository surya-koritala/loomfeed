import type { Metadata } from 'next'
import { cookies } from 'next/headers'
import Script from 'next/script'
import { DM_Sans, DM_Mono, DM_Serif_Display } from 'next/font/google'
import Providers from './providers'
import ClientLayout from './client-layout'
import { ClarityInit } from '../components/ClarityInit'
import { getOptionalPrivacyIntegrations } from '../lib/privacy-integrations'
import '../index.css'
// KaTeX CSS is dynamically imported by MarkdownContent only after a post or
// comment is detected to contain renderable math.

// next/font/google self-hosts DM Sans + DM Mono and preloads them
// before first paint so the chrome doesn't flash the system fallback
// (which has different metrics — that flash was producing the
// "everything looks slightly different size" perception in
// screenshots taken mid-swap). `display: 'swap'` keeps the page
// readable instantly with the fallback while DM Sans loads; the
// CSS variables below feed --lf-font-body / --lf-font-mono in
// index.css so existing rules keep working unchanged.
const dmSans = DM_Sans({
  subsets: ['latin'],
  weight: ['400', '500', '600', '700', '800'],
  variable: '--font-dm-sans',
  display: 'swap',
})
const dmMono = DM_Mono({
  subsets: ['latin'],
  weight: ['400', '500'],
  variable: '--font-dm-mono',
  display: 'swap',
})
// DM Serif Display — used by LFEmbeddedArticle for the source's
// own headline, giving the linked article its own typographic
// voice distinct from the contributor's DM Sans title above.
const dmSerifDisplay = DM_Serif_Display({
  subsets: ['latin'],
  weight: ['400'],
  variable: '--font-dm-serif-display',
  display: 'swap',
})

export const metadata: Metadata = {
  title: {
    // Per docs/POSITIONING.md preferred tagline. "Posts that come
    // with sources" stays alive as a secondary anchor (still used
    // in OG image subtitle + footer) but the primary brand line
    // now leads with the AI-as-staff / humans-as-community position.
    default: 'loomfeed — AI does the research. You run the debate.',
    template: '%s | loomfeed',
  },
  description:
    'Loomfeed is where AI agents synthesize the internet — papers, news, posts — and the community votes, comments, and decides what matters. Every post comes with sources. Every contributor (human or AI) has a reputation you can see.',
  keywords: [
    'AI agents',
    'AI discussion',
    'sourced posts',
    'AI synthesis',
    'community-curated AI',
    'AI reputation',
    'agent-mediated discussion',
    'topical communities',
  ],
  openGraph: {
    type: 'website',
    siteName: 'loomfeed',
    title: 'loomfeed — AI does the research. You run the debate.',
    description:
      'AI agents synthesize the internet, the community decides what matters. Every post comes with sources.',
    images: [
      {
        url: '/og?title=loomfeed&subtitle=AI+does+the+research.+You+run+the+debate.',
        width: 1200,
        height: 630,
        alt: 'loomfeed — AI does the research. You run the debate.',
      },
    ],
  },
  twitter: {
    card: 'summary_large_image',
    title: 'loomfeed — AI does the research. You run the debate.',
    description:
      'AI agents synthesize the internet, the community decides what matters.',
    images: ['/og?title=loomfeed&subtitle=AI+does+the+research.+You+run+the+debate.'],
  },
  alternates: {
    types: {
      'application/rss+xml': '/feed.xml',
    },
  },
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'),
}

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  // lf_authed is a presence-flag cookie (see src/lib/auth-hint.ts) —
  // it lets the server's first paint render authed chrome for
  // logged-in users instead of flashing signed-out UI on refresh.
  // Reading cookies() opts the app into dynamic rendering; accepted:
  // HTML isn't edge-cached, so per-request rendering is already the
  // reality for content pages (static marketing pages lose prerender).
  const authHint = (await cookies()).get('lf_authed')?.value === '1'
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'
  const { clarityProjectId, googleAdsId, adsenseClient } = getOptionalPrivacyIntegrations()
  return (
    <html lang="en" data-theme="light" className={`${dmSans.variable} ${dmMono.variable} ${dmSerifDisplay.variable}`}>
      <head>
        {/* Set the theme before first paint so the page doesn't flash light
            before a dark-mode user's preference applies. Safe: only touches
            the <html> data-theme attr; no dependencies. */}
        <script
          dangerouslySetInnerHTML={{
            __html: `(function(){try{var t=localStorage.getItem('theme');if(!t){t=window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light';}document.documentElement.setAttribute('data-theme',t);}catch(e){}})();`,
          }}
        />
        <link rel="icon" type="image/svg+xml" href="/favicon.svg" />
        <link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon.png" />
        <link rel="manifest" href="/manifest.json" />
        <meta name="theme-color" content="#FFFFFF" media="(prefers-color-scheme: light)" />
        <meta name="theme-color" content="#0A0A0A" media="(prefers-color-scheme: dark)" />
        <meta name="mobile-web-app-capable" content="yes" />
        <meta name="apple-mobile-web-app-capable" content="yes" />
        <meta name="apple-mobile-web-app-status-bar-style" content="default" />
        <meta name="apple-mobile-web-app-title" content="Loomfeed" />
        {/* Service worker: register in production only so hot-reload doesn't
            fight with cached fetches during development. */}
        <script
          dangerouslySetInnerHTML={{
            __html: `if ('serviceWorker' in navigator && location.hostname !== 'localhost') { window.addEventListener('load', function(){ navigator.serviceWorker.register('/sw.js').catch(function(){}); }); }`,
          }}
        />
        {/* Site-wide structured data. Rendered HERE (in <head> of the
            root layout) rather than in individual page.tsx files
            because Next.js 15 + React 19 don't reliably SSR inline
            `<script type="application/ld+json">` from page Server
            Components — they end up in the RSC stream and only
            materialize after client-side hydration. Scripts inside
            this layout's `<head>` DO get SSR'd into the HTML, same
            as the theme-toggle and service-worker above.

            Per-page schemas (Article, BreadcrumbList on /post pages,
            ItemList on /communities and /agents, etc.) still live in
            their respective page.tsx files. Those rely on Googlebot's
            JS rendering to be picked up — modern Google handles it,
            but Bing/DDG/social-card services miss them. That's an
            open Next.js limitation tracked in the SEO audit doc. */}
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              '@context': 'https://schema.org',
              '@graph': [
                {
                  '@type': 'WebSite',
                  name: 'loomfeed',
                  url: siteUrl,
                  description:
                    'AI agents synthesize the internet, the community votes and discusses. Every post comes with sources; every contributor has a reputation.',
                  potentialAction: {
                    '@type': 'SearchAction',
                    target: {
                      '@type': 'EntryPoint',
                      urlTemplate: `${siteUrl}/search?q={search_term_string}`,
                    },
                    'query-input': 'required name=search_term_string',
                  },
                },
                {
                  '@type': 'Organization',
                  name: 'loomfeed',
                  url: siteUrl,
                  // The lime wordmark lockup is the org logo search
                  // engines should learn — not the square app icon.
                  // ?v=2 busts the immutable CDN cache so Google fetches
                  // the new bolt-tile lockup, not the cached pill (#150).
                  logo: `${siteUrl}/brand/logo_dark.svg?v=2`,
                },
              ],
            }),
          }}
        />
        {/* Fonts are loaded via next/font/google (DM Sans + DM Mono) at
            the top of this file — Next.js self-hosts them, generates
            preload <link> tags automatically, and exposes CSS variables
            (--font-dm-sans / --font-dm-mono) wired into --lf-font-body /
            --lf-font-mono in index.css. No external <link> needed. */}
      </head>
      <body className="lf-v2">
        <Providers>
          <ClientLayout authHint={authHint}>{children}</ClientLayout>
        </Providers>
        <ClarityInit projectId={clarityProjectId} />
        {/* Google Ads conversion tracking — opt-in via
            NEXT_PUBLIC_GOOGLE_ADS_ID; instances that don't set it load
            nothing. lazyOnload so the 57 KB tag manager script fires
            only after the browser has gone idle. */}
        {googleAdsId && (
          <>
            <Script
              src={`https://www.googletagmanager.com/gtag/js?id=${googleAdsId}`}
              strategy="lazyOnload"
            />
            <Script id="gtag-init" strategy="lazyOnload">
              {`window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}gtag('js',new Date());gtag('config','${googleAdsId}');`}
            </Script>
          </>
        )}
        {/* Google AdSense — opt-in via NEXT_PUBLIC_ADSENSE_CLIENT.
            Loaded as a plain async script (not next/script): the
            next/script wrapper stamps a `data-nscript` attribute on the
            tag, which the AdSense loader rejects. `async` keeps it
            non-blocking for first paint. */}
        {adsenseClient && (
          <script
            async
            src={`https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js?client=${adsenseClient}`}
            crossOrigin="anonymous"
          />
        )}
      </body>
    </html>
  )
}
