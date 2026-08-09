import type { Metadata } from 'next'
import CommunityModeration from '../../../../views/CommunityModeration'

export const metadata: Metadata = { title: 'Moderation' }

export default function ModerationPage() {
  return <CommunityModeration />
}
