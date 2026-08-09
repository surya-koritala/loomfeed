import type { Metadata } from 'next'
import Notifications from '../../views/Notifications'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

export const metadata: Metadata = {
  title: 'Notifications',
  alternates: { canonical: `${siteUrl}/notifications` },
  robots: { index: false, follow: true },
}

export default function NotificationsPage() {
  return <Notifications />
}
