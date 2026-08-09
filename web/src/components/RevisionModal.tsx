'use client'

import { useEffect, useState } from 'react'
import { api } from '../api/client'

// Phase 1.4 — edit history modal.
//
// Shown when a reader clicks the "edited" link next to a post or
// comment timestamp. Fetches the revision list, renders each as a
// dated card with a line-level diff against the previous version
// (red strike on removed, lime highlight on added). The CURRENT
// (latest) text is what's still on the post, so we render that
// inline at the top — the modal is "what was different before."

interface Revision {
  id: string
  revision_number: number
  title?: string
  body: string
  created_at: string
}

type Target =
  | { kind: 'post'; id: string; current: { title?: string; body: string } }
  | { kind: 'comment'; id: string; current: { body: string } }

function formatTimestamp(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// Simple LCS-based line diff. Returns a flat list of "added",
// "removed", or "same" tokens. Good enough for the "see what
// changed" UX — not Myers-quality but visually unambiguous on
// typical edits.
type DiffOp = { kind: 'same' | 'added' | 'removed'; text: string }

function diffLines(oldText: string, newText: string): DiffOp[] {
  const a = (oldText ?? '').split('\n')
  const b = (newText ?? '').split('\n')
  // Build LCS DP table.
  const m = a.length
  const n = b.length
  const dp: number[][] = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0))
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      if (a[i] === b[j]) dp[i][j] = dp[i + 1][j + 1] + 1
      else dp[i][j] = Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }
  // Walk to produce ops.
  const ops: DiffOp[] = []
  let i = 0
  let j = 0
  while (i < m && j < n) {
    if (a[i] === b[j]) {
      ops.push({ kind: 'same', text: a[i] })
      i++
      j++
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      ops.push({ kind: 'removed', text: a[i] })
      i++
    } else {
      ops.push({ kind: 'added', text: b[j] })
      j++
    }
  }
  while (i < m) ops.push({ kind: 'removed', text: a[i++] })
  while (j < n) ops.push({ kind: 'added', text: b[j++] })
  return ops
}

export default function RevisionModal({
  target,
  onClose,
}: {
  target: Target
  onClose: () => void
}) {
  const [revisions, setRevisions] = useState<Revision[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const fetcher = target.kind === 'post' ? api.getPostRevisions : api.getCommentRevisions
    fetcher(target.id)
      .then((d: any) => {
        const arr: Revision[] = Array.isArray(d) ? d : (d?.data ?? d?.revisions ?? [])
        setRevisions(arr)
      })
      .catch((e: Error) => setError(e.message))
  }, [target])

  // Build "what changed" pairs. Backend returns newest-first; we
  // walk from oldest forward so each diff shows the change FROM
  // the previous version TO this one, ending with the current
  // (live) text shown as the latest "after" panel.
  const sorted = (revisions ?? []).slice().sort((a, b) => a.revision_number - b.revision_number)
  const live = target.kind === 'post' ? target.current.body : target.current.body

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(10, 10, 10, 0.35)',
        backdropFilter: 'blur(4px)',
        WebkitBackdropFilter: 'blur(4px)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: 'var(--lf-paper)',
          border: '1px solid var(--lf-ink)',
          borderRadius: 'var(--lf-radius)',
          maxWidth: 720,
          width: '100%',
          maxHeight: '90vh',
          overflowY: 'auto',
          boxShadow: '0 12px 36px rgba(10, 10, 10, 0.18)',
        }}
      >
        <header
          style={{
            position: 'sticky',
            top: 0,
            background: 'var(--lf-paper)',
            borderBottom: '1px solid var(--lf-rule-soft)',
            padding: '14px 18px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 12,
            zIndex: 1,
          }}
        >
          <div>
            <div
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 10,
                letterSpacing: '0.12em',
                textTransform: 'uppercase',
                color: 'var(--lf-muted)',
                fontWeight: 700,
              }}
            >
              Revisions
            </div>
            <h3
              style={{
                fontFamily: 'var(--lf-font-display)',
                fontWeight: 800,
                fontSize: 18,
                letterSpacing: '-0.02em',
                color: 'var(--lf-ink)',
                margin: '2px 0 0',
              }}
            >
              {target.kind === 'post' ? 'Edit history' : 'Comment edit history'}
            </h3>
          </div>
          <button
            onClick={onClose}
            style={{
              padding: '6px 12px',
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.1em',
              textTransform: 'uppercase',
              background: 'var(--lf-paper)',
              color: 'var(--lf-ink)',
              border: '1px solid var(--lf-ink)',
              borderRadius: 'var(--lf-radius-sm)',
              cursor: 'pointer',
              fontWeight: 600,
            }}
          >
            Close
          </button>
        </header>

        <div style={{ padding: '16px 18px' }}>
          {error && (
            <div
              style={{
                padding: '10px 12px',
                background: 'color-mix(in srgb, var(--lf-rose) 6%, transparent)',
                border: '1px solid color-mix(in srgb, var(--lf-rose) 30%, transparent)',
                borderRadius: 'var(--lf-radius-sm)',
                color: 'var(--lf-accent-2)',
                fontSize: 13,
                marginBottom: 12,
              }}
            >
              Failed to load revisions: {error}
            </div>
          )}

          {!error && revisions === null && (
            <div
              style={{
                padding: '24px 0',
                textAlign: 'center',
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

          {!error && revisions !== null && revisions.length === 0 && (
            <p
              style={{
                fontFamily: 'var(--lf-font-body)',
                fontStyle: 'italic',
                color: 'var(--lf-muted)',
                fontSize: 14,
                margin: 0,
              }}
            >
              No earlier versions stored. The post may have been edited before
              revision tracking was added, or the change was small enough that
              no revision was recorded.
            </p>
          )}

          {!error && revisions !== null && revisions.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              {sorted.map((rev, i) => {
                const prev = i === 0 ? null : sorted[i - 1]
                const ops = prev ? diffLines(prev.body, rev.body) : null
                return (
                  <RevisionCard
                    key={rev.id}
                    rev={rev}
                    ops={ops}
                    label={i === 0 ? 'Original' : `Revision ${rev.revision_number}`}
                  />
                )
              })}
              {/* Final card — current (live) version. We diff
                  against the most recent stored revision. */}
              <RevisionCard
                rev={{ id: 'current', revision_number: 0, body: live, created_at: '' }}
                ops={diffLines(sorted[sorted.length - 1]!.body, live)}
                label="Current"
              />
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function RevisionCard({
  rev,
  ops,
  label,
}: {
  rev: Revision
  ops: DiffOp[] | null
  label: string
}) {
  return (
    <article
      style={{
        border: '1px solid var(--lf-rule-soft)',
        background: 'var(--lf-paper-alt)',
        padding: '12px 14px',
      }}
    >
      <header
        style={{
          display: 'flex',
          alignItems: 'baseline',
          justifyContent: 'space-between',
          gap: 8,
          marginBottom: 8,
        }}
      >
        <span
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            letterSpacing: '0.12em',
            textTransform: 'uppercase',
            color: 'var(--lf-ink)',
            background: label === 'Current' ? 'var(--lf-accent)' : 'var(--lf-paper)',
            border: '1px solid var(--lf-ink)',
            padding: '2px 8px',
            borderRadius: 3,
            fontWeight: 700,
          }}
        >
          {label}
        </span>
        {rev.created_at && (
          <span style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 10, color: 'var(--lf-muted)' }}>
            {formatTimestamp(rev.created_at)}
          </span>
        )}
      </header>

      {/* If we have a diff, render it. Otherwise (Original), show
          the raw body. */}
      {ops ? (
        <div
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 13,
            lineHeight: 1.55,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          {ops.map((op, idx) => (
            <DiffLine key={idx} op={op} />
          ))}
        </div>
      ) : (
        <div
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 14,
            lineHeight: 1.5,
            color: 'var(--lf-ink)',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          {rev.body || <span style={{ color: 'var(--lf-muted)', fontStyle: 'italic' }}>(empty)</span>}
        </div>
      )}
    </article>
  )
}

function DiffLine({ op }: { op: DiffOp }) {
  if (op.kind === 'same') {
    return (
      <div style={{ color: 'var(--lf-muted)' }}>
        <span style={{ marginRight: 8, opacity: 0.5 }}> </span>
        {op.text || ' '}
      </div>
    )
  }
  if (op.kind === 'added') {
    return (
      <div
        style={{
          background: 'color-mix(in srgb, var(--lf-accent) 22%, transparent)',
          color: 'var(--lf-ink)',
          padding: '0 4px',
        }}
      >
        <span style={{ marginRight: 8, color: 'var(--lf-ink)', fontWeight: 700 }}>+</span>
        {op.text || ' '}
      </div>
    )
  }
  return (
    <div
      style={{
        background: 'color-mix(in srgb, var(--lf-accent-2) 12%, transparent)',
        color: 'var(--lf-muted)',
        padding: '0 4px',
        textDecoration: 'line-through',
      }}
    >
      <span style={{ marginRight: 8, fontWeight: 700, textDecoration: 'none' }}>−</span>
      {op.text || ' '}
    </div>
  )
}
