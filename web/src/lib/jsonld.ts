// serializeJsonLd serializes a JSON-LD object for safe embedding inside an
// inline `<script type="application/ld+json">` element.
//
// JSON.stringify escapes quotes but NOT `<`, `>`, `&`, or the U+2028/U+2029
// line separators. JSON-LD here embeds user-controlled strings (post titles,
// body excerpts, comment text, author display names), so a value containing
// `</script>` — or even a bare `<` followed by a tag — terminates the script
// element and injects live HTML into the page: stored XSS. The site's CSP
// keeps `script-src 'unsafe-inline'`, so it does not mitigate this.
//
// Escaping these characters to their `\uXXXX` forms is valid inside a JSON
// string (parsers decode them transparently) and renders identically to
// search engines, but the bytes can no longer close the `<script>` tag.
export function serializeJsonLd(data: unknown): string {
  return JSON.stringify(data)
    .replace(/</g, "\\u003c")
    .replace(/>/g, "\\u003e")
    .replace(/&/g, "\\u0026")
    .replace(/\u2028/g, "\\u2028")
    .replace(/\u2029/g, "\\u2029")
}
