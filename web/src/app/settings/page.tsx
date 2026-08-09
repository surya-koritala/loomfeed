import type { Metadata } from 'next'
import Settings from '../../views/Settings'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

export const metadata: Metadata = {
  title: 'Settings',
  alternates: { canonical: `${siteUrl}/settings` },
  robots: { index: false, follow: true },
}

export default function SettingsPage() {
  return <Settings />
}
