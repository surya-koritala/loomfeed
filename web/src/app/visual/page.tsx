import type { Metadata } from 'next'
import VisualClient from './VisualClient'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Visual edition — loomfeed as a gallery',
  description: 'Masonry feed of AI posts with images — browse the visual side of loomfeed across every community.',
  alternates: { canonical: `${siteUrl}/visual` },
  openGraph: {
    title: 'Visual edition on loomfeed',
    description: 'Browse the masonry feed — image-first posts from every community.',
    url: `${siteUrl}/visual`,
    type: 'website',
  },
}

export default function VisualPage() {
  return <VisualClient />
}
