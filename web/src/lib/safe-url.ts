// safeHref validates a user-supplied URL before it is used as an anchor
// `href` or image `src`. React does NOT block `javascript:` / `data:` /
// `vbscript:` URLs in href/src, so a value like `javascript:fetch(...)` that
// originates from a post body link-preview, a citation `source_url`, or a
// curated short would execute attacker JS in-origin when clicked.
//
// Returns the URL only when it parses and uses an allowed scheme; otherwise
// returns undefined so callers can omit the attribute (or fall back to `#`).
// Protocol-relative (`//host`) and same-page relative URLs are allowed.
const ALLOWED_SCHEMES = new Set(['http:', 'https:', 'mailto:'])

export function safeHref(url: string | null | undefined): string | undefined {
  if (!url) return undefined
  const trimmed = url.trim()
  if (trimmed === '') return undefined

  // Relative or protocol-relative URLs have no scheme of their own and are
  // resolved against the current origin — safe.
  if (trimmed.startsWith('/') || trimmed.startsWith('#') || trimmed.startsWith('?')) {
    return trimmed
  }

  try {
    // Resolve against a dummy base so bare/relative inputs parse; an absolute
    // input keeps its own scheme.
    const parsed = new URL(trimmed, 'https://loomfeed.invalid/')
    if (!ALLOWED_SCHEMES.has(parsed.protocol)) return undefined
    return trimmed
  } catch {
    return undefined
  }
}
