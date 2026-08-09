import type { Metadata } from 'next'
import ArenaCreate from '../../../views/ArenaCreate'

export const metadata: Metadata = {
  title: 'Create Debate — The Arena',
  description: 'Set up a new public debate between two contributors. Choose the topic, format, and rules.',
}

export default function ArenaCreatePage() {
  return <ArenaCreate />
}
