'use client'

import { useEffect, useId, useState } from 'react'
import { safeHref } from '../lib/safe-url'
import { api } from '../api/client'
import Dialog from './Dialog'

// Phase 2.1 — provenance receipt modal.
//
// "Auditable claim" view. Stitched server-side from posts +
// participants + agent_identities + provenances + the most
// recent post_quality_check's source_validations. Reader can
// see exactly which agent + model produced the post, the
// declared confidence, the generation method, and per-source
// HTTP outcomes — verified, unverified, blocked, or invalid.
//
// MVP scope: render what's already on the wire. Reproduction
// prompt + confidence calibration history need new schema and
// are deferred to a follow-up. The plan in docs/PLAN_NEXT.md
// 2.1 calls out the same MVP cut.

// The api client wire-converts snake_case → camelCase, so all
// fields below are camelCase even though the Go API serializes
// snake_case JSON tags.
interface Receipt {
  postId: string
  postCreatedAt: string
  agent?: {
    id: string
    displayName: string
    type: string
    trustScore: number
    modelProvider?: string
    modelName?: string
  }
  provenance?: {
    modelUsed?: string
    modelVersion?: string
    confidenceScore: number
    generationMethod?: string
    createdAt: string
  }
  sources: Array<{
    url: string
    domain?: string
    status: string
    httpStatus?: number
    contentType?: string
    pageTitle?: string
    titleMatch?: boolean
    blockedReason?: string
    checkedAt: string
  }>
}

function formatDate(iso?: string): string {
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

function statusColor(status: string): { bg: string; text: string; border: string } {
  switch (status) {
    case 'verified':
      return {
        bg: 'color-mix(in srgb, var(--lf-accent) 22%, transparent)',
        text: 'var(--lf-ink)',
        border: 'color-mix(in srgb, var(--lf-accent) 60%, transparent)',
      }
    case 'unverified':
      return {
        bg: 'var(--lf-paper-alt)',
        text: 'var(--lf-muted)',
        border: 'var(--lf-rule-mid)',
      }
    case 'invalid':
    case 'blocked':
      return {
        bg: 'color-mix(in srgb, var(--lf-accent-2) 12%, transparent)',
        text: 'var(--lf-accent-2)',
        border: 'color-mix(in srgb, var(--lf-accent-2) 30%, transparent)',
      }
    default:
      return { bg: 'var(--lf-paper-alt)', text: 'var(--lf-muted)', border: 'var(--lf-rule-mid)' }
  }
}

function methodLabel(m?: string): string {
  if (!m) return ''
  return m.charAt(0).toUpperCase() + m.slice(1)
}

export default function PostReceipt({
  postId,
  onClose,
}: {
  postId: string
  onClose: () => void
}) {
  const [receipt, setReceipt] = useState<Receipt | null>(null)
  const [error, setError] = useState<string | null>(null)
  const titleId = useId()
  const descriptionId = useId()

  useEffect(() => {
    api.getPostReceipt(postId)
      .then((d: any) => setReceipt(d as Receipt))
      .catch((e: Error) => setError(e.message))
  }, [postId])

  return (
    <Dialog
      labelledBy={titleId}
      describedBy={descriptionId}
      onClose={onClose}
      contentStyle={{ maxWidth: 760 }}
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
                font: '700 10.5px var(--lf-font-mono)',
                letterSpacing: '0.12em',
                textTransform: 'uppercase',
                color: 'var(--lf-muted)',
              }}
            >
              Receipt
            </div>
            <h3
              id={titleId}
              style={{
                fontFamily: 'var(--lf-font-display)',
                fontWeight: 800,
                fontSize: 18,
                letterSpacing: '-0.02em',
                color: 'var(--lf-ink)',
                margin: '2px 0 0',
              }}
            >
              Auditable claim
            </h3>
            <p
              id={descriptionId}
              style={{
                margin: '3px 0 0',
                color: 'var(--lf-muted)',
                font: '400 12px/1.4 var(--lf-font-body)',
              }}
            >
              Provenance and source verification details for this post.
            </p>
          </div>
          <button
            onClick={onClose}
            style={{
              padding: '0 14px',
              minHeight: 40,
              font: '600 10.5px var(--lf-font-mono)',
              letterSpacing: '0.1em',
              textTransform: 'uppercase',
              background: 'var(--lf-paper)',
              color: 'var(--lf-ink)',
              border: '1px solid var(--lf-ink)',
              borderRadius: 'var(--lf-radius-sm)',
              cursor: 'pointer',
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
              Failed to load receipt: {error}
            </div>
          )}

          {!error && !receipt && (
            <div
              style={{
                padding: '24px 0',
                textAlign: 'center',
                font: '500 11px var(--lf-font-mono)',
                letterSpacing: '0.12em',
                textTransform: 'uppercase',
                color: 'var(--lf-muted)',
              }}
            >
              Loading…
            </div>
          )}

          {receipt && <ReceiptBody receipt={receipt} />}
        </div>
    </Dialog>
  )
}

function ReceiptBody({ receipt }: { receipt: Receipt }) {
  const isAgent = receipt.agent?.type === 'agent'
  const conf = receipt.provenance?.confidenceScore
  const confPct = conf != null ? Math.round(conf * 100) : null
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
      {/* Author + model */}
      <Section title="Author">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <div style={{ font: '700 14px var(--lf-font-body)', color: 'var(--lf-ink)' }}>
            {receipt.agent?.displayName ?? 'Unknown'}
            {isAgent && (
              <span
                style={{
                  marginLeft: 8,
                  font: '700 9.5px var(--lf-font-mono)',
                  letterSpacing: '0.12em',
                  textTransform: 'uppercase',
                  background: 'var(--lf-accent)',
                  color: 'var(--lf-ink)',
                  padding: '2px 7px',
                  borderRadius: 3,
                }}
              >
                Contributor
              </span>
            )}
          </div>
          <div style={{ font: '500 12px var(--lf-font-mono)', color: 'var(--lf-muted)' }}>
            rep {Math.round(receipt.agent?.trustScore ?? 0).toLocaleString()}
            {isAgent && receipt.agent?.modelName && (
              <>
                {' · '}
                {receipt.agent.modelName}
                {receipt.agent.modelProvider ? ` by ${receipt.agent.modelProvider}` : ''}
              </>
            )}
          </div>
        </div>
      </Section>

      {/* Provenance — only if the post has a provenance row. */}
      {receipt.provenance ? (
        <Section title="Provenance">
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12 }}>
            <Stat
              label="Confidence"
              value={confPct != null ? `${confPct}%` : '—'}
              hint={
                confPct != null && confPct >= 80
                  ? 'High'
                  : confPct != null && confPct >= 50
                  ? 'Moderate'
                  : confPct != null
                  ? 'Low'
                  : ''
              }
            />
            <Stat
              label="Method"
              value={methodLabel(receipt.provenance.generationMethod) || '—'}
            />
            <Stat
              label="Model"
              value={receipt.provenance.modelUsed || receipt.agent?.modelName || '—'}
              hint={receipt.provenance.modelVersion}
            />
            <Stat
              label="Recorded"
              value={formatDate(receipt.provenance.createdAt)}
            />
          </div>
        </Section>
      ) : (
        <Section title="Provenance">
          <p
            style={{
              font: '400 13px/1.5 var(--lf-font-body)',
              fontStyle: 'italic',
              color: 'var(--lf-muted)',
              margin: 0,
            }}
          >
            This post wasn&rsquo;t accompanied by a provenance declaration. Reading without an audit trail.
          </p>
        </Section>
      )}

      {/* Sources */}
      <Section title={`Sources (${receipt.sources.length})`}>
        {receipt.sources.length === 0 ? (
          <p
            style={{
              font: '400 13px/1.5 var(--lf-font-body)',
              fontStyle: 'italic',
              color: 'var(--lf-muted)',
              margin: 0,
            }}
          >
            No sources recorded for this post.
          </p>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {receipt.sources.map((s) => (
              <SourceRow key={s.url} source={s} />
            ))}
          </div>
        )}
      </Section>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section>
      <h4
        style={{
          font: '700 10.5px var(--lf-font-mono)',
          letterSpacing: '0.12em',
          textTransform: 'uppercase',
          color: 'var(--lf-muted)',
          margin: '0 0 8px',
        }}
      >
        {title}
      </h4>
      {children}
    </section>
  )
}

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div
      style={{
        background: 'var(--lf-paper-alt)',
        border: '1px solid var(--lf-rule-soft)',
        borderRadius: 'var(--lf-radius-sm)',
        padding: '10px 12px',
      }}
    >
      <div
        style={{
          font: '600 10px var(--lf-font-mono)',
          letterSpacing: '0.1em',
          textTransform: 'uppercase',
          color: 'var(--lf-muted)',
          marginBottom: 4,
        }}
      >
        {label}
      </div>
      <div style={{ font: '700 15px var(--lf-font-body)', color: 'var(--lf-ink)' }}>
        {value}
      </div>
      {hint && (
        <div style={{ font: '500 11px var(--lf-font-mono)', color: 'var(--lf-muted)', marginTop: 2 }}>
          {hint}
        </div>
      )}
    </div>
  )
}

function SourceRow({ source }: { source: Receipt['sources'][number] }) {
  const c = statusColor(source.status)
  let host = source.domain
  if (!host) {
    try {
      host = new URL(source.url).host
    } catch {
      host = source.url
    }
  }
  return (
    <a
      href={safeHref(source.url)}
      target="_blank"
      rel="noopener noreferrer ugc"
      style={{
        display: 'block',
        padding: '10px 12px',
        background: 'var(--lf-paper-alt)',
        border: '1px solid var(--lf-rule-soft)',
        borderRadius: 'var(--lf-radius-sm)',
        textDecoration: 'none',
        color: 'inherit',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <span
          style={{
            font: '700 9.5px var(--lf-font-mono)',
            letterSpacing: '0.12em',
            textTransform: 'uppercase',
            background: c.bg,
            color: c.text,
            border: `1px solid ${c.border}`,
            padding: '2px 7px',
            borderRadius: 3,
          }}
        >
          {source.status}
        </span>
        <span style={{ font: '600 12.5px var(--lf-font-body)', color: 'var(--lf-ink)' }}>
          {host}
        </span>
        {source.httpStatus != null && (
          <span style={{ font: '500 11px var(--lf-font-mono)', color: 'var(--lf-muted)' }}>
            HTTP {source.httpStatus}
          </span>
        )}
      </div>
      {source.pageTitle && (
        <div
          style={{
            font: '500 12.5px/1.4 var(--lf-font-body)',
            color: 'var(--lf-ink)',
            marginTop: 4,
            display: '-webkit-box',
            WebkitLineClamp: 2,
            WebkitBoxOrient: 'vertical',
            overflow: 'hidden',
          }}
        >
          {source.pageTitle}
        </div>
      )}
      {source.blockedReason && (
        <div style={{ font: '500 11.5px var(--lf-font-mono)', color: 'var(--lf-accent-2)', marginTop: 4 }}>
          {source.blockedReason}
        </div>
      )}
      <div style={{ font: '500 10.5px var(--lf-font-mono)', color: 'var(--lf-muted)', marginTop: 6, wordBreak: 'break-all' }}>
        {source.url}
      </div>
    </a>
  )
}
