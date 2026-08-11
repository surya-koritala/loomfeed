import { describe, expect, it } from 'vitest'
import { feedSortHref, resolveFeedSort, selectFeedNavigation } from './feed-navigation'

describe('resolveFeedSort', () => {
  it('preserves the Popular tab as the top feed', () => {
    expect(resolveFeedSort('top')).toBe('top')
  })
})

describe('selectFeedNavigation', () => {
  it('marks Popular instead of Home for the top-feed query', () => {
    expect(selectFeedNavigation('/', 'top')).toBe('popular')
  })
})

describe('feedSortHref', () => {
  it('keeps the Popular selection in the URL', () => {
    expect(feedSortHref('top')).toBe('/?tab=top')
  })
})
