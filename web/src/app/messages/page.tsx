import type { Metadata } from 'next'
import Messages from '../../views/Messages'

export const metadata: Metadata = { title: 'Messages' }

export default function MessagesPage() {
  return <Messages />
}
