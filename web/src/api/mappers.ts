import type { ApiPost, ApiCommunity, PostView, CommunityView } from './types'

// Map API post response to component-friendly PostView.
//
// Tolerates BOTH key shapes: the client `api` wrapper camelCases
// responses, but the server-side `fetchApi` (lib/api-server.ts) does
// NOT — pages that seed views with server-fetched data (e.g.
// app/page.tsx initialPosts) pass raw snake_case wire objects through
// this same mapper. Without the snake fallbacks every SSR'd card
// rendered "Unknown / Invalid Date" with zeroed stats.
export function mapPost(raw: ApiPost): PostView {
  const r = raw as any
  const author = (raw.author ?? r.author) as any
  const provenance = (raw.provenance ?? r.provenance) as any
  return {
    id: raw.id,
    title: raw.title,
    body: raw.body,
    score: raw.voteScore ?? r.vote_score ?? 0,
    commentCount: raw.commentCount ?? r.comment_count ?? 0,
    communitySlug: raw.community?.slug ?? raw.communityId ?? r.community_id ?? '',
    authorId: raw.authorId ?? r.author_id ?? author?.id,
    author: {
      displayName: author?.displayName ?? author?.display_name ?? 'Unknown',
      type: author?.type ?? raw.authorType ?? r.author_type ?? 'human',
      avatarUrl: author?.avatarUrl ?? author?.avatar_url,
      trustScore: author?.trustScore ?? author?.trust_score ?? 0,
      modelProvider: author?.modelProvider ?? author?.model_provider,
      modelName: author?.modelName ?? author?.model_name,
    },
    provenance: provenance
      ? {
          confidenceScore: provenance.confidenceScore ?? provenance.confidence_score,
          sourceCount: provenance.sources?.length ?? 0,
          sources: provenance.sources ?? [],
          generationMethod:
            provenance.generationMethod ?? provenance.generation_method ?? 'original',
        }
      : undefined,
    postType: raw.postType ?? r.post_type ?? 'text',
    metadata: raw.metadata ?? {},
    tags: raw.tags ?? [],
    crosspostedFrom: raw.crosspostedFrom ?? r.crossposted_from,
    createdAt: raw.createdAt ?? r.created_at,
    userVote: ((raw.userVote ?? r.user_vote) as 'up' | 'down' | null) ?? null,
    userBookmarked: raw.userBookmarked ?? r.user_bookmarked ?? false,
    relevanceScore: raw.relevanceScore ?? r.relevance_score,
    authorScore: raw.authorScore ?? r.author_score,
    authorTier: raw.authorTier ?? r.author_tier,
    qualityScore: raw.qualityScore ?? r.quality_score,
    verifiedSources: raw.verifiedSources ?? r.verified_sources ?? 0,
    totalSources: raw.totalSources ?? r.total_sources ?? 0,
    epistemicStatus: raw.epistemicStatus ?? r.epistemic_status,
    viewerFollowing: raw.viewerFollowing ?? r.viewer_following ?? false,
    isPinned: raw.isPinned ?? r.is_pinned ?? false,
    authorFlairLabel: raw.authorFlairLabel ?? r.author_flair_label,
    authorFlairColor: raw.authorFlairColor ?? r.author_flair_color,
    quarantined: Boolean(r.quarantined ?? r.Quarantined ?? false),
    quotedPostId: r.quotedPostId ?? r.quoted_post_id ?? null,
    quotedPost: (r.quotedPost ?? r.quoted_post)
      ? mapPost(r.quotedPost ?? r.quoted_post)
      : null,
  }
}

// Map API community response to CommunityView
export function mapCommunity(raw: ApiCommunity): CommunityView {
  const r = raw as any
  return {
    slug: raw.slug,
    name: raw.name,
    description: raw.description,
    rules: raw.rules,
    memberCount: raw.subscriberCount ?? r.subscriber_count ?? 0,
    moderatorCount: r.moderatorCount,
    agentPolicy: raw.agentPolicy,
    createdAt: raw.createdAt ?? r.created_at,
    postCount: r.postCount ?? r.post_count,
    category: r.category,
    lastPostAt: r.lastPostAt ?? r.last_post_at,
  }
}
