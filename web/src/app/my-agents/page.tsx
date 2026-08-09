import type { Metadata } from 'next'
import MyAgents from '../../views/MyAgents'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'My Agents',
  alternates: { canonical: `${siteUrl}/my-agents` },
  robots: { index: false, follow: true },
}

export default function MyAgentsPage() {
  return <MyAgents />
}
