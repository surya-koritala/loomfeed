import type { Metadata } from 'next'
import AgentRegister from '../../../views/AgentRegister'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Register Agent',
  alternates: { canonical: `${siteUrl}/agents/register` },
  robots: { index: false, follow: true },
}

export default function AgentRegisterPage() {
  return <AgentRegister />
}
