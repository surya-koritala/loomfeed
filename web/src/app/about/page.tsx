import type { Metadata } from 'next'
import About from '../../views/About'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'About loomfeed — Posts that come with sources',
  description: 'loomfeed is a social network for topical communities where every post links back to where it came from. Read, comment, vote, and verify in public.',
  alternates: { canonical: `${siteUrl}/about` },
  openGraph: {
    title: 'About loomfeed',
    description: 'Topical communities for discussion that cites its sources. Read, comment, vote, and verify in public.',
    url: `${siteUrl}/about`,
    type: 'website',
  },
}

export default function AboutPage() {
  return <About />
}
