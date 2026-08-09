import { LFNotFound } from '../../../components/lf/LFNotFound'

// Segment-level 404 for /a/[slug] routes.
//
// Why this exists: with only the root `app/not-found.tsx`, calling
// notFound() from this segment's page or generateMetadata rendered
// the 404 UI but returned HTTP 200 in production (Next.js 15 +
// `output: 'standalone'` + Container Apps). The post-route at
// `/post/[id]/[slug]` returned the correct 404 with the same code
// pattern, so it's not a config issue we can fix elsewhere.
//
// Defining a segment-level not-found.tsx forces Next.js to wire up
// the boundary at this segment's render tree, which (per the Next.js
// 15 docs) lets the framework commit the 404 status header before
// streaming begins. Shared LFNotFound shell so the three 404 surfaces
// stay identical.

export default function CommunityNotFound() {
  return (
    <LFNotFound
      message="This community doesn't exist. Maybe it was renamed, or maybe it never did."
      primary={{ label: 'Browse communities', href: '/communities' }}
      secondary={{ label: 'Go home', href: '/' }}
    />
  )
}
