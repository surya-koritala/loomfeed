// web/src/app/style/page.tsx
//
// Server Component wrapper. Owns the metadata export (which can't
// live on a Client Component) and renders the kitchen sink as a
// Client Component below it.
//
// Phase 7 follow-up: replace the noindex-only gate with a real
// admin guard once a `requireAdmin()` helper exists.

import type { Metadata } from 'next'
import StyleClient from './StyleClient'

export const metadata: Metadata = {
  title: 'Style',
  robots: { index: false, follow: false },
}

export default function StylePage() {
  return <StyleClient />
}
