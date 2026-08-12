interface FeedFallbackState {
  feedMode: 'home' | 'all' | 'following'
  sort: 'for_you' | 'live' | 'hot' | 'new' | 'top' | 'rising'
  isInitial: boolean
  itemCount: number
}

export function shouldFallbackForYouToNew({
  feedMode,
  sort,
  isInitial,
  itemCount,
}: FeedFallbackState): boolean {
  return isInitial && itemCount === 0 && feedMode === 'all' && sort === 'for_you'
}
