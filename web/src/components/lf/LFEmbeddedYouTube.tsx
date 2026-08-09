// web/src/components/lf/LFEmbeddedYouTube.tsx
'use client'

import React from 'react'
import { safeHref } from '../../lib/safe-url'

// LFEmbeddedYouTube — typed source card for YouTube / Vimeo / generic
// video URLs. Sibling to LFEmbeddedArticle; rendered inside
// LFPostCard when the contributor links to a video host.
//
// Visual: full-width 16:9 hero thumbnail with a centered play
// button (ink circle, lime triangle — loomfeed brand, NOT
// YouTube red), a corner badge identifying the source, and a
// duration overlay when known. Title + channel below.
//
// Data is read from post.metadata.body_link_previews — same
// shape extractPrimaryEmbed in LFPostCard reads for the article
// variant. Duration and view count aren't in the preview scrape
// (would need the YouTube Data API); the card hides those rows
// when absent and still reads cleanly with title alone.
//
// Click opens the video on YouTube in a new tab. A future v2
// can swap the thumb for an inline iframe player on click.

export interface LFEmbeddedYouTubeProps {
  url: string
  title?: string
  description?: string
  /** og:image from the scrape. When absent we derive the
   *  YouTube watch-thumbnail URL from the videoId. */
  image?: string
  /** Channel / author name. Optional — most scrapes don't return
   *  it. When absent the source label reads just "YouTube". */
  channel?: string
  /** Pre-formatted duration ("3:21:47", "12:04"). Hidden when
   *  absent — most scrapes won't have this. */
  duration?: string
  /** Display "youtube.com" / "youtu.be" etc. on the corner. */
  source?: string
  flat?: boolean
}

// Strips common URL forms to a YouTube videoId. Returns null when
// the URL isn't a YouTube host or the ID can't be parsed.
//
// Handles: youtube.com/watch?v=ID, youtu.be/ID, youtube.com/embed/ID,
// youtube.com/shorts/ID, youtube.com/live/ID, m.youtube.com/*.
// Mirrors the existing logic in LFPostCard.thumbFromVideoUrl so
// posts that already extract a thumb the old way stay consistent.
export function youtubeIdFromUrl(url: string): string | null {
  try {
    const u = new URL(url)
    const host = u.hostname.replace(/^www\./, '').replace(/^m\./, '')
    if (host !== 'youtube.com' && host !== 'youtu.be') return null
    let id = ''
    if (host === 'youtu.be') id = u.pathname.slice(1)
    else if (u.pathname.startsWith('/embed/')) id = u.pathname.slice('/embed/'.length)
    else if (u.pathname.startsWith('/shorts/')) id = u.pathname.slice('/shorts/'.length)
    else if (u.pathname.startsWith('/live/')) id = u.pathname.slice('/live/'.length)
    else id = u.searchParams.get('v') ?? ''
    id = id.split('/')[0].split('?')[0]
    return id || null
  } catch {
    return null
  }
}

function proxiedImage(rawUrl: string): string {
  if (!rawUrl) return rawUrl
  if (rawUrl.startsWith('/') || rawUrl.startsWith('data:') || rawUrl.startsWith('blob:')) return rawUrl
  let host = ''
  try { host = new URL(rawUrl).hostname } catch { return rawUrl }
  if (host === 'www.loomfeed.com' || host === 'loomfeed.com' || host.endsWith('.loomfeed.com')) {
    return rawUrl
  }
  return `/api/v1/img?url=${encodeURIComponent(rawUrl)}`
}

export function LFEmbeddedYouTube({
  url,
  title,
  image,
  channel,
  duration,
  source,
  flat,
}: LFEmbeddedYouTubeProps) {
  // Prefer the scrape's image, fall back to deriving from videoId.
  // youtube.com/vi/<id>/maxresdefault.jpg is the 1280x720 HD frame;
  // older videos that lack a maxres frame return 404, and onError
  // falls back to hqdefault.jpg (always present).
  const videoId = youtubeIdFromUrl(url)
  const thumb = image
    || (videoId ? `https://img.youtube.com/vi/${videoId}/maxresdefault.jpg` : '')
  const corner = source || 'YouTube'

  return (
    <a
      href={safeHref(url)}
      target="_blank"
      rel="noopener noreferrer"
      onClick={(e) => e.stopPropagation()}
      className="lf-embedded-yt"
      data-flat={flat ? 'true' : undefined}
      aria-label={title ? `Watch on ${corner}: ${title}` : `Watch on ${corner}`}
    >
      <div className="yt-thumb-area">
        <span className="yt-corner" aria-hidden>{corner}</span>
        {thumb && (

          <img
            src={proxiedImage(thumb)}
            alt=""
            loading="lazy"
            decoding="async"
            className="yt-thumb-img"
            onError={(e) => {
              const img = e.currentTarget as HTMLImageElement
              const src = img.src
              if (src.includes('maxresdefault.jpg')) {
                img.src = src.replace('maxresdefault.jpg', 'hqdefault.jpg')
                return
              }
              // hqdefault failed too — hide the broken image so
              // the play button still anchors the card.
              img.style.display = 'none'
            }}
          />
        )}
        <span className="yt-play" aria-hidden>
          <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor">
            <path d="M8 5v14l11-7z" />
          </svg>
        </span>
        {duration && <span className="yt-duration" aria-hidden>{duration}</span>}
      </div>
      {(title || channel) && (
        <div className="yt-info">
          {title && <h3 className="yt-title">{title}</h3>}
          {channel && (
            <div className="yt-channel-row">
              <span className="yt-ch">{channel}</span>
            </div>
          )}
        </div>
      )}
    </a>
  )
}
