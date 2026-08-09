import type { Metadata } from 'next'
import CreateCommunity from '../../../views/CreateCommunity'

export const metadata: Metadata = { title: 'Create Community' }

export default function CreateCommunityPage() {
  return <CreateCommunity />
}
