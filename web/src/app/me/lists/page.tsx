'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../../../api/client'
import { LFButton } from '../../../components/lf'

interface ListRow {
  id: string
  title: string
  description: string
  isPublic: boolean
  itemCount: number
  createdAt: string
  updatedAt: string
}

export default function MyReadingListsPage() {
  const router = useRouter()
  const [lists, setLists] = useState<ListRow[]>([])
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [newTitle, setNewTitle] = useState('')
  const [newDesc, setNewDesc] = useState('')
  const [newPublic, setNewPublic] = useState(true)
  const [showForm, setShowForm] = useState(false)

  useEffect(() => {
    if (typeof window === 'undefined') return
    if (!localStorage.getItem('token')) {
      router.replace('/login?next=/me/lists')
      return
    }
    api
      .getMyReadingLists()
      .then((d: any) => setLists(d?.lists ?? []))
      .catch(() => setLists([]))
      .finally(() => setLoading(false))
  }, [router])

  const createList = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newTitle.trim() || creating) return
    setCreating(true)
    try {
      const created = (await api.createReadingList({
        title: newTitle.trim(),
        description: newDesc.trim(),
        is_public: newPublic,
      })) as any
      router.push(`/lists/${created.id}`)
    } catch {
      // keep the form open — server validation will usually already
      // have turned this into an error in the console
    } finally {
      setCreating(false)
    }
  }

  return (
    <div style={{ maxWidth: 760, margin: '0 auto' }}>
      <div className="head">
        <div>
          <div className="edition">Curation · Your reading lists</div>
          <h1>
            My <em>lists.</em>
          </h1>
          <div className="sub">
            Bundles of posts you&apos;ve collected. Public lists are shareable
            and show up on your profile.
          </div>
        </div>
        {!showForm && (
          <LFButton variant="primary" size="sm" onClick={() => setShowForm(true)}>
            + New list
          </LFButton>
        )}
      </div>

      {showForm && (
        <form
          onSubmit={createList}
          style={{
            border: '1px solid var(--lf-ink)',
            background: 'var(--lf-paper-alt)',
            padding: '16px 18px 14px',
            marginBottom: 22,
          }}
        >
          <label
            style={{
              display: 'block',
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.14em',
              textTransform: 'uppercase',
              color: 'var(--lf-muted)',
              marginBottom: 6,
            }}
          >
            Title
          </label>
          <input
            value={newTitle}
            onChange={(e) => setNewTitle(e.target.value)}
            placeholder="e.g. The best AI synthesis this month"
            maxLength={120}
            style={{
              width: '100%',
              fontFamily: 'var(--lf-font-body)',
              fontSize: 17,
              padding: '8px 10px',
              border: '1px solid var(--lf-ink)',
              background: 'var(--lf-paper)',
              color: 'var(--lf-ink)',
              outline: 'none',
              marginBottom: 12,
              boxSizing: 'border-box',
            }}
          />
          <label
            style={{
              display: 'block',
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.14em',
              textTransform: 'uppercase',
              color: 'var(--lf-muted)',
              marginBottom: 6,
            }}
          >
            Description (optional)
          </label>
          <textarea
            value={newDesc}
            onChange={(e) => setNewDesc(e.target.value)}
            rows={3}
            style={{
              width: '100%',
              fontFamily: 'var(--lf-font-body)',
              fontSize: 14,
              padding: 10,
              border: '1px solid var(--lf-ink)',
              background: 'var(--lf-paper)',
              color: 'var(--lf-ink)',
              outline: 'none',
              resize: 'vertical',
              marginBottom: 12,
              boxSizing: 'border-box',
            }}
          />
          <label
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 11,
              letterSpacing: '0.08em',
              textTransform: 'uppercase',
              color: 'var(--lf-ink)',
              marginBottom: 14,
            }}
          >
            <input
              type="checkbox"
              checked={newPublic}
              onChange={(e) => setNewPublic(e.target.checked)}
            />
            Public (anyone can open the link)
          </label>
          <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
            <button
              type="button"
              onClick={() => setShowForm(false)}
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 10,
                letterSpacing: '0.12em',
                textTransform: 'uppercase',
                background: 'transparent',
                color: 'var(--lf-muted)',
                border: 'none',
                padding: '6px 10px',
                cursor: 'pointer',
              }}
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={creating || !newTitle.trim()}
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 10,
                letterSpacing: '0.12em',
                textTransform: 'uppercase',
                background: creating ? 'var(--lf-ink)' : 'var(--lf-ink)',
                color: 'var(--lf-paper)',
                border: '1px solid var(--lf-ink)',
                padding: '7px 14px',
                cursor: creating ? 'wait' : 'pointer',
              }}
            >
              {creating ? 'Creating…' : 'Create list'}
            </button>
          </div>
        </form>
      )}

      {loading ? (
        <div
          style={{
            padding: '40px 0',
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            letterSpacing: '0.14em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
          }}
        >
          Loading…
        </div>
      ) : lists.length === 0 && !showForm ? (
        <p
          style={{
            padding: '28px 0',
            fontFamily: 'var(--lf-font-body)',
            fontStyle: 'italic',
            color: 'var(--lf-muted)',
            fontSize: 16,
          }}
        >
          You haven&apos;t curated a list yet. Click{' '}
          <button
            onClick={() => setShowForm(true)}
            style={{
              background: 'none',
              border: 'none',
              color: 'var(--lf-seal)',
              font: 'inherit',
              fontStyle: 'italic',
              padding: 0,
              cursor: 'pointer',
              textDecoration: 'underline',
            }}
          >
            New list
          </button>
          {' '}to start one.
        </p>
      ) : (
        <div>
          {lists.map((l) => (
            <Link
              key={l.id}
              href={`/lists/${l.id}`}
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr auto',
                gap: 12,
                padding: '16px 0',
                borderBottom: '1px solid var(--lf-ink)',
                textDecoration: 'none',
                color: 'var(--lf-ink)',
                alignItems: 'baseline',
              }}
            >
              <div style={{ minWidth: 0 }}>
                <div
                  style={{
                    fontFamily: 'var(--lf-font-body)',
                    fontSize: 20,
                    fontWeight: 500,
                    letterSpacing: '-0.015em',
                    lineHeight: 1.25,
                    marginBottom: 4,
                  }}
                >
                  {l.title}
                </div>
                <div
                  style={{
                    fontFamily: 'var(--lf-font-mono)',
                    fontSize: 10,
                    letterSpacing: '0.1em',
                    textTransform: 'uppercase',
                    color: 'var(--lf-muted)',
                  }}
                >
                  {l.itemCount} {l.itemCount === 1 ? 'post' : 'posts'} ·{' '}
                  {l.isPublic ? 'Public' : 'Private'}
                </div>
              </div>
              <span
                style={{
                  fontFamily: 'var(--lf-font-mono)',
                  fontSize: 10,
                  letterSpacing: '0.12em',
                  textTransform: 'uppercase',
                  color: 'var(--lf-seal)',
                }}
              >
                Open →
              </span>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
