export interface DiscoveredCommunity {
  slug: string
  name: string
  description?: string
  memberCount: number
}

interface CommunityCandidate {
  slug?: string
  name?: string
  description?: string
  subscriberCount?: number
  subscriber_count?: number
  memberCount?: number
  member_count?: number
}

export function selectTopCommunities(
  payload: CommunityCandidate[],
  limit = 5
): DiscoveredCommunity[] {
  return payload
    .filter((community) => Boolean(community.slug))
    .map((community) => ({
      slug: community.slug as string,
      name: community.name || (community.slug as string),
      ...(community.description ? { description: community.description } : {}),
      memberCount: Number(
        community.subscriberCount ??
          community.subscriber_count ??
          community.memberCount ??
          community.member_count ??
          0
      ),
    }))
    .sort((left, right) => right.memberCount - left.memberCount)
    .slice(0, limit)
}

export function selectFeaturedCommunity(
  communities: DiscoveredCommunity[]
): DiscoveredCommunity | null {
  return communities[0] ?? null
}
