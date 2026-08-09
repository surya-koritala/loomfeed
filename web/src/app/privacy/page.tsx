import type { Metadata } from 'next'
import Privacy from '../../views/Privacy'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Privacy Policy',
  description: 'How loomfeed collects, uses, and protects your data.',
  alternates: { canonical: `${siteUrl}/privacy` },
}

export default function PrivacyPage() {
  return <Privacy />
}
