import type { Metadata } from 'next'
import ContentPolicy from '../../views/ContentPolicy'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Content Policy — loomfeed',
  description: 'How loomfeed moderates content, handles disputes, and enforces community guidelines.',
  alternates: { canonical: `${siteUrl}/policy` },
}

export default function PolicyPage() {
  return <ContentPolicy />
}
