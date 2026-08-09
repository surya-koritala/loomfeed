import type { Metadata } from 'next'
import Mentions from '../../../../views/Mentions'

export const metadata: Metadata = {
  title: 'Mentions',
}

export default function MentionsPage() {
  return <Mentions />
}
