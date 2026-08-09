// Canonical post URL = /post/{uuid}/{slugified-title}. The UUID stays
// the load-bearing lookup key (cheap DB index, immutable across
// title edits) and the slug is a keyword-carrying tail Google ranks
// against — same shape Reddit, Stack Overflow, and the NYT use.
//
// Old URL: /post/bbac3f32-c2b4-4f97-a702-67575f42b2a3
// New URL: /post/bbac3f32-c2b4-4f97-a702-67575f42b2a3/why-agents-need-provenance
//
// The Next.js route handles both; when the tail is missing or wrong
// we 301-redirect to the canonical slug.

// Slugify — aggressive. Strips markdown, lowercases, replaces every
// non-alphanumeric run with a single hyphen, trims leading/trailing
// hyphens. When the slug is longer than our soft cap, truncates at
// the last word boundary so we never chop a word mid-letter
// (e.g. "changing-th" → "changing"). Always returns a non-empty value
// so /post/<uuid>/ never appears with a trailing slash.
const SOFT_MAX = 80   // target length
const HARD_MAX = 100  // absolute cap (covers long single-word titles)

export function slugifyTitle(title: string | undefined | null): string {
  if (!title) return 'post'
  let s = String(title).toLowerCase()
  // Strip markdown noise first so "**bold** `code`" doesn't turn into
  // a line of dashes.
  s = s
    .replace(/!\[.*?\]\([^)]*\)/g, '')
    .replace(/\[(.*?)\]\([^)]*\)/g, '$1')
    .replace(/```[\s\S]*?```/g, '')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/\*\*(.+?)\*\*/g, '$1')
    .replace(/\*(.+?)\*/g, '$1')
    .replace(/_(.+?)_/g, '$1')
    .replace(/#{1,6}\s+/g, '')
    .replace(/[\u2018\u2019\u201C\u201D]/g, '') // smart quotes

  s = s
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '') // strip accents
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')

  if (!s) return 'post'
  if (s.length <= SOFT_MAX) return s

  // Cut at the last hyphen that falls inside [SOFT_MAX/2, SOFT_MAX] so
  // the final slug ends on a word boundary. Fall back to SOFT_MAX if
  // there's no hyphen in that window (rare: one long "word"); if the
  // whole thing is one long hyphenless blob, hard-cap so the URL
  // never exceeds a sane length.
  const window = s.slice(0, SOFT_MAX)
  const lastHyphen = window.lastIndexOf('-')
  if (lastHyphen >= Math.floor(SOFT_MAX / 2)) {
    return window.slice(0, lastHyphen)
  }
  return s.slice(0, HARD_MAX).replace(/-+$/, '')
}

// Given a post object (or bare id + title), produce the canonical
// site-relative URL. Always starts with "/post/" — host is up to the
// caller to prepend if absolute is needed.
export function postUrl(
  post: { id: string; title?: string | null } | string,
  title?: string | null,
): string {
  const id = typeof post === 'string' ? post : post.id
  const t = typeof post === 'string' ? title : post.title
  return `/post/${id}/${slugifyTitle(t)}`
}
