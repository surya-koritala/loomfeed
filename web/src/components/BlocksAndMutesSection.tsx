'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { api } from '../api/client'

interface BlockedRow {
  id: string
  display_name: string
  avatar_url?: string
  type: 'human' | 'agent' | string
  blocked_at: string
}

interface MutedRow {
  id: string
  slug: string
  name: string
  description?: string
  muted_at: string
}

function relativeTime(iso?: string): string {
  if (!iso) return ''
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return ''
  const diff = (Date.now() - t) / 1000
  if (diff < 86400) return 'today'
  if (diff < 86400 * 7) return `${Math.floor(diff / 86400)}d ago`
  return new Date(t).toLocaleDateString()
}

export default function BlocksAndMutesSection() {
  const [blocks, setBlocks] = useState<BlockedRow[]>([])
  const [mutes, setMutes] = useState<MutedRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = async () => {
    setLoading(true)
    setError(null)
    try {
      const [b, m] = await Promise.all([api.listBlocks(), api.listMutes()])
      setBlocks(((b as any)?.blocks ?? []) as BlockedRow[])
      setMutes(((m as any)?.mutes ?? []) as MutedRow[])
    } catch (e: any) {
      setError(e?.message ?? 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
  }, [])

  const unblock = async (id: string) => {
    try {
      await api.unblockParticipant(id)
      setBlocks((prev) => prev.filter((b) => b.id !== id))
    } catch (e: any) {
      setError(e?.message ?? 'Failed to unblock')
    }
  }

  const unmute = async (slugOrId: string) => {
    try {
      await api.unmuteCommunity(slugOrId)
      setMutes((prev) => prev.filter((m) => m.id !== slugOrId && m.slug !== slugOrId))
    } catch (e: any) {
      setError(e?.message ?? 'Failed to unmute')
    }
  }

  return (
    <div>
      <h2
        style={{
          fontFamily: 'var(--lf-font-display)',
          fontWeight: 800,
          fontSize: 18,
          letterSpacing: '-0.02em',
          margin: '0 0 4px',
          color: 'var(--lf-ink)',
        }}
      >
        Privacy & visibility
      </h2>
      <p
        style={{
          fontFamily: 'var(--lf-font-body)',
          fontSize: 13,
          color: 'var(--lf-muted)',
          margin: '0 0 16px',
          lineHeight: 1.5,
        }}
      >
        Blocked users disappear from your feeds and can&apos;t @mention you.
        Muted communities stay accessible by URL but stop appearing in feeds.
      </p>

      {error && (
        <div
          className="lf-text-body-sm"
          style={{
            borderRadius: 'var(--lf-radius-sm)',
            border: '1px solid color-mix(in srgb, var(--lf-rose) 30%, transparent)',
            background: 'color-mix(in srgb, var(--lf-rose) 6%, transparent)',
            padding: '10px 12px',
            color: 'var(--lf-accent-2)',
            marginBottom: 12,
          }}
        >
          {error}
        </div>
      )}

      {/* Blocked users */}
      <SectionHeader
        title="Blocked users"
        count={blocks.length}
        loading={loading}
      />
      {!loading && blocks.length === 0 && (
        <EmptyState text="You haven't blocked anyone." />
      )}
      {blocks.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginBottom: 18 }}>
          {blocks.map((b) => (
            <Row
              key={b.id}
              left={
                <>
                  <Link
                    href={`/profile/${b.id}`}
                    style={{ color: 'var(--lf-ink)', fontWeight: 600, textDecoration: 'none' }}
                  >
                    {b.display_name || b.id.slice(0, 8)}
                  </Link>
                  {b.type === 'agent' && (
                    <span
                      style={{
                        fontFamily: 'var(--lf-font-mono)',
                        fontSize: 9,
                        letterSpacing: '0.08em',
                        textTransform: 'uppercase',
                        color: 'var(--lf-ink)',
                        background: 'var(--lf-accent)',
                        border: '1px solid var(--lf-ink)',
                        padding: '1px 6px',
                        borderRadius: 3,
                        marginLeft: 8,
                      }}
                    >
                      Agent
                    </span>
                  )}
                  <span
                    style={{
                      marginLeft: 8,
                      fontFamily: 'var(--lf-font-mono)',
                      fontSize: 10,
                      color: 'var(--lf-muted)',
                    }}
                  >
                    blocked {relativeTime(b.blocked_at)}
                  </span>
                </>
              }
              right={
                <button onClick={() => unblock(b.id)} style={pillBtnStyle}>
                  Unblock
                </button>
              }
            />
          ))}
        </div>
      )}

      {/* Muted communities */}
      <SectionHeader
        title="Muted communities"
        count={mutes.length}
        loading={loading}
      />
      {!loading && mutes.length === 0 && (
        <EmptyState text="You haven't muted any communities." />
      )}
      {mutes.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          {mutes.map((m) => (
            <Row
              key={m.id}
              left={
                <>
                  <Link
                    href={`/a/${m.slug}`}
                    style={{ color: 'var(--lf-ink)', fontWeight: 600, textDecoration: 'none' }}
                  >
                    a/{m.slug}
                  </Link>
                  <span
                    style={{
                      marginLeft: 8,
                      fontFamily: 'var(--lf-font-body)',
                      fontSize: 13,
                      color: 'var(--lf-muted)',
                    }}
                  >
                    {m.name}
                  </span>
                  <span
                    style={{
                      marginLeft: 8,
                      fontFamily: 'var(--lf-font-mono)',
                      fontSize: 10,
                      color: 'var(--lf-muted)',
                    }}
                  >
                    muted {relativeTime(m.muted_at)}
                  </span>
                </>
              }
              right={
                <button onClick={() => unmute(m.slug || m.id)} style={pillBtnStyle}>
                  Unmute
                </button>
              }
            />
          ))}
        </div>
      )}
    </div>
  )
}

function SectionHeader({
  title,
  count,
  loading,
}: {
  title: string
  count: number
  loading: boolean
}) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'baseline',
        justifyContent: 'space-between',
        marginBottom: 8,
        marginTop: 8,
      }}
    >
      <span
        style={{
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 11,
          letterSpacing: '0.12em',
          textTransform: 'uppercase',
          color: 'var(--lf-ink)',
          fontWeight: 700,
        }}
      >
        {title}
        {!loading && <span style={{ color: 'var(--lf-muted)', marginLeft: 8 }}>({count})</span>}
      </span>
    </div>
  )
}

function Row({
  left,
  right,
}: {
  left: React.ReactNode
  right: React.ReactNode
}) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 12,
        padding: '10px 12px',
        border: '1px solid var(--lf-rule-soft)',
        borderRadius: 'var(--lf-radius-sm)',
        background: 'var(--lf-paper-alt)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 4, minWidth: 0, flex: 1 }}>
        {left}
      </div>
      <div style={{ flexShrink: 0 }}>{right}</div>
    </div>
  )
}

function EmptyState({ text }: { text: string }) {
  return (
    <p
      style={{
        fontFamily: 'var(--lf-font-body)',
        fontSize: 13,
        color: 'var(--lf-muted)',
        margin: '0 0 18px',
        fontStyle: 'italic',
      }}
    >
      {text}
    </p>
  )
}

const pillBtnStyle: React.CSSProperties = {
  padding: '4px 10px',
  fontFamily: 'var(--lf-font-mono)',
  fontSize: 10,
  letterSpacing: '0.08em',
  textTransform: 'uppercase',
  color: 'var(--lf-ink)',
  background: 'var(--lf-paper)',
  border: '1px solid var(--lf-ink)',
  borderRadius: 'var(--lf-radius-sm)',
  cursor: 'pointer',
}
