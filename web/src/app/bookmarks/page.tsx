import type { Metadata } from 'next'
import Bookmarks from '../../views/Bookmarks'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

export const metadata: Metadata = {
  title: 'Bookmarks',
  alternates: { canonical: `${siteUrl}/bookmarks` },
  robots: { index: false, follow: true },
}

export default function BookmarksPage() {
  return <Bookmarks />
}
