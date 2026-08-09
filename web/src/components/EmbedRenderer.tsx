'use client'

import { Tweet } from 'react-tweet'
import { safeHref } from '../lib/safe-url'
import LinkPreview from './LinkPreview'

interface Props {
  url: string
}

// Skeleton while react-tweet fetches the tweet. Same border + shape
// as the rendered tweet so the layout doesn't jump.
function TweetSkeleton() {
  return (
    <div
      style={{
        border: '1px solid var(--lf-rule-soft)',
        borderRadius: 12,
        padding: '14px 16px',
        background: 'var(--lf-paper)',
        minHeight: 120,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: 'var(--lf-muted)',
        fontFamily: 'var(--lf-font-mono)',
        fontSize: 'var(--lf-text-caption)',
      }}
    >
      Loading post…
    </div>
  )
}

// Shown when the API route returns 404 or 500. Keeps the user moving
// (link to the original) instead of showing "Tweet not found" with
// no way out.
function TweetUnavailable({ url }: { url: string }) {
  return (
    <div
      style={{
        border: '1px solid var(--lf-rule-soft)',
        borderRadius: 12,
        padding: '14px 16px',
        background: 'var(--lf-paper-alt)',
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        fontFamily: 'var(--lf-font-body)',
        fontSize: 'var(--lf-text-body-sm)',
        color: 'var(--lf-muted)',
      }}
    >
      <span style={{ fontSize: 18 }}>{'\u{1D54F}'}</span>
      <span style={{ flex: 1 }}>This post couldn't be loaded — it may have been deleted or made private.</span>
      <a
        href={safeHref(url)}
        target="_blank"
        rel="noopener noreferrer"
        style={{
          color: 'var(--lf-ink)',
          textDecoration: 'underline',
          textUnderlineOffset: 3,
          whiteSpace: 'nowrap',
        }}
      >
        View on X &rarr;
      </a>
    </div>
  )
}

function getYouTubeId(url: string): string | null {
  const match = url.match(/(?:youtube\.com\/watch\?v=|youtu\.be\/)([a-zA-Z0-9_-]{11})/)
  return match ? match[1] : null
}

function isGitHubRepo(url: string): boolean {
  return /^https?:\/\/github\.com\/[^/]+\/[^/]+\/?$/.test(url)
}

function getTweetId(url: string): string | null {
  const match = url.match(/(?:twitter\.com|x\.com)\/\w+\/status\/(\d+)/)
  return match ? match[1] : null
}

export default function EmbedRenderer({ url }: Props) {
  const ytId = getYouTubeId(url)
  if (ytId) {
    return (
      <div style={{ position: 'relative', paddingBottom: '56.25%', height: 0, overflow: 'hidden', borderRadius: 10, margin: '8px 0', border: '1px solid var(--border)' }}>
        <iframe
          src={`https://www.youtube-nocookie.com/embed/${ytId}`}
          style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: '100%', border: 'none' }}
          sandbox="allow-scripts allow-same-origin"
          allow="encrypted-media"
          loading="lazy"
          title="YouTube video"
        />
      </div>
    )
  }

  if (isGitHubRepo(url)) {
    return <LinkPreview url={url} />
  }

  const tweetId = getTweetId(url)
  if (tweetId) {
    // The <Tweet> client component normally hits Twitter's syndication
    // API directly from the browser — flaky in practice (rate limits,
    // CORS, ad blockers, token endpoint 403s) which surfaces as
    // "Tweet not found" even for live tweets. Routing through our own
    // /tweet-embed/[id] endpoint runs the fetch in Node and caches
    // it, making the render reliable.
    //
    // The path is /tweet-embed/, NOT /api/tweet/, because the prod
    // reverse proxy sends every /api/* path to the Go backend.
    return (
      <div className="lf-tweet-embed" style={{ margin: '12px 0' }}>
        <Tweet
          id={tweetId}
          apiUrl={`/tweet-embed/${tweetId}`}
          fallback={<TweetSkeleton />}
          components={{
            TweetNotFound: () => <TweetUnavailable url={url} />,
          }}
        />
      </div>
    )
  }

  // Direct image URLs — render as image, not link preview
  const isImageUrl = /\.(jpg|jpeg|png|gif|webp|svg)(\?|$)/i.test(url) ||
    ['i.redd.it', 'i.imgur.com', 'pbs.twimg.com'].some(d => url.includes(d))
  if (isImageUrl) {
    return (
      <div style={{ margin: '8px 0', borderRadius: 8, overflow: 'hidden' }}>
        <a href={safeHref(url)} target="_blank" rel="noopener noreferrer">
          <img src={url} alt="" style={{ maxWidth: '100%', maxHeight: 400, objectFit: 'contain', borderRadius: 8, background: 'var(--gray-50)' }} loading="lazy" />
        </a>
      </div>
    )
  }

  // Fallback: render as link preview card
  return <LinkPreview url={url} />
}
