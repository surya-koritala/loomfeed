// web/src/components/lf/LFEmbeddedArticle.tsx
'use client'

import React from 'react'
import { safeHref } from '../../lib/safe-url'

// LFEmbeddedArticle — typed source card rendered inside LFPostCard
// when a post links to a news article / blog / arxiv paper /
// generic-web URL. Replaces the bare cover-image render with a
// branded card that knows the source.
//
// Design intent (see /Downloads/loomfeed-content-cards.html):
// the OUTER post card is the contributor's take (their title +
// body excerpt + action row). The INNER article card is what they
// linked to. Two visible layers, not one collapsed shape.
//
// Visual: cream-tinted card with hard ink border + 3px offset
// shadow. DM Serif Display for the article headline (different
// from the contributor's DM Sans title — gives the source line a
// different voice). Domain + reading-time strip on top.
// Right-side thumbnail at fixed 132px wide.
//
// Data shape mirrors what the backend writes into
// post.metadata.body_link_previews[url] at post-create time:
//   { title, description, image, domain, url }
//
// On click: opens the article in a new tab (target="_blank") with
// noopener/noreferrer. The outer LFPostCard's title still links
// to the in-platform /post/{id} for the discussion.

export interface LFEmbeddedArticleProps {
  url: string
  title?: string
  description?: string
  image?: string
  domain?: string
  /** Optional explicit reading-time string ("5 min read"). Caller
   *  passes whatever the scrape produced; we render as-is. */
  readingTime?: string
  /** When true, render without the offset shadow — for use inside
   *  dense lists where stacked shadows look busy. */
  flat?: boolean
}

// proxiedImage mirrors the helper in LFPostCard. External CDN
// images route through our caching proxy so subsequent fetches
// hit Cloudflare's edge instead of the origin.
function proxiedImage(rawUrl: string): string {
  if (!rawUrl) return rawUrl
  if (rawUrl.startsWith('/') || rawUrl.startsWith('data:') || rawUrl.startsWith('blob:')) {
    return rawUrl
  }
  let host = ''
  try { host = new URL(rawUrl).hostname } catch { return rawUrl }
  if (host === 'www.loomfeed.com' || host === 'loomfeed.com' || host.endsWith('.loomfeed.com')) {
    return rawUrl
  }
  return `/api/v1/img?url=${encodeURIComponent(rawUrl)}`
}

// Domain shown in the source bar. Strips www. so cards read
// "theguardian.com" not "www.theguardian.com" — matches how
// publishers print their own URLs.
function displayDomain(domain: string | undefined, fallbackUrl: string): string {
  if (domain) return domain.replace(/^www\./, '')
  try {
    return new URL(fallbackUrl).hostname.replace(/^www\./, '')
  } catch {
    return ''
  }
}

export function LFEmbeddedArticle({
  url,
  title,
  description,
  image,
  domain,
  readingTime,
  flat,
}: LFEmbeddedArticleProps) {
  const host = displayDomain(domain, url)
  const showThumb = Boolean(image)

  return (
    <a
      href={safeHref(url)}
      target="_blank"
      rel="noopener noreferrer"
      onClick={(e) => e.stopPropagation()}
      className="lf-embedded-article"
      data-flat={flat ? 'true' : undefined}
      aria-label={title ? `Read on ${host}: ${title}` : `Read on ${host}`}
    >
      <div className="ea-text">
        <div className="ea-source-bar">
          <span className="ea-domain">{host}</span>
          {readingTime && (
            <>
              <span className="ea-dot" aria-hidden />
              <span className="ea-reading">{readingTime}</span>
            </>
          )}
          <span className="ea-arrow" aria-hidden>↗</span>
        </div>
        {title && <h3 className="ea-headline">{title}</h3>}
        {description && <p className="ea-excerpt">{description}</p>}
      </div>
      {showThumb && (

        <img
          src={proxiedImage(image!)}
          alt=""
          loading="lazy"
          decoding="async"
          className="ea-thumb"
          onError={(e) => {
            // If the thumbnail 404s (publisher hotlink, expired
            // CDN, etc.) collapse the column — the card still
            // reads with text only.
            const img = e.currentTarget as HTMLImageElement
            img.style.display = 'none'
          }}
        />
      )}
    </a>
  )
}
