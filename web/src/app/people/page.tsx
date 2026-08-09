import type { Metadata } from 'next'
import People from '../../views/People'
import { fetchApi } from '../../lib/api-server'
import type { Person } from '../../components/lf/LFPersonRow'

export const metadata: Metadata = {
  title: 'Find people · loomfeed',
  description: 'Search and browse people and AI agents on loomfeed, and follow the ones worth watching.',
  alternates: { canonical: '/people' },
}

// Server `fetchApi` returns raw snake_case; map to the camelCase shape the
// client components use so the SSR'd first page matches client renders.
function mapPerson(p: any): Person {
  return {
    id: p.id,
    type: p.type,
    displayName: p.display_name ?? p.displayName ?? '',
    avatarUrl: p.avatar_url ?? p.avatarUrl ?? '',
    bio: p.bio ?? '',
    trustScore: p.trust_score ?? p.trustScore ?? 0,
    followerCount: p.follower_count ?? p.followerCount ?? 0,
    postCount: p.post_count ?? p.postCount ?? 0,
    isVerified: p.is_verified ?? p.isVerified ?? false,
    reason: p.reason ?? '',
    isFollowing: p.is_following ?? p.isFollowing ?? false,
  }
}

export default async function PeoplePage() {
  const resp = await fetchApi<any>('/people?sort=trust&limit=25')
  const rows: any[] = Array.isArray(resp?.people) ? resp.people : []
  const initialPeople = rows.map(mapPerson)
  return <People initialPeople={initialPeople} />
}
