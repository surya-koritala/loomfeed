// JsonLd — emits a `<script type="application/ld+json">` element.
//
// HEADS UP: in Next.js 15 + React 19, a `<script>` with
// `dangerouslySetInnerHTML` rendered inside a page Server Component
// (next to client components like our `<Home>`, `<PostDetail>`,
// `<Community>`, etc.) does NOT survive RSC streaming as a literal
// `<script>` tag in the SSR'd HTML — React 19's `<head>` auto-hoisting
// applies only to specific metadata elements and to scripts with
// `src` or `async`, not inline content. Wrapping the script in
// `<head>` does not change the outcome, and `next/script` with
// `strategy="beforeInteractive"` is constrained to `app/layout.tsx`
// in the App Router.
//
// Net effect: pages using this component still emit the JSON-LD,
// but it shows up in the React Server Components stream (via
// `self.__next_f.push(...)`) and only materializes after client-side
// hydration. Modern Googlebot renders JS and picks it up; Bing /
// DuckDuckGo / Yandex / social-card services don't.
//
// For site-wide schemas (WebSite, Organization), prefer placing the
// inline `<script>` directly inside `app/layout.tsx`'s `<head>` —
// scripts there DO get SSR'd, same as the theme-toggle and
// service-worker scripts.
//
// This component is kept for per-page schemas (Article on /post,
// BreadcrumbList, ItemList on /agents and /communities) where the
// JS-rendered fallback is acceptable for our SEO target (Google).

import { serializeJsonLd } from '../../lib/jsonld'

interface Props {
  data: unknown
}

export default function JsonLd({ data }: Props) {
  return (
    <script
      type="application/ld+json"
      dangerouslySetInnerHTML={{ __html: serializeJsonLd(data) }}
    />
  )
}
