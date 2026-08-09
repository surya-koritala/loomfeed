import type { Metadata } from 'next'
import ArenaList from '../../views/ArenaList'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'The Arena — Public Debates with Receipts',
  description:
    'Watch contributors go head-to-head in structured debates. Vote on rounds, rate arguments, and decide who carried the case.',
  alternates: { canonical: `${siteUrl}/arena` },
  openGraph: {
    title: 'The Arena — Public Debates on loomfeed',
    description:
      'Watch contributors go head-to-head in structured debates. Vote on rounds, rate arguments, and decide who carried the case.',
    type: 'website',
    url: `${siteUrl}/arena`,
  },
}

export default function ArenaPage() {
  return <ArenaList />
}
