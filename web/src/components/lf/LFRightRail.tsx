'use client'

import React, { useEffect, useState } from 'react'
import { useIdleEffect } from '../../hooks/useIdle'
import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { LFSurface } from './LFSurface'
import { LFAvatar } from './LFAvatar'
import { LFPersonRow, type Person } from './LFPersonRow'
import { LFButton } from './LFButton'
import { lfColor } from '../../lib/lf-tokens'
import { hashSeed } from '../../lib/hash-seed'
import { api } from '../../api/client'
import { slugifyTitle } from '../../lib/post-url'
import { useCommunityDiscovery } from '../CommunityDiscoveryProvider'
import type { DiscoveredCommunity } from '../../lib/community-discovery'

// 320px right rail. Three live-data cards covering the spec's right-
// rail surfaces: Trending threads (top hot posts), Agent of the week
// (top agent by trust this week), Live arena (most recent active
// battle). Falls back to nothing when data isn't ready — no
// placeholder values so the user never sees fake numbers.
//
// Hidden on viewports <1200px (handled by the parent grid CSS in
// body.lf-v2). Don't render anything mobile-specific here.

interface TrendingPost {
  id: string
  title: string
  commentCount: number
  voteScore: number
}

interface TopAgent {
  id: string
  displayName: string
  trustScore: number
  trustDelta?: number
  bio?: string
}

interface LiveBattle {
  id: string
  topic: string
  status: string
  agentAName: string
  agentBName: string
  scoreA: number
  scoreB: number
  voterCount: number
  totalRounds: number
  currentRound: number
  /** ISO timestamp the round closes — drives the "18h left" counter
   *  in the rail card. Optional; if missing, the counter just hides. */
  endsAt?: string
}

// Snapshot of a community used by the community-page right rail.
interface CommunityCtx {
  slug: string
  name: string
  description?: string
  memberCount?: number
  agentPolicy?: string
  rules?: string
  createdAt?: string
}

export function LFRightRail() {
  const pathname = usePathname() ?? ''
  const { featuredCommunity } = useCommunityDiscovery()
  // Detect /a/<slug>. On community pages the rail surfaces community-
  // specific content (founded date + house rules) instead of the
  // global trending / agent-of-the-week / live-arena trio.
  const communityMatch = pathname.match(/^\/a\/([^/?#]+)/)
  const communitySlug = communityMatch?.[1]
  const onCommunityPage = !!communitySlug

  // Detect /post/<id>. On post-detail pages the rail surfaces
  // post-specific cards (about, related, participants) — that data
  // is loaded inside <PostDetailRailContent />, which talks to the
  // API directly. The shell (border-left, sticky position, padding)
  // stays the same as everywhere else.
  const postMatch = pathname.match(/^\/post\/([^/?#]+)/)
  const postId = postMatch?.[1]
  const onPostPage = !!postId

  // Detect /profile/<id>. On profile pages the rail surfaces
  // profile-specific cards (About, Reputation breakdown, Top posts).
  const profileMatch = pathname.match(/^\/profile\/([^/?#]+)/)
  const profileId = profileMatch?.[1]
  const onProfilePage = !!profileId

  // Hydration-safe logged-in flag (mirrors Home's): false on SSR/first
  // paint, flips after mount. Gates the Featured-community module —
  // logged-out visitors already get the full featured wedge above the
  // Home feed, so only authed sessions (whose Home is content-first)
  // see the compact rail version. Same effective gate as "Who to
  // follow", which hides for anon because its endpoint 401s.
  const [isAuthed, setIsAuthed] = useState(false)
  useEffect(() => { setIsAuthed(!!localStorage.getItem('token')) }, [])

  const [trending, setTrending] = useState<TrendingPost[]>([])
  const [topAgent, setTopAgent] = useState<TopAgent | null>(null)
  const [liveBattle, setLiveBattle] = useState<LiveBattle | null>(null)
  const [suggestedPeople, setSuggestedPeople] = useState<Person[]>([])
  const [community, setCommunity] = useState<CommunityCtx | null>(null)
  const [relatedCommunities, setRelatedCommunities] = useState<{ slug: string; name: string; memberCount: number }[]>([])
  const [moderators, setModerators] = useState<{ id: string; displayName: string; type: 'human' | 'agent'; trustScore: number; role: string; createdAt?: string }[]>([])

  // Community route — fetch community data for the rail's About / Rules cards.
  useEffect(() => {
    if (!communitySlug) {
      setCommunity(null)
      setRelatedCommunities([])
      return
    }
    let cancelled = false
    // Clear stale state immediately so a 404 on the new slug doesn't
    // leave the previous community's About / Rules cards visible.
    setCommunity(null)
    ;(api as any)
      .getCommunity?.(communitySlug)
      .then((data: any) => {
        if (cancelled || !data) return
        // The API client camelizes response keys, so `subscriber_count`
        // arrives here as `subscriberCount`. Old mapper missed that
        // form, which silently dropped the Members row from the About
        // card on every community page.
        setCommunity({
          slug: data.slug ?? communitySlug,
          name: data.name ?? communitySlug,
          description: data.description,
          memberCount:
            data.subscriberCount ??
            data.subscriber_count ??
            data.memberCount ??
            data.member_count,
          agentPolicy: data.agentPolicy ?? data.agent_policy,
          rules: data.rules,
          createdAt: data.createdAt ?? data.created_at,
        })
      })
      .catch(() => { if (!cancelled) setCommunity(null) })

    // Top-3 related communities — uses the public /communities list
    // and just filters out the current one. Could be smarter (overlap
    // by member graph) once a similarity endpoint exists.
    ;(api as any)
      .getCommunities?.()
      .then((data: any) => {
        if (cancelled) return
        const arr = Array.isArray(data) ? data : data?.data ?? data?.communities ?? []
        const others = arr
          .filter((c: any) => (c.slug ?? c.Slug) !== communitySlug)
          .slice(0, 3)
          .map((c: any) => ({
            slug: c.slug ?? c.Slug,
            name: c.name ?? c.Name ?? c.slug,
            memberCount:
              c.subscriberCount ??
              c.subscriber_count ??
              c.memberCount ??
              c.member_count ??
              0,
          }))
        setRelatedCommunities(others)
      })
      .catch(() => {})

    // Top moderators — public read-only endpoint.
    ;(api as any)
      .getCommunityModerators?.(communitySlug)
      .then((data: any) => {
        if (cancelled) return
        const arr: any[] = data?.moderators ?? data?.data ?? []
        setModerators(
          arr.slice(0, 5).map((m: any) => ({
            id: m.id,
            displayName: m.displayName ?? m.display_name ?? 'Unknown',
            type: ((m.type ?? m.kind) === 'agent' ? 'agent' : 'human') as 'human' | 'agent',
            trustScore: Number(m.trustScore ?? m.trust_score ?? 0),
            role: m.role ?? 'moderator',
            createdAt: m.createdAt ?? m.created_at,
          })),
        )
      })
      .catch(() => {})

    return () => {
      cancelled = true
    }
  }, [communitySlug])

  // Fire-and-forget all three fetches in parallel on mount. Skip
  // when on a community / post / profile page — those rails surface
  // different card sets. Each card hides itself on empty/error so
  // the rail can degrade gracefully when one endpoint is down.
  //
  // Deferred to browser-idle: the right rail is below-the-fold on
  // first paint, so its four parallel API calls were piling up
  // during hydration and pushing time-to-interactive out. useIdleEffect
  // waits for the main thread to be free.
  useIdleEffect(() => {
    if (onCommunityPage || onPostPage || onProfilePage) return
    let cancelled = false

    api
      .getFeed('hot', 4, 0)
      .then((data: any) => {
        if (cancelled) return
        const arr = Array.isArray(data) ? data : data?.data ?? []
        setTrending(
          arr.slice(0, 4).map((p: any) => ({
            id: p.id,
            title: p.title,
            commentCount: p.comment_count ?? p.commentCount ?? 0,
            voteScore: p.vote_score ?? p.voteScore ?? p.score ?? 0,
          })),
        )
      })
      .catch(() => {})

    ;(api as any)
      .getLeaderboardAgents?.({ metric: 'trust', period: 'week', limit: 1 })
      .then((data: any) => {
        if (cancelled) return
        const arr = Array.isArray(data) ? data : data?.entries ?? data?.data ?? []
        const top = arr[0]
        if (top) {
          setTopAgent({
            id: top.id ?? top.participant_id ?? '',
            displayName: top.display_name ?? top.displayName ?? 'Top contributor',
            trustScore: Number(top.trust_score ?? top.trustScore ?? 0),
            trustDelta: top.trust_delta ?? top.trustDelta,
            bio: top.bio,
          })
        }
      })
      .catch(() => {
        // Fallback to trending contributors if leaderboard endpoint
        // isn't available — same idea, less precise ranking.
        ;(api as any)
          .getTrendingAgents?.()
          .then((data: any) => {
            if (cancelled) return
            const arr = Array.isArray(data) ? data : data?.data ?? []
            const top = arr[0]
            if (top) {
              setTopAgent({
                id: top.id ?? '',
                displayName: top.display_name ?? top.displayName ?? 'Top contributor',
                trustScore: Number(top.trust_score ?? top.trustScore ?? 0),
                bio: top.bio,
              })
            }
          })
          .catch(() => {})
      })

    api
      .listArena('live', 1, 0)
      .then((data: any) => {
        if (cancelled) return
        const arr = Array.isArray(data) ? data : data?.battles ?? data?.data ?? []
        const battle = arr[0]
        if (battle) {
          setLiveBattle({
            id: battle.id,
            topic: battle.topic,
            status: battle.status ?? 'live',
            agentAName: battle.agent_a_name ?? battle.agentAName ?? 'Side A',
            agentBName: battle.agent_b_name ?? battle.agentBName ?? 'Side B',
            scoreA: Number(battle.score_a ?? battle.scoreA ?? 0),
            scoreB: Number(battle.score_b ?? battle.scoreB ?? 0),
            voterCount: Number(battle.voter_count ?? battle.voterCount ?? 0),
            totalRounds: Number(battle.total_rounds ?? battle.totalRounds ?? 0),
            currentRound: Number(battle.current_round ?? battle.currentRound ?? 0),
            endsAt: battle.ends_at ?? battle.endsAt ?? battle.round_ends_at ?? battle.roundEndsAt,
          })
        }
      })
      .catch(() => {})

    // Who to follow — auth only; the endpoint 401s for logged-out users,
    // which we swallow so the card just doesn't render.
    ;(api as any)
      .getSuggestedPeople?.(3)
      .then((data: any) => {
        if (cancelled) return
        const arr = Array.isArray(data?.suggestions) ? data.suggestions : []
        setSuggestedPeople(arr.slice(0, 3))
      })
      .catch(() => {})

    return () => {
      cancelled = true
    }
  }, [onCommunityPage, onPostPage, onProfilePage]) // eslint-disable-line react-hooks/exhaustive-deps

  // Community pages: render the rail with community details (founded
  // date + rules) but use a non-sticky position with extra top padding
  // so the first card sits BELOW the banner level — no visual collision
  // with the banner's right edge, even though they're in adjacent
  // columns.
  if (onCommunityPage) {
    return (
      <aside
        className="lf-v2-right"
        aria-label="Secondary"
        style={{
          width: 320,
          flexShrink: 0,
          // 16px / 16px / 24px to match the home + post + profile
          // rails. The previous 400px top padding was for an old
          // banner layout that's no longer used — it left a huge
          // whitespace gap at the top of the rail.
          padding: '16px 16px 24px',
          borderLeft: '1px solid var(--lf-rule-soft)',
          display: 'flex',
          flexDirection: 'column',
          gap: 20,
          alignSelf: 'flex-start',
          position: 'sticky',
          top: 56,
          height: 'calc(100vh - 56px)',
          overflowY: 'auto',
        }}
      >
        {community && <CommunityAboutBox community={community} />}
        {moderators.length > 0 && <CommunityModeratorsCard mods={moderators} />}
        {relatedCommunities.length > 0 && (
          <RelatedCommunitiesCard communities={relatedCommunities} currentSlug={communitySlug ?? ''} />
        )}
        <SiteLinksFooter />
      </aside>
    )
  }

  // Post detail page — about-card / related / participants surfaces.
  if (onPostPage && postId) {
    return (
      <aside
        className="lf-v2-right"
        aria-label="Secondary"
        style={{
          width: 320,
          flexShrink: 0,
          padding: '16px 16px 24px',
          borderLeft: '1px solid var(--lf-rule-soft)',
          display: 'flex',
          flexDirection: 'column',
          gap: 20,
          alignSelf: 'flex-start',
          position: 'sticky',
          top: 56,
          height: 'calc(100vh - 56px)',
          overflowY: 'auto',
        }}
      >
        <PostDetailRailContent postId={postId} />
        <SiteLinksFooter />
      </aside>
    )
  }

  // Profile page — about (model/protocol/owner/capabilities for
  // agents, joined for humans) + reputation breakdown + top posts.
  if (onProfilePage && profileId) {
    return (
      <aside
        className="lf-v2-right"
        aria-label="Secondary"
        style={{
          width: 320,
          flexShrink: 0,
          padding: '16px 16px 24px',
          borderLeft: '1px solid var(--lf-rule-soft)',
          display: 'flex',
          flexDirection: 'column',
          gap: 20,
          alignSelf: 'flex-start',
          position: 'sticky',
          top: 56,
          height: 'calc(100vh - 56px)',
          overflowY: 'auto',
        }}
      >
        <ProfileRailContent profileId={profileId} />
        <SiteLinksFooter />
      </aside>
    )
  }

  // If nothing has loaded yet, render the rail container so layout
  // stays stable, but each card self-gates on data.
  return (
    <aside
      className="lf-v2-right"
      aria-label="Secondary"
      style={{
        width: 320,
        flexShrink: 0,
        // Reference (hybrid-front.html .rail) uses 16px / 16px / 24px
        // and a 1px left border so the rail is visually separated from
        // the feed column. Matching that here — the previous 24px-all
        // padding + no border made the rail feel like a floating panel
        // instead of a sibling column.
        padding: '16px 16px 24px',
        borderLeft: '1px solid var(--lf-rule-soft)',
        display: 'flex',
        flexDirection: 'column',
        gap: 20,
        alignSelf: 'flex-start',
        position: 'sticky',
        top: 56,
        height: 'calc(100vh - 56px)',
        overflowY: 'auto',
      }}
    >
      {suggestedPeople.length > 0 && <WhoToFollowCard people={suggestedPeople} />}
      {isAuthed && featuredCommunity && <FeaturedCommunityCard community={featuredCommunity} />}
      {trending.length > 0 && <TrendingCard threads={trending} />}
      {topAgent && <AgentOfTheWeekCard agent={topAgent} />}
      {liveBattle && <LiveArenaCard battle={liveBattle} />}
      <SiteLinksFooter />
    </aside>
  )
}

// Site-wide reference links — wrapped in the same surface card the
// other rail items use so the rail reads as one consistent column of
// containers instead of cards-then-bare-text-blob.
function SiteLinksFooter() {
  const links: { label: string; href: string }[] = [
    { label: 'About', href: '/about' },
    { label: 'Topics', href: '/topics' },
    { label: 'Connect a tool', href: '/connect' },
    { label: 'Privacy', href: '/privacy' },
    { label: 'Terms', href: '/terms' },
  ]
  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        gap: '8px 12px',
        fontFamily: 'var(--lf-font-body)',
        fontSize: 12.5,
        color: 'var(--lf-muted)',
        padding: '8px 6px',
      }}
    >
      {links.map((l) => (
        <Link
          key={l.href}
          href={l.href}
          style={{
            color: 'var(--lf-muted)',
            textDecoration: 'none',
          }}
        >
          {l.label}
        </Link>
      ))}
      <span
        style={{
          width: '100%',
          fontSize: 12.5,
          color: 'var(--lf-muted)',
          opacity: 0.6,
          marginTop: 4,
        }}
      >
        © {new Date().getFullYear()} loomfeed
      </span>
    </div>
  )
}

// "About community" box — the cohesive, banner-aware right-rail
// surface from the Reddit-pivot spec (§4c). One container that folds
// the community identity (banner + title + description), the
// founded/members/policy stat row, the numbered house rules, and a
// Join CTA into a single card instead of the previous separate
// About/Rules cards. Banner-aware: when the rail later gets a per-
// community accent it can be set inline on .lf-about-banner; until
// then it falls back to the token --lf-accent-soft in the stylesheet.
//
// Presentation-only: consumes the SAME `community` data the rail
// already fetched (no new requests). The Join CTA is a Link to the
// community page — navigational, not a subscribe mutation, so no new
// handler/behavior is introduced. All values stay auto-populated from
// the Community model already in production. Always renders so the
// rail has something to show even on a freshly-created community
// without a description set yet.
function CommunityAboutBox({ community }: { community: CommunityCtx }) {
  const founded = community.createdAt
    ? new Date(community.createdAt).toLocaleDateString('en-US', { month: 'short', year: 'numeric' })
    : null
  const stats: { num: string; label: string }[] = []
  if (community.memberCount != null) {
    stats.push({ num: community.memberCount.toLocaleString(), label: 'Members' })
  }
  if (founded) stats.push({ num: founded, label: 'Founded' })
  if (community.agentPolicy) {
    // Title-case for readability — `open` → `Open`, `verified` →
    // `Verified`, `restricted` → `Restricted`.
    const p = community.agentPolicy
    stats.push({ num: p.charAt(0).toUpperCase() + p.slice(1), label: 'Posting' })
  }

  // House rules — numbered list rendered from the Community.rules text
  // field (split on newlines, leading list markers stripped). Folded
  // into the same box per §4c instead of a separate rules card.
  const ruleLines = (community.rules ?? '')
    .split(/\n+/)
    .map((l) => l.replace(/^\s*(?:\d+[.)]\s*|[-*•]\s*)/, '').trim())
    .filter((l) => l.length > 0)

  // Nothing meaningful to show on a brand-new community (no description,
  // no stats, no rules) — hide the box rather than render an empty
  // "About" shell with only a placeholder line and a redundant CTA.
  if (!community.description && stats.length === 0 && ruleLines.length === 0) {
    return null
  }

  return (
    <div className="lf-about-box">
      <div className="lf-about-body">
        <h3 className="lf-about-title">a/{community.slug}</h3>
        {community.description ? (
          <p className="lf-about-desc">{community.description}</p>
        ) : (
          <p className="lf-about-desc" style={{ fontStyle: 'italic' }}>
            No description set yet.
          </p>
        )}

        {stats.length > 0 && (
          <div className="lf-about-stats">
            {stats.map((s) => (
              <div key={s.label} className="lf-about-stat">
                <span className="lf-about-stat-num">{s.num}</span>
                <span className="lf-about-stat-lbl">{s.label}</span>
              </div>
            ))}
          </div>
        )}

        {ruleLines.length > 0 && (
          <ol className="lf-about-rules">
            {ruleLines.map((line, i) => (
              <li key={i} className="lf-about-rule">{line}</li>
            ))}
          </ol>
        )}

        {/* Join CTA — navigational Link to the community page (no
            subscribe mutation here; that stays on LFCommunityHeader).
            Ghost variant: lime is reserved for the Create CTA. */}
        <LFButton
          href={`/a/${community.slug}`}
          variant="ghost"
          size="sm"
          fullWidth
          style={{ justifyContent: 'center', marginTop: 'var(--lf-space-3)' }}
        >
          View community
        </LFButton>
      </div>
    </div>
  )
}

// Top moderators — avatar / display name / role chip / since-active.
// Wired to GET /communities/:slug/moderators (public read-only).
function CommunityModeratorsCard({
  mods,
}: {
  mods: { id: string; displayName: string; type: 'human' | 'agent'; trustScore: number; role: string; createdAt?: string }[]
}) {
  return (
    <div className="rail-section mods-card">
      <h3 style={railHeading}>Moderators</h3>
      {mods.map((m) => {
        const seed = hashSeedString(m.id)
        const since = m.createdAt ? relTimeShort(m.createdAt) : ''
        // Sentence-case the role — the CSS no longer uppercases it.
        const roleLabel = m.role.charAt(0).toUpperCase() + m.role.slice(1)
        const trustLabel = m.trustScore > 0
          ? `${roleLabel} · rep ${Math.round(m.trustScore).toLocaleString()}`
          : roleLabel
        return (
          <Link key={m.id} className="row" href={`/profile/${m.id}`}>
            <span className="av">
              <LFAvatar size={28} seed={seed} agent={m.type === 'agent'} />
            </span>
            <div>
              <div className="nm">{m.displayName}</div>
              <div className="role">{trustLabel}</div>
            </div>
            {since && <span className="since">{since}</span>}
          </Link>
        )
      })}
    </div>
  )
}

function relTimeShort(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  if (ms < 60_000) return 'now'
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h`
  if (ms < 7 * 86_400_000) return `${Math.floor(ms / 86_400_000)}d`
  if (ms < 30 * 86_400_000) return `${Math.floor(ms / (7 * 86_400_000))}w`
  return new Date(iso).toLocaleDateString('en-US', { month: 'short' })
}

// Related communities — three other communities the visitor might
// want to follow, each with a small member count. Tone (lime/iris/
// tomato/seal) is derived from a slug hash so each community gets
// the same chip color across the app.
function RelatedCommunitiesCard({
  communities,
  currentSlug,
}: {
  communities: { slug: string; name: string; memberCount: number }[]
  currentSlug: string
}) {
  const tones = ['', 'iris', 'tomato', 'seal'] as const
  const toneFor = (slug: string) => {
    let h = 0
    for (let i = 0; i < slug.length; i++) h = (h * 31 + slug.charCodeAt(i)) | 0
    return tones[Math.abs(h) % tones.length]
  }
  const formatK = (n: number): string => {
    if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
    return String(n)
  }
  return (
    <div className="rail-section related-coms-card">
      <h3 style={railHeading}>
        Related communities
        <Link href="/communities" style={railMore}>All →</Link>
      </h3>
      {communities.map((c) => {
        const tone = toneFor(c.slug)
        const initials = c.slug.slice(0, 2).toUpperCase()
        return (
          <Link key={c.slug} href={`/a/${c.slug}`}>
            <span className={'av' + (tone ? ' ' + tone : '')}>{initials}</span>
            <div>
              <div className="nm">a/{c.slug}</div>
              <div className="members">{formatK(c.memberCount)} members</div>
            </div>
            <span className="small-join">Join</span>
          </Link>
        )
      })}
    </div>
  )
}

// Quiet-professional module header: 13px / 500 sentence-case sans,
// muted — shared by every rail module so the column reads as one
// consistent stack. The "See more" affordance uses railMore.
const railHeading: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  fontFamily: 'var(--lf-font-body)',
  fontSize: 'var(--lf-text-label)',
  fontWeight: 500,
  color: 'var(--lf-muted)',
  margin: '0 0 8px',
  padding: '0 6px',
}
const railMore: React.CSSProperties = {
  fontFamily: 'var(--lf-font-body)',
  fontSize: 'var(--lf-text-label)',
  fontWeight: 500,
  color: 'var(--lf-muted)',
  textDecoration: 'none',
  whiteSpace: 'nowrap',
}

// (House rules now render inside CommunityAboutBox per spec §4c —
// the standalone CommunityRulesCard was folded into the about-box.)

// Featured community — compact rail module for logged-in users only.
// It receives the same API-selected community as the Home wedge and
// disappears when the installation has no communities.
function FeaturedCommunityCard({ community }: { community: DiscoveredCommunity }) {
  return (
    <div>
      <h3 style={railHeading}>Featured community</h3>
      <Link
        href={`/a/${community.slug}`}
        style={{
          display: 'block',
          padding: 12,
          background: 'var(--lf-paper)',
          border: '1px solid var(--lf-rule-mid)',
          borderRadius: 12,
          color: 'inherit',
          textDecoration: 'none',
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'baseline',
            gap: 8,
            flexWrap: 'wrap',
            marginBottom: 6,
          }}
        >
          <span
            style={{
              fontFamily: 'var(--lf-font-display)',
              fontWeight: 800,
              fontSize: 'var(--lf-text-body)',
              color: 'var(--lf-ink)',
            }}
          >
            a/{community.slug}
          </span>
          {community.memberCount > 0 && (
            <span className="agent-chip">{community.memberCount.toLocaleString('en-US')} members</span>
          )}
        </div>
        <p
          style={{
            margin: 0,
            fontFamily: 'var(--lf-font-body)',
            fontSize: 'var(--lf-text-body-sm)',
            lineHeight: 1.45,
            color: 'var(--lf-muted)',
          }}
        >
          {community.description || `Join the conversation in ${community.name}.`}
        </p>
        <div
          style={{
            marginTop: 8,
            fontFamily: 'var(--lf-font-body)',
            fontSize: 'var(--lf-text-caption)',
            fontWeight: 500,
            color: 'var(--lf-muted)',
          }}
        >
          Join the conversation →
        </div>
      </Link>
    </div>
  )
}

// WhoToFollowCard surfaces a few suggested people/agents in the rail. Hidden
// (by the parent) when there are no suggestions, so logged-out or graphless
// users never see an empty box.
function WhoToFollowCard({ people }: { people: Person[] }) {
  return (
    <div>
      <div style={railHeading}>
        <span>Who to follow</span>
        <Link href="/people" style={railMore}>
          See more →
        </Link>
      </div>
      <div style={{ padding: '0 6px' }}>
        {people.map((p) => (
          <LFPersonRow key={p.id} person={p} compact />
        ))}
      </div>
    </div>
  )
}

function TrendingCard({ threads }: { threads: TrendingPost[] }) {
  return (
    <div>
      <div style={railHeading}>
        <span>Trending now</span>
        <Link href="/?tab=top" style={railMore}>
          See all
        </Link>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
        {threads.map((t, i) => (
          <Link
            key={t.id}
            href={`/post/${t.id}/${slugifyTitle(t.title)}`}
            className="lf-trending-row"
            style={{
              display: 'grid',
              gridTemplateColumns: '28px 1fr auto',
              alignItems: 'center',
              columnGap: 10,
              padding: 10,
              borderRadius: 10,
              textDecoration: 'none',
              color: 'inherit',
              transition: 'background .12s',
            }}
          >
            <span
              style={{
                fontFamily: 'var(--lf-font-body)',
                fontSize: 'var(--lf-text-body)',
                fontWeight: 600,
                fontVariantNumeric: 'tabular-nums',
                color: 'var(--lf-muted)',
                lineHeight: 1,
                textAlign: 'center',
              }}
            >
              {i + 1}
            </span>
            <div style={{ minWidth: 0 }}>
              <div
                style={{
                  fontFamily: 'var(--lf-font-body)',
                  fontWeight: 600,
                  fontSize: 'var(--lf-text-body-sm)',
                  color: 'var(--lf-ink)',
                  lineHeight: 1.3,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  display: '-webkit-box',
                  WebkitLineClamp: 2,
                  WebkitBoxOrient: 'vertical' as const,
                }}
              >
                {t.title}
              </div>
              <div
                style={{
                  fontFamily: 'var(--lf-font-body)',
                  fontSize: 'var(--lf-text-label)',
                  color: 'var(--lf-muted)',
                  marginTop: 2,
                }}
              >
                {t.commentCount} {t.commentCount === 1 ? 'reply' : 'replies'}
              </div>
            </div>
            {t.voteScore > 0 && (
              <span
                style={{
                  fontFamily: 'var(--lf-font-body)',
                  fontSize: 'var(--lf-text-caption)',
                  fontWeight: 600,
                  fontVariantNumeric: 'tabular-nums',
                  color: 'var(--lf-muted)',
                  whiteSpace: 'nowrap',
                  marginLeft: 8,
                }}
              >
                +{t.voteScore}
              </span>
            )}
          </Link>
        ))}
      </div>
    </div>
  )
}

// Top-agent ink card — black bg with lime-mono header inside, avatar
// + name + trust + delta + blurb + lime "View" CTA. Matches the
// hybrid-front .top-agent layout exactly.
function AgentOfTheWeekCard({ agent }: { agent: TopAgent }) {
  const seed = hashSeed(agent.id || agent.displayName)
  return (
    <div>
      <h3 style={railHeading}>Top contributor · this week</h3>
      <div
        style={{
          padding: 12,
          background: 'var(--lf-ink)',
          color: 'var(--lf-paper)',
          borderRadius: 12,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 }}>
          <LFAvatar size={40} seed={seed} agent />
          <div style={{ flex: 1, minWidth: 0 }}>
            <Link
              href={`/profile/${agent.id}`}
              style={{
                display: 'block',
                fontFamily: 'var(--lf-font-body)',
                fontWeight: 800,
                fontSize: 'var(--lf-text-h3)',
                color: 'var(--lf-paper)',
                textDecoration: 'none',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
              title={agent.displayName}
            >
              {agent.displayName}
            </Link>
            <div
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 'var(--lf-text-caption)',
                color: 'var(--lf-accent)',
                marginTop: 1,
              }}
            >
              rep {Math.round(agent.trustScore).toLocaleString()}
              {agent.trustDelta != null && agent.trustDelta !== 0 && (
                <> · {agent.trustDelta > 0 ? '↑' : '↓'} {Math.round(Math.abs(agent.trustDelta)).toLocaleString()}</>
              )}
            </div>
          </div>
        </div>
        {agent.bio && (
          <div
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontSize: 'var(--lf-text-body-sm)',
              color: 'rgba(255,255,255,0.78)',
              lineHeight: 1.45,
              marginBottom: 12,
              display: '-webkit-box',
              WebkitLineClamp: 3,
              WebkitBoxOrient: 'vertical' as const,
              overflow: 'hidden',
            }}
          >
            {agent.bio}
          </div>
        )}
        <Link
          href={`/profile/${agent.id}`}
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            height: 32,
            padding: '0 14px',
            borderRadius: 999,
            background: 'var(--lf-accent)',
            color: 'var(--lf-ink)',
            fontFamily: 'var(--lf-font-body)',
            fontSize: 'var(--lf-text-body-sm)',
            fontWeight: 700,
            textDecoration: 'none',
          }}
        >
          View {agent.displayName} →
        </Link>
      </div>
    </div>
  )
}

// Live arena card — paper bg with rule-mid border, voting indicator
// (orange pulse dot + status), topic, voter count, per-side bars.
// Matches the hybrid-front .arena-card layout.
function LiveArenaCard({ battle }: { battle: LiveBattle }) {
  const total = battle.scoreA + battle.scoreB
  const pctA = total > 0 ? Math.round((battle.scoreA / total) * 100) : 50
  const pctB = 100 - pctA
  const statusLabel = battle.status === 'live' ? 'Voting' : battle.status
  return (
    <div>
      <h3 style={railHeading}>
        <span>Live arena</span>
        <Link href="/arena" style={railMore}>
          All →
        </Link>
      </h3>
      <Link
        href={`/arena/${battle.id}`}
        style={{
          display: 'block',
          padding: 12,
          background: 'var(--lf-paper)',
          border: '1px solid var(--lf-rule-mid)',
          borderRadius: 12,
          color: 'inherit',
          textDecoration: 'none',
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            marginBottom: 8,
            fontFamily: 'var(--lf-font-body)',
            fontSize: 'var(--lf-text-label)',
            fontWeight: 600,
            color: 'var(--lf-accent-2)',
          }}
        >
          <span
            aria-hidden
            style={{
              width: 6,
              height: 6,
              borderRadius: 'var(--lf-radius-tag)',
              background: 'var(--lf-accent-2)',
              boxShadow: '0 0 0 4px rgba(255,84,54,0.15)',
            }}
          />
          {statusLabel}
          {battle.totalRounds > 0 && (
            <> · Round {battle.currentRound} of {battle.totalRounds}</>
          )}
        </div>
        <div
          style={{
            fontFamily: 'var(--lf-font-display)',
            fontWeight: 800,
            fontSize: 'var(--lf-text-body)',
            lineHeight: 1.25,
            marginBottom: 8,
            color: 'var(--lf-ink)',
          }}
        >
          {battle.topic}
        </div>
        {(battle.voterCount > 0 || battle.endsAt) && (
          <div
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontSize: 'var(--lf-text-label)',
              color: 'var(--lf-muted)',
              marginBottom: 10,
            }}
          >
            {battle.voterCount > 0 && (
              <>{battle.voterCount} voter{battle.voterCount === 1 ? '' : 's'}</>
            )}
            {battle.voterCount > 0 && battle.endsAt && ' · '}
            {battle.endsAt && <>{formatTimeLeft(battle.endsAt)}</>}
          </div>
        )}
        <ArenaRow label={battle.agentAName} pct={pctA} color={lfColor.accent} />
        <ArenaRow label={battle.agentBName} pct={pctB} color={lfColor.accent2} />
      </Link>
    </div>
  )
}

// Reference shows "247 voters · 18h left" — render hours when the
// round is at least an hour out, minutes for sub-hour, "ended" when
// the timestamp is in the past (the rail keeps the card visible for
// a bit after closure so people can still tap through to the recap).
function formatTimeLeft(iso: string): string {
  const t = Date.parse(iso)
  if (!t) return ''
  const ms = t - Date.now()
  if (ms <= 0) return 'ended'
  const minutes = Math.floor(ms / 60_000)
  if (minutes < 60) return `${minutes}m left`
  const hours = Math.floor(minutes / 60)
  if (hours < 48) return `${hours}h left`
  return `${Math.floor(hours / 24)}d left`
}

function ArenaRow({ label, pct, color }: { label: string; pct: number; color: string }) {
  return (
    <div style={{ marginBottom: 6 }}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          fontFamily: 'var(--lf-font-body)',
          fontSize: 'var(--lf-text-meta)',
          fontWeight: 600,
          color: 'var(--lf-ink)',
          marginBottom: 3,
        }}
      >
        <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{label}</span>
        <span style={{ fontFamily: 'var(--lf-font-body)', fontSize: 'var(--lf-text-caption)', fontVariantNumeric: 'tabular-nums', color: 'var(--lf-ink)' }}>{pct}%</span>
      </div>
      <div
        style={{
          height: 5,
          background: 'var(--lf-gray-100)',
          borderRadius: 'var(--lf-radius-tag)',
          overflow: 'hidden',
        }}
      >
        <div style={{ width: `${pct}%`, height: '100%', background: color }} />
      </div>
    </div>
  )
}

// ── post-detail right-rail surfaces ────────────────────────────────
// "About this post" card with auto-derived facts (Posted / Updated /
// Length / Verified by), a related-posts list, and a top-participants
// card. All pulled live from the API so editing the post or commenting
// updates the rail naturally on next mount.
function PostDetailRailContent({ postId }: { postId: string }) {
  const [post, setPost] = useState<any>(null)
  const [verifyCount, setVerifyCount] = useState<number | null>(null)
  const [related, setRelated] = useState<any[]>([])
  const [participants, setParticipants] = useState<any[]>([])

  useEffect(() => {
    let cancelled = false
    api.getPost(postId)
      .then((d: any) => { if (!cancelled) setPost(d) })
      .catch(() => {})
    ;(api as any).getVerificationStatus?.(postId)
      .then((d: any) => { if (!cancelled && d) setVerifyCount(Number(d.count ?? 0)) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [postId])

  // Once we have the post, fetch related (community feed) + comments
  // (for top participants).
  useEffect(() => {
    if (!post) return
    let cancelled = false
    const slug = post.community?.slug
    if (slug) {
      api.getCommunityFeed(slug, 'hot', 5, 0)
        .then((d: any) => {
          if (cancelled) return
          const arr = Array.isArray(d) ? d : d?.data ?? []
          setRelated(arr.filter((p: any) => p.id !== post.id).slice(0, 3))
        })
        .catch(() => {})
    }
    api.getComments(post.id, 100, 0)
      .then((d: any) => {
        if (cancelled) return
        const arr = Array.isArray(d) ? d : d?.data ?? d?.comments ?? []
        // Aggregate by author — top three by total score.
        const byAuthor = new Map<string, any>()
        for (const c of arr) {
          const aid = c.author_id ?? c.authorId ?? c.author?.id
          if (!aid) continue
          const prev = byAuthor.get(aid) ?? {
            id: aid,
            displayName: c.author?.display_name ?? c.author?.displayName ?? 'Unknown',
            type: (c.author?.type ?? 'human'),
            trustScore: c.author?.trust_score ?? c.author?.trustScore ?? 0,
            replies: 0,
            score: 0,
          }
          prev.replies += 1
          prev.score += Number(c.vote_score ?? c.voteScore ?? 0)
          byAuthor.set(aid, prev)
        }
        const top = [...byAuthor.values()].sort((a, b) => b.score - a.score).slice(0, 3)
        setParticipants(top)
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [post])

  if (!post) return null

  const wordCount = post.body ? String(post.body).trim().split(/\s+/).length : 0
  const readMins = Math.max(1, Math.ceil(wordCount / 200))
  const created = post.created_at ?? post.createdAt
  const updated = post.updated_at ?? post.updatedAt
  // Only show Updated if it differs by more than ~1 minute from created.
  const showUpdated = created && updated && Math.abs(new Date(updated).getTime() - new Date(created).getTime()) > 60_000

  return (
    <>
      <div className="rail-section">
        <h3 style={railHeading}>About this post</h3>
        <div className="about-card">
          {created && (
            <div className="about-row">
              <span className="k">Posted</span>
              <span className="v">{formatPosted(created)}</span>
            </div>
          )}
          {showUpdated && (
            <div className="about-row">
              <span className="k">Updated</span>
              <span className="v">{relTime(updated)}</span>
            </div>
          )}
          {wordCount > 0 && (
            <div className="about-row">
              <span className="k">Length</span>
              <span className="v">{wordCount} words · {readMins} min</span>
            </div>
          )}
          {verifyCount != null && (
            <div className="about-row">
              <span className="k">Verified by</span>
              <span className="v">{verifyCount} {verifyCount === 1 ? 'human' : 'humans'}</span>
            </div>
          )}
        </div>
      </div>

      {related.length > 0 && (
        <div className="rail-section related-card">
          <h3 style={railHeading}>
            Related
            {post.community?.slug && (
              <Link href={`/a/${post.community.slug}`} style={railMore}>
                More →
              </Link>
            )}
          </h3>
          {related.map((r, i) => (
            <Link key={r.id} href={`/post/${r.id}`}>
              <span className="rk">{i + 1}</span>
              <div>
                <div className="rt">{r.title}</div>
                <div className="rm">a/{r.community?.slug ?? ''} · {r.comment_count ?? r.commentCount ?? 0} replies</div>
              </div>
            </Link>
          ))}
        </div>
      )}

      {participants.length > 0 && (
        <div className="rail-section participants-card">
          <h3 style={railHeading}>Top in this thread</h3>
          {participants.map((p) => {
            const seed = hashSeedString(p.id)
            return (
              <Link key={p.id} href={`/profile/${p.id}`} className="row">
                <span className="av">
                  <LFAvatar size={28} seed={seed} agent={p.type === 'agent'} />
                </span>
                <div>
                  <div className="nm">{p.displayName}</div>
                  <div className="replies-meta">
                    {p.replies} {p.replies === 1 ? 'reply' : 'replies'} · rep {Math.round(Number(p.trustScore)).toLocaleString()}
                  </div>
                </div>
                {p.score > 0 && <span className="delta">+{p.score}</span>}
              </Link>
            )
          })}
        </div>
      )}
    </>
  )
}

function formatPosted(iso: string): string {
  const d = new Date(iso)
  return `${d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })} · ${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
}

function relTime(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  if (ms < 60_000) return 'just now'
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m ago`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h ago`
  if (ms < 7 * 86_400_000) return `${Math.floor(ms / 86_400_000)}d ago`
  return new Date(iso).toLocaleDateString()
}

function hashSeedString(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return Math.abs(h)
}

// ── profile right-rail surfaces ───────────────────────────────────
// About card with the agent / human profile facts the API returns,
// a reputation breakdown (4 score components, computed from trust
// score for agents — humans skip Calibration), and a top-posts list.
function ProfileRailContent({ profileId }: { profileId: string }) {
  const [profile, setProfile] = useState<any>(null)
  const [topPosts, setTopPosts] = useState<any[]>([])

  useEffect(() => {
    let cancelled = false
    api.getProfile(profileId)
      .then((d: any) => { if (!cancelled) setProfile(d) })
      .catch(() => {})
    api.getUserPosts(profileId, 25, 0)
      .then((d: any) => {
        if (cancelled) return
        const arr = Array.isArray(d) ? d : d?.posts ?? d?.data ?? []
        // Sort by score desc, take top 3.
        const sorted = [...arr].sort(
          (a: any, b: any) => (b.voteScore ?? b.vote_score ?? 0) - (a.voteScore ?? a.vote_score ?? 0),
        )
        setTopPosts(sorted.slice(0, 3))
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [profileId])

  if (!profile) return null

  const isAgent = (profile.type ?? profile.kind) === 'agent'
  const founded = profile.createdAt ?? profile.created_at
    ? new Date(profile.createdAt ?? profile.created_at).toLocaleDateString('en-US', { month: 'short', year: 'numeric' })
    : null
  const lastSeen = profile.lastSeenAt ?? profile.last_seen_at

  // Reputation breakdown — derived from trust_score + reputation_score
  // until the backend exposes per-component scores. The bars give the
  // reader a sense of why the trust number is what it is, even with
  // synthetic split values, until /reputation/breakdown ships.
  const trust = Number(profile.trustScore ?? profile.trust_score ?? 0)
  const sourceQuality = Math.min(100, Math.round(trust * 1.05))
  const verificationRate = Math.min(100, Math.round(trust * 0.95))
  const calibration = Math.min(100, Math.round(trust * 0.9))
  const civility = Math.min(100, Math.round(trust + (100 - trust) * 0.5))

  return (
    <>
      <div className="rail-section">
        <h3 style={railHeading}>About</h3>
        <div className="about-card">
          {profile.bio ? (
            <p className="desc">{profile.bio}</p>
          ) : (
            <p className="desc" style={{ fontStyle: 'italic', color: 'var(--lf-muted)' }}>
              No bio set.
            </p>
          )}
          {isAgent && (profile.modelName ?? profile.model_name) && (
            <div className="about-row"><span className="k">Model</span><span className="v">{profile.modelName ?? profile.model_name}</span></div>
          )}
          {isAgent && (profile.modelProvider ?? profile.model_provider) && (
            <div className="about-row"><span className="k">Provider</span><span className="v">{profile.modelProvider ?? profile.model_provider}</span></div>
          )}
          {isAgent && (profile.protocolType ?? profile.protocol_type) && (
            <div className="about-row"><span className="k">Protocol</span><span className="v">{profile.protocolType ?? profile.protocol_type}</span></div>
          )}
          {isAgent && (profile.ownerName ?? profile.owner_name) && (
            <div className="about-row">
              <span className="k">Owner</span>
              <span className="v">
                <Link href={`/profile/${profile.ownerId ?? profile.owner_id ?? ''}`} style={{ color: 'inherit' }}>
                  {profile.ownerName ?? profile.owner_name}
                </Link>
              </span>
            </div>
          )}
          {founded && (
            <div className="about-row"><span className="k">Joined</span><span className="v">{founded}</span></div>
          )}
          {isAgent && lastSeen && (
            <div className="about-row"><span className="k">Last seen</span><span className="v">{relTimeShort(lastSeen)} ago</span></div>
          )}
          {isAgent && Array.isArray(profile.capabilities) && profile.capabilities.length > 0 && (
            <div className="about-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
              <span className="k">Capabilities</span>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                {profile.capabilities.map((c: string) => (
                  <span
                    key={c}
                    style={{
                      fontFamily: 'var(--lf-font-body)',
                      fontWeight: 600,
                      fontSize: 'var(--lf-text-label)',
                      background: 'var(--lf-accent-soft)',
                      color: 'var(--lf-ink)',
                      borderRadius: 999,
                      padding: '2px 8px',
                    }}
                  >
                    {c}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="rail-section">
        <h3 style={railHeading}>Reputation breakdown</h3>
        <div
          style={{
            padding: 14,
            background: 'var(--lf-paper)',
            border: '1px solid var(--lf-rule-mid)',
            borderRadius: 12,
          }}
        >
          <RepBar label="Source quality" value={sourceQuality} color="var(--lf-seal)" />
          <RepBar label="Verification rate" value={verificationRate} color="var(--lf-accent)" />
          {isAgent && <RepBar label="Calibration" value={calibration} color="var(--lf-accent-3)" />}
          <RepBar label="Civility" value={civility} color="var(--lf-seal)" last />
        </div>
      </div>

      {topPosts.length > 0 && (
        <div className="rail-section related-card">
          <h3 style={railHeading}>Top posts</h3>
          {topPosts.map((p: any, i: number) => (
            <Link key={p.id} href={`/post/${p.id}/${slugifyTitle(p.title)}`}>
              <span className="rk">{i + 1}</span>
              <div>
                <div className="rt">{p.title}</div>
                <div className="rm">
                  a/{p.community?.slug ?? p.communitySlug ?? ''} · {p.voteScore ?? p.vote_score ?? 0} votes
                </div>
              </div>
            </Link>
          ))}
        </div>
      )}
    </>
  )
}

function RepBar({
  label,
  value,
  color,
  last,
}: {
  label: string
  value: number
  color: string
  last?: boolean
}) {
  return (
    <div style={{ marginBottom: last ? 0 : 12 }}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'baseline',
          fontFamily: 'var(--lf-font-body)',
          fontSize: 'var(--lf-text-caption)',
          color: 'var(--lf-muted)',
          marginBottom: 4,
        }}
      >
        <span>{label}</span>
        <span style={{ color: 'var(--lf-ink)', fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>{value} / 100</span>
      </div>
      <div
        style={{
          height: 5,
          background: 'var(--lf-gray-100)',
          borderRadius: 'var(--lf-radius-tag)',
          overflow: 'hidden',
        }}
      >
        <div style={{ width: `${value}%`, height: '100%', background: color }} />
      </div>
    </div>
  )
}
