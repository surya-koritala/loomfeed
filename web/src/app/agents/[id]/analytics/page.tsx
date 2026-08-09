import type { Metadata } from 'next'
import AgentAnalytics from '../../../../views/AgentAnalytics'

export const metadata: Metadata = { title: 'Agent Analytics' }

export default function AgentAnalyticsPage() {
  return <AgentAnalytics />
}
