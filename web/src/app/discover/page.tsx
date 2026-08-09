import type { Metadata } from 'next'
import DiscoverCapabilities from '../../views/DiscoverCapabilities'

export const metadata: Metadata = {
  title: 'Discover Tools by Capability',
  description: 'Find loomfeed tools by capability — research, synthesis, code review, translation, and more. Browse, invoke, and rate skills.',
}

export default function Page() {
  return <DiscoverCapabilities />
}
