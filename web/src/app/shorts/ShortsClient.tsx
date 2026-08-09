'use client'

import { useEffect, useRef, useState, useCallback } from 'react'
import { safeHref } from '../../lib/safe-url'
import { api } from '../../api/client'

// Swipeable feed of LLM-curated external shorts. Each card is a
// YouTube iframe playing a <60s Short in five loomfeed-adjacent
// categories (AI research · robotics · science · ML eng · tech
// critique). Viewport-filling scroll-snap layout = the same TikTok
// shape we had before, but with actual video — which is the whole
// point of /shorts existing as a route.
//
// We never host video. The iframe streams direct from YouTube's CDN,
// and every card shows bold creator + "Watch on YouTube" attribution
// so we stay inside YouTube's ToS.
//
// Important: cards render a thumbnail facade by default and only swap
// in the real YouTube iframe when scrolled into view. Loading 30
// iframes at once both crashes mobile Safari and gets aggressively
// targeted by ad-blocker / privacy extensions, which is the most
// likely reason iframes were rendering as black rectangles before.

interface CuratedShort {
  id: string
  platform: string
  platform_video_id: string
  title: string
  creator_name: string
  creator_url: string
  category: string
  embed_url: string
  watch_url: string
  thumbnail_url: string
  duration_sec: number
  view_count: number
  ai_score: number
  ai_rationale: string
}

interface Category {
  slug: string
  display_name: string
}

export default function ShortsClient() {
  const [shorts, setShorts] = useState<CuratedShort[]>([])
  const [categories, setCategories] = useState<Category[]>([])
  const [activeCategory, setActiveCategory] = useState<string>('') // '' = all
  const [loading, setLoading] = useState(false)
  const [offset, setOffset] = useState(0)
  const [hasMore, setHasMore] = useState(true)
  const containerRef = useRef<HTMLDivElement>(null)

  // Fetch category list once — drives the tab bar at the top.
  useEffect(() => {
    ;(api as any)
      .listCuratedCategories?.()
      .then((d: any) => setCategories(Array.isArray(d) ? d : []))
      .catch(() => setCategories([]))
  }, [])

  // Reset the feed whenever the user flips categories.
  useEffect(() => {
    setShorts([])
    setOffset(0)
    setHasMore(true)
  }, [activeCategory])

  const loadPage = useCallback(() => {
    if (loading || !hasMore) return
    setLoading(true)
    ;(api as any)
      .listCuratedShorts?.({ category: activeCategory, limit: 30, offset })
      .then((resp: any) => {
        const arr: CuratedShort[] = Array.isArray(resp) ? resp : resp?.data ?? []
        setShorts((prev) => [...prev, ...arr])
        if (arr.length < 30) setHasMore(false)
        setOffset((o) => o + 30)
      })
      .catch(() => setHasMore(false))
      .finally(() => setLoading(false))
  }, [loading, hasMore, offset, activeCategory])

  useEffect(() => {
    loadPage()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeCategory])

  // Infinite scroll: when the container is within two viewports of
  // the bottom, grab the next page.
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const onScroll = () => {
      const remaining = el.scrollHeight - el.scrollTop - el.clientHeight
      if (remaining < el.clientHeight * 2) loadPage()
    }
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => el.removeEventListener('scroll', onScroll)
  }, [loadPage])

  return (
    <div
      ref={containerRef}
      className="lf-shorts-scroll"
      style={{
        marginTop: -24,
        marginLeft: 'var(--lf-short-gutter)',
        marginRight: 'var(--lf-short-gutter)',
        marginBottom: 'var(--lf-short-mb)',
        height: 'var(--lf-short-h)',
        overflowY: 'auto',
        scrollSnapType: 'y mandatory',
        background: 'var(--lf-paper)',
      }}
    >
      {/* Category tabs — sticky to the top of the scroll container so
          the user can jump between lanes without losing position. */}
      <div
        style={{
          position: 'sticky',
          top: 0,
          zIndex: 5,
          background: 'var(--lf-paper)',
          borderBottom: '1px solid var(--lf-rule-soft)',
          display: 'flex',
          gap: 6,
          padding: '10px 20px',
          overflowX: 'auto',
          whiteSpace: 'nowrap',
        }}
      >
        <CategoryTab label="All" active={activeCategory === ''} onClick={() => setActiveCategory('')} />
        {categories.map((c) => (
          <CategoryTab
            key={c.slug}
            label={c.display_name}
            active={activeCategory === c.slug}
            onClick={() => setActiveCategory(c.slug)}
          />
        ))}
      </div>

      {shorts.length === 0 && !loading && (
        <div
          style={{
            height: 'calc(100dvh - 120px)',
            display: 'grid',
            placeItems: 'center',
            padding: '0 40px',
            fontFamily: 'var(--lf-font-body)',
            fontStyle: 'italic',
            color: 'var(--lf-muted)',
            textAlign: 'center',
          }}
        >
          No curated shorts in this lane yet. An admin needs to run a
          refresh + approve some picks before they land here.
        </div>
      )}

      {shorts.map((s) => (
        <ShortCard key={s.id} short={s} scrollRoot={containerRef.current} />
      ))}

      {loading && (
        <div
          style={{
            height: 80,
            display: 'grid',
            placeItems: 'center',
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            letterSpacing: '0.12em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
          }}
        >
          Loading…
        </div>
      )}

      {!hasMore && shorts.length > 0 && (
        <div
          style={{
            height: 'var(--lf-short-h)',
            scrollSnapAlign: 'start',
            display: 'grid',
            placeItems: 'center',
            fontFamily: 'var(--lf-font-body)',
            fontStyle: 'italic',
            color: 'var(--lf-muted)',
          }}
        >
          That's everything in this lane. Refresh runs daily.
        </div>
      )}
    </div>
  )
}

// One card. Renders the thumbnail until either (a) it scrolls into
// view, or (b) the user clicks the play button. Either way, the
// thumbnail gets replaced by an autoplaying-muted iframe so the feed
// behaves like TikTok without burning resources on offscreen videos.
function ShortCard({ short: s, scrollRoot }: { short: CuratedShort; scrollRoot: HTMLDivElement | null }) {
  const cardRef = useRef<HTMLElement | null>(null)
  const [activated, setActivated] = useState(false)

  // IntersectionObserver fires once the card is half on-screen — we
  // flip to the iframe and never go back. Releasing the iframe on
  // scroll-out helps memory but breaks position-restoration when the
  // user scrolls back up, so we keep it once activated.
  useEffect(() => {
    if (activated) return
    const el = cardRef.current
    if (!el || !scrollRoot) return
    const obs = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) {
            setActivated(true)
            obs.disconnect()
            break
          }
        }
      },
      { root: scrollRoot, threshold: 0.5 }
    )
    obs.observe(el)
    return () => obs.disconnect()
  }, [activated, scrollRoot])

  // Activated iframes get autoplay=1 + mute=1 + playsinline so mobile
  // browsers will actually play them. modestbranding hides YouTube's
  // big logo, rel=0 hides the recommendation grid at end-of-video.
  const embedSrc = activated
    ? `${s.embed_url}${s.embed_url.includes('?') ? '&' : '?'}autoplay=1&mute=1&playsinline=1`
    : null

  return (
    <article
      ref={cardRef as any}
      style={{
        height: 'var(--lf-short-h)',
        scrollSnapAlign: 'start',
        scrollSnapStop: 'always',
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        padding: '24px 20px',
        borderBottom: '1px solid var(--lf-rule-soft)',
        background: 'var(--lf-paper)',
        overflow: 'hidden',
        boxSizing: 'border-box',
      }}
    >
      <div
        style={{
          width: '100%',
          maxWidth: 420,
          display: 'flex',
          flexDirection: 'column',
          gap: 12,
          minHeight: 0,
        }}
      >
        {/* 9:16 frame so the YouTube Shorts player fills without
            letterboxing. Click-to-play fallback covers the case where
            the IntersectionObserver hasn't fired yet (or the user has
            an extension blocking the autoplay). */}
        <div
          style={{
            position: 'relative',
            width: '100%',
            aspectRatio: '9 / 16',
            background: '#000',
            border: '1px solid var(--lf-rule-soft)',
            overflow: 'hidden',
            cursor: embedSrc ? 'default' : 'pointer',
          }}
          onClick={() => !embedSrc && setActivated(true)}
        >
          {embedSrc ? (
            <iframe
              src={embedSrc}
              title={s.title}
              allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share"
              allowFullScreen
              style={{
                position: 'absolute',
                inset: 0,
                width: '100%',
                height: '100%',
                border: 0,
              }}
            />
          ) : (
            <>
              {/* Thumbnail uses YouTube's CDN — img-src in our CSP
                  allows any HTTPS so this loads regardless of which
                  i.ytimg.com host responds. */}
              <img
                src={s.thumbnail_url}
                alt={s.title}
                loading="lazy"
                style={{
                  position: 'absolute',
                  inset: 0,
                  width: '100%',
                  height: '100%',
                  objectFit: 'cover',
                  filter: 'brightness(0.7)',
                }}
              />
              <div
                style={{
                  position: 'absolute',
                  inset: 0,
                  display: 'grid',
                  placeItems: 'center',
                  pointerEvents: 'none',
                }}
              >
                <div
                  style={{
                    width: 64,
                    height: 64,
                    borderRadius: '50%',
                    background: 'rgba(0,0,0,0.55)',
                    border: '2px solid #fff',
                    display: 'grid',
                    placeItems: 'center',
                  }}
                >
                  <div
                    style={{
                      width: 0,
                      height: 0,
                      borderTop: '12px solid transparent',
                      borderBottom: '12px solid transparent',
                      borderLeft: '20px solid #fff',
                      marginLeft: 4,
                    }}
                  />
                </div>
              </div>
            </>
          )}
        </div>

        {/* Title + attribution strip. Creator link and "Watch on
            YouTube" button are both required by YouTube's embed
            ToS. Category pill sets expectations. */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.14em',
              textTransform: 'uppercase',
              color: 'var(--lf-muted)',
            }}
          >
            {s.category}
          </div>
          <h2
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontSize: 18,
              lineHeight: 1.3,
              color: 'var(--lf-ink)',
              margin: 0,
            }}
          >
            {s.title}
          </h2>
          <div
            style={{
              display: 'flex',
              gap: 12,
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.1em',
              textTransform: 'uppercase',
              color: 'var(--lf-muted)',
              flexWrap: 'wrap',
            }}
          >
            <a
              href={safeHref(s.creator_url)}
              target="_blank"
              rel="noopener noreferrer"
              style={{ color: 'var(--lf-ink)', textDecoration: 'none' }}
            >
              {s.creator_name}
            </a>
            <span>·</span>
            <span>{s.duration_sec}s</span>
            <a
              href={safeHref(s.watch_url)}
              target="_blank"
              rel="noopener noreferrer"
              style={{ color: 'var(--lf-ink)', marginLeft: 'auto', textDecoration: 'none' }}
            >
              Watch on YouTube ↗
            </a>
          </div>
          {s.ai_rationale && (
            <p
              style={{
                fontFamily: 'var(--lf-font-body)',
                fontStyle: 'italic',
                fontSize: 12,
                color: 'var(--lf-muted)',
                margin: 0,
                lineHeight: 1.4,
              }}
            >
              {s.ai_rationale}
            </p>
          )}
        </div>
      </div>
    </article>
  )
}

function CategoryTab({
  label,
  active,
  onClick,
}: {
  label: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      style={{
        padding: '6px 12px',
        fontFamily: 'var(--lf-font-mono)',
        fontSize: 10,
        letterSpacing: '0.14em',
        textTransform: 'uppercase',
        border: '1px solid var(--lf-ink)',
        background: active ? 'var(--lf-ink)' : 'transparent',
        color: active ? 'var(--lf-paper)' : 'var(--lf-ink)',
        cursor: 'pointer',
        whiteSpace: 'nowrap',
      }}
    >
      {label}
    </button>
  )
}
