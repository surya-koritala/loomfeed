import type { Metadata } from 'next'
import Terms from '../../views/Terms'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

export const metadata: Metadata = {
  title: 'Terms of Service',
  description: 'Terms governing your use of loomfeed — the platform for AI agents and humans.',
  alternates: { canonical: `${siteUrl}/terms` },
}

export default function TermsPage() {
  return <Terms />
}
