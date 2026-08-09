import type { MetadataRoute } from 'next'

// Single source of truth for robots.txt. Built to be valid whether or
// not Cloudflare's "Managed robots.txt / AI Content Signals" feature
// is on — but the user should turn that feature OFF, because it
// injects a second `User-agent: *` block and Google Search Console
// flags the resulting file as malformed (one UA should have exactly
// one group).
//
// Each section here mirrors what Cloudflare was adding, so disabling
// Cloudflare's injection doesn't lose any protection.
//
// Allow-everything groups are only written for crawlers we actively
// want (search + social preview bots). Everything else is covered by
// the default `*` block at the bottom.

export default function robots(): MetadataRoute.Robots {
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

  // Private / per-user routes — no SEO value, shouldn't be indexed.
  const privatePaths = [
    '/api/',
    '/settings',
    '/bookmarks',
    '/messages',
    '/notifications',
    '/webhooks',
    '/my-agents',
    '/my-communities',
    '/verify-email',
    '/forgot-password',
    '/embed/', // per-post embed widgets; noindex'd in meta anyway
  ]

  // AI training crawlers we block. Same set Cloudflare was injecting;
  // users who come through search (Google, Bing, DDG) are fine, but
  // LLM scrapers harvesting content for training get denied.
  const aiTrainingBots = [
    'Amazonbot',
    'Applebot-Extended',
    'Bytespider',
    'CCBot',
    'ClaudeBot',
    'Claude-Web',
    'cohere-ai',
    'Diffbot',
    'FacebookBot',
    'Google-Extended',
    'GPTBot',
    'ImagesiftBot',
    'meta-externalagent',
    'Omgilibot',
    'PerplexityBot',
    'Timpibot',
  ]

  return {
    rules: [
      // Block AI-training crawlers at the top so their specific
      // groups take precedence over the `*` allow-all below.
      ...aiTrainingBots.map((ua) => ({
        userAgent: ua,
        disallow: '/',
      })),
      // Default group for everyone else — Google, Bing, DDG, Yandex,
      // Baidu, social-preview bots, archive.org, etc.
      {
        userAgent: '*',
        allow: '/',
        disallow: privatePaths,
      },
    ],
    sitemap: `${siteUrl}/sitemap.xml`,
  }
}
