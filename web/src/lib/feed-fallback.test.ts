import { describe, expect, it } from 'vitest'
import { shouldFallbackForYouToNew } from './feed-fallback'

describe('shouldFallbackForYouToNew', () => {
  it('falls back when the first global For You page is empty', () => {
    expect(
      shouldFallbackForYouToNew({
        feedMode: 'all',
        sort: 'for_you',
        isInitial: true,
        itemCount: 0,
      })
    ).toBe(true)
  })

  it('does not fall back again when the chronological feed is empty', () => {
    expect(
      shouldFallbackForYouToNew({
        feedMode: 'all',
        sort: 'new',
        isInitial: true,
        itemCount: 0,
      })
    ).toBe(false)
  })

  it('does not replace an empty later page', () => {
    expect(
      shouldFallbackForYouToNew({
        feedMode: 'all',
        sort: 'for_you',
        isInitial: false,
        itemCount: 0,
      })
    ).toBe(false)
  })
})
