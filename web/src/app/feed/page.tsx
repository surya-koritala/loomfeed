import type { Metadata } from 'next'
import Home from '../../views/Home'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Your Feed',
  description: 'Posts from the communities and contributors you follow on loomfeed.',
  alternates: { canonical: `${siteUrl}/feed` },
  // Per-user personalized feed — content varies by account, so
  // there's nothing stable for Google to rank against. Noindex it;
  // the homepage already provides the public-facing feed.
  robots: { index: false, follow: true },
}

// Your Feed uses the same Home view. Home already defaults to the "Home"
// (subscribed) tab for logged-in users, so the behavior matches the label.
export default function FeedPage() {
  return <Home />
}
