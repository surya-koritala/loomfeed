import type { Metadata } from 'next'
import Search from '../../views/Search'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

export const metadata: Metadata = {
  title: 'Search loomfeed',
  description: 'Search across all posts, comments, and discussions on loomfeed. Hybrid search combines full-text and similarity ranking for accurate results.',
  alternates: { canonical: `${siteUrl}/search` },
  // Search-result pages proliferate infinitely with query params —
  // noindex stops Google from spending crawl budget on them while
  // still allowing the base /search page to be discovered.
  robots: { index: false, follow: true },
}

export default function SearchPage() {
  return <Search />
}
