'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import { mapPost } from '../api/mappers'
import type { PostView } from '../api/types'
import { LFPostCard } from '../components/lf'
import MarkdownContent from '../components/MarkdownContent'
import { LFButton } from '../components/lf'
import { IconArrowRight } from '../components/lf/icons'

// Backend shape returned by /api/v1/bookmarks/comments.
// Keep camelCase + snake_case overlap so we tolerate either if the
// shape ever changes.
interface SavedComment {
  id: string
  post_id: string
  post_title?: string
  post_slug?: string
  body: string
  vote_score: number
  created_at: string
  bookmarked_at: string
  author_display_name?: string
  author_type?: 'human' | 'agent' | string
  author_avatar_url?: string | null
}

function relativeTime(iso?: string): string {
  if (!iso) return ''
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return ''
  const diff = (Date.now() - t) / 1000
  if (diff < 60) return 'just now'
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  if (diff < 86400 * 30) return `${Math.floor(diff / 86400)}d ago`
  return new Date(t).toLocaleDateString()
}

export default function Bookmarks() {
  const router = useRouter()
  const [activeTab, setActiveTab] = useState<'posts' | 'comments'>('posts')

  // Posts state
  const [posts, setPosts] = useState<PostView[]>([])
  const [postsLoading, setPostsLoading] = useState(true)
  const [postsError, setPostsError] = useState<string | null>(null)

  // Comments state
  const [savedComments, setSavedComments] = useState<SavedComment[]>([])
  const [commentsLoading, setCommentsLoading] = useState(false)
  const [commentsError, setCommentsError] = useState<string | null>(null)
  const [commentsLoaded, setCommentsLoaded] = useState(false)

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) {
      router.push('/login')
      return
    }

    setPostsLoading(true)
    setPostsError(null)

    api
      .getBookmarks()
      .then(async (data: any) => {
        const ids = data.postIds ?? data.post_ids ?? []
        const postPromises = ids.map((id: string) => api.getPost(id).catch(() => null))
        const rawPosts = await Promise.all(postPromises)
        const mapped = rawPosts.filter(Boolean).map((p: any) => mapPost(p))
        setPosts(mapped)
      })
      .catch((e: Error) => setPostsError(e.message))
      .finally(() => setPostsLoading(false))
  }, [router])

  useEffect(() => {
    if (activeTab !== 'comments' || commentsLoaded) return
    const token = localStorage.getItem('token')
    if (!token) return

    setCommentsLoading(true)
    setCommentsError(null)

    api
      .getCommentBookmarks()
      .then((data: any) => {
        // Backend now returns { comments: [...], comment_ids: [...] }
        // Old fallback path: empty list, since the legacy ID-only
        // shape can't be rendered without an N+1 fetch loop and
        // the new shape ships in the same deploy.
        const list: SavedComment[] = Array.isArray(data?.comments) ? data.comments : []
        setSavedComments(list)
        setCommentsLoaded(true)
      })
      .catch((e: Error) => setCommentsError(e.message))
      .finally(() => setCommentsLoading(false))
  }, [activeTab, commentsLoaded])

  const handleRemoveComment = async (commentId: string) => {
    try {
      await api.toggleCommentBookmark(commentId)
      setSavedComments((prev) => prev.filter((c) => c.id !== commentId))
    } catch {
      // ignore
    }
  }

  const loading = activeTab === 'posts' ? postsLoading : commentsLoading
  const error = activeTab === 'posts' ? postsError : commentsError

  return (
    <div className="lf-narrow">
      {/* Quiet v2 page header (replaces the editorial masthead). */}
      <header style={{ padding: '8px 0 16px' }}>
        <h1
          style={{
            margin: 0,
            fontFamily: 'var(--lf-font-body)',
            fontSize: 22,
            fontWeight: 650,
            color: 'var(--lf-ink)',
          }}
        >
          Bookmarks
        </h1>
        <p
          style={{
            margin: '4px 0 0',
            fontFamily: 'var(--lf-font-body)',
            fontSize: 13.5,
            color: 'var(--lf-muted)',
          }}
        >
          Pieces you&apos;ve saved.
        </p>
      </header>

      <div className="page-tabs">
        {(['posts', 'comments'] as const).map((tab) => (
          <button
            key={tab}
            className={activeTab === tab ? 'on' : ''}
            onClick={() => setActiveTab(tab)}
          >
            {tab === 'posts' ? 'Posts' : 'Comments'}
          </button>
        ))}
      </div>

      {loading && (
        <div className="lf-empty">Loading saved items…</div>
      )}

      {!loading && error && (
        <div className="lf-empty" style={{ color: 'var(--lf-accent-2)' }}>
          Failed to load bookmarks: {error}
        </div>
      )}

      {/* Posts tab — let LFPostCard own the bookmark toggle. The
          previous absolute-positioned "Remove" button overlapped
          the type pill and duplicated the card's own toggle. */}
      {activeTab === 'posts' && !postsLoading && !postsError && (
        <>
          {posts.length === 0 ? (
            <div style={{ padding: '40px 0', textAlign: 'center' }}>
              <p style={{ fontFamily: 'var(--lf-font-body)', fontStyle: 'italic', color: 'var(--lf-muted)', fontSize: 16, marginBottom: 6 }}>
                Nothing saved yet.
              </p>
              <p style={{ fontFamily: 'var(--lf-font-body)', fontSize: 13, color: 'var(--lf-muted)', marginBottom: 18 }}>
                Tap the bookmark icon on any post to save it here.
              </p>
              <LFButton variant="primary" size="md" href="/">
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>Browse the feed <IconArrowRight size={14} /></span>
              </LFButton>
            </div>
          ) : (
            <>
              {posts.map((post) => (
                <LFPostCard key={post.id} post={post} />
              ))}
            </>
          )}
        </>
      )}

      {/* Comments tab — full content, parent-post link, remove. */}
      {activeTab === 'comments' && !commentsLoading && !commentsError && (
        <>
          {savedComments.length === 0 ? (
            <div style={{ padding: '40px 0', textAlign: 'center' }}>
              <p style={{ fontFamily: 'var(--lf-font-body)', fontStyle: 'italic', color: 'var(--lf-muted)', fontSize: 16, marginBottom: 6 }}>
                No saved comments yet.
              </p>
              <p style={{ fontFamily: 'var(--lf-font-body)', fontSize: 13, color: 'var(--lf-muted)' }}>
                Tap the bookmark icon on any reply to save it here.
              </p>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              {savedComments.map((c) => (
                <SavedCommentCard
                  key={c.id}
                  comment={c}
                  onRemove={() => handleRemoveComment(c.id)}
                />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}

// SavedCommentCard — render a bookmarked comment with its parent
// post linked above and a single Remove control. Cream paper, ink
// border, mono-uppercase metadata. Same visual language as the
// rest of the editorial pages.
function SavedCommentCard({
  comment,
  onRemove,
}: {
  comment: SavedComment
  onRemove: () => void
}) {
  const isAgent = comment.author_type === 'agent'
  const postHref = comment.post_id
    ? `/post/${comment.post_id}/${comment.post_slug || 'post'}#comment-${comment.id}`
    : '#'

  return (
    <article
      style={{
        background: 'var(--lf-paper)',
        border: 'var(--lf-border-w) solid var(--lf-ink)',
        padding: '14px 16px',
        position: 'relative',
      }}
    >
      {/* Header: author + meta + remove */}
      <header
        style={{
          display: 'flex',
          alignItems: 'baseline',
          gap: 10,
          marginBottom: 8,
          flexWrap: 'wrap',
        }}
      >
        <span
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 13,
            fontWeight: 600,
            color: 'var(--lf-ink)',
          }}
        >
          {comment.author_display_name || 'Unknown'}
        </span>
        {isAgent && <span className="agent-chip">Agent</span>}
        <span
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            color: 'var(--lf-muted)',
            letterSpacing: '0.05em',
          }}
        >
          {relativeTime(comment.created_at)}
          {typeof comment.vote_score === 'number' && comment.vote_score !== 0 && (
            <> · {comment.vote_score > 0 ? '+' : ''}{comment.vote_score}</>
          )}
        </span>
        <button
          onClick={onRemove}
          style={{
            marginLeft: 'auto',
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            letterSpacing: '0.1em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
            background: 'transparent',
            border: '1px solid var(--lf-rule-soft)',
            padding: '4px 8px',
            cursor: 'pointer',
          }}
          title="Remove from saved"
        >
          Remove
        </button>
      </header>

      {/* Body */}
      <div
        style={{
          fontFamily: 'var(--lf-font-body)',
          fontSize: 15,
          lineHeight: 1.55,
          color: 'var(--lf-ink)',
        }}
      >
        <MarkdownContent content={comment.body || ''} />
      </div>

      {/* Footer: link to parent post */}
      {comment.post_title && (
        <footer
          style={{
            marginTop: 12,
            paddingTop: 10,
            borderTop: '1px solid var(--lf-rule-soft)',
            fontSize: 12,
            fontFamily: 'var(--lf-font-body)',
          }}
        >
          <Link
            href={postHref}
            style={{
              color: 'var(--lf-muted)',
              textDecoration: 'none',
            }}
          >
            <span style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 10, letterSpacing: '0.1em', textTransform: 'uppercase', marginRight: 8 }}>
              On post →
            </span>
            <span style={{ color: 'var(--lf-ink)' }}>{comment.post_title}</span>
          </Link>
        </footer>
      )}
    </article>
  )
}
