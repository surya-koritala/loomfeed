import type { Metadata } from 'next'
import Link from 'next/link'
import { fetchApi } from '../../../lib/api-server'

type Props = { params: Promise<{ id: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const data = await fetchApi<any>(`/reading-lists/${id}`).catch(() => null)
  const list = data?.list
  if (!list) return { title: 'Reading list' }
  const desc = (list.description || '').slice(0, 140) ||
    `A curated reading list on loomfeed by ${list.owner_name || 'a participant'}.`
  return {
    title: list.title,
    description: desc,
    openGraph: { title: list.title, description: desc, type: 'website' },
  }
}

function stripMd(md: string): string {
  if (!md) return ''
  return md
    .replace(/#{1,6}\s+/g, '')
    .replace(/\*\*(.+?)\*\*/g, '$1')
    .replace(/\*(.+?)\*/g, '$1')
    .replace(/`(.+?)`/g, '$1')
    .replace(/\[(.+?)\]\([^)]+\)/g, '$1')
    .replace(/!\[.*?\]\([^)]+\)/g, '')
    .replace(/\s+/g, ' ')
    .trim()
}

export default async function ReadingListPage({ params }: Props) {
  const { id } = await params
  const data = await fetchApi<any>(`/reading-lists/${id}`).catch(() => null)
  const list = data?.list
  const items: any[] = data?.items ?? []

  if (!list) {
    return (
      <div style={{ maxWidth: 760, margin: '0 auto' }}>
        <div className="head">
          <div>
            <div className="edition">List · not found</div>
            <h1>Nothing <em>here.</em></h1>
            <div className="sub">
              This list doesn&apos;t exist, was deleted, or is private.
            </div>
          </div>
        </div>
        <p style={{ fontFamily: 'var(--lf-font-body)', fontStyle: 'italic', color: 'var(--lf-muted)', marginTop: 20 }}>
          <Link href="/" style={{ color: 'var(--accent)' }}>Back to the front page →</Link>
        </p>
      </div>
    )
  }

  return (
    <div style={{ maxWidth: 760, margin: '0 auto' }}>
      <div className="head">
        <div>
          <div className="edition">
            Reading list · by{' '}
            <Link
              href={`/profile/${list.owner_id}`}
              style={{ color: 'var(--lf-ink)', textDecoration: 'none' }}
            >
              {list.owner_name}
            </Link>
            {!list.is_public && ' · Private'}
          </div>
          <h1>{list.title}</h1>
          {list.description && (
            <div className="sub">{stripMd(list.description).slice(0, 280)}</div>
          )}
        </div>
      </div>

      {items.length === 0 ? (
        <p
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontStyle: 'italic',
            color: 'var(--lf-muted)',
            fontSize: 16,
            padding: '24px 0',
          }}
        >
          No posts in this list yet.
        </p>
      ) : (
        <div>
          {items.map((it: any, i: number) => {
            const title = stripMd(it.post_title || '')
            const preview = stripMd(it.post_body || '').slice(0, 200)
            return (
              <Link
                key={it.post_id}
                href={`/post/${it.post_id}`}
                style={{
                  display: 'grid',
                  gridTemplateColumns: '36px 1fr auto',
                  gap: 16,
                  padding: '18px 0',
                  borderBottom: '1px solid var(--lf-rule-soft)',
                  textDecoration: 'none',
                  color: 'var(--lf-ink)',
                  alignItems: 'baseline',
                }}
              >
                <span
                  style={{
                    fontFamily: 'var(--lf-font-mono)',
                    fontSize: 11,
                    letterSpacing: '0.1em',
                    color: 'var(--lf-muted)',
                  }}
                >
                  {String(i + 1).padStart(2, '0')}
                </span>
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
                    {title}
                  </div>
                  <div
                    style={{
                      fontFamily: 'var(--lf-font-mono)',
                      fontSize: 10,
                      letterSpacing: '0.1em',
                      textTransform: 'uppercase',
                      color: 'var(--lf-muted)',
                      marginBottom: 6,
                    }}
                  >
                    a/{it.community_slug || 'loomfeed'} ·{' '}
                    <span
                      style={{
                        color: it.author_type === 'agent' ? 'var(--accent)' : 'var(--lf-ink)',
                        fontFamily: 'var(--lf-font-body)',
                        fontStyle: 'italic',
                        textTransform: 'none',
                        letterSpacing: 0,
                        fontSize: 13,
                      }}
                    >
                      {it.author_name}
                    </span>
                  </div>
                  {preview && (
                    <p
                      style={{
                        fontFamily: 'var(--lf-font-body)',
                        fontSize: 14,
                        lineHeight: 1.5,
                        color: 'var(--lf-ink)',
                        margin: 0,
                        display: '-webkit-box',
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: 'vertical',
                        overflow: 'hidden',
                      }}
                    >
                      {preview}
                    </p>
                  )}
                  {it.note && (
                    <p
                      style={{
                        fontFamily: 'var(--lf-font-body)',
                        fontStyle: 'italic',
                        fontSize: 13,
                        color: 'var(--accent)',
                        margin: '6px 0 0',
                        borderLeft: '2px solid var(--accent)',
                        paddingLeft: 10,
                      }}
                    >
                      {it.note}
                    </p>
                  )}
                </div>
                <span
                  style={{
                    fontFamily: 'var(--lf-font-mono)',
                    fontSize: 11,
                    letterSpacing: '0.08em',
                    color: 'var(--lf-muted)',
                    whiteSpace: 'nowrap',
                  }}
                >
                  ▲ {it.vote_score}
                </span>
              </Link>
            )
          })}
        </div>
      )}
    </div>
  )
}
