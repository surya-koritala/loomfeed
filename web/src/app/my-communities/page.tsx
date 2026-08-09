'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { api } from '../../api/client'

/**
 * "My communities" page.
 *
 * Two sections:
 *   - Joined    : communities the user subscribes to (from /communities/subscriptions).
 *                 Previously hidden — users expected to see these here and didn't.
 *   - Run       : communities the user created or moderates (from /communities/mine).
 *
 * Editorial style: hairline rules, mono-caps kickers, ink + paper tokens,
 * no rounded corners, no legacy palette tokens.
 */

interface Community {
  id: string
  name: string
  slug: string
  description?: string
  subscriber_count?: number
  subscriberCount?: number
}

function SectionHeader({ label, count }: { label: string; count: number }) {
  return (
    <div
      style={{
        fontFamily: 'var(--lf-font-mono)',
        fontSize: 10,
        letterSpacing: '0.14em',
        textTransform: 'uppercase',
        color: 'var(--lf-muted)',
        padding: '10px 0',
        borderBottom: '1px solid var(--lf-ink)',
        marginBottom: 0,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'baseline',
      }}
    >
      <span>{label}</span>
      <span style={{ color: 'var(--lf-muted)', fontSize: 10 }}>{count}</span>
    </div>
  )
}

function CommunityRow({ c, role }: { c: Community; role: 'joined' | 'run' }) {
  const count = c.subscriberCount ?? c.subscriber_count ?? 0
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: '36px 1fr auto',
        gap: 14,
        padding: '14px 0',
        borderBottom: '1px solid var(--lf-ink)',
        alignItems: 'center',
      }}
    >
      <div
        style={{
          width: 36,
          height: 36,
          border: '1px solid var(--lf-ink)',
          background: 'var(--lf-paper-alt)',
          color: 'var(--lf-ink)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 11,
          fontWeight: 500,
        }}
      >
        {c.slug.slice(0, 2).toUpperCase()}
      </div>
      <div style={{ minWidth: 0 }}>
        <Link
          href={`/a/${c.slug}`}
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 16,
            fontWeight: 500,
            color: 'var(--lf-ink)',
            letterSpacing: '-0.005em',
            textDecoration: 'none',
            display: 'block',
            lineHeight: 1.2,
          }}
        >
          a/{c.slug}
        </Link>
        <div
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
            marginTop: 4,
          }}
        >
          {c.name} · {count} {count === 1 ? 'member' : 'members'}
        </div>
      </div>
      <div style={{ display: 'flex', gap: 6 }}>
        {role === 'run' && (
          <Link
            href={`/a/${c.slug}/moderation`}
            style={{
              padding: '6px 12px',
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.12em',
              textTransform: 'uppercase',
              border: '1px solid var(--lf-ink)',
              color: 'var(--lf-ink)',
              textDecoration: 'none',
              background: 'transparent',
            }}
          >
            Manage
          </Link>
        )}
        <Link
          href={`/submit?community=${c.slug}`}
          style={{
            padding: '6px 12px',
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            letterSpacing: '0.12em',
            textTransform: 'uppercase',
            background: 'var(--lf-ink)',
            color: 'var(--lf-paper)',
            textDecoration: 'none',
          }}
        >
          Write
        </Link>
      </div>
    </div>
  )
}

function EmptyBlock({ msg, cta, href }: { msg: string; cta: string; href: string }) {
  return (
    <div
      style={{
        padding: '28px 0',
        fontFamily: 'var(--lf-font-body)',
        fontStyle: 'italic',
        color: 'var(--lf-muted)',
        fontSize: 15,
        borderBottom: '1px solid var(--lf-ink)',
      }}
    >
      <p style={{ margin: '0 0 12px' }}>{msg}</p>
      <Link
        href={href}
        style={{
          display: 'inline-block',
          padding: '7px 14px',
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 10,
          letterSpacing: '0.12em',
          textTransform: 'uppercase',
          border: '1px solid var(--lf-ink)',
          color: 'var(--lf-ink)',
          textDecoration: 'none',
          fontStyle: 'normal',
        }}
      >
        {cta}
      </Link>
    </div>
  )
}

export default function MyCommunitiesPage() {
  const [joined, setJoined] = useState<Community[]>([])
  const [run, setRun] = useState<Community[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!localStorage.getItem('token')) {
      window.location.href = '/login'
      return
    }
    Promise.all([
      api.getSubscribedCommunities().catch(() => []),
      api.getMyCommunities().catch(() => []),
    ])
      .then(([subs, mine]: any) => {
        setJoined(Array.isArray(subs) ? subs : [])
        setRun(Array.isArray(mine) ? mine : [])
      })
      .finally(() => setLoading(false))
  }, [])

  return (
    <div style={{ maxWidth: 820, margin: '0 auto' }}>
      <div className="head">
        <div>
          <div className="edition">Library · Your communities</div>
          <h1>
            My <em>communities.</em>
          </h1>
          <div className="sub">
            The ones you joined, the ones you run.
          </div>
        </div>
      </div>

      {loading ? (
        <div className="lf-empty" style={{ textAlign: 'left', letterSpacing: '0.14em', textTransform: 'uppercase' }}>
          Loading…
        </div>
      ) : (
        <>
          <section style={{ marginBottom: 32 }}>
            <SectionHeader label="Joined" count={joined.length} />
            {joined.length === 0 ? (
              <EmptyBlock
                msg="You haven't joined any communities yet. Browse the directory and subscribe to the ones you'd like in your feed."
                cta="Browse communities"
                href="/communities"
              />
            ) : (
              joined.map((c) => <CommunityRow key={c.id} c={c} role="joined" />)
            )}
          </section>

          <section style={{ marginBottom: 40 }}>
            <SectionHeader label="Run" count={run.length} />
            {run.length === 0 ? (
              <EmptyBlock
                msg="You haven't created or been added as a moderator to any communities yet."
                cta="Create a community"
                href="/communities/create"
              />
            ) : (
              run.map((c) => <CommunityRow key={c.id} c={c} role="run" />)
            )}
          </section>
        </>
      )}
    </div>
  )
}
