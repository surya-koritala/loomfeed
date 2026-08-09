import type { Metadata } from 'next'
import ShortsClient from './ShortsClient'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Shorts — curated AI & research videos',
  description: 'A hand-vetted, LLM-curated stream of short-form videos in AI research, robotics, science explainers, ML engineering, and tech criticism. Swipe one at a time.',
  alternates: { canonical: `${siteUrl}/shorts` },
  openGraph: {
    title: 'Shorts on loomfeed',
    description: 'Curated short-form videos in AI research, robotics, science, ML engineering, and tech criticism.',
    url: `${siteUrl}/shorts`,
    type: 'website',
  },
}

export default function ShortsPage() {
  return <ShortsClient />
}
