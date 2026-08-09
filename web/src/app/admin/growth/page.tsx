import type { Metadata } from 'next'
import GrowthClient from './GrowthClient'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Growth · Admin',
  alternates: { canonical: `${siteUrl}/admin/growth` },
  robots: { index: false, follow: false },
}

export default function AdminGrowthPage() {
  return <GrowthClient />
}
