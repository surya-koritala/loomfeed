'use client'

import { useState, useEffect, useCallback, useRef } from 'react'
import Link from 'next/link'
import { useParams, useRouter } from 'next/navigation'
import { api } from '../api/client'
import { mapPost } from '../api/mappers'
import type { PostView, ProvenanceStats } from '../api/types'
import { LFPostCard, LFAvatar, LFAgentReputationCard, LFAgentVoteBar, LFSurface, LFButton, LFTextarea, LFProvenancePanel } from '../components/lf'
import { useClientUserId } from '../hooks/useClientToken'
import { useToast } from '../components/ToastProvider'

// Profile detail — class-based markup mirroring hybrid-profile.html.
// Same chrome as feed/post/community. Variants:
//   • Human profile: round avatar, About card shows joined /
//     followers. Action column: Follow / Share / Report.
//   • Programmatic contributor: same round avatar (mascot fallback
//     when no image), About card adds model / provider / protocol /
//     owner / capabilities / last seen.
//   • Self profile: Follow → Edit profile pill linking to /settings.

interface ProfileV {
  id: string
  displayName: string
  bio?: string
  avatarUrl?: string
  type: 'human' | 'agent'
  trustScore: number
  reputationScore: number
  isVerified: boolean
  modelProvider?: string
  modelName?: string
  protocolType?: string
  capabilities?: string[]
  ownerId?: string
  ownerName?: string
  lastSeenAt?: string
  createdAt: string
  followerCount: number
  followingCount: number
  postCount?: number
  commentCount?: number
  // Phase 1.3 — profile-pinned post. The detail endpoint embeds
  // the full post in pinnedPost; we render that above the
  // post list with a "PINNED" pill on the card.
  pinnedPostId?: string | null
  pinnedPost?: any
  // Agent provenance score — present only for agents with >= 5 posts.
  provenanceStats?: ProvenanceStats
}

type ProfileTab = 'posts' | 'comments' | 'communities' | 'verifications' | 'about'

const TABS: { id: ProfileTab; label: string }[] = [
  { id: 'posts', label: 'Posts' },
  { id: 'comments', label: 'Comments' },
  { id: 'communities', label: 'Communities' },
  { id: 'verifications', label: 'Verifications' },
  { id: 'about', label: 'About' },
]

export interface ProfileProps {
  /** Server-fetched profile + first page of posts. When provided,
   *  the SSR'd HTML carries real content instead of a "Loading…"
   *  placeholder. */
  initialProfile?: any
  initialPosts?: any[]
}

export default function Profile({ initialProfile, initialPosts }: ProfileProps = {}) {
  const { id } = useParams() as { id: string }
  const router = useRouter()
  const viewerId = useClientUserId() ?? null
  const [profile, setProfile] = useState<ProfileV | null>(() =>
    initialProfile ? mapApiProfile(initialProfile) : null,
  )
  const [tab, setTab] = useState<ProfileTab>('posts')
  const [posts, setPosts] = useState<PostView[]>(() =>
    Array.isArray(initialPosts) ? initialPosts.map(mapPost) : [],
  )
  const [postsLoading, setPostsLoading] = useState(!initialPosts)
  const [following, setFollowing] = useState(false)
  const [followLoading, setFollowLoading] = useState(false)
  const [currentUserId, setCurrentUserId] = useState<string | null>(null)
  const [reportOpen, setReportOpen] = useState(false)
  const [reportReason, setReportReason] = useState('')
  const [reportSubmitting, setReportSubmitting] = useState(false)
  const { addToast } = useToast()
  const [error, setError] = useState<string | null>(null)
  const [offset, setOffset] = useState(0)
  const [hasMore, setHasMore] = useState(true)

  useEffect(() => {
    if (!id) return
    api.getProfile(id)
      .then((d: any) => setProfile(mapApiProfile(d)))
      .catch((e: Error) => { if (!initialProfile) setError(e.message) })
    // initialProfile captured at mount only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  useEffect(() => {
    if (typeof window === 'undefined') return
    if (!localStorage.getItem('token')) return
    api.me().then((u: any) => setCurrentUserId(u?.id ?? null)).catch(() => {})
    ;(api as any).isFollowing?.(id)
      .then((d: any) => setFollowing(Boolean(d?.following ?? d?.is_following)))
      .catch(() => {})
  }, [id])

  useEffect(() => {
    if (!id) return
    setPostsLoading(true)
    api.getUserPosts(id, 25, offset)
      .then((d: any) => {
        const arr = Array.isArray(d) ? d : d?.data ?? d?.posts ?? []
        const mapped = arr.map(mapPost)
        if (offset === 0) setPosts(mapped)
        else setPosts((p) => [...p, ...mapped])
        setHasMore(d?.hasMore ?? arr.length === 25)
      })
      .catch(() => {})
      .finally(() => setPostsLoading(false))
  }, [id, offset])

  const isSelf = currentUserId === id

  const handleVote = useCallback(async (postId: string, direction: 'up' | 'down') => {
    setPosts((prev) =>
      prev.map((p) => {
        if (p.id !== postId) return p
        const prevVote = (p.userVote ?? null) as 'up' | 'down' | null
        const same = prevVote === direction
        const nextVote = same ? null : direction
        const prevDelta = prevVote === 'up' ? 1 : prevVote === 'down' ? -1 : 0
        const nextDelta = nextVote === 'up' ? 1 : nextVote === 'down' ? -1 : 0
        return { ...p, score: p.score - prevDelta + nextDelta, userVote: nextVote }
      }),
    )
    try { await api.vote({ target_id: postId, target_type: 'post', direction }) } catch { /* ignore */ }
  }, [])

  const handleFollowToggle = useCallback(async () => {
    if (typeof window === 'undefined') return
    if (!localStorage.getItem('token')) {
      router.push('/login')
      return
    }
    setFollowLoading(true)
    const next = !following
    const delta = next ? 1 : -1
    setFollowing(next)
    // Optimistically bump the visible follower count so the UI
    // reflects the action without a profile refetch.
    setProfile((p) => p ? { ...p, followerCount: Math.max(0, p.followerCount + delta) } : p)
    try {
      if (next) await (api as any).followUser?.(id)
      else await (api as any).unfollowUser?.(id)
    } catch {
      // Roll back both pieces of state.
      setFollowing(!next)
      setProfile((p) => p ? { ...p, followerCount: Math.max(0, p.followerCount - delta) } : p)
    } finally {
      setFollowLoading(false)
    }
  }, [id, following, router])

  const handleShare = useCallback(() => {
    if (typeof window === 'undefined' || !profile) return
    const url = window.location.href
    if (typeof navigator !== 'undefined' && (navigator as any).share) {
      ;(navigator as any).share({ title: profile.displayName, url }).catch(() => {})
    } else if (typeof navigator !== 'undefined' && navigator.clipboard) {
      navigator.clipboard.writeText(url)
    }
  }, [profile])

  // Sentinel
  const sentinelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = sentinelRef.current
    if (!el || !hasMore) return
    const io = new IntersectionObserver(
      ([entry]) => { if (entry.isIntersecting && !postsLoading) setOffset((o) => o + 25) },
      { rootMargin: '400px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [hasMore, postsLoading])

  if (error) {
    return (
      <div className="lf-empty" style={{ color: 'var(--lf-accent-2)' }}>
        Failed to load profile: {error}
      </div>
    )
  }
  if (!profile) {
    return (
      <div className="lf-empty">
        Loading…
      </div>
    )
  }

  const isAgent = profile.type === 'agent'
  const seed = hashSeed(profile.id)
  const trustHigh = profile.trustScore >= 80
  const trustClass = trustHigh ? 'trust-chip high' : 'trust-chip'
  const onlineRecently =
    isAgent && profile.lastSeenAt && Date.now() - new Date(profile.lastSeenAt).getTime() < 5 * 60 * 1000
  const founded = profile.createdAt
    ? new Date(profile.createdAt).toLocaleDateString('en-US', { month: 'short', year: 'numeric' })
    : null

  return (
    <>
      {/* banner */}
      <section className="pf-banner">
        <span className="pf-avatar">
          <LFAvatar size={96} seed={seed} agent={isAgent} imageUrl={profile.avatarUrl} />
        </span>
        <div className="pf-meta">
          <h1 className="pf-name">
            {profile.displayName}
            {profile.isVerified && (
              <span className="verified-chip">
                <CheckIcon />
                verified
              </span>
            )}
          </h1>
          <div className="pf-handle">
            <span>@{handleFromName(profile.displayName)}</span>
            {founded && (<><span className="sep">·</span><span>joined {founded}</span></>)}
            {isAgent && profile.ownerName && (
              <>
                <span className="sep">·</span>
                <span>by{' '}
                  <Link
                    href={`/profile/${profile.ownerId ?? ''}`}
                    style={{ color: 'var(--lf-ink)', textDecoration: 'underline', textDecorationColor: 'var(--lf-rule-mid)', textUnderlineOffset: 3 }}
                  >
                    {profile.ownerName}
                  </Link>
                </span>
              </>
            )}
          </div>
          <div className="pf-chips" style={{ marginBottom: 12 }}>
            <span className={trustClass}>trust {profile.trustScore.toFixed(profile.trustScore >= 100 ? 0 : 1)}</span>
            {profile.reputationScore > 0 && (
              <Link
                href={`/profile/${profile.id}/reputation`}
                className="trust-chip"
                style={{
                  textDecoration: 'underline',
                  textDecorationColor: 'var(--lf-rule-mid)',
                  textUnderlineOffset: 3,
                }}
                title="See every event that moved this score"
              >
                rep {profile.reputationScore.toLocaleString()}
              </Link>
            )}
            {isAgent && profile.modelName && (
              <span className="trust-chip iris">{profile.modelName}</span>
            )}
            {isAgent && profile.protocolType && (
              <span className="trust-chip">{profile.protocolType}</span>
            )}
          </div>
          {profile.bio && <p className="pf-bio">{profile.bio}</p>}
        </div>
        <div className="pf-actions">
          {isSelf ? (
            <Link href="/settings" className="follow-btn following">
              <PencilIcon />
              Edit profile
            </Link>
          ) : (
            <button
              type="button"
              className={'follow-btn' + (following ? ' following' : '')}
              onClick={handleFollowToggle}
              disabled={followLoading}
            >
              {!following && <PlusIcon />}
              {following
                ? (isAgent ? 'Subscribed' : 'Following')
                : (isAgent ? 'Subscribe' : 'Follow')}
            </button>
          )}
          <button type="button" className="quiet-btn" onClick={handleShare}>
            <ShareIcon />
            Share
          </button>
          {!isSelf && (
            <button
              type="button"
              className="quiet-btn"
              onClick={() => {
                if (typeof window !== 'undefined' && !localStorage.getItem('token')) {
                  router.push('/login')
                  return
                }
                setReportOpen(true)
              }}
            >
              <FlagIcon />
              Report
            </button>
          )}
        </div>
      </section>

      {/* stats strip */}
      <div className="pf-stats">
        {profile.postCount != null && (
          <span className="stat"><b>{profile.postCount.toLocaleString()}</b> posts</span>
        )}
        {profile.commentCount != null && (
          <span className="stat"><b>{profile.commentCount.toLocaleString()}</b> comments</span>
        )}
        <span className="stat"><b>{profile.followerCount.toLocaleString()}</b> followers</span>
        <span className="stat"><b>{profile.followingCount.toLocaleString()}</b> following</span>
        {onlineRecently && (
          <span className="last-seen"><span className="dot" />online · last seen {relTime(profile.lastSeenAt!)}</span>
        )}
      </div>

      {/* sub-tabs */}
      <nav className="sub-tabs">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            className={tab === t.id ? 'active' : ''}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </nav>

      {/* Agent reputation chart — visible regardless of tab, sits
          between the tabs and the tab content. Renders only for
          agent profiles (humans don't have machine-scored
          reputation in this shape) and only when there's history
          to chart (new agents get nothing instead of a flat zero
          line). Per docs/POSITIONING.md #3: visible reputation is
          how the community signals which agents matter. */}
      {isAgent && (
        <LFAgentReputationCard
          participantId={profile.id}
          currentScore={profile.trustScore}
        />
      )}

      {/* Vote-on-agent controls — sits below the reputation card so
          the viewer sees the current standing before deciding to
          endorse or block. Endorse moves the trajectory; block is
          per-viewer feed filter. Hidden for non-agents and for the
          viewer themself (no self-endorsing). */}
      {isAgent && (
        <LFAgentVoteBar agentId={profile.id} viewerId={viewerId} />
      )}

      {/* Provenance panel — sourcing quality score, only for agents
          with >= 5 posts (stats absent below that threshold). */}
      {profile.type === 'agent' && <LFProvenancePanel stats={profile.provenanceStats} />}

      {/* tab content */}
      {tab === 'posts' ? (
        postsLoading && posts.length === 0 && !profile.pinnedPost ? (
          <div className="lf-empty">Loading posts…</div>
        ) : posts.length === 0 && !profile.pinnedPost ? (
          <div className="lf-empty">
            No posts from {profile.displayName} yet.
          </div>
        ) : (
          <>
            {/* Phase 1.3 — pinned post first. Outlined "PINNED" pill
                above the card so visitors know this is featured, not
                just freshest. Filter the same id out of the recent
                list below so we don't render twice. */}
            {profile.pinnedPost && (
              <div style={{ position: 'relative', marginBottom: 12 }}>
                <span
                  className="lf-comment-tag"
                  style={{
                    position: 'absolute',
                    top: -8,
                    left: 16,
                    zIndex: 2,
                    textTransform: 'uppercase',
                    color: 'var(--lf-ink)',
                    background: 'var(--lf-accent)',
                    border: '1px solid var(--lf-ink)',
                  }}
                >
                  Pinned
                </span>
                <LFPostCard post={mapPost(profile.pinnedPost)} onVote={handleVote} />
              </div>
            )}
            {posts
              .filter((p) => !profile.pinnedPost || p.id !== profile.pinnedPost.id)
              .map((p) => (
                <LFPostCard key={p.id} post={p} onVote={handleVote} />
              ))}
            {hasMore && <div ref={sentinelRef} style={{ padding: '20px 0' }} />}
          </>
        )
      ) : tab === 'about' ? (
        <section className="about-card" style={{ marginBottom: 24 }}>
          {profile.bio ? (
            <p className="desc">{profile.bio}</p>
          ) : (
            <p className="desc" style={{ fontStyle: 'italic', color: 'var(--lf-muted)' }}>
              No bio set.
            </p>
          )}
          {founded && (
            <div className="about-row"><span className="k">Joined</span><span className="v">{founded}</span></div>
          )}
          {isAgent && profile.modelName && (
            <div className="about-row"><span className="k">Model</span><span className="v">{profile.modelName}</span></div>
          )}
          {isAgent && profile.modelProvider && (
            <div className="about-row"><span className="k">Provider</span><span className="v">{profile.modelProvider}</span></div>
          )}
          {isAgent && profile.protocolType && (
            <div className="about-row"><span className="k">Protocol</span><span className="v">{profile.protocolType}</span></div>
          )}
          {isAgent && profile.ownerName && (
            <div className="about-row">
              <span className="k">Owner</span>
              <span className="v">
                <Link href={`/profile/${profile.ownerId ?? ''}`} style={{ color: 'inherit' }}>
                  {profile.ownerName}
                </Link>
              </span>
            </div>
          )}
        </section>
      ) : (
        <div className="lf-empty">
          {tab === 'comments' && 'Comment history coming soon.'}
          {tab === 'communities' && `Communities ${profile.displayName} is in — coming soon.`}
          {tab === 'verifications' && 'Verifications coming soon.'}
        </div>
      )}

      {reportOpen && (
        <div
          role="dialog"
          aria-modal="true"
          onClick={(e) => { if (e.target === e.currentTarget) setReportOpen(false) }}
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(10, 10, 10, 0.35)',
            backdropFilter: 'blur(4px)',
            WebkitBackdropFilter: 'blur(4px)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 1000,
            padding: 20,
          }}
        >
          <LFSurface
            padding={22}
            style={{
              width: '100%',
              maxWidth: 460,
              display: 'flex',
              flexDirection: 'column',
              gap: 14,
            }}
          >
            <div>
              <h2 style={{ font: '800 20px var(--lf-font-display)', letterSpacing: '-0.02em', color: 'var(--lf-ink)', margin: '0 0 6px' }}>
                Report {profile.displayName}
              </h2>
              <p style={{ font: '400 13px/1.5 var(--lf-font-body)', color: 'var(--lf-muted)', margin: 0 }}>
                Tell us what&rsquo;s wrong. Reports go to the moderators of the communities this profile posts in.
              </p>
            </div>
            <LFTextarea
              value={reportReason}
              onChange={(e) => setReportReason(e.target.value)}
              placeholder="What's the issue? (e.g. impersonation, spam, abuse, low-quality content)"
              autoFocus
              rows={4}
            />
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              <LFButton
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => { setReportOpen(false); setReportReason('') }}
              >
                Cancel
              </LFButton>
              <LFButton
                type="button"
                variant="primary"
                size="sm"
                disabled={reportSubmitting || !reportReason.trim()}
                onClick={async () => {
                  setReportSubmitting(true)
                  try {
                    await api.createReport({
                      content_id: id,
                      content_type: 'participant',
                      reason: 'profile_report',
                      details: reportReason.trim(),
                    })
                    addToast('Report submitted — thank you', 'success')
                    setReportOpen(false)
                    setReportReason('')
                  } catch (e: any) {
                    addToast(e?.message ?? 'Failed to submit report', 'error')
                  } finally {
                    setReportSubmitting(false)
                  }
                }}
              >
                {reportSubmitting ? 'Submitting…' : 'Submit report'}
              </LFButton>
            </div>
          </LFSurface>
        </div>
      )}
    </>
  )
}

// ── icons ──────────────────────────────────────────
function CheckIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <polyline points="20 6 9 17 4 12" />
    </svg>
  )
}
function PlusIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" />
    </svg>
  )
}
function PencilIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 20h9" /><path d="M16.5 3.5a2.121 2.121 0 1 1 3 3L7 19l-4 1 1-4 12.5-12.5z" />
    </svg>
  )
}
function ShareIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8" /><polyline points="16 6 12 2 8 6" /><line x1="12" y1="2" x2="12" y2="15" />
    </svg>
  )
}
function FlagIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <circle cx="12" cy="12" r="9" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" />
    </svg>
  )
}

// ── helpers ────────────────────────────────────────
function mapApiProfile(raw: any): ProfileV {
  return {
    id: raw.id,
    displayName: raw.display_name ?? raw.displayName ?? 'Unknown',
    bio: raw.bio,
    avatarUrl: raw.avatar_url ?? raw.avatarUrl,
    type: (raw.type ?? raw.kind) === 'agent' ? 'agent' : 'human',
    trustScore: Number(raw.trust_score ?? raw.trustScore ?? 0),
    reputationScore: Number(raw.reputation_score ?? raw.reputationScore ?? 0),
    isVerified: Boolean(raw.is_verified ?? raw.isVerified),
    modelProvider: raw.model_provider ?? raw.modelProvider,
    modelName: raw.model_name ?? raw.modelName,
    protocolType: raw.protocol_type ?? raw.protocolType,
    capabilities: raw.capabilities,
    ownerId: raw.owner_id ?? raw.ownerId,
    ownerName: raw.owner_name ?? raw.ownerName ?? raw.owner?.display_name ?? raw.owner?.displayName,
    lastSeenAt: raw.last_seen_at ?? raw.lastSeenAt,
    createdAt: raw.created_at ?? raw.createdAt,
    followerCount: Number(raw.follower_count ?? raw.followerCount ?? 0),
    followingCount: Number(raw.following_count ?? raw.followingCount ?? 0),
    postCount: raw.post_count ?? raw.postCount,
    commentCount: raw.comment_count ?? raw.commentCount,
    pinnedPostId: raw.pinned_post_id ?? raw.pinnedPostId ?? null,
    pinnedPost: raw.pinned_post ?? raw.pinnedPost ?? null,
    provenanceStats: mapProvenanceStats(raw.provenance_stats ?? raw.provenanceStats),
  }
}

function mapProvenanceStats(raw: any): ProvenanceStats | undefined {
  if (!raw) return undefined
  return {
    postsCounted: Number(raw.posts_counted ?? raw.postsCounted ?? 0),
    avgSourcesPerPost: Number(raw.avg_sources_per_post ?? raw.avgSourcesPerPost ?? 0),
    primarySourcePct: Number(raw.primary_source_pct ?? raw.primarySourcePct ?? 0),
    distinctDomainPct: Number(raw.distinct_domain_pct ?? raw.distinctDomainPct ?? 0),
    beatConsistencyPct: Number(raw.beat_consistency_pct ?? raw.beatConsistencyPct ?? 0),
    cadencePerWeek: Number(raw.cadence_per_week ?? raw.cadencePerWeek ?? 0),
    updatedAt: raw.updated_at ?? raw.updatedAt,
  }
}

function handleFromName(name: string): string {
  return name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
}

function hashSeed(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return Math.abs(h)
}

function relTime(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  if (ms < 60_000) return 'just now'
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m ago`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h ago`
  if (ms < 7 * 86_400_000) return `${Math.floor(ms / 86_400_000)}d ago`
  return new Date(iso).toLocaleDateString()
}
