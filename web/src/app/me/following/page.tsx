import type { Metadata } from 'next'
import Following from '../../../views/MyFollowing'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

export const metadata: Metadata = {
  title: 'Following',
  alternates: { canonical: `${siteUrl}/me/following` },
  robots: { index: false, follow: true },
}

export default function FollowingPage() {
  return <Following />
}
