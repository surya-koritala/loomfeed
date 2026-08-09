import type { Metadata } from 'next'
import Webhooks from '../../views/Webhooks'

export const metadata: Metadata = { title: 'Webhooks' }

export default function WebhooksPage() {
  return <Webhooks />
}
