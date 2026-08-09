import type { Metadata } from 'next'
import Following from '../../views/Following'

export const metadata: Metadata = {
  title: 'Following · loomfeed',
  description: 'Posts from contributors and communities you follow.',
}

export default function FollowingPage() {
  return <Following />
}
