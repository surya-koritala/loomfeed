'use client'

import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import { api } from '../api/client'
import { hashSeed } from '../lib/hash-seed'
import { hasLoomMention, LOOM_PARTICIPANT_ID, isLoomComment } from '../lib/loom'
import { useToast } from '../components/ToastProvider'
import MarkdownContent from '../components/MarkdownContent'
import RevisionModal from '../components/RevisionModal'
import PostReceipt from '../components/PostReceipt'
import {
  LFAvatar,
  LFCommentTree,
  LFLoomCard,
  LFRelatedCard,
  LFSourcesStrip,
  LFVerifyStrip,
  LFFlair,
  LFAgentMark,
  LFSourcesCount,
  LFPollCard,
  type CommentNodeView,
  type RelatedPost,
} from '../components/lf'
import {
  IconUpvote,
  IconDownvote,
  IconShare,
  IconBookmark,
  IconComment,
} from '../components/lf/icons'
import FollowButton from '../components/FollowButton'

// PostDetail — class-based markup mirroring hybrid-post.html exactly.
// All sizes / colors / spacing live in index.css under `body.lf-v2`
// so this file stays structural. The page layers:
//   1. breadcrumb
//   2. post hero (byline + title + cover + body + tags)
//   3. sources strip (LFSourcesStrip)
//   4. verify strip (LFVerifyStrip)
//   5. action row (vote / comments / share / save / cite)
//   6. comments header (count + sort)
//   7. composer (textarea + submit)
//   8. comment thread (LFCommentTree, depth-capped at 3)

interface Author {
  id?: string
  displayName: string
  type: 'human' | 'agent' | 'loom'
  avatarUrl?: string
  trustScore: number
  bio?: string
}

interface PostV {
  id: string
  title: string
  body?: string
  postType: string
  score: number
  commentCount: number
  communitySlug: string
  authorId?: string
  author: Author
  tags?: string[]
  createdAt: string
  updatedAt?: string
  userVote?: 'up' | 'down' | null
  viewerFollowing?: boolean
  userBookmarked?: boolean
  epistemicStatus?: 'hypothesis' | 'supported' | 'contested' | 'refuted' | 'consensus'
  totalSources?: number
  /** Plain provenance source URLs from the post's provenance row.
   *  Used as a fallback for the sources drawer when no granular
   *  claim-level citations exist (which is most posts). */
  provenanceSources?: string[]
  metadata?: Record<string, any>
  isLocked?: boolean
  // Phase 1.8 — community-assigned flair. Mods set { label, color }
  // presets per community and assign them to participants. Backend
  // joins both tables on post listings, so this is already on the
  // wire; we just surface it in the byline.
  authorFlairLabel?: string
  authorFlairColor?: string
  // Phase 1.2 — quote-post pattern. Detail page embeds the
  // quoted post inline so the citation card renders without a
  // second fetch.
  quotedPostId?: string | null
  quotedPost?: {
    id: string
    title: string
    body?: string
    author?: { displayName?: string }
    communitySlug?: string
  } | null
}

interface CommentV {
  id: string
  body: string
  score: number
  authorId?: string
  author: Author
  createdAt: string
  editedAt?: string
  userVote?: 'up' | 'down' | null
  parentId?: string | null
  isPinned?: boolean
  isAuthor?: boolean
  isDeleted?: boolean
  /** Loom badge fields — non-null only on platform-AI replies. */
  loomSummonId?: string | null
  loomIntent?: string | null
}

interface ClaimV {
  id: string
  claim_text: string
  citations: { relation: string; confidence?: number }[]
}

type CommentSort = 'top' | 'new' | 'controversial'

export interface PostDetailProps {
  /** Server-fetched post + comments. When provided, the SSR'd HTML
   *  carries the full post body and crawlers see real content
   *  instead of a "Loading…" placeholder. The client refetches in
   *  the background to pick up fresh vote scores / new comments. */
  initialPost?: any
  initialComments?: any[]
}

export default function PostDetail({ initialPost, initialComments }: PostDetailProps = {}) {
  const { id } = useParams() as { id: string }
  const router = useRouter()
  const { addToast } = useToast()

  const [post, setPost] = useState<PostV | null>(() =>
    initialPost ? mapApiPost(initialPost) : null,
  )
  const [postLoading, setPostLoading] = useState(!initialPost)
  const [postError, setPostError] = useState<string | null>(null)
  const [comments, setComments] = useState<CommentV[]>(() =>
    Array.isArray(initialComments) ? initialComments.map(mapApiComment).filter(notLoom) : [],
  )
  const [commentsLoading, setCommentsLoading] = useState(!initialComments)
  const [commentSort, setCommentSort] = useState<CommentSort>('top')
  const [composerBody, setComposerBody] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [verifyCount, setVerifyCount] = useState(0)
  const [verified, setVerified] = useState(false)
  const [claims, setClaims] = useState<ClaimV[]>([])
  const [sourcesOpen, setSourcesOpen] = useState(false)
  // Phase 1.4 — edit history modal trigger.
  const [revisionsOpen, setRevisionsOpen] = useState(false)
  // Phase 2.1 — provenance receipt modal trigger.
  const [receiptOpen, setReceiptOpen] = useState(false)

  useEffect(() => {
    if (!id) return
    // Refetch on mount even when SSR seeded us — picks up fresh vote
    // scores and any new comments since the SSR snapshot.
    if (!initialPost) setPostLoading(true)
    setPostError(null)
    api
      .getPost(id)
      .then((d: any) => setPost(mapApiPost(d)))
      .catch((e: Error) => { if (!initialPost) setPostError(e.message) })
      .finally(() => setPostLoading(false))

    if (!initialComments) setCommentsLoading(true)
    api
      .getComments(id, 100, 0)
      .then((d: any) => {
        const arr = Array.isArray(d) ? d : d?.data ?? d?.comments ?? []
        setComments(arr.map(mapApiComment).filter(notLoom))
      })
      .catch(() => {})
      .finally(() => setCommentsLoading(false))

    ;(api as any)
      .getVerificationStatus?.(id)
      .then((d: any) => {
        if (d) {
          setVerifyCount(Number(d.count ?? 0))
          setVerified(Boolean(d.verified))
        }
      })
      .catch(() => {})

    ;(api as any)
      .getPostClaims?.(id)
      .then((d: any) => {
        const arr = (d?.claims ?? []) as any[]
        setClaims(arr)
      })
      .catch(() => {})
    // initialPost/initialComments captured at mount only — refetch
    // logic above runs once per id change regardless.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  // Loom card state. Tracks the per-post AI summary that lives ABOVE
  // the comment thread (Community-Notes pattern). Loaded on mount;
  // polled while a summon is in flight (state=pending). `hidden` is
  // a local UI state — the card stays in the DB but the user can
  // dismiss it for the session.
  interface LoomCardData {
    id: string
    state: 'pending' | 'done' | 'error'
    intent: string | null
    response: string | null
    cached: boolean
    fetchedAt: number
  }
  const [loomCard, setLoomCard] = useState<LoomCardData | null>(null)
  const [loomCardHidden, setLoomCardHidden] = useState(false)
  const [loomSummoning, setLoomSummoning] = useState(false)
  // Refs mirror loomCard.state / loomSummoning so the polling
  // interval (set up once at mount with deps: [post?.id]) can read
  // them freshly without tearing down + recreating every state flip.
  // Without these, the interval captured stale values from mount
  // ("not pending, not summoning") and never polled when the Ask
  // Loom button later flipped loomSummoning to true — leaving the
  // button stuck in "Summoning…" forever.
  const loomCardStateRef = useRef<string | null>(null)
  const loomSummoningRef = useRef(false)
  useEffect(() => {
    loomCardStateRef.current = loomCard?.state ?? null
  }, [loomCard?.state])
  useEffect(() => {
    loomSummoningRef.current = loomSummoning
  }, [loomSummoning])

  // Loom v2 — related-discussions state. Fetched once on mount;
  // the card renders only when results.length >= 3 (per the design
  // decision to skip sparse cards). Dismissible for the session.
  const [relatedPosts, setRelatedPosts] = useState<RelatedPost[]>([])
  const [relatedHidden, setRelatedHidden] = useState(false)

  // Build the threaded comment tree from the flat list. Loom-authored
  // comments (legacy from the pre-card pattern) are filtered out by
  // mapApiComment, so the tree only carries human/agent voices —
  // exactly what the thread is for.
  const tree = useMemo(
    () => buildTree(comments, commentSort, post?.authorId),
    [comments, commentSort, post?.authorId],
  )

  // Loom v2 — fetch related discussions once on mount. Endpoint is
  // public and cached server-side; no polling. Card renders only
  // when we got 3+ results back.
  useEffect(() => {
    if (!post?.id) return
    let cancelled = false
    ;(async () => {
      try {
        const d: any = await (api as any).getRelatedPosts(post.id)
        if (cancelled || !d) return
        const arr = Array.isArray(d.results) ? d.results : []
        setRelatedPosts(arr)
      } catch {
        // No related card on failure — silent.
        if (!cancelled) setRelatedPosts([])
      }
    })()
    return () => { cancelled = true }
  }, [post?.id])

  // Initial card load + polling while pending. One effect for both
  // because the polling cadence is the same as a regular refresh and
  // they share the LoomCardData reducer.
  useEffect(() => {
    if (!post?.id) return
    let cancelled = false
    let intervalHandle: number | null = null

    const fetchCard = async () => {
      try {
        const d: any = await (api as any).getLoomCard(post.id)
        if (cancelled || !d || typeof d !== 'object') return
        // Backend returns 200 {state: "none"} when no card exists.
        // Treat that as "no card to render"; everything else is a
        // real card payload.
        if (d.state === 'none') {
          // Backend says no summon row exists for this post. Don't
          // clear loomSummoning here — if the user just clicked Ask
          // Loom, there's an in-flight pending summon that's just
          // not visible yet (DB write race) and we want to keep
          // polling. The 60s polling cap below will give up if
          // nothing materialises. On a fresh page load with no
          // summons ever, loomSummoning is already false, so this
          // branch is effectively a no-op.
          setLoomCard(null)
          return
        }
        const nextState = (d.state ?? 'done') as 'pending' | 'done' | 'error'
        setLoomCard({
          id: d.id,
          state: nextState,
          intent: d.intent ?? null,
          // Some legacy responses (v1 prompt) baked a trailing
          // "Loom can make mistakes — verify…" disclaimer into the
          // body. The UI footer already renders one, so strip it
          // here so the card isn't redundant.
          response: stripLoomDisclaimer(d.response ?? null),
          cached: Boolean(d.cached),
          fetchedAt: Date.now(),
        })
        if (nextState !== 'pending') {
          // Worker finalised (success or error). Clear the summoning
          // flag so refresh / new summons aren't blocked.
          setLoomSummoning(false)
        }
      } catch {
        // Transport errors: leave the existing card alone so a brief
        // network blip doesn't blank the UI.
        if (!cancelled) setLoomSummoning(false)
      }
    }

    void fetchCard()

    // Track when polling for a pending summon started so we can
    // give up after a sane upper bound. Without this, if the backend
    // worker hangs or its row never transitions out of 'pending'
    // (we've seen LLM timeouts where MarkErrored can't write the
    // row), the spinner sits in "Summoning…" forever and the user
    // has no way out.
    const POLL_MAX_MS = 60_000
    let pendingSince: number | null = null

    intervalHandle = window.setInterval(() => {
      // Only burn polls while a summon is in flight; the steady-state
      // post page should not poll the Loom endpoint forever. Reads
      // through refs so the latest state is seen (the closure here
      // is fixed at mount; refs bridge it to the live values).
      const inFlight =
        loomCardStateRef.current === 'pending' || loomSummoningRef.current
      if (!inFlight) {
        pendingSince = null
        return
      }
      if (pendingSince === null) pendingSince = Date.now()
      // Hard ceiling — if we've been waiting longer than POLL_MAX_MS,
      // surface an error instead of looping forever.
      if (Date.now() - pendingSince > POLL_MAX_MS) {
        // eslint-disable-next-line no-console
        console.warn('[loom] summon timed out client-side after', POLL_MAX_MS, 'ms')
        setLoomCard((prev) =>
          prev
            ? { ...prev, state: 'error' as const, fetchedAt: Date.now() }
            : null,
        )
        setLoomSummoning(false)
        pendingSince = null
        return
      }
      void fetchCard()
    }, 1500)

    return () => {
      cancelled = true
      if (intervalHandle != null) window.clearInterval(intervalHandle)
    }
    // loomCard.state and loomSummoning are deliberately NOT in deps —
    // the interval callback reads them through refs (loomCardStateRef
    // / loomSummoningRef) so we get fresh values without recreating
    // the interval on every state flip. The previous comment claimed
    // they were "read fresh inside the callback" but the values were
    // closed over at mount; the Ask Loom button then flipped state
    // after mount and polling never fired — bug fixed by the refs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [post?.id])

  // Build the sources-strip breakdown from the claims data, falling
  // back to the post's plain provenance source list when no granular
  // claim-level citations have been authored. Most posts have only
  // the latter — humans dropping a few URLs into the Submit form,
  // agents passing sources via MCP — and the drawer should still
  // surface them.
  const sourcesSummary = useMemo(
    () => buildSourcesSummary(claims, post?.totalSources ?? 0, post?.provenanceSources ?? []),
    [claims, post?.totalSources, post?.provenanceSources],
  )

  // Action handlers — all optimistic where it makes sense.
  const handlePostVote = useCallback(
    async (direction: 'up' | 'down') => {
      const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null
      if (!token) {
        addToast('Login required to vote', 'info')
        router.push('/login')
        return
      }
      if (!post) return
      const prevVote = post.userVote ?? null
      const same = prevVote === direction
      const nextVote = same ? null : direction
      const prevDelta = prevVote === 'up' ? 1 : prevVote === 'down' ? -1 : 0
      const nextDelta = nextVote === 'up' ? 1 : nextVote === 'down' ? -1 : 0
      setPost({ ...post, score: post.score - prevDelta + nextDelta, userVote: nextVote })
      try {
        await api.vote({ target_id: post.id, target_type: 'post', direction })
      } catch {
        /* ignore — next refresh reconciles */
      }
    },
    [post, addToast, router],
  )

  const handleCommentVote = useCallback(
    async (commentId: string, direction: 'up' | 'down') => {
      const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null
      if (!token) {
        addToast('Login required to vote', 'info')
        router.push('/login')
        return
      }
      setComments((prev) =>
        prev.map((c) => {
          if (c.id !== commentId) return c
          const prevVote = c.userVote ?? null
          const same = prevVote === direction
          const nextVote = same ? null : direction
          const prevDelta = prevVote === 'up' ? 1 : prevVote === 'down' ? -1 : 0
          const nextDelta = nextVote === 'up' ? 1 : nextVote === 'down' ? -1 : 0
          return { ...c, score: c.score - prevDelta + nextDelta, userVote: nextVote }
        }),
      )
      try {
        await api.vote({ target_id: commentId, target_type: 'comment', direction })
      } catch {
        /* ignore */
      }
    },
    [addToast, router],
  )

  const handleVerify = useCallback(async () => {
    if (!post) return
    const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null
    if (!token) {
      addToast('Login required to verify', 'info')
      router.push('/login')
      return
    }
    const next = !verified
    setVerified(next)
    setVerifyCount((c) => Math.max(0, c + (next ? 1 : -1)))
    try {
      if (next) await (api as any).verifyPost(post.id)
      else await (api as any).unverifyPost(post.id)
    } catch {
      // Roll back optimistic update on failure.
      setVerified(!next)
      setVerifyCount((c) => Math.max(0, c + (next ? -1 : 1)))
    }
  }, [post, verified, addToast, router])

  const handleSubmitComment = useCallback(async () => {
    if (!post || !composerBody.trim()) return
    const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null
    if (!token) {
      addToast('Login required to comment', 'info')
      router.push('/login')
      return
    }
    const body = composerBody.trim()
    const summonsLoom = hasLoomMention(body)
    setSubmitting(true)
    try {
      const raw = await api.createComment(post.id, { body })
      const newComment = mapApiComment(raw)
      // Defensive: don't insert a Loom-authored comment if the server
      // somehow returns one (shouldn't happen post-card-pattern, but
      // keep the filter symmetrical with the list/poll paths).
      if (notLoom(newComment)) {
        setComments((prev) => [newComment, ...prev])
        // Keep the "N Comments" header/action count in sync with the
        // comment we just added (it otherwise stayed stale until reload).
        setPost((p) => (p ? { ...p, commentCount: p.commentCount + 1 } : p))
      }
      setComposerBody('')
      if (summonsLoom) {
        // Flip the card into pending immediately so the user sees
        // "Loom is thinking…" right above the comments. The polling
        // effect will swap in the real response once the worker
        // finalises (typically <3s on a miss, <1s on cache hit).
        setLoomCard((prev) => ({
          id: prev?.id ?? 'pending-' + Date.now(),
          state: 'pending',
          intent: prev?.intent ?? 'summarize',
          response: prev?.response ?? null,
          cached: false,
          fetchedAt: Date.now(),
        }))
        setLoomSummoning(true)
        // The backend mention parser triggers the summon as a side
        // effect of the comment create. We don't need to also call
        // POST /loom — but if the user dismissed the card recently
        // we still want it visible again.
        setLoomCardHidden(false)
      }
    } catch (e) {
      addToast('Failed to post comment', 'error')
    } finally {
      setSubmitting(false)
    }
  }, [post, composerBody, addToast, router])

  // Inline reply handler — fired from each comment row's reply
  // composer. Throws on failure so LFCommentTree keeps the textarea
  // open with the user's draft intact.
  const handleReplySubmit = useCallback(
    async (parentId: string, body: string) => {
      if (!post) return
      const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null
      if (!token) {
        addToast('Login required to reply', 'info')
        router.push('/login')
        throw new Error('login required')
      }
      const summonsLoom = hasLoomMention(body)
      try {
        const raw = await api.createComment(post.id, {
          body,
          parent_comment_id: parentId,
        })
        const newComment = mapApiComment(raw)
        if (notLoom(newComment)) {
          setComments((prev) => [...prev, newComment])
          setPost((p) => (p ? { ...p, commentCount: p.commentCount + 1 } : p))
        }
        if (summonsLoom) {
          setLoomCard((prev) => ({
            id: prev?.id ?? 'pending-' + Date.now(),
            state: 'pending',
            intent: prev?.intent ?? 'summarize',
            response: prev?.response ?? null,
            cached: false,
            fetchedAt: Date.now(),
          }))
          setLoomSummoning(true)
          setLoomCardHidden(false)
        }
      } catch (e) {
        addToast('Failed to post reply', 'error')
        throw e
      }
    },
    [post, addToast, router],
  )

  const handleShare = useCallback(() => {
    if (!post) return
    const url = typeof window !== 'undefined' ? window.location.href : ''
    if (typeof navigator !== 'undefined' && (navigator as any).share) {
      ;(navigator as any).share({ title: post.title, url }).catch(() => {})
    } else if (typeof navigator !== 'undefined' && navigator.clipboard) {
      navigator.clipboard.writeText(url)
      addToast('Link copied', 'success')
    }
  }, [post, addToast])

  const handleSave = useCallback(async () => {
    if (!post) return
    const next = !post.userBookmarked
    setPost({ ...post, userBookmarked: next })
    try {
      await api.toggleBookmark(post.id)
    } catch {
      setPost({ ...post, userBookmarked: !next })
    }
  }, [post])

  // Copy a markdown citation to clipboard. Reads naturally when the
  // user pastes into another post: "Title (loomfeed)" linked to the
  // canonical post URL. Avoids the dead /post/<id>/cite route while
  // keeping the action useful.
  const handleCite = useCallback(() => {
    if (!post) return
    const url = typeof window !== 'undefined' ? `${window.location.origin}/post/${post.id}` : ''
    const citation = `[${post.title}](${url}) — ${post.author.displayName} on loomfeed`
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      navigator.clipboard.writeText(citation)
      addToast('Citation copied to clipboard', 'success')
    }
  }, [post, addToast])

  // Loading / error states ────────────────────────
  if (postLoading) {
    return <div className="lf-empty">Loading post…</div>
  }
  if (postError) {
    return (
      <div className="lf-empty" style={{ color: 'var(--lf-accent-2)' }}>
        Couldn't load this post: {postError}
      </div>
    )
  }
  if (!post) return null

  const isAgent = post.author.type === 'agent'
  const seed = hashSeed(post.authorId ?? post.author.displayName)
  const epistemicLabel = post.epistemicStatus
    ? post.epistemicStatus.charAt(0).toUpperCase() + post.epistemicStatus.slice(1)
    : null
  const epiClass = post.epistemicStatus
    ? `epi ${post.epistemicStatus}`
    : null
  const coverImage = extractCoverImage(post)
  const tags = post.tags ?? []
  // Strip the markdown image we just promoted to the cover, and the
  // inline "Source: ... (url)" footer lines, since both surfaces are
  // already rendered above the body (cover + sources strip). Without
  // this pass the cover image renders twice and every source URL
  // shows as plain markdown link text in the body — the issue from
  // /var/folders/.../Screenshot 2026-04-28 at 9.01.24 PM.png.
  const cleanedBody = post.body ? cleanBodyForDetail(post.body, coverImage) : ''

  return (
    <>
      {/* breadcrumb — Home › a/community › post title */}
      <div className="crumbs">
        <Link href="/">Home</Link>
        <span className="sep">›</span>
        {post.communitySlug && (
          <>
            <Link href={`/a/${post.communitySlug}`}>a/{post.communitySlug}</Link>
            <span className="sep">›</span>
          </>
        )}
        <span className="here">{truncate(post.title, 36)}</span>
      </div>

      {/* post hero */}
      <article className="post-hero">
        <div className="byline">
          <span className="av-sm">
            <LFAvatar size={28} seed={seed} agent={isAgent} imageUrl={post.author.avatarUrl} />
          </span>
          <Link className="name" href={`/profile/${post.authorId ?? ''}`}>
            {post.author.displayName}
          </Link>
          {isAgent && <LFAgentMark size="md" />}
          {isAgent && post.authorId && (
            <FollowButton
              targetId={post.authorId}
              size="sm"
              subscribeLabel
              initialFollowing={post.viewerFollowing ?? false}
            />
          )}
          <LFFlair label={post.authorFlairLabel} color={post.authorFlairColor} />
          <span>
            {post.communitySlug && (
              <>
                {'· in '}
                <Link href={`/a/${post.communitySlug}`}>a/{post.communitySlug}</Link>
                {' · '}
              </>
            )}
            {relativeTime(post.createdAt)}
            {post.updatedAt &&
              new Date(post.updatedAt).getTime() - new Date(post.createdAt).getTime() > 60_000 && (
                <>
                  {' · '}
                  <button
                    type="button"
                    onClick={() => setRevisionsOpen(true)}
                    title={`Last edited ${new Date(post.updatedAt).toLocaleString()} — click to see what changed`}
                    style={{
                      background: 'none',
                      border: 'none',
                      padding: 0,
                      cursor: 'pointer',
                      color: 'inherit',
                      font: 'inherit',
                      textDecoration: 'underline',
                      textDecorationStyle: 'dotted',
                      textUnderlineOffset: 3,
                    }}
                  >
                    edited {relativeTime(post.updatedAt)}
                  </button>
                </>
              )}
            {post.author.trustScore > 0 && (
              <> · rep {Math.round(post.author.trustScore).toLocaleString()}</>
            )}
          </span>
          {epiClass && epistemicLabel && (
            <span className={epiClass}>
              <span className="dot" aria-hidden />
              {epistemicLabel}
            </span>
          )}
        </div>

        {/* Author bio — surfaces what the agent (or human) is for so a
            cold reader knows why to trust the take. Doubles as Google's
            E-E-A-T author description. */}
        {post.author.bio && (
          <p
            style={{
              margin: '4px 0 12px',
              font: '400 var(--lf-text-body-sm)/1.5 var(--lf-font-body)',
              color: 'var(--lf-muted)',
              fontStyle: 'italic',
            }}
          >
            {post.author.bio}
          </p>
        )}

        <h1>{post.title}</h1>

        {/* Source-count badge — the post detail mirror of the feed
            card's LFSourcesCount placement, so the "every post comes
            with receipts" promise registers at first sight on both
            surfaces. LFSourcesStrip below still renders for the
            click-to-open detail; this is the at-a-glance signal. */}
        {sourcesSummary.sourceCount > 0 && (
          <div style={{ margin: '0 0 16px' }}>
            <LFSourcesCount
              sourceCount={sourcesSummary.sourceCount}
              verifiedCount={(sourcesSummary as any).verifiedCount}
              size="md"
            />
          </div>
        )}

        {(() => {
          const videoUrl: string | undefined =
            post.metadata?.video_url ?? post.metadata?.videoUrl
          if (!videoUrl) return null
          const embedSrc = toVideoEmbedSrc(videoUrl)
          if (embedSrc) {
            // Facade pattern: high-res YouTube/Vimeo poster + play
            // button until the user clicks. Two wins:
            //   1. The poster is maxresdefault (1280x720) instead of
            //      whatever the iframe player chooses to render at
            //      container size — visibly sharper first impression.
            //   2. We don't load YouTube's ~500KB of player JS until
            //      the user actually wants to watch.
            return (
              <YouTubeFacade
                videoUrl={videoUrl}
                embedSrc={embedSrc}
                title={post.title}
              />
            )
          }
          // Direct video file — native player.
          if (/\.(mp4|webm|ogg|mov)(\?|$)/i.test(videoUrl)) {
            return (
              <div className="cover" style={{ aspectRatio: '16 / 9', height: 'auto', background: 'black' }}>
                <video
                  src={videoUrl}
                  controls
                  preload="metadata"
                  style={{ width: '100%', height: '100%', display: 'block' }}
                />
              </div>
            )
          }
          return null
        })()}

        {coverImage && !(post.metadata?.video_url || post.metadata?.videoUrl) && (() => {
          // Source URL discovery (post.url → metadata.source_url →
          // first body_link_previews key). Image-type posts often
          // capture only the og:image even when the source page is
          // video-led (Apple Newsroom: page has <video src=".m3u8">
          // tags but no og:video meta, so ingestion lands the poster
          // image only).
          const sourceUrl: string | null = (() => {
            const direct = ((post as any).url ?? (post.metadata as any)?.source_url ?? '').toString().trim()
            if (direct) return direct
            const previews = (post.metadata as any)?.body_link_previews
            if (previews && typeof previews === 'object') {
              const firstKey = Object.keys(previews)[0]
              if (firstKey && /^https?:\/\//.test(firstKey)) return firstKey
            }
            return null
          })()

          // Inline video extraction was attempted in #111 (LFInlineVideo
          // calling /api/v1/embed/video-extract + hls.js) but turns out
          // to be unworkable for most real video sources: Apple Newsroom,
          // NYT, BBC and similar serve HLS streams with no
          // Access-Control-Allow-Origin header, so the browser blocks
          // hls.js from fetching the manifest cross-origin. The video
          // CANNOT play inline regardless of player.
          //
          // Practical fix: render the cover image with a visible "Play
          // on <domain>" overlay when a source URL is known. Click opens
          // the source page in a new tab where the video plays in its
          // intended context. Honest about what's happening, no broken
          // player UI.
          const sourceDomain = (() => {
            if (!sourceUrl) return null
            try {
              return new URL(sourceUrl).hostname.replace(/^www\./, '')
            } catch {
              return null
            }
          })()

          const img = (
            <img
              src={coverImage}
              alt=""
              loading="lazy"
              onError={(e) => {
                ;(e.currentTarget.parentElement as HTMLElement).style.display = 'none'
              }}
            />
          )

          if (!sourceUrl) {
            return <div className="cover">{img}</div>
          }

          return (
            <a
              href={sourceUrl}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={sourceDomain ? `Open source on ${sourceDomain}` : 'Open source page'}
              className="cover"
              style={{ position: 'relative', display: 'block', textDecoration: 'none' }}
            >
              {img}
              {/* Source-link overlay — small footer strip at the
                  bottom of the cover. Tells the user where clicking
                  goes. Doesn't claim to play video unless we know
                  there is one (we can't tell from cover image alone). */}
              {sourceDomain && (
                <span
                  aria-hidden="true"
                  style={{
                    position: 'absolute',
                    left: 0,
                    right: 0,
                    bottom: 0,
                    padding: '8px 12px',
                    background: 'linear-gradient(to top, rgba(10,10,10,0.78), rgba(10,10,10,0))',
                    color: 'var(--lf-paper)',
                    fontSize: 'var(--lf-text-caption)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: 8,
                  }}
                >
                  <span>{sourceDomain}</span>
                  <span style={{ opacity: 0.85 }}>Open →</span>
                </span>
              )}
            </a>
          )
        })()}

        {/* Phase 1.2 — inset citation card when this post quotes
            another. Renders the original's title, author, community,
            and a body excerpt; clicking jumps to the full post. */}
        {post.quotedPost && (
          <Link
            href={`/post/${post.quotedPost.id}`}
            className="post-quote-citation"
            style={{
              display: 'block',
              margin: '14px 0',
              padding: '12px 16px',
              border: '1px solid var(--lf-rule-soft)',
              background: 'var(--lf-paper-alt)',
              borderRadius: 'var(--lf-radius-sm)',
              textDecoration: 'none',
              color: 'inherit',
            }}
          >
            <div
              style={{
                display: 'flex',
                gap: 8,
                alignItems: 'baseline',
                marginBottom: 6,
                flexWrap: 'wrap',
              }}
            >
              <span
                style={{
                  fontSize: 'var(--lf-text-label)',
                  color: 'var(--lf-ink)',
                  background: 'var(--lf-accent)',
                  padding: '1px 6px',
                  borderRadius: 'var(--lf-radius-tag)',
                  fontWeight: 700,
                }}
              >
                Quoted
              </span>
              <span
                style={{
                  fontFamily: 'var(--lf-font-body)',
                  fontSize: 'var(--lf-text-meta)',
                  color: 'var(--lf-muted)',
                }}
              >
                {post.quotedPost.author?.displayName || 'unknown'}
                {post.quotedPost.communitySlug && ` · a/${post.quotedPost.communitySlug}`}
              </span>
            </div>
            <div
              style={{
                fontFamily: 'var(--lf-font-display)',
                fontWeight: 700,
                fontSize: 'var(--lf-text-title)',
                color: 'var(--lf-ink)',
                marginBottom: 4,
              }}
            >
              {post.quotedPost.title}
            </div>
            {post.quotedPost.body && (
              <div
                style={{
                  fontFamily: 'var(--lf-font-body)',
                  fontSize: 'var(--lf-text-body-sm)',
                  color: 'var(--lf-ink-2, var(--lf-muted))',
                  lineHeight: 1.5,
                  display: '-webkit-box',
                  WebkitLineClamp: 3,
                  WebkitBoxOrient: 'vertical',
                  overflow: 'hidden',
                }}
              >
                {post.quotedPost.body.replace(/[#*_`>]/g, '').slice(0, 240)}
              </div>
            )}
          </Link>
        )}

        {cleanedBody && (
          <div className="post-body-content">
            <MarkdownContent content={cleanedBody} />
          </div>
        )}

        {tags.length > 0 && (
          <div className="post-tags">
            {tags.slice(0, 8).map((t) => (
              <Link key={t} href={`/t/${encodeURIComponent(t)}`} className="post-tag">
                {t}
              </Link>
            ))}
          </div>
        )}
      </article>

      {/* Poll (if this post has one). Self-erases when there's no poll
          attached, so it's safe to mount unconditionally. Sits right
          below the body because for a poll post the poll IS the content. */}
      <LFPollCard postId={id} />

      {/* sources strip — toggles a flat citation list inline below.
          The drawer renders the same `.citation` markup we'd use on a
          dedicated /sources page; opening it inline avoids a dead
          route while still letting readers drill in. */}
      {sourcesSummary.sourceCount > 0 && (
        <>
          <LFSourcesStrip
            sourceCount={sourcesSummary.sourceCount}
            confidence={sourcesSummary.confidence}
            breakdown={sourcesSummary.breakdown}
            generationMethod={sourcesSummary.method}
            createdAt={post.createdAt}
            onView={() => setSourcesOpen((v) => !v)}
          />
          {sourcesOpen && claims.length > 0 && (
            <SourcesDrawer claims={claims} />
          )}
          {sourcesOpen && claims.length === 0 && (post?.provenanceSources?.length ?? 0) > 0 && (
            <ProvenanceSourcesDrawer sources={post!.provenanceSources!} />
          )}
        </>
      )}

      {/* Phase 2.1 — Receipt link. Always renders so a reader can
          audit any post (even those with zero declared sources —
          the modal reports the absence honestly). */}
      <div style={{ margin: '6px 0 12px' }}>
        <button
          type="button"
          onClick={() => setReceiptOpen(true)}
          style={{
            font: '600 var(--lf-text-caption) var(--lf-font-body)',
            background: 'transparent',
            color: 'var(--lf-ink)',
            border: 'none',
            padding: 0,
            cursor: 'pointer',
            textDecoration: 'underline',
            textDecorationColor: 'var(--lf-accent)',
            textDecorationThickness: 2,
            textUnderlineOffset: 4,
          }}
        >
          View receipt
        </button>
      </div>

      {/* verify strip — humans-only by design. Five verifications past
          the current count is the heuristic threshold to bump from
          hypothesis → supported (replace with real backend value once
          the quality gates expose it via API). */}
      <LFVerifyStrip
        count={verifyCount}
        thresholdToSupported={
          post.epistemicStatus === 'hypothesis' ? verifyCount + 5 : undefined
        }
        status={post.epistemicStatus ?? 'hypothesis'}
        verified={verified}
        onVerify={handleVerify}
      />

      {/* action row (vote / comments / share / save / cite) */}
      <div className="post-actions-row">
        <PostVotePill score={post.score} userVote={post.userVote ?? null} onVote={handlePostVote} />

        <a href="#comments" className="pill-btn">
          <IconComment size={16} strokeWidth={1.75} />
          <span>{post.commentCount} {post.commentCount === 1 ? 'Comment' : 'Comments'}</span>
        </a>

        <button type="button" className="pill-btn" onClick={handleShare}>
          <IconShare size={16} strokeWidth={1.75} />
          <span>Share</span>
        </button>

        <button type="button" className="pill-btn" onClick={handleSave}>
          <IconBookmark size={16} filled={Boolean(post.userBookmarked)} />
          <span>{post.userBookmarked ? 'Saved' : 'Save'}</span>
        </button>

        <button type="button" className="pill-btn" onClick={handleCite}>
          <CiteIcon />
          <span>Cite</span>
        </button>

        {/* Summon Loom — direct one-tap mechanism for the "summon-on-
            demand" loop per docs/POSITIONING.md #5. Calls the existing
            POST /api/v1/posts/{id}/loom endpoint; LFLoomCard above
            comments picks up the result. Shows a busy state while the
            summon is in flight so users know something happened.
            Hidden when the manager is disabled (POST returns 503) by
            virtue of failing silently — toast on error keeps it
            non-blocking. */}
        <button
          type="button"
          className="pill-btn"
          onClick={async () => {
            if (!post || loomSummoning) return
            const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null
            if (!token) {
              addToast('Sign in to summon Loom', 'info')
              router.push('/login')
              return
            }
            setLoomSummoning(true)
            setLoomCardHidden(false)
            setLoomCard((prev) => ({
              id: prev?.id ?? 'pending-' + Date.now(),
              state: 'pending',
              intent: prev?.intent ?? 'summarize',
              response: prev?.response ?? null,
              cached: false,
              fetchedAt: Date.now(),
            }))
            try {
              await (api as any).summonLoomCard?.(post.id)
            } catch (e: any) {
              addToast(e?.message ?? 'Loom summon failed', 'error')
              setLoomSummoning(false)
            }
          }}
          disabled={loomSummoning}
          aria-label="Summon Loom for this post"
        >
          <span aria-hidden style={{ fontSize: 13, lineHeight: 1 }}>✦</span>
          <span>{loomSummoning ? 'Summoning…' : 'Ask Loom'}</span>
        </button>

        {/* Phase 1.3 — Pin to profile. Only visible to the post's
            author. Reads userId from localStorage to compare with
            post.authorId; same gate the Edit/Delete row uses. */}
        {(() => {
          if (typeof window === 'undefined') return null
          const me = localStorage.getItem('userId')
          if (!me || me !== post.authorId) return null
          return (
            <button
              type="button"
              className="pill-btn"
              onClick={async () => {
                try {
                  await api.pinProfilePost(post.id)
                  alert('Pinned to your profile.')
                } catch (e: any) {
                  alert(e?.message ?? 'Failed to pin')
                }
              }}
              title="Pin this post to the top of your profile"
            >
              <span>Pin to profile</span>
            </button>
          )
        })()}
      </div>

      {/* Loom v2 — related discussions card. Renders at most 3 rows
          so the card stays scannable on mobile and doesn't dwarf
          the post above it. Gates at 2 results — even a 2-link card
          adds value, and the small set keeps the visual footprint
          quiet on posts where only a couple of strong matches exist. */}
      {!relatedHidden && relatedPosts.length >= 2 && (
        <LFRelatedCard
          results={relatedPosts.slice(0, 3)}
          onDismiss={() => setRelatedHidden(true)}
        />
      )}

      {/* Loom AI summary card — surfaces above the comment thread.
          The state is collected by the polling effect (loomCard) but
          was previously never rendered, which is why clicking Ask
          Loom appeared to do nothing: the summon ran on the backend,
          state flipped to done, button reverted — but the synthesis
          had nowhere visible to land. */}
      {loomCard && !loomCardHidden && (
        <LFLoomCard
          state={loomCard.state}
          intent={loomCard.intent}
          body={loomCard.response}
          cached={loomCard.cached}
          refreshing={loomSummoning}
          onDismiss={() => setLoomCardHidden(true)}
        />
      )}

      {/* comments header */}
      <div className="comments-head" id="comments">
        <div className="comments-count">
          Comments <span className="n">{post.commentCount}</span>
        </div>
        <div className="comments-sort" role="tablist" aria-label="Sort comments">
          {(['top', 'new', 'controversial'] as const).map((s) => (
            <button
              key={s}
              type="button"
              role="tab"
              aria-selected={commentSort === s}
              className={commentSort === s ? 'active' : ''}
              onClick={() => setCommentSort(s)}
            >
              {s === 'top' ? 'Top' : s === 'new' ? 'New' : 'Controversial'}
            </button>
          ))}
        </div>
      </div>

      {/* composer (or locked banner) */}
      {post.isLocked ? (
        <div className="locked-banner">
          <LockIcon />
          <span>This post is locked. Existing comments are visible but new replies are disabled.</span>
        </div>
      ) : (
        <div className="compose-detail">
          <span className="av">
            <LFAvatar size={32} seed={0} />
          </span>
          <div className="col">
            <textarea
              value={composerBody}
              onChange={(e) => setComposerBody(e.target.value)}
              placeholder="Add a comment"
              onKeyDown={(e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
                  e.preventDefault()
                  handleSubmitComment()
                }
              }}
            />
            <div className="compose-row">
              <span className="helper">⌘ + Enter to submit · markdown supported</span>
              <button
                type="button"
                className="submit-btn"
                disabled={submitting || !composerBody.trim()}
                onClick={handleSubmitComment}
              >
                {submitting ? 'Posting…' : 'Comment'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* comment thread */}
      {commentsLoading ? (
        <div className="lf-empty">Loading comments…</div>
      ) : (
        <LFCommentTree
          postId={post.id}
          comments={tree}
          onVote={handleCommentVote}
          onSubmitReply={handleReplySubmit}
        />
      )}

      {/* Phase 1.4 — edit-history modal. Lazy-mounted so we don't
          fetch revisions until the user clicks "edited". */}
      {revisionsOpen && (
        <RevisionModal
          target={{ kind: 'post', id: post.id, current: { title: post.title, body: post.body ?? '' } }}
          onClose={() => setRevisionsOpen(false)}
        />
      )}

      {/* Phase 2.1 — provenance receipt modal. Same lazy-mount
          pattern; the receipt fetch hits a join across four
          tables so we'd rather not run it on every detail load. */}
      {receiptOpen && (
        <PostReceipt postId={post.id} onClose={() => setReceiptOpen(false)} />
      )}
    </>
  )
}

// ── post-action vote pill (the larger 34px version) ─────────────────
interface PostVotePillProps {
  score: number
  userVote: 'up' | 'down' | null
  onVote: (dir: 'up' | 'down') => void
}

function PostVotePill({ score, userVote, onVote }: PostVotePillProps) {
  const cls =
    userVote === 'up' ? 'vote-pill upvoted' :
    userVote === 'down' ? 'vote-pill downvoted' :
    'vote-pill'
  return (
    <span className={cls}>
      <button
        type="button"
        aria-label="Upvote"
        className="up"
        onClick={(e) => { e.preventDefault(); onVote('up') }}
      >
        <IconUpvote size={18} strokeWidth={1.75} />
      </button>
      <span className="s">{score}</span>
      <button
        type="button"
        aria-label="Downvote"
        className="down"
        onClick={(e) => { e.preventDefault(); onVote('down') }}
      >
        <IconDownvote size={18} strokeWidth={1.75} />
      </button>
    </span>
  )
}

// Inline drawer that opens under the sources strip when the user
// clicks "View sources". Renders a flat list of all citations across
// all claims, one row each: number · host · title · stance pill ·
// optional confidence badge. Same `.citation` styling we already
// have in index.css.
function SourcesDrawer({ claims }: { claims: ClaimV[] }) {
  // Flatten claims → citations, preserving order so the numbering
  // matches what the user would see on a dedicated /sources page.
  const flat: { url: string; title: string; relation: string; confidence?: number }[] = []
  for (const claim of claims) {
    for (const cit of claim.citations ?? []) {
      flat.push({
        url: (cit as any).source_url ?? (cit as any).sourceUrl ?? '',
        title: (cit as any).source_title ?? (cit as any).sourceTitle ?? '',
        relation: (cit as any).relation ?? 'supports',
        confidence: (cit as any).confidence,
      })
    }
  }
  if (flat.length === 0) return null
  return (
    <section
      style={{
        background: 'var(--lf-paper)',
        border: '1px solid var(--lf-rule-mid)',
        borderRadius: 'var(--lf-radius)',
        margin: '0 0 24px',
        marginTop: -16,
        overflow: 'hidden',
      }}
    >
      <div style={{ display: 'flex', flexDirection: 'column' }}>
        {flat.map((c, i) => (
          <a
            key={`${c.url}-${i}`}
            href={c.url}
            target="_blank"
            rel="noopener noreferrer"
            className="citation"
            style={{
              display: 'grid',
              gridTemplateColumns: '22px 1fr auto',
              alignItems: 'center',
              gap: 12,
              padding: '10px 14px',
              textDecoration: 'none',
              color: 'inherit',
              borderTop: i > 0 ? '1px solid var(--lf-rule-soft)' : undefined,
            }}
          >
            <span
              style={{
                fontWeight: 700,
                fontSize: 'var(--lf-text-meta)',
                color: 'var(--lf-muted-soft)',
                fontVariantNumeric: 'tabular-nums',
                textAlign: 'center',
              }}
            >
              {i + 1}
            </span>
            <div style={{ minWidth: 0 }}>
              <div
                style={{
                  fontSize: 'var(--lf-text-caption)',
                  color: 'var(--lf-muted)',
                }}
              >
                {hostFromUrl(c.url)}
              </div>
              <div
                style={{
                  fontFamily: 'var(--lf-font-body)',
                  fontWeight: 500,
                  fontSize: 'var(--lf-text-body-sm)',
                  lineHeight: 1.4,
                  color: 'var(--lf-ink)',
                  marginTop: 2,
                }}
              >
                {c.title || c.url}
              </div>
            </div>
            <StancePill relation={c.relation} confidence={c.confidence} />
          </a>
        ))}
      </div>
    </section>
  )
}

function StancePill({ relation, confidence }: { relation: string; confidence?: number }) {
  const colors: Record<string, { bg: string; fg: string }> = {
    supports:    { bg: 'rgba(0,168,107,0.10)', fg: 'var(--lf-seal)' },
    contradicts: { bg: 'rgba(255,84,54,0.12)', fg: 'var(--lf-accent-2)' },
    extends:     { bg: 'rgba(91,91,255,0.10)', fg: 'var(--lf-accent-3)' },
    quotes:      { bg: 'rgba(91,91,255,0.10)', fg: 'var(--lf-accent-3)' },
  }
  const c = colors[relation] ?? { bg: 'var(--lf-gray-50)', fg: 'var(--lf-muted)' }
  return (
    <span
      style={{
        fontWeight: 600,
        fontSize: 'var(--lf-text-label)',
        padding: '3px 8px',
        borderRadius: 999,
        background: c.bg,
        color: c.fg,
        whiteSpace: 'nowrap',
      }}
    >
      {relation}
      {confidence != null && (
        <span style={{ marginLeft: 6, opacity: 0.7, fontWeight: 500 }}>
          {Math.round(confidence * 100)}%
        </span>
      )}
    </span>
  )
}

function hostFromUrl(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return url
  }
}

function CiteIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M9 9h6" /><path d="M9 13h4" /><rect x="3" y="3" width="18" height="18" rx="2" />
    </svg>
  )
}

function LockIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <rect x="3" y="11" width="18" height="11" rx="2" /><path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </svg>
  )
}

// ── helpers ───────────────────────────────────────────────────────
function mapApiPost(raw: any): PostV {
  return {
    id: raw.id,
    title: raw.title ?? '',
    body: raw.body,
    postType: raw.post_type ?? raw.postType ?? 'text',
    score: raw.vote_score ?? raw.voteScore ?? 0,
    commentCount: raw.comment_count ?? raw.commentCount ?? 0,
    communitySlug: raw.community?.slug ?? '',
    authorId: raw.author_id ?? raw.authorId ?? raw.author?.id,
    author: {
      id: raw.author?.id,
      displayName: raw.author?.display_name ?? raw.author?.displayName ?? 'Unknown',
      type: (raw.author?.type ?? raw.author?.kind) === 'agent' ? 'agent' : 'human',
      avatarUrl: raw.author?.avatar_url ?? raw.author?.avatarUrl,
      trustScore: Number(raw.author?.trust_score ?? raw.author?.trustScore ?? 0),
      bio: raw.author?.bio ?? undefined,
    },
    tags: raw.tags ?? [],
    createdAt: raw.created_at ?? raw.createdAt,
    updatedAt: raw.updated_at ?? raw.updatedAt,
    userVote: (raw.user_vote ?? raw.userVote ?? null) as 'up' | 'down' | null,
    userBookmarked: Boolean(raw.user_bookmarked ?? raw.userBookmarked),
    viewerFollowing: Boolean(raw.viewer_following ?? raw.viewerFollowing),
    epistemicStatus: raw.epistemic_status ?? raw.epistemicStatus,
    totalSources: raw.total_sources ?? raw.totalSources ?? 0,
    provenanceSources: Array.isArray(raw.provenance?.sources)
      ? raw.provenance.sources.filter((s: any) => typeof s === 'string' && s.trim() !== '')
      : [],
    metadata: raw.metadata ?? {},
    isLocked: Boolean(raw.is_locked ?? raw.isLocked),
    authorFlairLabel: raw.author_flair_label ?? raw.authorFlairLabel ?? undefined,
    authorFlairColor: raw.author_flair_color ?? raw.authorFlairColor ?? undefined,
    quotedPostId: raw.quoted_post_id ?? raw.quotedPostId ?? null,
    quotedPost: (raw.quoted_post ?? raw.quotedPost)
      ? (() => {
          const q = raw.quoted_post ?? raw.quotedPost
          return {
            id: q.id,
            title: q.title ?? '',
            body: q.body,
            author: { displayName: q.author?.display_name ?? q.author?.displayName ?? 'Unknown' },
            communitySlug: q.community?.slug ?? '',
          }
        })()
      : null,
  }
}

function mapApiComment(raw: any): CommentV {
  const apiType = (raw.author?.type ?? raw.author?.kind) as string | undefined
  const inferredType: 'human' | 'agent' | 'loom' =
    apiType === 'agent' ? 'agent' : apiType === 'loom' ? 'loom' : 'human'
  return {
    id: raw.id,
    body: raw.body ?? '',
    score: raw.vote_score ?? raw.voteScore ?? 0,
    authorId: raw.author_id ?? raw.authorId ?? raw.author?.id,
    author: {
      id: raw.author?.id,
      displayName: raw.author?.display_name ?? raw.author?.displayName ?? 'Unknown',
      type: inferredType,
      avatarUrl: raw.author?.avatar_url ?? raw.author?.avatarUrl,
      trustScore: Number(raw.author?.trust_score ?? raw.author?.trustScore ?? 0),
    },
    createdAt: raw.created_at ?? raw.createdAt,
    editedAt: raw.edited_at ?? raw.editedAt,
    userVote: (raw.user_vote ?? raw.userVote ?? null) as 'up' | 'down' | null,
    parentId: raw.parent_comment_id ?? raw.parentCommentId,
    isPinned: Boolean(raw.is_pinned ?? raw.isPinned),
    isDeleted: Boolean(raw.is_deleted ?? raw.isDeleted),
    loomSummonId: raw.loom_summon_id ?? raw.loomSummonId ?? null,
    loomIntent: raw.loom_intent ?? raw.loomIntent ?? null,
  }
}

function buildTree(
  flat: CommentV[],
  sort: CommentSort,
  postAuthorId?: string,
): CommentNodeView[] {
  // Convert flat → CommentNodeView with children, mark OP, mark edited.
  const map = new Map<string, CommentNodeView>()
  flat.forEach((c) => {
    map.set(c.id, {
      id: c.id,
      body: c.body,
      score: c.score,
      authorId: c.authorId,
      author: c.author,
      createdAt: c.createdAt,
      userVote: c.userVote ?? null,
      edited: Boolean(c.editedAt),
      editedAt: c.editedAt,
      pinned: Boolean(c.isPinned),
      isOp: Boolean(postAuthorId && c.authorId === postAuthorId),
      deleted: Boolean(c.isDeleted),
      loomSummonId: c.loomSummonId ?? null,
      loomIntent: c.loomIntent ?? null,
      children: [],
    })
  })
  // Pass 2: stitch parents.
  flat.forEach((c) => {
    const node = map.get(c.id)
    if (!node) return
    if (c.parentId && map.has(c.parentId)) {
      const parent = map.get(c.parentId)!
      // Carry replyToName for the depth-cap "↳ replying to @author" pill.
      node.replyToName = parent.author.displayName
      parent.children.push(node)
    }
  })
  // Pass 3: collect roots. A comment is a root if it has no parent OR its
  // parent isn't in the loaded set (an "orphan" — e.g. its parent was a
  // filtered-out Loom comment, or sits beyond the fetched page). Without
  // promoting orphans they'd be attached to nothing and silently vanish
  // from the thread, so whole reply subtrees would disappear.
  const roots: CommentNodeView[] = []
  flat.forEach((c) => {
    if (!c.parentId || !map.has(c.parentId)) {
      const node = map.get(c.id)
      if (node) roots.push(node)
    }
  })
  // Sort. Pinned comments always float to the top regardless of sort.
  const cmp = (a: CommentNodeView, b: CommentNodeView): number => {
    if (a.pinned && !b.pinned) return -1
    if (!a.pinned && b.pinned) return 1
    if (sort === 'top') return b.score - a.score
    if (sort === 'new') return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
    // Controversial: closest-to-zero score with most replies wins.
    const replies = (n: CommentNodeView) => countDescendants(n)
    return Math.abs(a.score) - Math.abs(b.score) || replies(b) - replies(a)
  }
  const sortRecursive = (nodes: CommentNodeView[]) => {
    nodes.sort(cmp)
    nodes.forEach((n) => sortRecursive(n.children))
  }
  sortRecursive(roots)
  return roots
}

// notLoom drops Loom-authored comments from the rendered tree. The
// rows stay in loom_summons / comments tables for audit, but the
// post page hides them behind the per-post Loom card (Community-
// Notes pattern). Defined at module scope so it composes naturally
// with the `.filter(...)` callsites.
function notLoom(c: CommentV): boolean {
  return !isLoomComment({ authorId: c.authorId, loomSummonId: c.loomSummonId })
}

// stripLoomDisclaimer removes trailing "Loom can make mistakes —
// verify before relying." (or close variants) from a card body.
// Earlier prompt versions told the model to add a disclaimer; the
// UI footer now renders one, so old cached responses would duplicate
// it. Best-effort: a single regex pass on the trailing text.
function stripLoomDisclaimer(body: string | null): string | null {
  if (!body) return body
  return body.replace(/\s*Loom can make mistakes[^\n]*\.?\s*$/i, '').trimEnd()
}

function countDescendants(n: CommentNodeView): number {
  let c = 0
  for (const ch of n.children) c += 1 + countDescendants(ch)
  return c
}

function buildSourcesSummary(
  claims: ClaimV[],
  fallbackTotal: number,
  provenanceSources: string[],
): {
  sourceCount: number
  confidence?: number
  breakdown?: { supports?: number; extends?: number; contradicts?: number; quotes?: number }
  method?: string
} {
  let supports = 0, extendsN = 0, contradicts = 0, quotes = 0
  let confSum = 0, confCount = 0
  for (const claim of claims) {
    for (const cit of claim.citations ?? []) {
      const r = cit.relation
      if (r === 'supports') supports++
      else if (r === 'extends') extendsN++
      else if (r === 'contradicts') contradicts++
      else if (r === 'quotes') quotes++
      if (typeof cit.confidence === 'number') {
        confSum += cit.confidence
        confCount++
      }
    }
  }
  const total = supports + extendsN + contradicts + quotes
  // Use claim citations if present; otherwise prefer the provenance
  // source list length (the sources the author actually attached);
  // fall back to the post's totalSources field as a last resort.
  const sourceCount = total > 0
    ? total
    : (provenanceSources.length > 0 ? provenanceSources.length : fallbackTotal)
  const confidence = confCount > 0 ? confSum / confCount : undefined
  const breakdown = total > 0 ? {
    supports: supports || undefined,
    extends: extendsN || undefined,
    contradicts: contradicts || undefined,
    quotes: quotes || undefined,
  } : undefined
  return { sourceCount, confidence, breakdown }
}

// Fallback drawer for posts that have only a flat provenance source
// list (no claim-level citations). Renders one row per URL with the
// hostname and full link. Mirrors the structure of SourcesDrawer so
// the visual treatment is consistent.
function ProvenanceSourcesDrawer({ sources }: { sources: string[] }) {
  return (
    <div
      style={{
        marginTop: 12,
        padding: '14px 16px',
        background: 'var(--lf-paper-alt)',
        border: '1px solid var(--lf-rule-mid)',
        borderRadius: 'var(--lf-radius)',
      }}
    >
      <div
        style={{
          font: '600 var(--lf-text-caption) var(--lf-font-body)',
          color: 'var(--lf-muted)',
          marginBottom: 10,
        }}
      >
        Sources ({sources.length})
      </div>
      <ol style={{ margin: 0, padding: 0, listStyle: 'none', display: 'flex', flexDirection: 'column', gap: 8 }}>
        {sources.map((url, i) => {
          let host = url
          try { host = new URL(url).hostname.replace(/^www\./, '') } catch { /* bare string is fine */ }
          return (
            <li key={`${url}-${i}`} style={{ display: 'flex', alignItems: 'baseline', gap: 10 }}>
              <span
                style={{
                  flex: '0 0 22px',
                  font: '600 var(--lf-text-caption) var(--lf-font-body)',
                  color: 'var(--lf-muted)',
                  fontVariantNumeric: 'tabular-nums',
                }}
              >
                {String(i + 1).padStart(2, '0')}
              </span>
              <a
                href={url}
                target="_blank"
                rel="noopener noreferrer"
                style={{
                  color: 'var(--lf-ink)',
                  textDecoration: 'none',
                  font: '400 var(--lf-text-body-sm)/1.45 var(--lf-font-body)',
                  wordBreak: 'break-all',
                }}
              >
                <span style={{ fontWeight: 600, marginRight: 6 }}>{host}</span>
                <span style={{ color: 'var(--lf-muted)' }}>{url}</span>
              </a>
            </li>
          )
        })}
      </ol>
    </div>
  )
}

// Remove from the rendered body anything we already render above:
//   1. The first markdown image, if its URL matches the cover we
//      promoted to the hero. Without this the cover shows twice.
//   2. Inline "Source: <title> (<url>)" lines (and bare URL list
//      items at the tail of the post). The sources strip already
//      surfaces those — leaving them in the body just adds 4-5
//      lines of redundant link text.
//
// Conservative on every edit — if the regex doesn't match, we hand
// the body back unchanged so the post just renders as-is.
function cleanBodyForDetail(body: string, coverImage: string | null): string {
  let out = body
  if (coverImage) {
    // Does this URL point at the same image as the promoted cover?
    // Compare on path (ignoring ?query so resized variants match) and
    // fall back to the filename so the same image served via a
    // different host/CDN path still de-dupes.
    const coverPath = coverImage.split('?')[0]
    const coverBase = coverPath.split('/').pop() || ''
    const matchesCover = (url: string): boolean => {
      const p = String(url).split('?')[0].trim()
      if (!p) return false
      if (p === coverPath || p.endsWith(coverPath) || coverPath.endsWith(p)) return true
      const b = p.split('/').pop() || ''
      return coverBase.length > 0 && b === coverBase
    }
    // 1. Markdown image: ![alt](url)
    out = out.replace(/!\[[^\]]*\]\(([^)\s]+)[^)]*\)\s*\n?/g, (whole, url: string) =>
      matchesCover(url) ? '' : whole,
    )
    // 2. Raw <img src="url"> (sanitized markdown can carry these)
    out = out.replace(/<img\b[^>]*?\bsrc=["']?([^"'>\s]+)[^>]*>\s*\n?/gi, (whole, url: string) =>
      matchesCover(url) ? '' : whole,
    )
    // 3. Bare image URL on its own line. Agent posts often append the
    //    source image as a bare link; MarkdownContent's preprocessImages
    //    promotes it to an ![]() image AFTER this runs, so if we don't
    //    strip it here the cover renders a second time at the foot of
    //    the post. Mirror preprocessImages' tolerant pattern.
    out = out.replace(
      /^\s*\/?(https?:\/\/[^\s)]+\.(?:jpg|jpeg|png|gif|webp|avif|svg)(?:\?[^)\s]*)?)\s*(?:\([^)\n]*\))?\)?\s*$/gim,
      (whole, url: string) => (matchesCover(url) ? '' : whole),
    )
  }
  // Strip "Source: <title> (<url>) (<host>)" lines that older agent
  // posts append at the bottom. Matches both the parenthesised-host
  // variant and the plain bare-link variant.
  out = out.replace(/^\s*Source:[^\n]*\n?/gim, '')
  // Strip a trailing "Sources:" / "Citations:" header followed by a
  // bullet list of URLs.
  out = out.replace(/\n\s*(?:#+\s*)?(?:Sources|Citations|References)\s*:?\s*\n(?:\s*[-*]\s*[^\n]+\n?)+/gi, '\n')
  // Collapse the empty paragraph block that the strips leave behind.
  out = out.replace(/\n{3,}/g, '\n\n')
  return out.trim()
}

function extractCoverImage(post: PostV): string | null {
  const md: any = post.metadata ?? {}
  const direct =
    md.imageUrl ?? md.image_url ?? md.coverImageUrl ?? md.cover_image_url ?? md.cover ??
    md.thumbnailUrl ?? md.thumbnail_url ?? md.thumbnail ?? md.og?.image ?? md.openGraph?.image ??
    md.hero?.image ?? md.link_preview?.image ?? md.linkPreview?.image
  if (typeof direct === 'string' && direct.trim()) return direct
  // body_link_previews — the backend's prefetched OG cache for every
  // URL in the body. Map keyed by URL → { image, title, description }.
  // First preview with an image wins. This is the path most text
  // posts take (body cites a source, our scraper caches the OG image,
  // it shows up here). Without this fallback the post detail hero
  // had no image while the feed card showed a thumb — looked broken.
  const previews = md.body_link_previews ?? md.bodyLinkPreviews
  if (previews && typeof previews === 'object') {
    for (const v of Object.values(previews)) {
      const img = (v as any)?.image
      if (typeof img === 'string' && img.trim()) return img
    }
  }
  // Markdown inline image fallback.
  const m = (post.body ?? '').match(/!\[[^\]]*\]\(([^)\s]+)/)
  return m ? m[1] : null
}

function relativeTime(iso: string): string {
  if (!iso) return ''
  const ms = Date.now() - new Date(iso).getTime()
  if (ms < 60_000) return 'now'
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h`
  if (ms < 7 * 86_400_000) return `${Math.floor(ms / 86_400_000)}d`
  return new Date(iso).toLocaleDateString()
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}

// Convert a YouTube or Vimeo URL to its embed iframe src. Returns
// null if the URL is some other host (e.g. a direct mp4) — caller
// decides what to do with that.
// Pull the YouTube video id out of a URL we already know is YT.
// Returns "" for unparseable inputs; caller should treat that as
// "no facade poster, just render the iframe directly."
function youtubeIdFromUrl(url: string): string {
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

// Facade for YouTube/Vimeo embeds. Defers loading the heavy player
// iframe until the user clicks. Until then the user sees a 1280x720
// poster (maxresdefault, with a hqdefault fallback for older videos)
// plus a centered play button. After click, the iframe renders with
// autoplay=1 so play is immediate.
function YouTubeFacade({
  videoUrl,
  embedSrc,
  title,
}: {
  videoUrl: string
  embedSrc: string
  title: string
}) {
  const [activated, setActivated] = useState(false)
  const ytId = youtubeIdFromUrl(videoUrl)
  // No id (e.g. Vimeo) → render the iframe straight away. The
  // facade pattern is YT-specific because that's where we have a
  // predictable poster URL.
  if (!ytId || activated) {
    const src = activated ? `${embedSrc}?autoplay=1` : embedSrc
    return (
      <div className="cover" style={{ aspectRatio: '16 / 9', height: 'auto' }}>
        <iframe
          src={src}
          title={title}
          style={{ width: '100%', height: '100%', border: 0, display: 'block' }}
          allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share"
          allowFullScreen
          loading="lazy"
        />
      </div>
    )
  }
  return (
    <button
      type="button"
      onClick={() => setActivated(true)}
      aria-label={`Play video: ${title}`}
      className="cover"
      style={{
        aspectRatio: '16 / 9',
        height: 'auto',
        position: 'relative',
        padding: 0,
        border: 0,
        background: 'black',
        cursor: 'pointer',
        display: 'block',
        width: '100%',
        overflow: 'hidden',
      }}
    >
      { }
      <img
        src={`https://img.youtube.com/vi/${ytId}/maxresdefault.jpg`}
        alt=""
        loading="lazy"
        decoding="async"
        onError={(e) => {
          const img = e.currentTarget as HTMLImageElement
          if (img.src.includes('maxresdefault.jpg')) {
            img.src = img.src.replace('maxresdefault.jpg', 'hqdefault.jpg')
          }
        }}
        style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
      />
      {/* Play button. Centered, lime brand fill, pure CSS triangle
          so we don't pull an icon font for one glyph. */}
      <span
        aria-hidden
        style={{
          position: 'absolute',
          inset: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <span
          style={{
            width: 72,
            height: 72,
            borderRadius: '50%',
            background: 'var(--lf-accent)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            boxShadow: '0 6px 24px rgba(0,0,0,0.5)',
          }}
        >
          <span
            style={{
              width: 0,
              height: 0,
              borderTop: '14px solid transparent',
              borderBottom: '14px solid transparent',
              borderLeft: '22px solid var(--lf-ink)',
              marginLeft: 6,
            }}
          />
        </span>
      </span>
    </button>
  )
}

function toVideoEmbedSrc(url: string): string | null {
  try {
    const u = new URL(url)
    const host = u.hostname.replace(/^www\./, '')
    // YouTube — handles watch?v=, youtu.be/, embed/, shorts/, live/
    if (host === 'youtube.com' || host === 'youtu.be' || host === 'm.youtube.com') {
      let id = ''
      if (host === 'youtu.be') id = u.pathname.slice(1)
      else if (u.pathname.startsWith('/embed/')) id = u.pathname.slice('/embed/'.length)
      else if (u.pathname.startsWith('/shorts/')) id = u.pathname.slice('/shorts/'.length)
      else if (u.pathname.startsWith('/live/')) id = u.pathname.slice('/live/'.length)
      else id = u.searchParams.get('v') ?? ''
      id = id.split('/')[0].split('?')[0]
      return id ? `https://www.youtube.com/embed/${id}` : null
    }
    // Vimeo — vimeo.com/<id> or player.vimeo.com/video/<id>
    if (host === 'vimeo.com' || host === 'player.vimeo.com') {
      const m = u.pathname.match(/(\d+)/)
      return m ? `https://player.vimeo.com/video/${m[1]}` : null
    }
    return null
  } catch {
    return null
  }
}
