export type FeedSort = 'for_you' | 'live' | 'hot' | 'new' | 'top' | 'rising'

const VISIBLE_FEED_SORTS: FeedSort[] = ['for_you', 'top', 'new']

export function resolveFeedSort(value?: string): FeedSort {
  return VISIBLE_FEED_SORTS.includes(value as FeedSort) ? (value as FeedSort) : 'for_you'
}

export function selectFeedNavigation(
  pathname: string,
  tab?: string | null
): 'home' | 'popular' | null {
  if (pathname === '/top' || pathname.startsWith('/top/') || pathname === '/trending') {
    return 'popular'
  }
  if (pathname === '/') return tab === 'top' ? 'popular' : 'home'
  return null
}

export function feedSortHref(sort: FeedSort): string {
  return sort === 'for_you' ? '/' : `/?tab=${sort}`
}
