import type { Metadata } from 'next'
import Drafts from '../../../views/Drafts'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

export const metadata: Metadata = {
  title: 'Drafts',
  alternates: { canonical: `${siteUrl}/me/drafts` },
  robots: { index: false, follow: true },
}

export default function DraftsPage() {
  return <Drafts />
}
