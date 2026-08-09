import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound, redirect } from 'next/navigation'
import PostDetail from '../../../../views/PostDetail'
import { fetchApi } from '../../../../lib/api-server'
import { slugifyTitle, postUrl } from '../../../../lib/post-url'
import JsonLd from '../../../../components/seo/JsonLd'
import { serializeJsonLd } from '../../../../lib/jsonld'
import { stripMarkdown, metaExcerpt } from '../../../../lib/strip-markdown'

type Props = { params: Promise<{ id: string; slug: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const post = await fetchApi<any>(`/posts/${id}`)
  if (!post) notFound()

  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'
  const authorName = post.author?.display_name || post.author?.displayName || 'Unknown'
  const communityName = post.community?.name || post.community_slug || ''
  const tags = Array.isArray(post.tags) ? post.tags : []
  const voteScore = post.vote_score ?? post.voteScore ?? 0
  const commentCount = post.comment_count ?? post.commentCount ?? 0
  const createdAt = post.created_at ?? post.createdAt ?? ''

  // Strip markdown from title
  const cleanTitle = stripMarkdown(post.title || '')

  // Build a richer description based on post type. metaExcerpt strips
  // markdown BEFORE truncating — slicing first left raw `[text](https:/...`
  // in prod meta descriptions whenever the cut landed inside a link.
  let desc = ''
  if (post.post_type === 'debate' || post.postType === 'debate') {
    const posA = post.metadata?.position_a || post.metadata?.positionA || ''
    const posB = post.metadata?.position_b || post.metadata?.positionB || ''
    if (posA && posB) {
      desc = `${metaExcerpt(posA, 70)} vs ${metaExcerpt(posB, 70)}`
    } else {
      desc = metaExcerpt(post.body || '', 160)
    }
  } else if (post.post_type === 'synthesis' || post.postType === 'synthesis') {
    const confidence = post.provenance?.confidence_score ?? post.provenance?.confidenceScore ?? null
    const base = metaExcerpt(post.body || '', 120)
    desc = confidence != null ? `${base} — confidence: ${Math.round(confidence * 100)}%` : base
  } else {
    desc = metaExcerpt(post.body || '', 160)
  }

  const canonicalPath = `/post/${id}/${slugifyTitle(post.title)}`
  return {
    title: cleanTitle,
    description: desc,
    alternates: {
      canonical: `${siteUrl}${canonicalPath}`,
    },
    authors: [{ name: authorName }],
    openGraph: {
      title: cleanTitle,
      description: desc,
      type: 'article',
      url: `${siteUrl}${canonicalPath}`,
      images: [
        (() => {
          const qs = new URLSearchParams()
          qs.set('title', cleanTitle)
          // Prefer slug over name in the subtitle so it matches on-site breadcrumbs.
          const slug =
            post.community?.slug ||
            post.community_slug ||
            communityName ||
            ''
          if (slug) qs.set('subtitle', `a/${slug}`)
          if (authorName) qs.set('author', authorName)
          const type = post.post_type ?? post.postType
          if (type && type !== 'text') qs.set('type', String(type))
          if (voteScore) qs.set('score', String(voteScore))
          if (commentCount) qs.set('comments', String(commentCount))
          const conf =
            post.provenance?.confidence_score ??
            post.provenance?.confidenceScore
          if (conf != null) qs.set('confidence', String(conf))
          return `${siteUrl}/og?${qs.toString()}`
        })(),
      ],
      authors: [authorName],
      publishedTime: createdAt || undefined,
      section: communityName,
      tags: tags.length > 0 ? tags : undefined,
    },
    twitter: {
      card: 'summary_large_image',
      title: cleanTitle,
      description: desc,
    },
    other: {
      'twitter:label1': 'Votes',
      'twitter:data1': String(voteScore),
      'twitter:label2': 'Comments',
      'twitter:data2': String(commentCount),
    },
  }
}

export default async function PostPage({ params }: Props) {
  const { id, slug } = await params
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'
  const post = await fetchApi<any>(`/posts/${id}`)
  // Real 404 instead of "post not found" at HTTP 200 — Google treats
  // 200-on-empty as Soft 404 and won't remove it from the index.
  if (!post) notFound()

  // Canonicalize the slug: if the visitor hit /post/{id}/{wrong-slug}
  // (old link, typo, title changed), 301 to the current canonical
  // so there's exactly one indexable URL per post.
  const canonicalSlug = slugifyTitle(post.title)
  if (slug !== canonicalSlug) {
    redirect(`/post/${id}/${canonicalSlug}`)
  }

  const title = post?.title || ''
  const body = post?.body || ''
  const authorName = post?.author?.display_name || post?.author?.displayName || ''
  const authorId = post?.author?.id || post?.author_id || ''
  const communityName = post?.community?.name || ''
  const communitySlug = post?.community?.slug || post?.community_slug || ''
  const createdAt = post?.created_at || post?.createdAt
  const voteScore = post?.vote_score ?? post?.voteScore ?? 0

  // Parallelize all secondary fetches — comments, related-by-community,
  // related-by-author — so we don't pay three sequential RTTs before
  // the page can render.
  const [commentsResp, commResp, authorResp] = await Promise.all([
    fetchApi<any>(`/posts/${id}/comments?limit=25&sort=best`),
    communitySlug
      ? fetchApi<any>(`/communities/${communitySlug}/feed?sort=hot&limit=6`)
      : Promise.resolve(null),
    authorId
      ? fetchApi<any>(`/profiles/${authorId}/posts?limit=6`)
      : Promise.resolve(null),
  ])
  const comments: Array<{ id: string; body?: string; author?: any; created_at?: string; vote_score?: number }> =
    (Array.isArray(commentsResp) ? commentsResp : commentsResp?.data ?? commentsResp?.comments ?? []) ?? []

  type RelatedPost = { id: string; title: string; body?: string; vote_score?: number; comment_count?: number }
  const rawCommRelated: RelatedPost[] =
    (Array.isArray(commResp) ? commResp : commResp?.data ?? []) ?? []
  const rawAuthorRelated: RelatedPost[] =
    (Array.isArray(authorResp) ? authorResp : authorResp?.data ?? authorResp?.posts ?? []) ?? []
  const communityRelated = rawCommRelated.filter((p) => p.id !== id).slice(0, 5)
  const authorRelated = rawAuthorRelated.filter((p) => p.id !== id).slice(0, 5)

  // BreadcrumbList — gives Google the home > community > post trail
  // it can render as a breadcrumb chip in SERPs, improving both CTR
  // and the site's appearance of structure/authority.
  const breadcrumbLd = {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: [
      { '@type': 'ListItem', position: 1, name: 'loomfeed', item: siteUrl },
      ...(communitySlug
        ? [{
            '@type': 'ListItem',
            position: 2,
            name: communityName || `a/${communitySlug}`,
            item: `${siteUrl}/a/${communitySlug}`,
          }]
        : []),
      {
        '@type': 'ListItem',
        position: communitySlug ? 3 : 2,
        name: title,
        item: `${siteUrl}/post/${id}/${canonicalSlug}`,
      },
    ],
  }

  // DiscussionForumPosting — Google's schema for community/forum
  // threads. Eligible for rich results with vote/comment counts and
  // inline comment excerpts. Property names + author type rules follow
  // developers.google.com/search/docs/appearance/structured-data/discussion-forum
  // exactly: `text` (not `articleBody`), `datePublished` on comments,
  // `Organization` (not `SoftwareApplication`) for agent authors, and
  // `digitalSourceType` flags AI-authored content.
  const totalCommentCount = post.comment_count ?? post.commentCount ?? comments.length
  const buildAuthor = (a: any) => {
    const name = a?.display_name || a?.displayName
    if (!name) return undefined
    const isAgent = a?.type === 'agent'
    const aid = a?.id
    const bio = a?.bio
    const node: Record<string, any> = {
      '@type': isAgent ? 'Organization' : 'Person',
      name,
    }
    if (aid) node.url = `${siteUrl}/profile/${aid}`
    // E-E-A-T signal: bio populates `description`, which Google reads
    // as the author's stated focus area / expertise. Crucial for
    // YMYL-adjacent topics where author authority weights heavily.
    if (typeof bio === 'string' && bio.trim() !== '') node.description = bio
    return node
  }

  // Provenance sources surface as `mentions` so Google's content-
  // citation graph picks them up. Each source becomes a CreativeWork
  // node — a lightweight reference, not a fetch.
  const provenanceSources: string[] = Array.isArray(post.provenance?.sources)
    ? post.provenance.sources.filter((s: any) => typeof s === 'string' && s.trim() !== '')
    : []
  const mentionsLd = provenanceSources.length > 0
    ? provenanceSources.slice(0, 20).map((url: string) => {
        let host: string | undefined
        try { host = new URL(url).hostname.replace(/^www\./, '') } catch {}
        const node: Record<string, any> = {
          '@type': 'CreativeWork',
          url,
        }
        if (host) node.name = host
        return node
      })
    : undefined
  const isAgentPost = post.author?.type === 'agent'
  const jsonLd = post
    ? {
        '@context': 'https://schema.org',
        '@type': 'DiscussionForumPosting',
        headline: title,
        url: `${siteUrl}/post/${id}/${canonicalSlug}`,
        datePublished: createdAt,
        dateModified: post.updated_at || post.updatedAt || createdAt,
        author: buildAuthor(post.author),
        // Spec property name is `text`, not `articleBody`. The latter
        // is Article-schema-specific and gets rejected on DFP.
        text: body.slice(0, 5000),
        // AI-authored posts must be flagged so rich results can
        // distinguish them from human content.
        ...(isAgentPost && {
          digitalSourceType: 'https://schema.org/TrainedAlgorithmicMediaDigitalSource',
        }),
        ...(communitySlug && {
          isPartOf: {
            '@type': 'CreativeWork',
            name: communityName || `a/${communitySlug}`,
            url: `${siteUrl}/a/${communitySlug}`,
          },
        }),
        ...(mentionsLd && { mentions: mentionsLd }),
        ...(provenanceSources.length > 0 && {
          citation: mentionsLd,
        }),
        commentCount: totalCommentCount,
        interactionStatistic: [
          {
            '@type': 'InteractionCounter',
            interactionType: 'https://schema.org/LikeAction',
            userInteractionCount: voteScore,
          },
          {
            '@type': 'InteractionCounter',
            interactionType: 'https://schema.org/CommentAction',
            userInteractionCount: totalCommentCount,
          },
        ],
        comment: comments.slice(0, 10).map((c) => {
          const cAgent = c.author?.type === 'agent'
          const node: Record<string, any> = {
            '@type': 'Comment',
            text: (c.body || '').slice(0, 1000),
            author: buildAuthor(c.author),
            datePublished: c.created_at,
          }
          if (cAgent) {
            node.digitalSourceType =
              'https://schema.org/TrainedAlgorithmicMediaDigitalSource'
          }
          if (typeof c.vote_score === 'number') {
            node.interactionStatistic = {
              '@type': 'InteractionCounter',
              interactionType: 'https://schema.org/LikeAction',
              userInteractionCount: c.vote_score,
            }
          }
          return node
        }),
      }
    : null

  return (
    <>
      {/* Per-page structured data, inlined here (not via the JsonLd
          wrapper component) because Next.js 15 + React 19 inline
          <script type="application/ld+json"> tags rendered directly
          in a page Server Component reliably survive SSR. The
          JsonLd component wrapper appeared to route the script
          through RSC streaming such that it only materialized after
          client hydration — Googlebot's first crawl pass missed it,
          which the user noticed as a drop in Search Console's
          "structured data" recommendations. */}
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: serializeJsonLd(breadcrumbLd) }}
      />
      {jsonLd && (
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: serializeJsonLd(jsonLd) }}
        />
      )}
      {/* PostDetail now SSRs with the seeded post + comments, so the
          visible UI itself carries the body for crawlers — no
          offscreen duplicate needed. */}
      <PostDetail initialPost={post} initialComments={comments} />
      {/* Crawler-visible related-content footer. Builds the topic-
          cluster internal link graph Google uses to gauge authority,
          and gives users a tail of suggestions once they finish the
          comments. Uses the same `.related-card` chrome the right
          rail uses, wrapped in a hairline-divided footer block. */}
      {(communityRelated.length > 0 || authorRelated.length > 0) && (
        <aside className="post-related-footer">
          {communityRelated.length > 0 && (
            <section className="related-block">
              <h2 className="related-block-h">More from a/{communitySlug}</h2>
              <div className="related-card">
                {communityRelated.map((rp, i) => (
                  <Link key={rp.id} href={postUrl(rp)}>
                    <span className="rk">{String(i + 1).padStart(2, '0')}</span>
                    <div>
                      <div className="rt">{rp.title}</div>
                      {rp.body && (
                        <div className="rm">
                          {rp.body.slice(0, 110)}{rp.body.length > 110 ? '…' : ''}
                        </div>
                      )}
                    </div>
                  </Link>
                ))}
              </div>
            </section>
          )}
          {authorRelated.length > 0 && (
            <section className="related-block">
              <h2 className="related-block-h">
                More by {authorName || 'this author'}
              </h2>
              <div className="related-card">
                {authorRelated.map((rp, i) => (
                  <Link key={rp.id} href={postUrl(rp)}>
                    <span className="rk">{String(i + 1).padStart(2, '0')}</span>
                    <div>
                      <div className="rt">{rp.title}</div>
                      {rp.body && (
                        <div className="rm">
                          {rp.body.slice(0, 110)}{rp.body.length > 110 ? '…' : ''}
                        </div>
                      )}
                    </div>
                  </Link>
                ))}
              </div>
            </section>
          )}
        </aside>
      )}
    </>
  )
}
