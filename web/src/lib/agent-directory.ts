export interface AgentDirectoryParams {
  capability?: string
  provider?: string
  sort?: 'trust' | 'posts' | 'newest'
  minTrust?: number
  cursor?: string
  limit?: number
  offset?: number
}

export interface AgentDirectoryEntry {
  id: string
  displayName: string
  avatarUrl?: string
  bio: string
  trustScore: number
  reputationScore: number
  postCount: number
  commentCount: number
  isVerified: boolean
  createdAt: string
  modelProvider: string
  modelName: string
  modelVersion: string
  capabilities: string[]
  protocolType: string
  agentUrl: string
}

export function buildAgentDirectoryQuery(params: AgentDirectoryParams = {}): URLSearchParams {
  const query = new URLSearchParams()
  if (params.capability) query.set('capability', params.capability)
  if (params.provider) query.set('provider', params.provider)
  if (params.sort) query.set('sort', params.sort)
  if (params.minTrust) query.set('min_trust', String(params.minTrust))
  if (params.cursor) query.set('cursor', params.cursor)
  if (params.limit !== undefined) query.set('limit', String(params.limit))
  if (params.offset !== undefined) query.set('offset', String(params.offset))
  return query
}

export function mapAgentDirectoryEntry(raw: any): AgentDirectoryEntry {
  return {
    id: raw.id,
    displayName: raw.displayName ?? raw.display_name ?? 'Unknown agent',
    avatarUrl: (raw.avatarUrl ?? raw.avatar_url) || undefined,
    bio: raw.bio ?? '',
    trustScore: Number(raw.trustScore ?? raw.trust_score ?? 0),
    reputationScore: Number(raw.reputationScore ?? raw.reputation_score ?? 0),
    postCount: Number(raw.postCount ?? raw.post_count ?? 0),
    commentCount: Number(raw.commentCount ?? raw.comment_count ?? 0),
    isVerified: Boolean(raw.isVerified ?? raw.is_verified),
    createdAt: raw.createdAt ?? raw.created_at ?? '',
    modelProvider: raw.modelProvider ?? raw.model_provider ?? '',
    modelName: raw.modelName ?? raw.model_name ?? '',
    modelVersion: raw.modelVersion ?? raw.model_version ?? '',
    capabilities: Array.isArray(raw.capabilities)
      ? raw.capabilities.filter((capability: unknown): capability is string => typeof capability === 'string')
      : [],
    protocolType: raw.protocolType ?? raw.protocol_type ?? '',
    agentUrl: raw.agentUrl ?? raw.agent_url ?? '',
  }
}
