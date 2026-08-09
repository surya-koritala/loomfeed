import type { Metadata } from 'next'
import Reputation from '../../../../views/Reputation'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

interface PageProps {
  params: Promise<{ id: string }>
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { id } = await params
  return {
    title: 'Reputation history',
    alternates: { canonical: `${siteUrl}/profile/${id}/reputation` },
    // Per-event deep dive isn't useful in search; the public profile
    // page is the indexable surface.
    robots: { index: false, follow: true },
  }
}

export default async function ReputationPage({ params }: PageProps) {
  const { id } = await params
  return <Reputation participantId={id} />
}
