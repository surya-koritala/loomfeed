'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'

interface Draft {
  id: string
  postType?: string
  post_type?: string
  title?: string
  body?: string
  url?: string
  tags?: string[]
  updatedAt?: string
  updated_at?: string
  communityId?: string
  community_id?: string
}

export default function Drafts() {
  const router = useRouter()
  const [drafts, setDrafts] = useState<Draft[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<string | null>(null)

  const load = () => {
    api.listDrafts()
      .then((d: any) => {
        const arr = Array.isArray(d?.drafts) ? d.drafts : Array.isArray(d) ? d : []
        setDrafts(arr)
      })
      .catch((e: Error) => setError(e.message))
  }

  useEffect(() => {
    if (typeof window === 'undefined') return
    if (!localStorage.getItem('token')) {
      router.push('/login')
      return
    }
    load()
  }, [router])

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this draft?')) return
    setDeleting(id)
    try {
      await api.deleteDraft(id)
      setDrafts((prev) => prev?.filter((d) => d.id !== id) ?? null)
    } catch (e: any) {
      setError(e?.message ?? 'Failed to delete')
    } finally {
      setDeleting(null)
    }
  }

  return (
    <div style={{ maxWidth: 740, margin: '0 auto', padding: '24px 16px 96px' }}>
      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: 18 }}>
        <h1 className="lf-page-h1">
          Drafts
        </h1>
        <Link
          href="/submit"
          style={{
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center', gap: 6,
            padding: '0 18px', minHeight: 44, borderRadius: 999,
            background: 'var(--lf-ink)', color: 'var(--lf-paper)',
            font: '600 13px var(--lf-font-body)', textDecoration: 'none',
          }}
        >
          + New post
        </Link>
      </div>

      {error && (
        <div style={{ padding: '10px 14px', background: 'rgba(255,84,54,0.08)', border: '1px solid rgba(255,84,54,0.25)', borderRadius: 10, color: 'var(--lf-accent-2)', font: '500 13px var(--lf-font-body)', marginBottom: 14 }}>
          {error}
        </div>
      )}

      {drafts === null && (
        <div className="lf-empty">Loading…</div>
      )}

      {drafts !== null && drafts.length === 0 && (
        <div style={{ padding: '60px 24px', textAlign: 'center', background: 'var(--lf-paper)', border: '1px solid var(--lf-rule-mid)', borderRadius: 'var(--lf-radius)' }}>
          <div style={{ font: '600 16px var(--lf-font-body)', color: 'var(--lf-ink)', marginBottom: 6 }}>
            No drafts yet
          </div>
          <div style={{ font: '400 13.5px var(--lf-font-body)', color: 'var(--lf-muted)' }}>
            Hit <strong>Save Draft</strong> on the post composer to keep work in progress.
          </div>
        </div>
      )}

      {drafts !== null && drafts.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {drafts.map((d) => {
            const updated = d.updatedAt ?? d.updated_at ?? ''
            const type = d.postType ?? d.post_type ?? 'text'
            const title = d.title?.trim() || '(untitled draft)'
            const preview = (d.body || d.url || '').slice(0, 160)
            return (
              <div
                key={d.id}
                style={{
                  padding: '14px 16px',
                  background: 'var(--lf-paper-alt)',
                  border: '1px solid var(--lf-rule-mid)',
                  borderRadius: 'var(--lf-radius)',
                  display: 'flex', flexDirection: 'column', gap: 6,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
                  <span style={{ font: '700 10.5px var(--lf-font-mono)', color: 'var(--lf-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', padding: '2px 8px', background: 'var(--lf-gray-100)', borderRadius: 999 }}>
                    {type}
                  </span>
                  <span style={{ font: '500 11px var(--lf-font-mono)', color: 'var(--lf-muted-soft)' }}>
                    edited {relTime(updated)}
                  </span>
                </div>
                <Link
                  href={`/submit?draft=${d.id}`}
                  style={{ font: '700 16px var(--lf-font-body)', color: 'var(--lf-ink)', textDecoration: 'none', letterSpacing: '-0.01em' }}
                >
                  {title}
                </Link>
                {preview && (
                  <div style={{ font: '400 13px/1.45 var(--lf-font-body)', color: 'var(--lf-muted)', display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>
                    {preview}
                  </div>
                )}
                <div style={{ display: 'flex', gap: 8, marginTop: 6 }}>
                  <Link
                    href={`/submit?draft=${d.id}`}
                    style={{
                      display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
                      padding: '0 16px', minHeight: 44, borderRadius: 999,
                      border: '1px solid var(--lf-rule-mid)',
                      background: 'var(--lf-paper)', color: 'var(--lf-ink)',
                      font: '600 12px var(--lf-font-body)', textDecoration: 'none',
                    }}
                  >
                    Edit
                  </Link>
                  <button
                    type="button"
                    onClick={() => handleDelete(d.id)}
                    disabled={deleting === d.id}
                    style={{
                      padding: '0 16px', minHeight: 44, borderRadius: 999,
                      border: '1px solid var(--lf-rule-mid)',
                      background: 'var(--lf-paper)', color: 'var(--lf-accent-2)',
                      font: '600 12px var(--lf-font-body)', cursor: 'pointer',
                    }}
                  >
                    {deleting === d.id ? 'Deleting…' : 'Delete'}
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

function relTime(iso: string): string {
  if (!iso) return ''
  const ms = Date.now() - new Date(iso).getTime()
  if (ms < 60_000) return 'just now'
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m ago`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h ago`
  if (ms < 7 * 86_400_000) return `${Math.floor(ms / 86_400_000)}d ago`
  return new Date(iso).toLocaleDateString()
}
