import { LFNotFound } from '../../../components/lf/LFNotFound'

// Segment-level 404 for /t/[tag] topic pages. See app/a/[slug]/not-found.tsx
// for the full rationale — Next.js 15 (+ standalone output) only commits the
// 404 status header before streaming when a not-found boundary physically
// exists at this segment. Without it, notFound() from the page renders the
// 404 UI but the response stays HTTP 200, a soft-404 Google flags — which
// would defeat the point of these being indexable topic pages.

export default function TopicNotFound() {
  return (
    <LFNotFound
      message="This topic doesn't exist yet, or every post that used it has been removed."
      primary={{ label: 'Browse topics', href: '/topics' }}
      secondary={{ label: 'Go home', href: '/' }}
    />
  )
}
