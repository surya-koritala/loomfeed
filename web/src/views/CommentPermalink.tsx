'use client'

import { useState, useCallback, useMemo } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import { useToast } from '../components/ToastProvider'
import { LFCommentTree, type CommentNodeView } from '../components/lf'

// /post/<postId>/comment/<commentId> — renders a deep comment as
// the new depth-0 root with a parent-chain breadcrumb above. Reading
// width never compresses regardless of how deep the conversation
// went on the original post page.

interface Ancestor {
  id: string
  authorId: string
  displayName: string
}

interface ThreadData {
  comment: any
  ancestors: any[]
  descendants: any[]
  postId: string
  truncated?: boolean
}

export interface CommentPermalinkProps {
  postId: string
  initialThread: ThreadData
}

export default function CommentPermalink({ postId, initialThread }: CommentPermalinkProps) {
  const router = useRouter()
  const { addToast } = useToast()
  const [thread, setThread] = useState<ThreadData>(initialThread)

  // Build the depth-0 tree: root comment + descendants stitched.
  const tree = useMemo<CommentNodeView[]>(() => {
    const rootRaw = thread.comment
    const rootNode = mapApiComment(rootRaw)
    const descRaw = thread.descendants ?? []

    // Map of id → node so we can hang each descendant off its
    // parent. Stash parentCommentId on each raw row alongside the
    // mapped node so we can stitch in one pass.
    const nodes = new Map<string, CommentNodeView>()
    nodes.set(rootNode.id, rootNode)

    type Pair = { raw: any; node: CommentNodeView }
    const pairs: Pair[] = descRaw.map((raw: any) => {
      const node = mapApiComment(raw)
      nodes.set(node.id, node)
      return { raw, node }
    })

    for (const { raw, node } of pairs) {
      const parentId = raw.parentCommentId ?? raw.parent_comment_id
      const parent = parentId ? nodes.get(parentId) : null
      if (parent) {
        parent.children.push(node)
      } else {
        // Orphaned (parent not in the descendants list — should not
        // happen given the recursive CTE, but be defensive). Attach
        // to root so the comment is at least visible.
        rootNode.children.push(node)
      }
    }
    return [rootNode]
  }, [thread])

  const ancestors: Ancestor[] = useMemo(
    () => (thread.ancestors ?? []).map((a: any) => ({
      id: a.id,
      authorId: a.authorId ?? a.author_id,
      displayName: a.displayName ?? a.display_name,
    })),
    [thread],
  )

  const handleSubmitReply = useCallback(
    async (parentId: string, body: string) => {
      if (typeof window !== 'undefined' && !localStorage.getItem('token')) {
        addToast('Login required to reply', 'info')
        router.push('/login')
        throw new Error('login required')
      }
      try {
        const raw = await api.createComment(postId, {
          body,
          parent_comment_id: parentId,
        })
        setThread((prev) => ({
          ...prev,
          descendants: [...(prev.descendants ?? []), raw],
        }))
      } catch (e) {
        addToast('Failed to post reply', 'error')
        throw e
      }
    },
    [postId, addToast, router],
  )

  const handleVote = useCallback(async (commentId: string, direction: 'up' | 'down') => {
    if (typeof window === 'undefined') return
    if (!localStorage.getItem('token')) {
      addToast('Login required to vote', 'info')
      router.push('/login')
      return
    }
    // Optimistic vote on whichever node holds this comment id.
    setThread((prev) => {
      const flip = (c: any): any => {
        if (!c) return c
        const cid = c.id
        if (cid === commentId) {
          const prevVote = (c.userVote ?? c.user_vote ?? null) as 'up' | 'down' | null
          const same = prevVote === direction
          const nextVote = same ? null : direction
          const prevDelta = prevVote === 'up' ? 1 : prevVote === 'down' ? -1 : 0
          const nextDelta = nextVote === 'up' ? 1 : nextVote === 'down' ? -1 : 0
          return {
            ...c,
            voteScore: (c.voteScore ?? c.vote_score ?? 0) - prevDelta + nextDelta,
            userVote: nextVote,
          }
        }
        return c
      }
      return {
        ...prev,
        comment: flip(prev.comment),
        descendants: (prev.descendants ?? []).map(flip),
      }
    })
    try {
      await api.vote({ target_id: commentId, target_type: 'comment', direction })
    } catch { /* ignore */ }
  }, [addToast, router])

  // h1 for SEO + a11y. Hidden from sighted users (the visible "lead"
  // line below carries the same context) but read by screen readers
  // and search engines. The comment author name comes from thread.comment;
  // when initial data hasn't hydrated yet we fall back to a generic title.
  const commentAuthor =
    thread.comment?.author?.display_name ??
    thread.comment?.author?.displayName ??
    'a participant'
  const headingText = `Comment by ${commentAuthor}`

  return (
    <>
      <h1
        style={{
          position: 'absolute',
          width: 1,
          height: 1,
          padding: 0,
          margin: -1,
          overflow: 'hidden',
          clip: 'rect(0, 0, 0, 0)',
          whiteSpace: 'nowrap',
          border: 0,
        }}
      >
        {headingText}
      </h1>
      {/* parent-chain breadcrumb. Reads "Replying to a thread by
          Anika → Vector → …". Final entry is the comment we're
          showing, so we omit it from the chain (the comment renders
          right below the breadcrumb anyway). */}
      <div className="perma-breadcrumb">
        <div className="lead">
          {ancestors.length > 0 ? 'Replying to a thread by' : 'Permalink to a comment'}
        </div>
        {ancestors.length > 0 && (
          <div className="chain">
            {ancestors.map((a, i) => (
              <span key={a.id} style={{ display: 'inline-flex', alignItems: 'center', gap: '6px 4px' }}>
                {i > 0 && <span className="arrow">→</span>}
                <Link href={`/post/${postId}/comment/${a.id}`}>{a.displayName}</Link>
              </span>
            ))}
          </div>
        )}
        <div>
          <Link className="full" href={`/post/${postId}`}>
            View full thread (start of post) →
          </Link>
        </div>
      </div>

      {/* The deep comment + its subtree, rendered as if it were a
          fresh top-level conversation. Reading width is the full
          720px main column. */}
      <LFCommentTree
        postId={postId}
        comments={tree}
        onVote={handleVote}
        onSubmitReply={handleSubmitReply}
      />

      {thread.truncated && (
        <div
          style={{
            margin: '24px 0 0',
            padding: '14px 16px',
            background: 'var(--lf-paper-alt)',
            border: '1px solid var(--lf-rule-mid)',
            borderRadius: 'var(--lf-radius)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 14,
            flexWrap: 'wrap',
          }}
        >
          <div style={{ font: '500 13px/1.45 var(--lf-font-body)', color: 'var(--lf-muted)' }}>
            More replies in this thread were trimmed for readability.
          </div>
          <Link
            href={`/post/${postId}`}
            style={{
              font: '600 13px var(--lf-font-body)',
              color: 'var(--lf-ink)',
              textDecoration: 'none',
              padding: '7px 14px',
              border: '1px solid var(--lf-rule-mid)',
              borderRadius: 'var(--lf-radius-pill)',
              background: 'var(--lf-paper)',
              whiteSpace: 'nowrap',
            }}
          >
            View full thread →
          </Link>
        </div>
      )}
    </>
  )
}

function mapApiComment(raw: any): CommentNodeView {
  return {
    id: raw.id,
    body: raw.body ?? '',
    score: Number(raw.voteScore ?? raw.vote_score ?? 0),
    authorId: raw.authorId ?? raw.author_id ?? raw.author?.id,
    author: {
      displayName: raw.author?.displayName ?? raw.author?.display_name ?? 'Unknown',
      type: ((raw.author?.type ?? raw.author?.kind) === 'agent' ? 'agent' : 'human') as 'human' | 'agent',
      avatarUrl: raw.author?.avatarUrl ?? raw.author?.avatar_url,
      trustScore: Number(raw.author?.trustScore ?? raw.author?.trust_score ?? 0),
    },
    createdAt: raw.createdAt ?? raw.created_at,
    userVote: (raw.userVote ?? raw.user_vote ?? null) as 'up' | 'down' | null,
    children: [],
  }
}

