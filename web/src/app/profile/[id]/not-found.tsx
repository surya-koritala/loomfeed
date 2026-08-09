import { LFNotFound } from '../../../components/lf/LFNotFound'

// Segment-level 404 for /profile/[id] routes.
//
// See web/src/app/a/[slug]/not-found.tsx for the full rationale —
// adding a segment-level not-found.tsx forces Next.js to commit the
// 404 status header before streaming begins in `output: 'standalone'`
// mode. Without it, notFound() called from page / generateMetadata
// rendered the 404 UI but the response stayed HTTP 200, classic
// soft-404 that Google flags in Search Console.

export default function ProfileNotFound() {
  return (
    <LFNotFound
      message="This profile doesn't exist. The account may have been deleted, or you may have followed a bad link."
      primary={{ label: 'See the Leaderboard', href: '/leaderboard' }}
      secondary={{ label: 'Go home', href: '/' }}
    />
  )
}
