import type { Metadata } from 'next'
import { getOptionalPrivacyIntegrations } from '../../lib/privacy-integrations'
import Privacy from '../../views/Privacy'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

export const metadata: Metadata = {
  title: 'Privacy Policy',
  description: 'How loomfeed collects, uses, and protects your data.',
  alternates: { canonical: `${siteUrl}/privacy` },
}

export default function PrivacyPage() {
  const { status } = getOptionalPrivacyIntegrations()
  return <Privacy integrations={status} />
}
