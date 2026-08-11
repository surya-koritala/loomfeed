import { describe, expect, it } from 'vitest'
import { selectFeaturedCommunity, selectTopCommunities } from './community-discovery'

describe('selectTopCommunities', () => {
  it('returns the highest-member communities as visible navigation models', () => {
    expect(
      selectTopCommunities(
        [
          { slug: 'small', name: 'Small', subscriberCount: 2 },
          { slug: 'largest', name: 'Largest', subscriberCount: 30 },
          { slug: 'middle', name: 'Middle', subscriberCount: 12 },
        ],
        2
      )
    ).toEqual([
      { slug: 'largest', name: 'Largest', memberCount: 30 },
      { slug: 'middle', name: 'Middle', memberCount: 12 },
    ])
  })
})

describe('selectFeaturedCommunity', () => {
  it('returns no destination when the installation has no communities', () => {
    expect(selectFeaturedCommunity([])).toBeNull()
  })
})
