// web/src/components/lf/LFPostCard.tsx
'use client'

import React from 'react'
import Link from 'next/link'
import { LFAvatar } from './LFAvatar'
import { LFFlair } from './LFFlair'
import { LFAgentMark } from './LFAgentMark'
import { LFSourcesCount } from './LFSourcesCount'
import { type LFEpistemicKind } from './LFEpistemic'
import { IconComment, IconBookmark, IconShare, IconCornerUpRight, IconUpvote, IconDownvote } from './icons'
import { slugifyTitle } from '../../lib/post-url'
import { stripMarkdown } from '../../lib/strip-markdown'
import { api } from '../../api/client'
import type { PostView } from '../../api/types'
import FollowButton from '../FollowButton'

// LFPostCard — the cornerstone of the new design. Reused on Feed,
// profile post lists, community pages, search results, post detail
// thread replies. Single shape: takes a PostView, calls onVote when
// the upvote arrow is clicked (caller handles optimistic state).
//
// Design fidelity: meta row → title → body preview → citation badge
// (when provenance present) → action row. All paddings, borders,
// shadows come from `--lf-*` CSS variables (Phase 0 tokens).

export interface LFPostCardProps {
  post: PostView
  onVote?: (postId: string, direction: 'up' | 'down') => void
  /** When true, the card renders without the hard offset shadow.
   *  Use inside dense lists (e.g. profile-tab post list) so stacked
   *  cards don't look busy. */
  flat?: boolean
}

// Map our existing post.epistemicStatus values to LFEpistemicKind.
// Existing system uses 'supported' | 'contested' | 'refuted' |
// 'consensus' | 'hypothesis' — already a 1:1 match. Anything we don't
// recognize is omitted (the post just doesn't render an epistemic
// chip).
function mapEpistemic(status: string | undefined): LFEpistemicKind | null {
  switch (status) {
    case 'hypothesis':
    case 'supported':
    case 'contested':
    case 'refuted':
    case 'consensus':
      return status
    default:
      return null
  }
}

// Hash an author id into a stable seed so LFAvatar's color palette
// produces a consistent color per user. Cheap polynomial — same idea
// as Java's String.hashCode but constrained to non-negative.
function seedFromId(id: string | undefined): number {
  if (!id) return 0
  let h = 0
  for (let i = 0; i < id.length; i++) {
    h = (h * 31 + id.charCodeAt(i)) | 0
  }
  return Math.abs(h)
}

function relativeTime(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  if (ms < 60_000) return 'now'
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h`
  if (ms < 7 * 86_400_000) return `${Math.floor(ms / 86_400_000)}d`
  return new Date(iso).toLocaleDateString()
}

// proxiedImage routes external image URLs through our caching proxy
// at /api/v1/img. External news-site CDNs are slow (krebsonsecurity
// measured at 2s per image); proxying through us puts them on
// Cloudflare's edge and serves subsequent fetches in <50ms.
//
// Loomfeed-hosted images (uploaded by users via /api/uploads/* or
// served from our own CDN) are passed through unchanged — no point
// proxying our own assets.
function proxiedImage(rawUrl: string, opts?: { w?: number }): string {
  if (!rawUrl) return rawUrl
  // Already proxied or a relative / data URL — pass through.
  if (rawUrl.startsWith('/') || rawUrl.startsWith('data:')) return rawUrl
  if (rawUrl.startsWith('blob:')) return rawUrl
  let host = ''
  try { host = new URL(rawUrl).hostname } catch { return rawUrl }
  // Don't double-proxy our own domains.
  if (host === 'www.loomfeed.com' || host === 'loomfeed.com' ||
      host.endsWith('.loomfeed.com')) {
    return rawUrl
  }
  // `w` asks the proxy to downscale + re-encode to JPEG at that max
  // width (the proxy caches each variant separately). Cards show
  // images far smaller than their multi-MB source, so this is the
  // dominant payload win.
  const wParam = opts?.w ? `&w=${opts.w}` : ''
  return `/api/v1/img?url=${encodeURIComponent(rawUrl)}${wParam}`
}

const BODY_EXCERPT_MAX = 240

// Hosts whose own typed embed variant hasn't shipped yet — skip
// the dispatcher so they don't fall through to article. When the
// tweet / instagram variants land they get their own branches in
// classifyHost below and come out of this skip list.
const UNSUPPORTED_TYPED_HOSTS = new Set([
  'twitter.com', 'x.com', 'mobile.twitter.com',
  'instagram.com', 'www.instagram.com',
])

const YOUTUBE_HOSTS = new Set([
  'youtube.com', 'www.youtube.com', 'm.youtube.com', 'youtu.be',
])

type EmbedKind = 'article' | 'youtube'

interface PrimaryEmbed {
  kind: EmbedKind
  url: string
  title?: string
  description?: string
  image?: string
  domain?: string
}

function classifyHost(host: string): EmbedKind | null {
  if (YOUTUBE_HOSTS.has(host)) return 'youtube'
  if (UNSUPPORTED_TYPED_HOSTS.has(host)) return null
  return 'article'
}

// extractPrimaryEmbed finds the post's primary linked source and
// returns the data needed to dispatch to a typed embed component
// (LFEmbeddedArticle, LFEmbeddedYouTube, etc.). Returns null when:
//   - the post type is image/video (cover-image render is correct)
//   - no body_link_previews exists on metadata
//   - every linked URL belongs to a host whose typed variant
//     hasn't shipped yet (twitter / instagram for now)
//
// Picks the first preview entry that classifies into a supported
// kind. transformKeys (in api/client.ts) has already converted
// snake_case keys to camelCase recursively, so we read
// `bodyLinkPreviews` first and fall back to the raw
// `body_link_previews` for older cached responses. The URL is
// read from the `.url` field inside each entry — map keys can be
// mangled by snake→camel on URLs containing `_`.
function extractPrimaryEmbed(post: PostView): PrimaryEmbed | null {
  const postType = (post as any).postType ?? (post as any).post_type
  if (postType === 'image' || postType === 'video') return null

  const metadata: any = post.metadata || {}
  const previews =
    metadata.bodyLinkPreviews ||
    metadata.body_link_previews ||
    null
  if (!previews || typeof previews !== 'object') return null

  for (const entry of Object.values(previews) as any[]) {
    if (!entry || typeof entry !== 'object') continue
    const url: string | undefined = entry.url
    if (!url || typeof url !== 'string') continue

    let host = ''
    try { host = new URL(url).hostname.toLowerCase() } catch { continue }
    const kind = classifyHost(host)
    if (!kind) continue

    return {
      kind,
      url,
      title: typeof entry.title === 'string' ? entry.title : undefined,
      description: typeof entry.description === 'string' ? entry.description : undefined,
      image: typeof entry.image === 'string' ? entry.image : undefined,
      domain: typeof entry.domain === 'string' ? entry.domain : host,
    }
  }
  return null
}

// Pull a cover image from the post's metadata blob. The codebase
// scatters image URLs across many fields depending on post type and
// API version (image_url, imageUrl, thumbnailUrl, og.image, link
// preview, etc.). Mirrors the legacy PostCard's resolution chain so
// existing posts keep showing their images on the new card.
function extractCoverImage(post: PostView): string | null {
  const metadata: any = post.metadata || {}
  const direct =
    metadata.imageUrl ||
    metadata.image_url ||
    metadata.coverImageUrl ||
    metadata.cover_image_url ||
    metadata.cover ||
    metadata.thumbnailUrl ||
    metadata.thumbnail_url ||
    metadata.thumbnail ||
    metadata.og?.image ||
    metadata.openGraph?.image ||
    metadata.hero?.image ||
    metadata.link_preview?.image ||
    metadata.linkPreview?.image
  if (typeof direct === 'string' && direct.trim()) return direct
  // Video posts: derive a thumbnail from the embedded video URL so
  // the feed card has a visual hook (otherwise post_type=video
  // shows as plain text on the feed even though the post detail
  // page renders the iframe).
  const videoUrl: string | undefined =
    metadata.video_url || metadata.videoUrl
  if (videoUrl) {
    const thumb = thumbFromVideoUrl(videoUrl)
    if (thumb) return thumb
  }
  // body_link_previews — the backend's prefetched OG cache for every
  // URL in the body. Map keyed by URL → { image, title, description }.
  // First preview with an image wins. Same fallback PostDetail uses,
  // kept in sync so feed card and detail page show the same hero
  // image for the same post.
  const previews = metadata.body_link_previews || metadata.bodyLinkPreviews
  if (previews && typeof previews === 'object') {
    for (const v of Object.values(previews)) {
      const img = (v as any)?.image
      if (typeof img === 'string' && img.trim()) return img
    }
  }
  // First markdown image inline in the body, e.g. ![alt](url).
  const m = (post.body || '').match(/!\[[^\]]*\]\(([^)\s]+)/)
  if (m && m[1]) return m[1]
  return null
}

// Derive a poster image from a video URL. Returns maxresdefault.jpg
// (1280x720) when the host is YouTube — old hqdefault.jpg (480x360)
// upscaled blurry on retina viewports. Not every YT video has a
// maxres frame, so the consuming <img> falls back to hqdefault on
// error (see onError in the cover render).
// Extract a YouTube video id from any of the URL shapes YT uses
// (watch?v=, youtu.be/, /embed/, /shorts/, /live/). Returns '' for
// non-YouTube or unparseable URLs.
function youtubeId(url: string): string {
  try {
    const u = new URL(url)
    const host = u.hostname.replace(/^www\./, '')
    if (host !== 'youtube.com' && host !== 'youtu.be' && host !== 'm.youtube.com') return ''
    let id = ''
    if (host === 'youtu.be') id = u.pathname.slice(1)
    else if (u.pathname.startsWith('/embed/')) id = u.pathname.slice('/embed/'.length)
    else if (u.pathname.startsWith('/shorts/')) id = u.pathname.slice('/shorts/'.length)
    else if (u.pathname.startsWith('/live/')) id = u.pathname.slice('/live/'.length)
    else id = u.searchParams.get('v') ?? ''
    return id.split('/')[0].split('?')[0]
  } catch {
    return ''
  }
}

function thumbFromVideoUrl(url: string): string | null {
  // maxresdefault.jpg (1280x720); the consuming <img> falls back to
  // hqdefault on error (older videos lack a maxres frame).
  const id = youtubeId(url)
  return id ? `https://img.youtube.com/vi/${id}/maxresdefault.jpg` : null
}

// Feed video cover that autoplays muted when scrolled into view
// (Reddit/X-style "lively feed") and unloads when scrolled away so we
// never run more than the in-view players. Respects
// prefers-reduced-motion — those users keep the static, click-to-open
// thumbnail. The whole card still navigates to the discussion; the
// inline player only captures clicks on the video surface itself.
function FeedVideoCover({
  videoId,
  poster,
  href,
  title,
}: {
  videoId: string
  poster: string
  href: string
  title: string
}) {
  const ref = React.useRef<HTMLDivElement>(null)
  const [active, setActive] = React.useState(false)

  React.useEffect(() => {
    const el = ref.current
    if (!el) return
    // Honor reduced-motion: never autoplay; keep the static poster.
    const mq = typeof window !== 'undefined' && window.matchMedia
      ? window.matchMedia('(prefers-reduced-motion: reduce)')
      : null
    if (mq?.matches) return

    const obs = new IntersectionObserver(
      (entries) => {
        const e = entries[0]
        // Activate only when comfortably in view; deactivate (and thus
        // unmount the iframe) once it scrolls mostly out.
        setActive(e.isIntersecting && e.intersectionRatio >= 0.6)
      },
      { threshold: [0, 0.6, 1] },
    )
    obs.observe(el)
    return () => obs.disconnect()
  }, [])

  return (
    <div ref={ref} className={`post-cover is-video${active ? ' is-playing' : ''}`} onClick={(e) => e.stopPropagation()}>
      {active ? (
        <iframe
          src={`https://www.youtube-nocookie.com/embed/${videoId}?autoplay=1&mute=1&playsinline=1&controls=1&modestbranding=1&rel=0`}
          title={title}
          loading="lazy"
          allow="autoplay; encrypted-media; picture-in-picture"
          allowFullScreen
          style={{ width: '100%', height: '100%', border: 0, display: 'block' }}
        />
      ) : (
        <Link href={href} aria-label={`Open post: ${title}`} style={{ display: 'block', width: '100%', height: '100%' }}>
          <img
            src={proxiedImage(poster, { w: 900 })}
            alt=""
            loading="lazy"
            decoding="async"
            onError={(ev) => {
              const img = ev.currentTarget as HTMLImageElement
              if (img.src.includes('maxresdefault.jpg')) {
                img.src = img.src.replace('maxresdefault.jpg', 'hqdefault.jpg')
                return
              }
              const wrapper = img.parentElement?.parentElement
              if (wrapper) (wrapper as HTMLElement).style.display = 'none'
            }}
          />
        </Link>
      )}
    </div>
  )
}

export function LFPostCard({ post, onVote, flat }: LFPostCardProps) {
  const isAgent = post.author.type === 'agent'
  const epistemic = mapEpistemic((post as any).epistemicStatus)
  const sealed = Boolean((post as any).isSealed ?? (post as any).sealed)
  const sourceCount = post.provenance?.sourceCount ?? 0
  const seed = seedFromId(post.authorId ?? post.author.displayName)
  const href = `/post/${post.id}/${slugifyTitle(post.title)}`
  const primaryEmbed = extractPrimaryEmbed(post)

  // Reddit-style feed card: source is a small badge in the meta
  // row + a small right-side thumb. No hero-embed cards inline
  // (those live on the detail page now — LFEmbeddedArticle /
  // LFEmbeddedYouTube components remain available for that use).
  // Goal: the title is the editorial framing, the thumb is just
  // a visual hook, the actions are scannable. Article previews
  // dominated the card and made it read as "republished
  // article" instead of "discussion entry."
  const coverImage = extractCoverImage(post)
  const thumbImage = primaryEmbed?.image || coverImage
  const isVideoThumb =
    primaryEmbed?.kind === 'youtube'
    || (post as any).postType === 'video'
    || (post as any).post_type === 'video'
    || Boolean((post.metadata as any)?.video_url || (post.metadata as any)?.videoUrl)
  // YouTube id for the in-feed autoplay player (empty for non-YouTube
  // videos, which fall back to the static thumbnail).
  const videoEmbedId = isVideoThumb
    ? youtubeId(
        (post.metadata as any)?.video_url
        || (post.metadata as any)?.videoUrl
        || primaryEmbed?.url
        || '',
      )
    : ''
  // Source URL (external article / video) when this post has a
  // typed embed. The source badge in the meta row routes here in
  // a new tab; the cover image stays in-platform (clicking the
  // card anywhere opens the discussion, matching Reddit's
  // convention).
  const sourceUrl = primaryEmbed?.url ?? null
  // Source badge text shown after the time in the meta row.
  // Article posts → publisher domain ("theguardian.com"); video
  // posts → "▶ YouTube"; everything else → nothing.
  const sourceBadge = primaryEmbed
    ? primaryEmbed.kind === 'youtube'
      ? '▶ YouTube'
      : (primaryEmbed.domain ?? '').replace(/^www\./, '') || null
    : null

  // Tags from the post (PostView.tags). May be undefined.
  const tags = ((post as any).tags as string[] | undefined) ?? []
  const epistemicClass =
    epistemic === 'hypothesis' ? 'epi hypothesis' :
    epistemic === 'supported' ? 'epi supported' :
    epistemic === 'contested' ? 'epi contested' :
    epistemic === 'refuted' ? 'epi refuted' :
    null
  // Title-case label + source-count suffix to match hybrid-front.html
  // exactly: "Hypothesis", "Supported · 5 sources", "Contested · 3
  // dissents". Reference shows the suffix only when there's something
  // to count; we mirror that.
  const dissentCount = (post as any).dissentCount ?? (post as any).dissent_count ?? 0
  const epistemicLabel = (() => {
    if (!epistemic) return ''
    const word = epistemic.charAt(0).toUpperCase() + epistemic.slice(1)
    if (epistemic === 'supported' && sourceCount > 0) {
      return `${word} · ${sourceCount} ${sourceCount === 1 ? 'source' : 'sources'}`
    }
    if (epistemic === 'contested' && dissentCount > 0) {
      return `${word} · ${dissentCount} ${dissentCount === 1 ? 'dissent' : 'dissents'}`
    }
    if (epistemic === 'refuted' && sourceCount > 0) {
      return `${word} · ${sourceCount} ${sourceCount === 1 ? 'source' : 'sources'}`
    }
    return word
  })()

  // Meta line is always at the top of the post — outside post-row in
  // the reference. The reference's post-row contains [content column |
  // thumb], where content column is title + body + tags + actions all
  // inside one flex:1 child. Matching that exact structure here so the
  // thumb sits beside the WHOLE inner content (including the actions
  // pill row) instead of just title/body, which kept the post visually
  // taller than reference.
  // Current effective vote direction for the left rail (read-only,
  // sourced from PostView.userVote — same value the old VotePill read).
  const userVote = (post.userVote as 'up' | 'down' | null | undefined) ?? null

  // Bookmark / Save — self-contained optimistic toggle. Vote is a prop
  // (onVote) because the parent owns post-list score state; a bookmark is
  // a per-user flag with no sibling/aggregate to update, so the card owns
  // it directly (mirrors PostDetail.handleSave). This makes the Save pill
  // work in every surface — feed, profile, community, search — with no
  // caller wiring. Previously its onClick only preventDefault'd: a dead
  // button on the most-used surface.
  const [bookmarked, setBookmarked] = React.useState<boolean>(
    Boolean((post as any).userBookmarked),
  )
  const [savePending, setSavePending] = React.useState(false)
  const handleSave = React.useCallback(
    async (e: React.MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      if (savePending) return
      const next = !bookmarked
      setBookmarked(next)
      setSavePending(true)
      try {
        await api.toggleBookmark(post.id)
      } catch {
        setBookmarked(!next) // roll back on failure
      } finally {
        setSavePending(false)
      }
    },
    [bookmarked, savePending, post.id],
  )

  // Meta is split into two groups: identity (avatar / name / AI mark /
  // Subscribe) and context (community / time / source / epistemic tag).
  // On desktop both groups are `display: contents`, so the row renders
  // exactly as one flex line. On mobile each group becomes its own row —
  // a DETERMINISTIC two-line layout instead of fit-dependent flex-wrap,
  // which made every card arrange its byline differently depending on
  // name/community length.
  const Meta = (
    <div className="lf-post-meta meta">
      <span className="lf-post-meta-id">
        <span className="av-sm">
          <LFAvatar
            size={22}
            seed={seed}
            agent={isAgent}
            imageUrl={post.author.avatarUrl}
          />
        </span>
        <Link className="lf-post-community name" href={`/profile/${post.authorId ?? ''}`}>
          {post.author.displayName}
        </Link>
        {isAgent && <LFAgentMark size="sm" />}
        {isAgent && post.authorId && (
          <FollowButton
            targetId={post.authorId}
            size="sm"
            subscribeLabel
            initialFollowing={post.viewerFollowing ?? false}
          />
        )}
        <LFFlair label={post.authorFlairLabel} color={post.authorFlairColor} />
      </span>
      <span className="lf-post-meta-ctx">
        <span>
          {post.communitySlug && (
            <>
              <span className="lf-post-meta-sep">
                {/* The leading dot reads as a separator after the name on
                    one line, but as stray punctuation when the context
                    group starts its own row on mobile — so it gets its
                    own hideable span. */}
                <span className="lf-post-meta-lead-dot">{'· '}</span>
                {'in '}
              </span>
              <Link className="lf-post-community" href={`/a/${post.communitySlug}`}>
                a/{post.communitySlug}
              </Link>
              <span className="lf-post-meta-sep">{' · '}</span>
            </>
          )}
          <time className="lf-post-time">{relativeTime(post.createdAt)}</time>
        </span>
        {sourceBadge && sourceUrl && (
          <a
            className="post-source-badge"
            href={sourceUrl}
            target="_blank"
            rel="noopener noreferrer"
            title={`Open source: ${primaryEmbed?.domain ?? sourceUrl}`}
            onClick={(e) => e.stopPropagation()}
            aria-label={`Open source: ${primaryEmbed?.domain ?? sourceUrl}`}
          >
            {sourceBadge}
            <span aria-hidden style={{ marginLeft: 4, opacity: 0.7 }}>↗</span>
          </a>
        )}
        {epistemicClass && epistemic && (
          <span className={epistemicClass}>
            <span className="dot" aria-hidden />
            {epistemicLabel}
          </span>
        )}
      </span>
    </div>
  )


  return (
    <article
      className={`lf-post post${flat ? ' is-flat' : ''}`}
      style={
        sealed ? { borderLeft: '3px solid var(--lf-seal)' } : undefined
      }
    >
      {/* LEFT VOTE RAIL — vertical up / score / down. Same onVote
          signature and userVote source as the old horizontal VotePill;
          presentation-only relocation. */}
      <div className="lf-post-votes">
        <button
          type="button"
          className={`lf-post-vote-btn is-up${userVote === 'up' ? ' is-active' : ''}`}
          aria-pressed={userVote === 'up'}
          aria-label="Upvote"
          onClick={(e) => {
            e.preventDefault()
            e.stopPropagation()
            onVote?.(post.id, 'up')
          }}
        >
          <IconUpvote size={18} strokeWidth={1.75} />
        </button>

        <span className="lf-post-score">{post.score}</span>

        <button
          type="button"
          className={`lf-post-vote-btn is-down${userVote === 'down' ? ' is-active' : ''}`}
          aria-pressed={userVote === 'down'}
          aria-label="Downvote"
          onClick={(e) => {
            e.preventDefault()
            e.stopPropagation()
            onVote?.(post.id, 'down')
          }}
        >
          <IconDownvote size={18} strokeWidth={1.75} />
        </button>
      </div>

      {/* CONTENT COLUMN */}
      <div className="lf-post-main">
      {Meta}
      {/* Phase 0.4: when this post is quarantined, the only viewer
          who actually receives it is its author or a moderator.
          Render a small "Pending review" callout so the author
          isn't confused about why their post isn't on the feed. */}
      {post.quarantined && (
        <div
          style={{
            margin: '6px 0 10px',
            padding: '8px 12px',
            border: '1px solid var(--lf-warn, #D97706)',
            background: 'color-mix(in srgb, var(--lf-warn, #D97706) 8%, transparent)',
            borderRadius: 'var(--lf-radius-sm)',
            fontFamily: 'var(--lf-font-body)',
            fontSize: 12,
            color: 'var(--lf-ink)',
            lineHeight: 1.4,
          }}
        >
          <span
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 'var(--lf-text-label)',
              letterSpacing: '0.12em',
              textTransform: 'uppercase',
              color: 'var(--lf-warn, #D97706)',
              fontWeight: 700,
              marginRight: 8,
            }}
          >
            Pending review
          </span>
          New-account posts are reviewed before they appear publicly. Usually under an hour.
        </div>
      )}
      <div className="post-content">
        <Link className="lf-post-title post-title" href={href}>
          {post.title}
        </Link>
        {/* Source-count badge — the central loomfeed promise made
            visible (per docs/POSITIONING.md "every post comes with
            receipts"). Placed right under the title so it's the
            second thing a reader notices after the headline. */}
        {sourceCount > 0 && (
          <div style={{ marginTop: 6, marginBottom: 4 }}>
            <LFSourcesCount
              sourceCount={sourceCount}
              verifiedCount={(post as any).verifiedSources ?? 0}
              size="sm"
            />
          </div>
        )}
        {/* Phase 1.2 — feed-level Quoted pill. Detail page renders
            the full inset citation; on the feed we keep it light
            so the card doesn't blow up. */}
        {post.quotedPostId && (
          <div
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 6,
              marginTop: 4,
              padding: '2px 8px',
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 'var(--lf-text-label)',
              letterSpacing: '0.1em',
              textTransform: 'uppercase',
              color: 'var(--lf-ink)',
              background: 'var(--lf-accent)',
              border: '1px solid var(--lf-ink)',
              borderRadius: 'var(--lf-radius-tag)',
              fontWeight: 700,
            }}
          >
            Quoted post
          </div>
        )}
        {/* Cover image renders full-width below the title.
            Click goes to the SOURCE URL in a new tab when the
            post has a typed embed (article / video), else to the
            in-platform post detail. The hero-embed render that
            #73 + #74 added is gone — those components stay in
            the LF primitive set for the detail-page revamp; on
            the feed they made every post read as "republished
            article" instead of "discussion entry." Source
            attribution moved to the .post-source-badge in the
            meta row above. */}
        {thumbImage && videoEmbedId && (
          <FeedVideoCover videoId={videoEmbedId} poster={thumbImage} href={href} title={post.title} />
        )}
        {thumbImage && !videoEmbedId && (
          <Link
            href={href}
            className={`post-cover${isVideoThumb ? ' is-video' : ''}`}
            onClick={(e) => e.stopPropagation()}
            aria-label={`Open post: ${post.title}`}
          >

            <img
              src={proxiedImage(thumbImage, { w: 1200 })}
              alt=""
              loading="lazy"
              decoding="async"
              onError={(e) => {
                // YouTube maxresdefault.jpg 404s on older videos.
                // Fall back to hqdefault, then hide the wrapper
                // on total failure so the layout doesn't reserve
                // dead space.
                const img = e.currentTarget as HTMLImageElement
                const src = img.src
                if (src.includes('maxresdefault.jpg')) {
                  img.src = src.replace('maxresdefault.jpg', 'hqdefault.jpg')
                  return
                }
                const wrapper = img.parentElement
                if (wrapper) wrapper.style.display = 'none'
              }}
            />
          </Link>
        )}
        {(() => {
          const plain = stripMarkdown(post.body || '').trim()
          if (!plain) return null
          const text = plain.length > BODY_EXCERPT_MAX
            ? plain.slice(0, BODY_EXCERPT_MAX).trimEnd() + '…'
            : plain
          return (
            <Link className="lf-post-body post-body" href={href}>
              {text}
            </Link>
          )
        })()}

        {tags.length > 0 && (
          <div className="post-tags">
            {tags.slice(0, 4).map((t) => (
              <Link
                key={t}
                href={`/t/${encodeURIComponent(t)}`}
                className="post-tag"
                onClick={(e) => e.stopPropagation()}
              >
                {t}
              </Link>
            ))}
          </div>
        )}

        <div className="lf-post-actions post-actions">
          {/* Mobile-only inline vote chip — on phones the left vote rail
              is hidden (a stray ↑ n ↓ row under the actions read as
              broken layout) and this pill takes its place at the start
              of the action row, Reddit-app style. Desktop hides this
              chip and keeps the rail; display:none keeps whichever is
              hidden out of the a11y tree, so there's never a duplicate
              control announced. Same onVote/userVote wiring as the rail. */}
          <span className="lf-vote-inline" role="group" aria-label="Vote">
            <button
              type="button"
              className={`lf-vote-inline-btn is-up${userVote === 'up' ? ' is-active' : ''}`}
              aria-pressed={userVote === 'up'}
              aria-label="Upvote"
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                onVote?.(post.id, 'up')
              }}
            >
              <IconUpvote size={16} strokeWidth={1.75} />
            </button>
            <span className="lf-vote-inline-score">{post.score}</span>
            <button
              type="button"
              className={`lf-vote-inline-btn is-down${userVote === 'down' ? ' is-active' : ''}`}
              aria-pressed={userVote === 'down'}
              aria-label="Downvote"
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                onVote?.(post.id, 'down')
              }}
            >
              <IconDownvote size={16} strokeWidth={1.75} />
            </button>
          </span>

          <Link
            href={href}
            className="pill-btn pill-btn-comments"
            aria-label={`${post.commentCount} ${post.commentCount === 1 ? 'comment' : 'comments'}`}
            onClick={(e) => e.stopPropagation()}
          >
            <IconComment size={14} strokeWidth={1.75} />
            <span>{post.commentCount} Comments</span>
          </Link>

          <button
            type="button"
            className="pill-btn"
            aria-label="Share"
            onClick={(e) => {
              e.preventDefault()
              e.stopPropagation()
              const url =
                typeof window !== 'undefined' ? `${window.location.origin}${href}` : href
              if (typeof navigator !== 'undefined' && (navigator as any).share) {
                ;(navigator as any).share({ title: post.title, url }).catch(() => {})
              } else if (typeof navigator !== 'undefined' && navigator.clipboard) {
                navigator.clipboard.writeText(url).catch(() => {})
              }
            }}
          >
            <IconShare size={14} strokeWidth={1.75} />
            <span>Share</span>
          </button>

          <button
            type="button"
            className="pill-btn"
            aria-label={bookmarked ? 'Saved' : 'Save'}
            aria-pressed={bookmarked}
            onClick={handleSave}
          >
            <IconBookmark size={14} filled={bookmarked} />
            <span>{bookmarked ? 'Saved' : 'Save'}</span>
          </button>

          {/* Phase 1.2 — Quote. Drops the user into the composer
              with ?quote=<post-id> set; Submit.tsx renders the
              "Quoting:" inset and ships quoted_post_id with the
              new post. */}
          <Link
            href={`/submit?quote=${post.id}`}
            className="pill-btn"
            aria-label="Quote"
            onClick={(e) => e.stopPropagation()}
          >
            <IconCornerUpRight size={14} strokeWidth={1.75} />
            <span>Quote</span>
          </Link>
        </div>
      </div>
      </div>
    </article>
  )
}

// NOTE: the former horizontal `VotePill` (up · score · down inside one
// pill) has been relocated to the canonical LEFT vote rail in the card
// markup above (.lf-post-votes). It read the same `post.userVote` /
// `post.score` and called the same `onVote(post.id, dir)` — the change
// is presentation-only (vertical rail instead of inline pill).

