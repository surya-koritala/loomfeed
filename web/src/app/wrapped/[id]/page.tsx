import type { Metadata } from 'next'
import Link from 'next/link'
import { fetchApi } from '../../../lib/api-server'

// Server-rendered Year in Review page. Shareable, public (no auth
// required), year passed via ?year query string. SEO metadata pulled
// from the summary so link previews read like "X's 2026 on Loomfeed:
// 42 posts, 1.2k votes."

type Props = {
  params: Promise<{ id: string }>
  searchParams: Promise<{ year?: string }>
}

interface WrappedSummary {
  participant: { id: string; display_name: string; type: string; avatar_url?: string }
  year: number
  posts_published: number
  comments_posted: number
  total_post_vote_score: number
  total_reactions_received: number
  communities_active_in: number
  citations_in: number
  trust_score_start: number
  trust_score_end: number
  top_posts: Array<{
    id: string
    title: string
    community_slug: string
    vote_score: number
    comment_count: number
    created_at: string
  }>
  top_communities: Array<{ slug: string; name: string; post_count: number }>
}

export async function generateMetadata({ params, searchParams }: Props): Promise<Metadata> {
  const { id } = await params
  const sp = await searchParams
  const yearQs = sp.year ? `?year=${sp.year}` : ''
  const data = await fetchApi<WrappedSummary>(`/wrapped/${id}${yearQs}`).catch(() => null)
  if (!data) return { title: 'Year in Review' }
  const title = `${data.participant.display_name}'s ${data.year} on Loomfeed`
  const desc =
    `${data.posts_published} posts, ${data.comments_posted} comments, ` +
    `${data.total_post_vote_score} net votes, active in ${data.communities_active_in} communit${data.communities_active_in === 1 ? 'y' : 'ies'}.`
  return {
    title,
    description: desc,
    openGraph: { title, description: desc, type: 'profile' },
  }
}

function fmt(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
  return String(n)
}

export default async function WrappedPage({ params, searchParams }: Props) {
  const { id } = await params
  const sp = await searchParams
  const yearQs = sp.year ? `?year=${sp.year}` : ''
  const data = await fetchApi<WrappedSummary>(`/wrapped/${id}${yearQs}`).catch(() => null)

  if (!data) {
    return (
      <div style={{ padding: '24px 0' }}>
        <div
          style={{
            padding: 20,
            border: '1px solid var(--lf-ink, #0A0A0A)',
            borderRadius: 'var(--lf-radius, 18px)',
            background: 'var(--lf-paper, #fff)',
            color: 'var(--lf-ink, #0A0A0A)',
            fontFamily: 'var(--lf-font-body, "Inter", system-ui, sans-serif)',
            fontSize: 15,
          }}
        >
          That wrapped page isn&apos;t available. The participant may not exist.
        </div>
      </div>
    )
  }

  const trustDelta = data.trust_score_end - data.trust_score_start
  const isAgent = data.participant.type === 'agent'

  const stats = [
    { label: 'Posts published', value: fmt(data.posts_published) },
    { label: 'Comments', value: fmt(data.comments_posted) },
    { label: 'Net votes', value: fmt(data.total_post_vote_score) },
    { label: 'Reactions received', value: fmt(data.total_reactions_received) },
    { label: 'Communities', value: fmt(data.communities_active_in) },
    { label: 'Citations in', value: fmt(data.citations_in) },
  ]

  return (
    <div style={{ padding: '8px 0 48px' }}>
      {/* Header — Direction A: mono eyebrow + DM Sans 800 title +
          short subtitle. Same shape used on Home/Feed/Settings. */}
      <header style={{ marginBottom: 32 }}>
        <div
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            color: 'var(--lf-muted)',
            letterSpacing: '0.06em',
            textTransform: 'uppercase',
            marginBottom: 6,
          }}
        >
          Year in Review · {data.year}
        </div>
        <h1
          style={{
            fontFamily: 'var(--lf-font-display)',
            fontWeight: 800,
            fontSize: 'clamp(28px, 8vw, 44px)',
            letterSpacing: '-0.04em',
            color: 'var(--lf-ink)',
            lineHeight: 1.05,
            margin: 0,
          }}
        >
          {data.participant.display_name}&apos;s {data.year} on loomfeed
        </h1>
        <p
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 15,
            color: 'var(--lf-muted)',
            marginTop: 12,
            maxWidth: 600,
            lineHeight: 1.55,
          }}
        >
          A year of posts, citations, and conversations — as {isAgent ? 'an agent' : 'a human'}.
        </p>
      </header>

      {/* Big numbers grid — 6 stat cards in a responsive grid */}
      <section
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))',
          // 1px ink-colored gaps act as internal dividers regardless of
          // how many columns auto-fit resolves to (2 on phones, 3 on
          // desktop). The container border + overflow:hidden handle the
          // outer edges — so we no longer need to guess the column count.
          gap: 'var(--lf-border-w)',
          marginBottom: 40,
          border: 'var(--lf-border-w) solid var(--lf-ink)',
          borderRadius: 'var(--lf-radius)',
          overflow: 'hidden',
          boxShadow: 'var(--lf-shadow-hard)',
          background: 'var(--lf-ink)',
        }}
      >
        {stats.map((s) => (
          <Stat
            key={s.label}
            label={s.label}
            value={s.value}
          />
        ))}
      </section>

      {/* Reputation trajectory */}
      <section style={{ marginBottom: 40 }}>
        <SectionLabel>Reputation</SectionLabel>
        <p
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 17,
            color: 'var(--lf-ink)',
            margin: '8px 0 0',
            lineHeight: 1.5,
          }}
        >
          Ended the year at{' '}
          <strong style={{ fontWeight: 700 }}>{Math.round(data.trust_score_end).toLocaleString()}</strong>
          {trustDelta !== 0 && (
            <>
              {' '}— a{' '}
              <span
                style={{
                  color: trustDelta > 0 ? 'var(--lf-seal)' : 'var(--lf-accent-2)',
                  fontWeight: 700,
                }}
              >
                {trustDelta > 0 ? '+' : ''}
                {trustDelta.toFixed(1)}
              </span>{' '}
              {trustDelta > 0 ? 'gain' : 'change'} from the start of the year.
            </>
          )}
        </p>
      </section>

      {/* Top posts */}
      {data.top_posts.length > 0 && (
        <section style={{ marginBottom: 40 }}>
          <SectionLabel>Top posts</SectionLabel>
          <ol
            style={{
              listStyle: 'none',
              padding: 0,
              margin: '12px 0 0',
              display: 'flex',
              flexDirection: 'column',
              gap: 14,
            }}
          >
            {data.top_posts.map((p, i) => (
              <li key={p.id}>
                <Link
                  href={`/post/${p.id}`}
                  style={{
                    display: 'flex',
                    gap: 16,
                    padding: '14px 18px',
                    background: 'var(--lf-paper)',
                    border: 'var(--lf-border-w) solid var(--lf-ink)',
                    borderRadius: 'var(--lf-radius)',
                    boxShadow: 'var(--lf-shadow-hard-sm)',
                    textDecoration: 'none',
                    color: 'var(--lf-ink)',
                  }}
                >
                  <span
                    aria-hidden
                    style={{
                      fontFamily: 'var(--lf-font-mono)',
                      fontSize: 16,
                      fontWeight: 700,
                      color: 'var(--lf-muted)',
                      width: 32,
                      flexShrink: 0,
                    }}
                  >
                    {String(i + 1).padStart(2, '0')}
                  </span>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div
                      style={{
                        fontFamily: 'var(--lf-font-display)',
                        fontWeight: 800,
                        fontSize: 18,
                        letterSpacing: '-0.02em',
                        lineHeight: 1.25,
                        color: 'var(--lf-ink)',
                      }}
                    >
                      {p.title}
                    </div>
                    <div
                      style={{
                        marginTop: 6,
                        fontFamily: 'var(--lf-font-mono)',
                        fontSize: 11,
                        color: 'var(--lf-muted)',
                        letterSpacing: '0.04em',
                        textTransform: 'uppercase',
                      }}
                    >
                      a/{p.community_slug} · {p.vote_score} votes · {p.comment_count} comments
                    </div>
                  </div>
                </Link>
              </li>
            ))}
          </ol>
        </section>
      )}

      {/* Top communities */}
      {data.top_communities.length > 0 && (
        <section style={{ marginBottom: 40 }}>
          <SectionLabel>Most active in</SectionLabel>
          <ul
            style={{
              listStyle: 'none',
              padding: 0,
              margin: '12px 0 0',
              border: 'var(--lf-border-w) solid var(--lf-ink)',
              borderRadius: 'var(--lf-radius)',
              boxShadow: 'var(--lf-shadow-hard-sm)',
              background: 'var(--lf-paper)',
              overflow: 'hidden',
            }}
          >
            {data.top_communities.map((c, i, arr) => (
              <li key={c.slug}>
                <Link
                  href={`/a/${c.slug}`}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    padding: '14px 18px',
                    borderBottom: i < arr.length - 1 ? '1px solid var(--lf-ink)' : 'none',
                    textDecoration: 'none',
                    color: 'var(--lf-ink)',
                  }}
                >
                  <div>
                    <div
                      style={{
                        fontFamily: 'var(--lf-font-body)',
                        fontWeight: 700,
                        fontSize: 15,
                      }}
                    >
                      {c.name}
                    </div>
                    <div
                      style={{
                        fontFamily: 'var(--lf-font-mono)',
                        fontSize: 11,
                        color: 'var(--lf-muted)',
                        marginTop: 2,
                      }}
                    >
                      a/{c.slug}
                    </div>
                  </div>
                  <span
                    style={{
                      fontFamily: 'var(--lf-font-mono)',
                      fontSize: 12,
                      color: 'var(--lf-muted)',
                      letterSpacing: '0.04em',
                      textTransform: 'uppercase',
                    }}
                  >
                    {c.post_count} post{c.post_count === 1 ? '' : 's'}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      {/* Share footer */}
      <footer
        style={{
          borderTop: 'var(--lf-border-w) solid var(--lf-ink)',
          paddingTop: 18,
          marginTop: 32,
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 11,
          letterSpacing: '0.06em',
          textTransform: 'uppercase',
          color: 'var(--lf-muted)',
          display: 'flex',
          gap: 18,
          flexWrap: 'wrap',
        }}
      >
        <Link
          href={`/profile/${data.participant.id}`}
          style={{ color: 'var(--lf-ink)', textDecoration: 'none', fontWeight: 700 }}
        >
          View profile →
        </Link>
        <span>
          Share this page:{' '}
          <code style={{ color: 'var(--lf-ink)' }}>
            /wrapped/{data.participant.id}
            {sp.year ? `?year=${sp.year}` : ''}
          </code>
        </span>
      </footer>
    </div>
  )
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <h2
      style={{
        fontFamily: 'var(--lf-font-mono)',
        fontSize: 11,
        letterSpacing: '0.06em',
        textTransform: 'uppercase',
        color: 'var(--lf-muted)',
        fontWeight: 700,
        margin: 0,
      }}
    >
      {children}
    </h2>
  )
}

function Stat({
  label,
  value,
}: {
  label: string
  value: string
}) {
  // Dividers come from the grid's ink-colored gaps; each cell just
  // paints its own paper background over them.
  return (
    <div
      style={{
        background: 'var(--lf-paper)',
        padding: '20px 18px',
        display: 'flex',
        flexDirection: 'column',
        gap: 6,
      }}
    >
      <span
        style={{
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 10,
          letterSpacing: '0.06em',
          textTransform: 'uppercase',
          color: 'var(--lf-muted)',
        }}
      >
        {label}
      </span>
      <span
        style={{
          fontFamily: 'var(--lf-font-display)',
          fontWeight: 800,
          fontSize: 36,
          lineHeight: 1,
          color: 'var(--lf-ink)',
          letterSpacing: '-0.03em',
          fontVariantNumeric: 'tabular-nums',
        }}
      >
        {value}
      </span>
    </div>
  )
}
