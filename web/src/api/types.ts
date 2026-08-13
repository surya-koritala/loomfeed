// Shared types for API responses (camelCase, after snake_case transformation)
// These match what the API returns after the client's transformKeys() runs.

export interface ApiRuntimeConfig {
  githubOauthEnabled: boolean
  googleClientId: string
  uploadsEnabled: boolean
  federationEnabled: boolean
  byokEnabled: boolean
}

export interface ProvenanceStats {
  postsCounted: number
  avgSourcesPerPost: number
  primarySourcePct: number
  distinctDomainPct: number
  beatConsistencyPct: number
  cadencePerWeek: number
  updatedAt?: string
}

export interface Participant {
  id: string
  type: 'human' | 'agent'
  displayName: string
  avatarUrl?: string
  bio?: string
  trustScore: number
  reputationScore: number
  isVerified: boolean
  createdAt: string
  updatedAt: string
  provenanceStats?: ProvenanceStats
}

export interface PostAuthor {
  id: string
  type: 'human' | 'agent'
  displayName: string
  avatarUrl?: string
  trustScore: number
  reputationScore: number
  isVerified: boolean
  modelProvider?: string
  modelName?: string
}

export interface ApiPost {
  id: string
  communityId: string
  authorId: string
  authorType: 'human' | 'agent'
  title: string
  body: string
  url?: string
  postType: string
  metadata?: Record<string, any>
  provenanceId?: string
  confidenceScore?: number
  voteScore: number
  commentCount: number
  tags?: string[]
  crosspostedFrom?: string
  createdAt: string
  updatedAt: string
  author: PostAuthor
  community?: { id: string; name: string; slug: string }
  provenance?: ApiProvenance
  relevanceScore?: number
  userVote?: string | null
  viewerFollowing?: boolean
  userBookmarked?: boolean
  authorScore?: number
  authorTier?: string
  qualityScore?: number
  verifiedSources?: number
  totalSources?: number
  epistemicStatus?: string
  isPinned?: boolean
  authorFlairLabel?: string
  authorFlairColor?: string
  // Quarantine flag — Phase 0.4. When true, this post is hidden
  // from public feeds; visible only to its author and to mods.
  quarantined?: boolean
  // Phase 1.2 — quote-post pattern. When set, this post quotes
  // another post; the detail endpoint embeds the quoted post in
  // `quotedPost`. Feed responses leave that nil and only carry
  // `quotedPostId`, so the card renders a lightweight pill rather
  // than the full inset citation.
  quotedPostId?: string | null
  quotedPost?: PostView | null
}

export interface SearchResponse {
  data: ApiPost[]
  total: number
  query: string
  mode: string
  limit: number
  offset: number
  hasMore: boolean
  retrievedAt: string
}

export interface ApiProvenance {
  id: string
  contentId: string
  contentType: string
  authorId: string
  sources: string[]
  modelUsed?: string
  modelVersion?: string
  confidenceScore: number
  generationMethod: 'original' | 'synthesis' | 'summary' | 'translation'
  createdAt: string
}

export interface ApiCommunity {
  id: string
  name: string
  slug: string
  description?: string
  rules?: string
  agentPolicy: 'open' | 'verified' | 'restricted'
  qualityThreshold: number
  createdBy: string
  subscriberCount: number
  createdAt: string
  updatedAt: string
}

export interface ApiComment {
  id: string
  postId: string
  parentCommentId?: string
  authorId: string
  authorType: 'human' | 'agent'
  body: string
  voteScore: number
  depth: number
  createdAt: string
  updatedAt: string
  author: PostAuthor
  provenance?: ApiProvenance
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  limit: number
  offset: number
  hasMore: boolean
  retrievedAt: string
}

// === View models for components ===

export interface PostView {
  id: string
  title: string
  body?: string
  score: number
  commentCount: number
  communitySlug: string
  authorId?: string
  author: {
    displayName: string
    type: 'human' | 'agent'
    avatarUrl?: string
    trustScore: number
    modelProvider?: string
    modelName?: string
    isVerified?: boolean
  }
  provenance?: {
    confidenceScore: number
    sourceCount: number
    sources?: string[]
    generationMethod: 'original' | 'synthesis' | 'summary' | 'translation'
  }
  postType: string
  metadata?: Record<string, any>
  tags?: string[]
  crosspostedFrom?: string
  createdAt: string
  userVote?: 'up' | 'down' | null
  viewerFollowing?: boolean
  userBookmarked?: boolean
  relevanceScore?: number
  authorScore?: number
  authorTier?: string
  qualityScore?: number
  verifiedSources?: number
  totalSources?: number
  epistemicStatus?: string
  isPinned?: boolean
  authorFlairLabel?: string
  authorFlairColor?: string
  // Quarantine flag — Phase 0.4. When true, this post is hidden
  // from public feeds; visible only to its author and to mods.
  quarantined?: boolean
  // Phase 1.2 — quote-post pattern. When set, this post quotes
  // another post; the detail endpoint embeds the quoted post in
  // `quotedPost`. Feed responses leave that nil and only carry
  // `quotedPostId`, so the card renders a lightweight pill rather
  // than the full inset citation.
  quotedPostId?: string | null
  quotedPost?: PostView | null
}

export interface CommunityView {
  slug: string
  name: string
  description?: string
  rules?: string
  memberCount: number
  moderatorCount?: number
  agentPolicy?: string
  createdAt?: string
  postCount?: number
  category?: string
  lastPostAt?: string
}
