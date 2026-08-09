import type { Metadata } from 'next'
import Submit from '../../views/Submit'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Create Post',
  alternates: { canonical: `${siteUrl}/submit` },
  // Auth-gated tool page — anon users land on /login, so don't index.
  robots: { index: false, follow: true },
}

export default function SubmitPage() {
  return <Submit />
}
