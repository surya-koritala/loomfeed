'use client'

import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import { api } from '../api/client'
import {
  selectFeaturedCommunity,
  selectTopCommunities,
  type DiscoveredCommunity,
} from '../lib/community-discovery'

interface CommunityDiscoveryValue {
  communities: DiscoveredCommunity[]
  featuredCommunity: DiscoveredCommunity | null
}

const CommunityDiscoveryContext = createContext<CommunityDiscoveryValue>({
  communities: [],
  featuredCommunity: null,
})

export function CommunityDiscoveryProvider({ children }: { children: React.ReactNode }) {
  const [communities, setCommunities] = useState<DiscoveredCommunity[]>([])

  useEffect(() => {
    let cancelled = false

    api
      .getCommunities({ sort: 'subscribers', limit: 5 })
      .then((data: unknown) => {
        if (cancelled) return
        setCommunities(selectTopCommunities(Array.isArray(data) ? data : [], 5))
      })
      .catch(() => {
        if (!cancelled) setCommunities([])
      })

    return () => {
      cancelled = true
    }
  }, [])

  const value = useMemo(
    () => ({
      communities,
      featuredCommunity: selectFeaturedCommunity(communities),
    }),
    [communities]
  )

  return (
    <CommunityDiscoveryContext.Provider value={value}>
      {children}
    </CommunityDiscoveryContext.Provider>
  )
}

export function useCommunityDiscovery(): CommunityDiscoveryValue {
  return useContext(CommunityDiscoveryContext)
}
