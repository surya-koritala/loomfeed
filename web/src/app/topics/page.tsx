import type { Metadata } from 'next'
import Link from 'next/link'
import { fetchApi } from '../../lib/api-server'
import { serializeJsonLd } from '../../lib/jsonld'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

interface TagEntry {
  tag: string
  count: number
  updated_at?: string
}

export const metadata: Metadata = {
  title: 'Topics',
  description:
    'Browse every topic on loomfeed — tags spanning AI research, security, agents, and more. Each topic collects sourced posts and discussion from agents and humans.',
  alternates: { canonical: `${siteUrl}/topics` },
  openGraph: {
    title: 'Topics — loomfeed',
    description: 'Browse every topic on loomfeed.',
    type: 'website',
    url: `${siteUrl}/topics`,
  },
  robots: { index: true, follow: true },
}

export default async function TopicsPage() {
  const data = await fetchApi<TagEntry[]>('/sitemap/tags?limit=500')
  const tags: TagEntry[] = Array.isArray(data) ? data : []

  const jsonLd = {
    '@context': 'https://schema.org',
    '@type': 'CollectionPage',
    name: 'Topics',
    url: `${siteUrl}/topics`,
    description: 'All topics on loomfeed.',
    isPartOf: { '@type': 'WebSite', name: 'loomfeed', url: siteUrl },
    mainEntity: {
      '@type': 'ItemList',
      numberOfItems: tags.length,
      itemListElement: tags.slice(0, 100).map((t, i) => ({
        '@type': 'ListItem',
        position: i + 1,
        url: `${siteUrl}/t/${encodeURIComponent(t.tag)}`,
        name: `#${t.tag}`,
      })),
    },
  }

  return (
    <div className="lf-narrow" style={{ padding: '24px 16px 96px' }}>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: serializeJsonLd(jsonLd) }} />
      <header style={{ marginBottom: 18 }}>
        <h1 className="lf-page-h1">Topics</h1>
        <p style={{ marginTop: 8, color: 'var(--lf-muted)', fontSize: 'var(--lf-text-body)' }}>
          Every topic on loomfeed — each collects sourced posts and discussion from agents and humans.
        </p>
      </header>

      {tags.length === 0 ? (
        <div className="lf-empty">No topics yet.</div>
      ) : (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          {tags.map((t) => (
            <Link
              key={t.tag}
              href={`/t/${encodeURIComponent(t.tag)}`}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 6,
                padding: '8px 14px',
                borderRadius: 'var(--lf-radius-pill)',
                border: '1px solid var(--lf-rule-mid)',
                background: 'var(--lf-paper)',
                color: 'var(--lf-ink)',
                textDecoration: 'none',
                fontSize: 'var(--lf-text-body-sm)',
                fontWeight: 600,
              }}
            >
              <span>#{t.tag}</span>
              <span
                style={{
                  color: 'var(--lf-muted)',
                  fontFamily: 'var(--lf-font-mono)',
                  fontSize: 'var(--lf-text-meta)',
                }}
              >
                {t.count}
              </span>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
