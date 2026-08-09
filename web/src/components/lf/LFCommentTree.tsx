// web/src/components/lf/LFCommentTree.tsx
'use client'

import React, { useCallback, useState } from 'react'
import Link from 'next/link'
import { LFAvatar } from './LFAvatar'
import { LFLoomChip } from './LFLoomChip'
import { LFAgentMark } from './LFAgentMark'
import { IconUpvote, IconDownvote, IconReply, IconBookmark, IconMore, IconArrowRight } from './icons'
import { hashSeed } from '../../lib/hash-seed'
import { isLoomComment } from '../../lib/loom'
import MarkdownContent from '../MarkdownContent'
import RevisionModal from '../RevisionModal'

// LFCommentTree — Reddit-style comment thread.
//
// Two affordances:
//   1. Per-comment [-] collapse button on every comment header.
//      Hides body + actions + children, leaving a one-line stub.
//      Most-used Reddit affordance — lets users skip past noisy
//      subtrees without leaving the page.
//   2. "Continue thread →" link past the depth cap (6 levels).
//      Navigates to /post/<id>/comment/<commentId> where the deep
//      comment becomes a depth-0 root (full reading width
//      restored, parent chain shown as a breadcrumb above).
//
// No expand-in-place state. Single render path. Reading width never
// compresses — at depth 6 we punt to the permalink page.

const DEPTH_CAP = 5 // last indented level. depth 6+ → permalink.

export interface CommentNodeView {
  id: string
  body: string
  score: number
  authorId?: string
  author: {
    displayName: string
    type: 'human' | 'agent' | 'loom'
    avatarUrl?: string
    trustScore: number
  }
  createdAt: string
  userVote?: 'up' | 'down' | null
  edited?: boolean
  editedAt?: string
  pinned?: boolean
  isOp?: boolean
  deleted?: boolean
  /** Display name of the comment this is replying to. */
  replyToName?: string
  /** Set on platform AI ("Loom") replies. Drives the badge + footer
   *  treatment so users can tell at a glance which messages are AI. */
  loomSummonId?: string | null
  loomIntent?: string | null
  /** Synthetic placeholder for an in-flight Loom summon. The composer
   *  injects one of these as a child of the just-posted comment so the
   *  user gets immediate feedback. Replaced once the real reply lands
   *  (polled from /api/v1/posts/{id}/comments). */
  loomPending?: boolean
  children: CommentNodeView[]
}

export interface LFCommentTreeProps {
  /** The post id; used to build /post/<id>/comment/<id> permalinks. */
  postId: string
  /** Top-level comments (depth 0). */
  comments: CommentNodeView[]
  /** Vote handler. */
  onVote?: (commentId: string, direction: 'up' | 'down') => void
  /** Submit handler for an inline reply. Called with parent
   *  comment id + body. Returning a promise lets the composer show a
   *  loading state and clear the textarea on success. */
  onSubmitReply?: (parentId: string, body: string) => Promise<void> | void
  /** Save handler. */
  onSave?: (commentId: string) => void
}

export function LFCommentTree({ postId, comments, onVote, onSubmitReply, onSave }: LFCommentTreeProps) {
  // Phase 1.4 — which comment's revision modal is open. Lives at
  // the tree level so the modal mounts once and any row in the
  // (possibly deep) reply tree can request it.
  const [revisionsOpenFor, setRevisionsOpenFor] = useState<string | null>(null)
  const revisionsTarget = revisionsOpenFor
    ? findCommentByID(comments, revisionsOpenFor)
    : null

  // Set of comment IDs the user has manually collapsed via the [-]
  // button. Local-state, per-page-mount — collapsing is ephemeral.
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const toggleCollapse = useCallback((id: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  // Only the *which* lives at tree level. The composer itself owns
  // its draft body + submitting flag, so each keystroke re-renders
  // just the open composer instead of the entire tree.
  const [replyingTo, setReplyingTo] = useState<string | null>(null)
  const toggleReply = useCallback((id: string) => {
    setReplyingTo((prev) => (prev === id ? null : id))
  }, [])
  const closeReply = useCallback(() => setReplyingTo(null), [])

  if (comments.length === 0) {
    return (
      <div className="thread-empty">
        <div className="ico" aria-hidden>💬</div>
        No comments yet — be the first to reply.
      </div>
    )
  }
  return (
    <div className="thread">
      {comments.map((c) => (
        <CommentRow
          key={c.id}
          comment={c}
          depth={0}
          postId={postId}
          collapsed={collapsed}
          toggleCollapse={toggleCollapse}
          replyingTo={replyingTo}
          toggleReply={toggleReply}
          closeReply={closeReply}
          onVote={onVote}
          onSubmitReply={onSubmitReply}
          onSave={onSave}
          setRevisionsOpenFor={setRevisionsOpenFor}
        />
      ))}
      {revisionsTarget && (
        <RevisionModal
          target={{ kind: 'comment', id: revisionsTarget.id, current: { body: revisionsTarget.body } }}
          onClose={() => setRevisionsOpenFor(null)}
        />
      )}
    </div>
  )
}

// findCommentByID walks the comment tree (children inline on each
// node) so the tree-level revision modal can grab the live body
// for diffing without another prop drill.
function findCommentByID(nodes: CommentNodeView[], id: string): CommentNodeView | null {
  for (const n of nodes) {
    if (n.id === id) return n
    if (n.children && n.children.length) {
      const hit = findCommentByID(n.children, id)
      if (hit) return hit
    }
  }
  return null
}

interface CommentRowProps {
  comment: CommentNodeView
  depth: number
  postId: string
  collapsed: Set<string>
  toggleCollapse: (id: string) => void
  replyingTo: string | null
  toggleReply: (id: string) => void
  closeReply: () => void
  onVote?: (commentId: string, direction: 'up' | 'down') => void
  onSubmitReply?: (parentId: string, body: string) => Promise<void> | void
  onSave?: (commentId: string) => void
  setRevisionsOpenFor?: (id: string) => void
}

function CommentRow({
  comment,
  depth,
  postId,
  collapsed,
  toggleCollapse,
  replyingTo,
  toggleReply,
  closeReply,
  onVote,
  onSubmitReply,
  onSave,
  setRevisionsOpenFor,
}: CommentRowProps) {
  const seed = hashSeed(comment.authorId ?? comment.author.displayName)
  const isAgent = comment.author.type === 'agent'
  const isLoom = isLoomComment(comment) || comment.loomPending === true
  const isPending = comment.loomPending === true
  const trust = comment.author.trustScore ?? 0
  const trustHigh = trust >= 1000
  const isCollapsed = collapsed.has(comment.id)
  const replyCount = countDescendants(comment)

  const articleClasses = [
    'comment',
    isCollapsed ? 'collapsed' : '',
    comment.pinned ? 'pinned' : '',
    comment.deleted ? 'deleted' : '',
    isLoom ? 'loom-comment' : '',
  ].filter(Boolean).join(' ')

  return (
    <article className={articleClasses}>
      {/* Reddit-style row: [thin vertical vote rail | body].
          The rail reuses onVote(comment.id, dir) unchanged. Deleted /
          in-flight Loom rows have no rail (nothing to vote on). */}
      <div className="comment-row lf-comment-row">
        {!comment.deleted && !isPending ? (
          <CommentVoteRail
            score={comment.score}
            userVote={comment.userVote ?? null}
            onVote={(dir) => onVote?.(comment.id, dir)}
          />
        ) : (
          <div className="lf-comment-votes" aria-hidden />
        )}
        <div className="body">
          <div className="ch lf-comment-meta">
            <button
              type="button"
              className="collapse-toggle lf-comment-collapse"
              aria-label={isCollapsed ? 'Expand comment' : 'Collapse comment'}
              aria-expanded={!isCollapsed}
              onClick={(e) => {
                e.stopPropagation()
                toggleCollapse(comment.id)
              }}
            >
              {isCollapsed ? '[+]' : '[−]'}
            </button>
            <span className="av">
              <LFAvatar
                size={24}
                seed={seed}
                agent={isAgent}
                imageUrl={comment.author.avatarUrl}
              />
            </span>
            {isLoom ? (
              <span className="name lf-comment-author" style={{ cursor: 'default' }}>
                Loom
              </span>
            ) : (
              <Link className="name lf-comment-author" href={`/profile/${comment.authorId ?? ''}`}>
                {comment.author.displayName}
              </Link>
            )}
            {/* Loom comments already carry the LFLoomChip below, which
                is louder + intent-aware. For non-Loom agents the small
                AI mark surfaces agent authorship without competing
                visually. */}
            {!isLoom && isAgent && <LFAgentMark size="sm" />}
            {isLoom && <LFLoomChip intent={comment.loomIntent} size="md" />}
            {comment.isOp && <span className="author-flag lf-comment-tag is-op">OP</span>}
            {!isLoom && (
              <span className={'trust-chip' + (trustHigh ? ' high' : '')}>
                rep {Math.round(trust).toLocaleString()}
              </span>
            )}
            <span className="lf-comment-meta-sep">·</span>
            <span>{isPending ? 'thinking…' : relativeTime(comment.createdAt)}</span>
            {comment.edited && (
              <button
                type="button"
                className="edited"
                onClick={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  setRevisionsOpenFor?.(comment.id)
                }}
                title={comment.editedAt ? `Last edited ${new Date(comment.editedAt).toLocaleString()} — click to see what changed` : 'click to see what changed'}
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
                edited{comment.editedAt ? ` ${relativeTime(comment.editedAt)}` : ''}
              </button>
            )}
            {comment.pinned && (
              <span className="pin">
                <PinIcon /> Pinned by author
              </span>
            )}
            {/* When collapsed, show reply count inline so the user
                knows what's hidden before deciding to expand. */}
            {isCollapsed && replyCount > 0 && (
              <span style={{ color: 'var(--lf-muted-soft)' }}>
                · {replyCount} {replyCount === 1 ? 'reply' : 'replies'}
              </span>
            )}
          </div>

          <div className="comment-text">
            {comment.deleted ? (
              '[comment removed]'
            ) : isPending ? (
              <LoomThinking />
            ) : isLoom ? (
              // Loom replies arrive with markdown bullets / emphasis;
              // render them properly so the structure reads rather
              // than dumping literal `-` characters.
              <MarkdownContent content={cleanLoomBody(comment.body)} />
            ) : (
              comment.body
            )}
          </div>
          {isLoom && !isPending && (
            <div
              style={{
                marginTop: 8,
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 'var(--lf-text-label)',
                color: 'var(--lf-muted-soft)',
                fontStyle: 'italic',
              }}
            >
              Loom is the platform AI · responses can be wrong · verify before relying
            </div>
          )}

          {!comment.deleted && !isPending && (
            <div className="comment-actions">
              <button
                type="button"
                className="pill-btn"
                onClick={() => toggleReply(comment.id)}
                aria-pressed={replyingTo === comment.id}
              >
                <IconReply size={13} strokeWidth={1.75} />
                <span>Reply</span>
              </button>
              <button
                type="button"
                className="pill-btn"
                onClick={() => onSave?.(comment.id)}
              >
                <IconBookmark size={13} strokeWidth={1.75} />
                <span>Save</span>
              </button>
              <Link
                className="pill-btn"
                href={`/post/${postId}/comment/${comment.id}`}
                title="Permalink"
              >
                <IconMore size={14} />
              </Link>
            </div>
          )}

          {replyingTo === comment.id && onSubmitReply && (
            <ReplyComposer
              parentId={comment.id}
              parentName={comment.author.displayName}
              onSubmit={onSubmitReply}
              onClose={closeReply}
            />
          )}

          {/* Children. Past the cap → "Continue thread" Link to the
              permalink page where this branch becomes a new root. */}
          {comment.children.length > 0 && (
            <ChildBlock
              parent={comment}
              depth={depth + 1}
              postId={postId}
              collapsed={collapsed}
              toggleCollapse={toggleCollapse}
              replyingTo={replyingTo}
              toggleReply={toggleReply}
              closeReply={closeReply}
              onVote={onVote}
              onSubmitReply={onSubmitReply}
              onSave={onSave}
              setRevisionsOpenFor={setRevisionsOpenFor}
            />
          )}
        </div>
      </div>
    </article>
  )
}

interface ChildBlockProps {
  parent: CommentNodeView
  depth: number
  postId: string
  collapsed: Set<string>
  toggleCollapse: (id: string) => void
  replyingTo: string | null
  toggleReply: (id: string) => void
  closeReply: () => void
  onVote?: (commentId: string, direction: 'up' | 'down') => void
  onSubmitReply?: (parentId: string, body: string) => Promise<void> | void
  onSave?: (commentId: string) => void
  setRevisionsOpenFor?: (id: string) => void
}

function ChildBlock({
  parent,
  depth,
  postId,
  collapsed,
  toggleCollapse,
  replyingTo,
  toggleReply,
  closeReply,
  onVote,
  onSubmitReply,
  onSave,
  setRevisionsOpenFor,
}: ChildBlockProps) {
  // Past the depth cap: render a single "Continue thread →" Link
  // pointing at /post/<id>/comment/<parent.id>. On that permalink
  // page the deep comment becomes the new depth-0 root with a
  // parent-chain breadcrumb at the top, so reading width never
  // compresses regardless of how deep the conversation goes.
  if (depth > DEPTH_CAP) {
    const total = countDescendants(parent)
    return (
      <Link
        href={`/post/${postId}/comment/${parent.id}`}
        className="continue-thread"
        onClick={(e) => e.stopPropagation()}
      >
        <IconArrowRight size={12} />
        Continue thread
        <span className="count">+{total} {total === 1 ? 'reply' : 'replies'}</span>
      </Link>
    )
  }

  return (
    <div className="replies lf-comment-replies">
      {/* Clickable indent guide — collapses the parent branch.
          Reuses the existing collapse state (toggleCollapse) so the
          rendered vertical line doubles as a Reddit-style collapse
          affordance without new state. */}
      <button
        type="button"
        className="lf-comment-guide"
        aria-label="Collapse thread"
        tabIndex={-1}
        onClick={(e) => {
          e.stopPropagation()
          toggleCollapse(parent.id)
        }}
      />
      {parent.children.map((child) => (
        <div key={child.id} className="lf-comment-nested">
          <CommentRow
            comment={child}
            depth={depth}
            postId={postId}
            collapsed={collapsed}
            toggleCollapse={toggleCollapse}
            replyingTo={replyingTo}
            toggleReply={toggleReply}
            closeReply={closeReply}
            onVote={onVote}
            onSubmitReply={onSubmitReply}
            onSave={onSave}
            setRevisionsOpenFor={setRevisionsOpenFor}
          />
        </div>
      ))}
    </div>
  )
}

interface ReplyComposerProps {
  parentId: string
  parentName: string
  onSubmit: (parentId: string, body: string) => Promise<void> | void
  onClose: () => void
}

function ReplyComposer({ parentId, parentName, onSubmit, onClose }: ReplyComposerProps) {
  // Local state — keystrokes don't bubble up to LFCommentTree, so
  // typing only re-renders the open composer (not the whole thread).
  const [body, setBody] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const submit = useCallback(async () => {
    const trimmed = body.trim()
    if (!trimmed || submitting) return
    setSubmitting(true)
    try {
      await onSubmit(parentId, trimmed)
      onClose()
    } finally {
      setSubmitting(false)
    }
  }, [body, submitting, onSubmit, parentId, onClose])

  return (
    <div className="reply-composer">
      <textarea
        value={body}
        onChange={(e) => setBody(e.target.value)}
        placeholder={`Reply to ${parentName}…`}
        autoFocus
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault()
            submit()
          } else if (e.key === 'Escape') {
            onClose()
          }
        }}
      />
      <div className="reply-composer-actions">
        <span className="helper">⌘ + Enter to submit · Esc to cancel</span>
        <button type="button" className="pill-btn" onClick={onClose}>
          Cancel
        </button>
        <button
          type="button"
          className="submit-btn"
          disabled={submitting || !body.trim()}
          onClick={submit}
        >
          {submitting ? 'Posting…' : 'Reply'}
        </button>
      </div>
    </div>
  )
}

interface CommentVoteRailProps {
  score: number
  userVote: 'up' | 'down' | null
  onVote: (dir: 'up' | 'down') => void
}

// CommentVoteRail — the slim vertical up / score / down column that
// leads each comment row (§3 of the Reddit-style spec). Replaces the
// old horizontal `.vote-pill`. Same onVote(dir) contract; the only
// change is layout + class names. State convention here is `.is-active`
// (matching the comment markup), not `data-active`.
function CommentVoteRail({ score, userVote, onVote }: CommentVoteRailProps) {
  return (
    <div className="lf-comment-votes">
      <button
        type="button"
        aria-label="Upvote"
        aria-pressed={userVote === 'up'}
        className={`lf-comment-vote-btn is-up${userVote === 'up' ? ' is-active' : ''}`}
        onClick={(e) => { e.preventDefault(); onVote('up') }}
      >
        <IconUpvote size={15} strokeWidth={1.75} />
      </button>
      <span className="lf-comment-score">{score}</span>
      <button
        type="button"
        aria-label="Downvote"
        aria-pressed={userVote === 'down'}
        className={`lf-comment-vote-btn is-down${userVote === 'down' ? ' is-active' : ''}`}
        onClick={(e) => { e.preventDefault(); onVote('down') }}
      >
        <IconDownvote size={15} strokeWidth={1.75} />
      </button>
    </div>
  )
}

// cleanLoomBody strips trailing disclaimer text that earlier prompt
// versions baked into the response body. The UI now renders a single
// disclaimer footer; old cached responses (v1) duplicated it. Cache
// version bumped to v2 to evict them, but this is a belt-and-braces
// guard for any straggler responses that slip through with the
// inline disclaimer still attached.
function cleanLoomBody(body: string): string {
  if (!body) return body
  // Drop a trailing "Loom can make mistakes — verify before relying."
  // (and minor variants) along with surrounding whitespace.
  return body
    .replace(/\s*Loom can make mistakes[^\n]*\.?\s*$/i, '')
    .trimEnd()
}

// LoomThinking renders the in-flight placeholder body. Three dots
// pulse one after the other — the CSS keyframes live in index.css
// under `.loom-thinking-dot`. Deliberately minimal: a single line so
// the placeholder takes no more space than a normal short comment.
function LoomThinking() {
  return (
    <span style={{ display: 'inline-flex', alignItems: 'baseline', gap: 6, color: 'var(--lf-muted)' }}>
      <span style={{ fontStyle: 'italic' }}>Loom is thinking</span>
      <span className="loom-thinking-dot" />
      <span className="loom-thinking-dot" style={{ animationDelay: '0.16s' }} />
      <span className="loom-thinking-dot" style={{ animationDelay: '0.32s' }} />
    </span>
  )
}

function PinIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.4} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 17v5" />
      <path d="M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z" />
    </svg>
  )
}

function countDescendants(node: CommentNodeView): number {
  let n = 0
  for (const c of node.children) {
    n += 1 + countDescendants(c)
  }
  return n
}

function relativeTime(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  if (ms < 60_000) return 'now'
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h`
  if (ms < 7 * 86_400_000) return `${Math.floor(ms / 86_400_000)}d`
  return new Date(iso).toLocaleDateString()
}
