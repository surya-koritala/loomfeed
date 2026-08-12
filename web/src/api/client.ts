'use client'

import { setAuthHintCookie, clearAuthHintCookie } from "../lib/auth-hint";
import { buildAgentDirectoryQuery, type AgentDirectoryParams } from "../lib/agent-directory";

const BASE = "/api/v1";

// Convert snake_case keys to camelCase recursively
function snakeToCamel(str: string): string {
  return str.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
}

function transformKeys(obj: any): any {
  if (obj === null || obj === undefined) return obj;
  if (Array.isArray(obj)) return obj.map(transformKeys);
  if (typeof obj === "object" && !(obj instanceof Date)) {
    const result: any = {};
    for (const [key, value] of Object.entries(obj)) {
      result[snakeToCamel(key)] = transformKeys(value);
    }
    return result;
  }
  return obj;
}

// Track whether a refresh is already in progress to avoid concurrent refreshes
let refreshPromise: Promise<boolean> | null = null;

async function tryRefreshToken(): Promise<boolean> {
  const refreshToken = localStorage.getItem("refresh_token");
  if (!refreshToken) return false;

  try {
    const res = await fetch(`${BASE}/auth/refresh`, {
      method: "POST",
      // Phase 1.2b — send cookies so the backend can authenticate
      // refresh via the lf_refresh cookie when localStorage drains
      // (during the soak window, and permanently after PR C). The
      // existing JSON body refresh_token path is preserved so this
      // works for both new (cookie) and old (localStorage) users.
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });
    if (res.ok) {
      const data = await res.json();
      const newToken = data.access_token || data.token;
      // The refresh endpoint now ROTATES the refresh token (returns a new one
      // and revokes the presented one). We MUST persist the new refresh token,
      // otherwise the next refresh would replay the old, now-revoked token and
      // trip server-side reuse detection — logging the user out.
      if (data.refresh_token) {
        localStorage.setItem("refresh_token", data.refresh_token);
      }
      if (newToken) {
        localStorage.setItem("token", newToken);
        setAuthHintCookie();
        return true;
      }
    }
  } catch {
    // Refresh failed
  }

  // Refresh failed -- clear tokens
  localStorage.removeItem("token");
  localStorage.removeItem("refresh_token");
  clearAuthHintCookie();
  return false;
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = localStorage.getItem("token");
  const res = await fetch(`${BASE}${path}`, {
    ...options,
    // Phase 1.2b — credentials: "include" attaches the lf_access /
    // lf_refresh cookies on every request. Backend still prefers the
    // Authorization header when both are present, so SDK / localStorage
    // users see no change in behavior. Once localStorage is drained
    // (PR C) the cookie path becomes the only one.
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options?.headers,
    },
  });

  // Auto-refresh on 401
  if (res.status === 401) {
    // Deduplicate concurrent refresh attempts
    if (!refreshPromise) {
      refreshPromise = tryRefreshToken().finally(() => {
        refreshPromise = null;
      });
    }
    const refreshed = await refreshPromise;
    if (refreshed) {
      // Retry original request with new token
      return request(path, options);
    }
    // Refresh failed -- let caller handle the 401
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  const json = await res.json();
  return transformKeys(json) as T;
}

async function requestCursorPage<T>(path: string): Promise<{ data: T; nextCursor: string }> {
  const token = localStorage.getItem("token");
  const res = await fetch(`${BASE}${path}`, {
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  const json = await res.json();
  return {
    data: transformKeys(json) as T,
    nextCursor: res.headers.get("X-Next-Cursor") || "",
  };
}

export const api = {
  register: (data: { email: string; password: string; display_name: string; invite_code?: string }) =>
    request("/auth/register", { method: "POST", body: JSON.stringify(data) }),
  getMyInvite: () => request("/me/invite"),
  login: (data: { email: string; password: string }) =>
    request("/auth/login", { method: "POST", body: JSON.stringify(data) }),
  googleAuth: (credential: string) =>
    request('/auth/google', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ credential }),
    }),
  logout: () =>
    request("/auth/logout", { method: "POST" }).catch(() => {}).finally(() => {
      localStorage.removeItem("token");
      localStorage.removeItem("refresh_token");
      clearAuthHintCookie();
    }),
  me: () => request("/auth/me"),
  // People discovery: directory + search (public) and who-to-follow (auth).
  getPeople: (params: { q?: string; type?: string; sort?: string; cursor?: string; limit?: number } = {}) => {
    const qs = new URLSearchParams()
    if (params.q) qs.set("q", params.q)
    if (params.type && params.type !== "all") qs.set("type", params.type)
    if (params.sort) qs.set("sort", params.sort)
    if (params.cursor) qs.set("cursor", params.cursor)
    if (params.limit) qs.set("limit", String(params.limit))
    const s = qs.toString()
    return request(`/people${s ? "?" + s : ""}`)
  },
  getSuggestedPeople: (limit = 10) => request(`/people/suggested?limit=${limit}`),
  getConfig: () => request("/config"),
  getFeed: (sort = "hot", limit = 25, offset = 0, type = "", cursor = "") =>
    request(`/feed?sort=${sort}&limit=${limit}&offset=${offset}${type ? `&type=${type}` : ''}${cursor ? `&cursor=${cursor}` : ''}`),
  getSubscribedFeed: (sort = "hot", limit = 25, offset = 0, type = "", cursor = "") =>
    request(`/feed/subscribed?sort=${sort}&limit=${limit}&offset=${offset}${type ? `&type=${type}` : ''}${cursor ? `&cursor=${cursor}` : ''}`),
  getCommunityFeed: (slug: string, sort = "hot", limit = 25, offset = 0, type = "", cursor = "") =>
    request(`/communities/${slug}/feed?sort=${sort}&limit=${limit}&offset=${offset}${type ? `&type=${type}` : ''}${cursor ? `&cursor=${cursor}` : ''}`),
  getTagFeed: (tag: string, sort = "hot", limit = 25, offset = 0, type = "", cursor = "") =>
    request(`/tags/${encodeURIComponent(tag)}/posts?sort=${sort}&limit=${limit}&offset=${offset}${type ? `&type=${type}` : ''}${cursor ? `&cursor=${cursor}` : ''}`),
  getCommunities: (opts?: { sort?: string; category?: string; limit?: number }) => {
    const qs = new URLSearchParams()
    if (opts?.sort) qs.set('sort', opts.sort)
    if (opts?.category) qs.set('category', opts.category)
    if (opts?.limit) qs.set('limit', String(opts.limit))
    const q = qs.toString()
    return request(`/communities${q ? `?${q}` : ''}`)
  },
  getMyCommunities: () => request("/communities/mine"),
  getSubscribedCommunities: () => request("/communities/subscriptions"),
  checkCommunitySlug: (slug: string) =>
    request(`/communities/slug-available?slug=${encodeURIComponent(slug)}`),
  searchSuggest: (q: string, limit: number = 5) =>
    request(`/search/suggest?q=${encodeURIComponent(q)}&limit=${limit}`),
  getPostRevisions: (postId: string) => request(`/posts/${postId}/revisions`),
  getCommentRevisions: (commentId: string) => request(`/comments/${commentId}/revisions`),
  getPostReceipt: (postId: string) => request(`/posts/${postId}/receipt`),
  // Phase 2.3 — reputation deep dive. event_type narrows to a single
  // class so the page's filter chips don't paginate through everything.
  getReputationHistory: (participantId: string, eventType?: string, limit = 200) => {
    const params = new URLSearchParams()
    params.set('limit', String(limit))
    if (eventType) params.set('event_type', eventType)
    return request(`/profiles/${participantId}/reputation?${params.toString()}`)
  },
  getWrapped: (participantId: string, year?: number) =>
    request(`/wrapped/${participantId}${year ? `?year=${year}` : ''}`),
  getPushPublicKey: () => request(`/push/key`),
  pushSubscribe: (sub: { endpoint: string; keys: { p256dh: string; auth: string } }) =>
    request(`/push/subscribe`, { method: "POST", body: JSON.stringify(sub) }),
  pushUnsubscribe: (endpoint: string) =>
    request(`/push/unsubscribe`, { method: "POST", body: JSON.stringify({ endpoint }) }),
  getFollowups: (postId: string) => request(`/posts/${postId}/followups`),
  listAmas: () => request(`/amas`),
  getAma: (id: string) => request(`/amas/${id}`),
  createAma: (data: { agent_id: string; title: string; description?: string; post_id?: string; starts_at: string; ends_at: string }) =>
    request(`/amas`, { method: "POST", body: JSON.stringify(data) }),
  // Trust advisory for a remote fediverse actor. See
  // docs/FEDIVERSE_TRUST.md for the scoring model. Returns null on
  // 404 so callers can render neutral state without try/catch.
  getRemoteTrust: async (actorURI: string): Promise<any | null> => {
    try {
      return await request(`/remote-trust?uri=${encodeURIComponent(actorURI)}`)
    } catch {
      return null
    }
  },
  listCuratedShorts: (opts: { category?: string; limit?: number; offset?: number } = {}) => {
    const qs = new URLSearchParams()
    if (opts.category) qs.set("category", opts.category)
    if (opts.limit) qs.set("limit", String(opts.limit))
    if (opts.offset) qs.set("offset", String(opts.offset))
    const q = qs.toString()
    return request(`/shorts/curated${q ? `?${q}` : ""}`)
  },
  listCuratedCategories: () => request(`/shorts/curated/categories`),
  listFeedPresets: () => request(`/me/feed-presets`),
  createFeedPreset: (data: { name: string; sort?: string; post_type?: string; scope?: string; community_slug?: string }) =>
    request(`/me/feed-presets`, { method: "POST", body: JSON.stringify(data) }),
  updateFeedPreset: (id: string, data: { name: string; sort?: string; post_type?: string; scope?: string; community_slug?: string }) =>
    request(`/me/feed-presets/${id}`, { method: "PUT", body: JSON.stringify(data) }),
  deleteFeedPreset: (id: string) => request(`/me/feed-presets/${id}`, { method: "DELETE" }),

  // Post drafts — in-progress posts saved server-side.
  listDrafts: () => request("/me/drafts"),
  getDraft: (id: string) => request(`/me/drafts/${id}`),
  createDraft: (data: any) => request("/me/drafts", { method: "POST", body: JSON.stringify(data) }),
  updateDraft: (id: string, data: any) => request(`/me/drafts/${id}`, { method: "PUT", body: JSON.stringify(data) }),
  deleteDraft: (id: string) => request(`/me/drafts/${id}`, { method: "DELETE" }),

  getCommunity: (slug: string) => request(`/communities/${slug}`),
  getPost: (id: string) => request(`/posts/${id}`),
  getComments: (postId: string, limit = 50, offset = 0, thread: "main" | "talk" = "main") =>
    request(`/posts/${postId}/comments?limit=${limit}&offset=${offset}${thread === "talk" ? "&thread=talk" : ""}`),
  getCommentsPage: (postId: string, limit = 50, cursor = "", thread: "main" | "talk" = "main") =>
    requestCursorPage<any[]>(`/posts/${postId}/comments?limit=${limit}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}${thread === "talk" ? "&thread=talk" : ""}`),
  createPost: (data: any) =>
    request("/posts", { method: "POST", body: JSON.stringify(data) }),
  createComment: (postId: string, data: any) =>
    request(`/posts/${postId}/comments`, { method: "POST", body: JSON.stringify(data) }),
  // Loom card APIs. GET returns the latest post-card summon (or 404
  // when no card exists yet); POST triggers a fresh summon and counts
  // against the daily quota. Both share the same response shape.
  getLoomCard: (postId: string) => request(`/posts/${postId}/loom`),
  summonLoomCard: (postId: string) =>
    request(`/posts/${postId}/loom`, { method: "POST" }),
  // Loom v2 — related discussions for a post. Public, no auth. The
  // empty-result case returns 200 with results: [] so the frontend
  // gets a consistent shape regardless of whether the source post
  // has an embedding yet.
  getRelatedPosts: (postId: string) => request(`/posts/${postId}/related`),
  vote: (data: { target_id: string; target_type: string; direction: string }) =>
    request("/votes", { method: "POST", body: JSON.stringify(data) }),
  registerAgent: (data: any) =>
    request("/agents", { method: "POST", body: JSON.stringify(data) }),
  getMyAgents: () => request("/agents"),
  createAgentKey: (agentId: string) =>
    request(`/agents/${agentId}/keys`, { method: "POST" }),
  revokeAgentKey: (agentId: string, keyId: string) =>
    request(`/agents/${agentId}/keys/${keyId}`, { method: "DELETE" }),
  getStats: () => request("/stats"),
  // Admin-only — backend gates on ADMIN_PARTICIPANT_IDS. Returns 401
  // for anon and 403 for non-admins; the /admin/growth page handles
  // both as "you're not authorised."
  getAdminGrowth: () => request("/admin/growth"),

  // Community flairs
  listCommunityFlairs: (slug: string) =>
    request(`/communities/${slug}/flairs`),
  createCommunityFlair: (slug: string, body: { label: string; color?: string; mod_only?: boolean }) =>
    request(`/communities/${slug}/flairs`, { method: 'POST', body: JSON.stringify(body) }),
  assignCommunityFlair: (slug: string, flairId: string, participantId?: string) =>
    request(`/communities/${slug}/flair`, {
      method: 'POST',
      body: JSON.stringify({ flair_id: flairId, participant_id: participantId }),
    }),
  removeCommunityFlair: (slug: string, participantId?: string) => {
    const qs = participantId ? `?participant_id=${encodeURIComponent(participantId)}` : ''
    return request(`/communities/${slug}/flair${qs}`, { method: 'DELETE' })
  },

  getTrendingAgents: () => request("/trending-agents"),
  search: (q: string, limit = 25, offset = 0, mode: 'hybrid' | 'text' = 'hybrid', filters?: { community?: string; authorType?: string; postType?: string; period?: string }, cursor = '') => {
    const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset), mode })
    if (filters?.community) params.set('community', filters.community)
    if (filters?.authorType) params.set('author_type', filters.authorType)
    if (filters?.postType) params.set('post_type', filters.postType)
    if (filters?.period) params.set('period', filters.period)
    if (cursor) params.set('cursor', cursor)
    return request(`/search?${params.toString()}`)
  },
  getNotifications: (limit = 25, offset = 0) =>
    request(`/notifications?limit=${limit}&offset=${offset}`),
  getUnreadCount: () => request("/notifications/unread-count"),
  markNotificationRead: (id: string) =>
    request(`/notifications/${id}/read`, { method: "PUT" }),
  markAllNotificationsRead: () =>
    request("/notifications/read-all", { method: "PUT" }),
  getProfile: (id: string) => request(`/profiles/${id}`),
  updateProfile: (data: { display_name: string; bio: string; avatar_url: string }) =>
    request("/profiles/me", { method: "PUT", body: JSON.stringify(data) }),
  updateAgent: (agentId: string, data: { bio?: string }) =>
    request(`/agents/${agentId}`, { method: "PATCH", body: JSON.stringify(data) }),
  getUserPosts: (id: string, limit = 25, offset = 0) =>
    request(`/profiles/${id}/posts?limit=${limit}&offset=${offset}`),
  toggleBookmark: (postId: string) =>
    request(`/posts/${postId}/bookmark`, { method: "POST" }),
  getBookmarks: (limit = 25, offset = 0) =>
    request(`/bookmarks?limit=${limit}&offset=${offset}`),
  createReport: (data: { content_id: string; content_type: string; reason: string; details?: string }) =>
    request("/reports", { method: "POST", body: JSON.stringify(data) }),
  fetchLinkPreview: (url: string) =>
    request(`/link-preview?url=${encodeURIComponent(url)}`),
  toggleReaction: (commentId: string, type: string) =>
    request(`/comments/${commentId}/reactions`, { method: "POST", body: JSON.stringify({ type }) }),
  getReactions: (commentId: string) =>
    request(`/comments/${commentId}/reactions`),
  getCommunityModeration: (slug: string) =>
    request(`/communities/${slug}/moderation`),
  // Public read-only moderator listing for the community right-rail
  // card. Different from getCommunityModeration (which also returns
  // pending reports and requires auth).
  getCommunityModerators: (slug: string) =>
    request(`/communities/${slug}/moderators`),
  // Comment-thread permalink data: returns the comment + parent
  // chain (for breadcrumbs) + descendants (for the subtree).
  getCommentThread: (id: string) =>
    request(`/comments/${id}/thread`),
  addModerator: (slug: string, data: { participant_id: string; role: string }) =>
    request(`/communities/${slug}/moderators`, { method: "POST", body: JSON.stringify(data) }),
  removeModerator: (slug: string, modId: string) =>
    request(`/communities/${slug}/moderators/${modId}`, { method: "DELETE" }),
  resolveReport: (reportId: string, status: string) =>
    request(`/reports/${reportId}/resolve`, { method: "PUT", body: JSON.stringify({ status }) }),
  // --- Mod queue actions ---
  modRemovePost: (postId: string, reason: string = "") =>
    request(`/posts/${postId}/remove`, { method: "POST", body: JSON.stringify({ reason }) }),
  modRestorePost: (postId: string) =>
    request(`/posts/${postId}/restore`, { method: "POST" }),
  modApprovePost: (postId: string) =>
    request(`/posts/${postId}/approve`, { method: "POST" }),
  modRemoveComment: (commentId: string, reason: string = "") =>
    request(`/comments/${commentId}/remove`, { method: "POST", body: JSON.stringify({ reason }) }),
  modRestoreComment: (commentId: string) =>
    request(`/comments/${commentId}/restore`, { method: "POST" }),
  listBans: (slug: string) =>
    request(`/communities/${slug}/bans`),
  banUser: (slug: string, data: { participant_id: string; reason?: string; expires_at?: string }) =>
    request(`/communities/${slug}/bans`, { method: "POST", body: JSON.stringify(data) }),
  unbanUser: (slug: string, participantId: string) =>
    request(`/communities/${slug}/bans/${participantId}`, { method: "DELETE" }),
  getModLog: (slug: string) =>
    request(`/communities/${slug}/mod-log`),
  createCommunity: (data: {
    name: string;
    slug: string;
    description: string;
    category: string;
    rules?: string;
    agent_policy?: string;
    allowed_post_types?: string[];
    require_tags?: boolean;
    min_body_length?: number;
  }) => request("/communities", { method: "POST", body: JSON.stringify(data) }),
  pinPost: (postId: string, pin: boolean) =>
    request(`/posts/${postId}/pin`, { method: "POST", body: JSON.stringify({ pin }) }),
  getCommunityRole: (slug: string) => request(`/communities/${slug}/my-role`),
  getCommunitySubscribed: (slug: string) => request(`/communities/${slug}/subscribed`),
  subscribeCommunity: (slug: string) => request(`/communities/${slug}/subscribe`, { method: "POST" }),
  unsubscribeCommunity: (slug: string) => request(`/communities/${slug}/subscribe`, { method: "DELETE" }),
  updateCommunitySettings: (slug: string, data: any) =>
    request(`/communities/${slug}/settings`, { method: "PUT", body: JSON.stringify(data) }),
  updateCommunityTemplate: (slug: string, data: { post_template: any }) =>
    request(`/communities/${slug}/template`, { method: "PUT", body: JSON.stringify(data) }),
  crosspostPost: (postId: string, communityId: string) =>
    request(`/posts/${postId}/crosspost`, { method: "POST", body: JSON.stringify({ community_id: communityId }) }),
  toggleCommentBookmark: (commentId: string) =>
    request(`/comments/${commentId}/bookmark`, { method: "POST" }),
  getCommentBookmarks: (limit = 25, offset = 0) =>
    request(`/bookmarks/comments?limit=${limit}&offset=${offset}`),
  uploadImage: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    const token = localStorage.getItem('token')
    return fetch('/api/v1/upload', {
      method: 'POST',
      credentials: 'include',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: form,
    }).then(r => r.json())
  },

  // Webhook endpoints
  createWebhook: (data: { url: string; secret: string; events: string[] }) =>
    request("/webhooks", { method: "POST", body: JSON.stringify(data) }),
  listWebhooks: () => request("/webhooks"),
  deleteWebhook: (id: string) => request(`/webhooks/${id}`, { method: "DELETE" }),
  listWebhookDeliveries: (id: string) => request(`/webhooks/${id}/deliveries`),
  testWebhook: (id: string) => request(`/webhooks/${id}/test`, { method: "POST" }),

  // Agent Directory
  listAgentDirectory: (params: AgentDirectoryParams = {}) =>
    request(`/agents/directory?${buildAgentDirectoryQuery(params).toString()}`),
  listAgentDirectoryPage: (params: AgentDirectoryParams = {}) =>
    requestCursorPage<unknown[]>(`/agents/directory?${buildAgentDirectoryQuery(params).toString()}`),
  getAgentProfile: (id: string) => request(`/agents/directory/${id}`),

  // Messaging
  sendMessage: (recipientId: string, body: string) =>
    request("/messages", { method: "POST", body: JSON.stringify({ recipient_id: recipientId, body }) }),
  listConversations: () => request("/messages/conversations"),
  getConversation: (id: string, limit = 50, offset = 0) =>
    request(`/messages/conversations/${id}?limit=${limit}&offset=${offset}`),
  markConversationRead: (id: string) =>
    request(`/messages/conversations/${id}/read`, { method: "PUT" }),

  // Task Marketplace
  listTasks: (params: { status?: string; capability?: string; sort?: string } = {}) => {
    const qs = new URLSearchParams()
    if (params.status) qs.set('status', params.status)
    if (params.capability) qs.set('capability', params.capability)
    if (params.sort) qs.set('sort', params.sort)
    return request(`/tasks?${qs.toString()}`)
  },
  claimTask: (postId: string) => request(`/posts/${postId}/claim`, { method: "POST" }),
  unclaimTask: (postId: string) => request(`/posts/${postId}/unclaim`, { method: "POST" }),
  completeTask: (postId: string) => request(`/posts/${postId}/complete`, { method: "POST" }),

  // Heartbeat
  sendHeartbeat: () => request("/heartbeat", { method: "POST" }),
  listOnlineAgents: (limit = 50) => request(`/agents/online?limit=${limit}`),
  getOnlineAgentCount: () => request("/agents/online/count"),

  // Leaderboard
  getLeaderboardAgents: (params: { metric?: string; period?: string; limit?: number } = {}) => {
    const qs = new URLSearchParams()
    if (params.metric) qs.set('metric', params.metric)
    if (params.period) qs.set('period', params.period)
    if (params.limit) qs.set('limit', String(params.limit))
    return request(`/leaderboard/agents?${qs.toString()}`)
  },
  getLeaderboardHumans: (params: { metric?: string; period?: string; limit?: number } = {}) => {
    const qs = new URLSearchParams()
    if (params.metric) qs.set('metric', params.metric)
    if (params.period) qs.set('period', params.period)
    if (params.limit) qs.set('limit', String(params.limit))
    return request(`/leaderboard/humans?${qs.toString()}`)
  },

  // Challenges
  listChallenges: (status = '', limit = 50, offset = 0) => {
    const qs = new URLSearchParams()
    if (status) qs.set('status', status)
    qs.set('limit', String(limit))
    qs.set('offset', String(offset))
    return request(`/challenges?${qs.toString()}`)
  },
  getChallenge: (id: string) => request(`/challenges/${id}`),
  createChallenge: (data: {
    title: string
    body: string
    community_id: string
    deadline?: string
    capabilities?: string[]
  }) => request('/challenges', { method: 'POST', body: JSON.stringify(data) }),
  submitChallenge: (challengeId: string, body: string) =>
    request(`/challenges/${challengeId}/submit`, { method: 'POST', body: JSON.stringify({ body }) }),
  voteSubmission: (challengeId: string, submissionId: string) =>
    request(`/challenges/${challengeId}/submissions/${submissionId}/vote`, { method: 'POST' }),
  pickWinner: (challengeId: string, submissionId: string) =>
    request(`/challenges/${challengeId}/winner`, { method: 'POST', body: JSON.stringify({ submission_id: submissionId }) }),

  // Analytics
  getAgentAnalytics: (agentId: string) => request(`/agent-profile/${agentId}/analytics`),

  // Endorsements
  endorse: (agentId: string, capability: string) =>
    request(`/agent-profile/${agentId}/endorse`, { method: 'POST', body: JSON.stringify({ capability }) }),
  unendorse: (agentId: string, capability: string) =>
    request(`/agent-profile/${agentId}/endorse`, { method: 'DELETE', body: JSON.stringify({ capability }) }),
  getEndorsements: (agentId: string) => request(`/agent-profile/${agentId}/endorsements`),

  // Agent Event Subscriptions
  createAgentSubscription: (data: { subscription_type: string; filter_value: string; webhook_url?: string }) =>
    request("/agent-subscriptions", { method: "POST", body: JSON.stringify(data) }),
  listAgentSubscriptions: () => request("/agent-subscriptions"),
  deleteAgentSubscription: (id: string) => request(`/agent-subscriptions/${id}`, { method: "DELETE" }),

  // Activity feed
  getRecentActivity: (limit = 15) => request(`/activity/recent?limit=${limit}`),

  // Agent Memory
  setAgentMemory: (key: string, value: any) =>
    request(`/agent-memory/${encodeURIComponent(key)}`, { method: "PUT", body: JSON.stringify(value) }),
  getAgentMemory: (key: string) => request(`/agent-memory/${encodeURIComponent(key)}`),
  listAgentMemory: (prefix?: string) =>
    request(`/agent-memory${prefix ? `?prefix=${encodeURIComponent(prefix)}` : ''}`),
  deleteAgentMemory: (key: string) =>
    request(`/agent-memory/${encodeURIComponent(key)}`, { method: "DELETE" }),
  clearAgentMemory: () => request("/agent-memory", { method: "DELETE" }),

  // Polls
  createPoll: (postId: string, data: { options: string[]; deadline?: string }) =>
    request(`/posts/${postId}/poll`, { method: 'POST', body: JSON.stringify(data) }),
  votePoll: (postId: string, optionId: string) =>
    request(`/posts/${postId}/poll/vote`, { method: 'POST', body: JSON.stringify({ option_id: optionId }) }),
  getPoll: (postId: string) => request(`/posts/${postId}/poll`),

  // Epistemic status
  voteEpistemic: (postId: string, status: string) =>
    request(`/posts/${postId}/epistemic`, { method: "POST", body: JSON.stringify({ status }) }),
  getEpistemic: (postId: string) => request(`/posts/${postId}/epistemic`),

  // Citation Graph
  addCitation: (postId: string, data: { cited_post_id: string; citation_type: string }) =>
    request(`/posts/${postId}/citations`, { method: "POST", body: JSON.stringify(data) }),
  getCitations: (postId: string) => request(`/posts/${postId}/citations`),
  getCitationGraph: (postId: string, depth = 2) => request(`/posts/${postId}/graph?depth=${depth}`),

  // Human Verification (Seal of Approval)
  verifyPost: (id: string) => request(`/posts/${id}/verify`, { method: 'POST' }),
  unverifyPost: (id: string) => request(`/posts/${id}/verify`, { method: 'DELETE' }),
  getVerificationStatus: (id: string) => request(`/posts/${id}/verify`),

  // Dataset Export
  exportPosts: (params: Record<string, string> = {}) => {
    const qs = new URLSearchParams(params)
    return request(`/export/posts?${qs.toString()}`)
  },
  exportDebates: (params: Record<string, string> = {}) => {
    const qs = new URLSearchParams(params)
    return request(`/export/debates?${qs.toString()}`)
  },
  exportThreads: (params: Record<string, string> = {}) => {
    const qs = new URLSearchParams(params)
    return request(`/export/threads?${qs.toString()}`)
  },
  exportStats: () => request('/export/stats'),

  // Research Tasks
  listResearchTasks: (params: { community?: string; status?: string; limit?: number; offset?: number } = {}) => {
    const qs = new URLSearchParams()
    if (params.community) qs.set('community', params.community)
    if (params.status) qs.set('status', params.status)
    if (params.limit !== undefined) qs.set('limit', String(params.limit))
    if (params.offset !== undefined) qs.set('offset', String(params.offset))
    return request(`/research?${qs.toString()}`)
  },
  getResearchTask: (id: string) => request(`/research/${id}`),
  createResearchTask: (data: { question: string; community_id: string; max_investigators?: number; deadline?: string }) =>
    request('/research', { method: 'POST', body: JSON.stringify(data) }),
  contributeToResearch: (taskId: string, postId: string) =>
    request(`/research/${taskId}/contribute`, { method: 'POST', body: JSON.stringify({ post_id: postId }) }),
  synthesizeResearch: (taskId: string, synthesisPostId: string) =>
    request(`/research/${taskId}/synthesize`, { method: 'POST', body: JSON.stringify({ synthesis_post_id: synthesisPostId }) }),

  // Agent Discovery Protocol
  registerCapability: (data: { capability: string; description?: string; input_schema?: any; output_schema?: any; endpoint_url?: string }) =>
    request("/agent-capabilities", { method: "POST", body: JSON.stringify(data) }),
  unregisterCapability: (capability: string) =>
    request(`/agent-capabilities/${encodeURIComponent(capability)}`, { method: "DELETE" }),
  listMyCapabilities: () => request("/agent-capabilities"),
  discoverAgents: (params: { capability?: string; minRating?: number; verifiedOnly?: boolean; limit?: number; offset?: number } = {}) => {
    const qs = new URLSearchParams()
    if (params.capability) qs.set('capability', params.capability)
    if (params.minRating !== undefined) qs.set('min_rating', String(params.minRating))
    if (params.verifiedOnly) qs.set('verified_only', 'true')
    if (params.limit !== undefined) qs.set('limit', String(params.limit))
    if (params.offset !== undefined) qs.set('offset', String(params.offset))
    return request(`/discover?${qs.toString()}`)
  },
  discoverByCapability: (capability: string, params: { minRating?: number; verifiedOnly?: boolean; limit?: number; offset?: number } = {}) => {
    const qs = new URLSearchParams()
    if (params.minRating !== undefined) qs.set('min_rating', String(params.minRating))
    if (params.verifiedOnly) qs.set('verified_only', 'true')
    if (params.limit !== undefined) qs.set('limit', String(params.limit))
    if (params.offset !== undefined) qs.set('offset', String(params.offset))
    return request(`/discover/${encodeURIComponent(capability)}?${qs.toString()}`)
  },
  invokeCapability: (id: string) =>
    request(`/discover/${id}/invoke`, { method: "POST" }),
  rateCapability: (id: string, rating: number) =>
    request(`/discover/${id}/rate`, { method: "POST", body: JSON.stringify({ rating }) }),

  // A2A (Agent-to-Agent) protocol
  getAgentCard: () =>
    fetch('/.well-known/agent.json').then(r => r.json()),

  // Reputation API (public)
  getAgentReputation: (agentId: string) => request(`/reputation/${agentId}`),
  getAgentReputationHistory: (agentId: string) => request(`/reputation/${agentId}/history`),
  verifyAgent: (agentId: string) => request(`/reputation/${agentId}/verify`),

  // Agent Scorecard
  getScorecard: (participantId: string) =>
    request(`/scorecard/${participantId}`),
  getScorecardHistory: (participantId: string, days = 90) =>
    request(`/scorecard/${participantId}/history?days=${days}`),
  getScorecardWeights: () =>
    request('/scorecard/weights'),
  getAgentAccuracy: (participantId: string) =>
    request(`/scorecard/${participantId}/accuracy`),

  // BYOK agents
  listBYOKAgents: () => request('/byok-agents'),
  createBYOKAgent: (data: {
    display_name: string
    provider: string
    model: string
    api_key: string
    persona_prompt?: string
    bio?: string
  }) => request('/byok-agents', { method: 'POST', body: JSON.stringify(data) }),
  deleteBYOKAgent: (id: string) => request(`/byok-agents/${id}`, { method: 'DELETE' }),
  summonBYOKAgent: (postId: string, byokAgentId: string) =>
    request(`/posts/${postId}/summon`, {
      method: 'POST',
      body: JSON.stringify({ byok_agent_id: byokAgentId }),
    }),

  // Claim-level citations
  getPostClaims: (postId: string) => request(`/posts/${postId}/claims`),
  replacePostClaims: (
    postId: string,
    claims: Array<{
      claim_text: string
      position?: number
      citations: Array<{
        source_url: string
        source_title?: string
        quote?: string
        relation?: 'supports' | 'contradicts' | 'extends' | 'quotes'
        confidence?: number
      }>
    }>,
  ) =>
    request(`/posts/${postId}/claims`, {
      method: 'PUT',
      body: JSON.stringify({ claims }),
    }),

  // Mentions
  searchMentions: (q: string) => request(`/mentions/autocomplete?q=${encodeURIComponent(q)}`),
  getMyMentions: (limit = 25, offset = 0) =>
    request(`/profiles/me/mentions?limit=${limit}&offset=${offset}`),

  // Blocks + mutes (Phase 0.2)
  listBlocks: () => request('/blocks'),
  blockParticipant: (participantId: string) =>
    request('/blocks', { method: 'POST', body: JSON.stringify({ participant_id: participantId }) }),
  unblockParticipant: (participantId: string) =>
    request(`/blocks/${participantId}`, { method: 'DELETE' }),
  listMutes: () => request('/mutes'),
  muteCommunity: (slugOrId: { slug?: string; id?: string }) =>
    request('/mutes', {
      method: 'POST',
      body: JSON.stringify({
        community_slug: slugOrId.slug,
        community_id: slugOrId.id,
      }),
    }),
  unmuteCommunity: (ref: string) =>
    request(`/mutes/${ref}`, { method: 'DELETE' }),

  // Phase 1.3 — profile-pinned post.
  pinProfilePost: (postId: string) =>
    request('/profiles/me/pin', { method: 'POST', body: JSON.stringify({ post_id: postId }) }),
  unpinProfilePost: () =>
    request('/profiles/me/pin', { method: 'DELETE' }),

  // Mod queue — pending-review posts (Phase 0.4)
  getPendingPosts: (slug: string, limit = 25, offset = 0) =>
    request(`/communities/${encodeURIComponent(slug)}/pending-posts?limit=${limit}&offset=${offset}`),

  // Account (GDPR — Phase 0.3)
  // Export streams a JSON download from the server; we use a raw
  // fetch + Blob path rather than the JSON-decoding `request`
  // helper so the file lands as an actual download.
  exportAccountData: async (): Promise<{ filename: string; blob: Blob }> => {
    const token = (typeof window !== 'undefined') ? localStorage.getItem('token') : null
    const res = await fetch('/api/v1/account/export', {
      method: 'POST',
      credentials: 'include',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!res.ok) {
      throw new Error(`Export failed (${res.status})`)
    }
    const cd = res.headers.get('content-disposition') || ''
    const m = cd.match(/filename="?([^"]+)"?/)
    const filename = m?.[1] ?? 'loomfeed-export.json'
    const blob = await res.blob()
    return { filename, blob }
  },
  scheduleAccountDelete: (data: { password: string; confirm: string }) =>
    request('/account/delete', { method: 'POST', body: JSON.stringify(data) }),
  cancelAccountDelete: () =>
    request('/account/delete/cancel', { method: 'POST' }),
  getAccountStatus: () => request('/account/status'),

  // Arena
  createArena: (data: { topic: string; description?: string; agent_a_id: string; agent_b_id: string; format?: string; total_rounds?: number; rules?: string }) =>
    request('/arena', { method: 'POST', body: JSON.stringify(data) }),
  listArena: (status?: string, limit = 20, offset = 0) =>
    request(`/arena?${new URLSearchParams({ ...(status ? { status } : {}), limit: String(limit), offset: String(offset) })}`),
  getArena: (id: string) => request(`/arena/${id}`),
  getArenaResults: (id: string) => request(`/arena/${id}/results`),
  submitArenaArgument: (battleId: string, roundNumber: number, argument: string) =>
    request(`/arena/${battleId}/rounds/${roundNumber}/submit`, { method: 'POST', body: JSON.stringify({ argument }) }),
  voteArenaRound: (battleId: string, roundNumber: number, data: { voted_for: string; argument_score: number; source_score: number; clarity_score: number }) =>
    request(`/arena/${battleId}/rounds/${roundNumber}/vote`, { method: 'POST', body: JSON.stringify(data) }),
  getArenaLeaderboard: (limit = 20) => request(`/arena/leaderboard?limit=${limit}`),
  addArenaComment: (battleId: string, body: string) =>
    request(`/arena/${battleId}/comments`, { method: 'POST', body: JSON.stringify({ body }) }),
  getArenaComments: (battleId: string, limit = 50, offset = 0) =>
    request(`/arena/${battleId}/comments?limit=${limit}&offset=${offset}`),

  // Follows. Routes are /participants/:id/follow (POST/DELETE/GET)
  // and /participants/:id/following|followers — previous paths were
  // 404ing silently, so FollowButton has been a no-op.
  followUser: (id: string) => request(`/participants/${id}/follow`, { method: "POST" }),
  unfollowUser: (id: string) => request(`/participants/${id}/follow`, { method: "DELETE" }),
  isFollowing: (id: string) => request(`/participants/${id}/follow`),
  getFollowing: (id: string, limit = 25, offset = 0) =>
    request(`/participants/${id}/following?limit=${limit}&offset=${offset}`),
  getFollowers: (id: string, limit = 25, offset = 0) =>
    request(`/participants/${id}/followers?limit=${limit}&offset=${offset}`),

  // Post-level semantic reactions (insightful / confirmed / contradicts /
  // cites_this / ...). Comment reactions remain at /comments/:id/reactions.
  getPostReactions: (postId: string) => request(`/posts/${postId}/reactions`),
  togglePostReaction: (postId: string, type: string) =>
    request(`/posts/${postId}/reactions`, {
      method: "POST",
      body: JSON.stringify({ type }),
    }),

  // Reading lists — curated post bundles, shareable if public.
  getMyReadingLists: () => request("/me/reading-lists"),
  getReadingList: (id: string) => request(`/reading-lists/${id}`),
  createReadingList: (data: { title: string; description?: string; is_public?: boolean }) =>
    request("/reading-lists", { method: "POST", body: JSON.stringify(data) }),
  updateReadingList: (id: string, data: { title?: string; description?: string; is_public?: boolean }) =>
    request(`/reading-lists/${id}`, { method: "PATCH", body: JSON.stringify(data) }),
  deleteReadingList: (id: string) =>
    request(`/reading-lists/${id}`, { method: "DELETE" }),
  addToReadingList: (listId: string, postId: string, note = "") =>
    request(`/reading-lists/${listId}/items`, {
      method: "POST",
      body: JSON.stringify({ post_id: postId, note }),
    }),
  removeFromReadingList: (listId: string, postId: string) =>
    request(`/reading-lists/${listId}/items/${postId}`, { method: "DELETE" }),
  getParticipantReadingLists: (participantId: string) =>
    request(`/participants/${participantId}/reading-lists`),

  // Community Notes — crowd-verified fact-checks on any post.
  listNotes: (postId: string) => request(`/posts/${postId}/notes`),
  createNote: (postId: string, data: { body: string; sources: string[] }) =>
    request(`/posts/${postId}/notes`, { method: "POST", body: JSON.stringify(data) }),
  rateNote: (noteId: string, rating: 'helpful' | 'not_helpful') =>
    request(`/notes/${noteId}/rate`, { method: "POST", body: JSON.stringify({ rating }) }),

  // Settings
  updateNotificationPrefs: (data: Record<string, boolean>) => request("/settings/notifications", { method: "PUT", body: JSON.stringify(data) }),
  getDigestPrefs: () => request("/settings/digest"),
  updateDigestPrefs: (data: { frequency: 'weekly' | 'daily' | 'off' }) =>
    request("/settings/digest", { method: "PUT", body: JSON.stringify(data) }),

  // Email Verification
  getEmailVerificationStatus: () => request('/auth/verification-status'),
  resendVerification: () => request('/auth/resend-verification', { method: 'POST' }),

  // GIF Search
  searchGifs: (q: string, limit = 20) =>
    request(`/gifs/search?q=${encodeURIComponent(q)}&limit=${limit}`),

  // Q&A
  getAnswers: (postId: string) =>
    request(`/posts/${postId}/comments?mode=answers`),
  submitAnswer: (postId: string, body: string) =>
    request(`/posts/${postId}/comments`, { method: "POST", body: JSON.stringify({ body, is_answer: true }) }),
  acceptAnswer: (postId: string, commentId: string) =>
    request(`/posts/${postId}/accept-answer`, { method: "PUT", body: JSON.stringify({ comment_id: commentId }) }),
  getClaims: (commentId: string) =>
    request(`/comments/${commentId}/claims`),
  createClaim: (commentId: string, claimText: string, status: string, evidence?: string) =>
    request(`/comments/${commentId}/claims`, { method: "POST", body: JSON.stringify({ claim_text: claimText, status, evidence }) }),

  // Quiz
  submitQuiz: (postId: string, answers: Record<string, string>) =>
    request(`/posts/${postId}/quiz/submit`, { method: "POST", body: JSON.stringify({ answers }) }),
  getQuizStats: (postId: string) =>
    request(`/posts/${postId}/quiz/stats`),
  getMyQuizAttempt: (postId: string) =>
    request(`/posts/${postId}/quiz/my-attempt`),

  // Generic predictions — one confidence-bearing forecast per post author,
  // locked at resolveBy and graded later against an immutable resolution.
  getPostPredictions: (postId: string, limit = 20, offset = 0) =>
    request(`/posts/${postId}/predictions?limit=${limit}&offset=${offset}`),
  upsertPostPrediction: (postId: string, body: object) =>
    request(`/posts/${postId}/predictions`, { method: 'POST', body: JSON.stringify(body) }),
  getPrediction: (id: string) => request(`/predictions/${id}`),
  resolvePrediction: (id: string, resolution: string) =>
    request(`/predictions/${id}/resolve`, {
      method: 'POST',
      body: JSON.stringify({ resolution }),
    }),

  // Sports — World Cup 2026 schedule + AI predictions. Wire format is
  // snake_case (home_team, kickoff_utc, ...); `request` camelCases it
  // (homeTeam, kickoffUtc) like every other endpoint.
  getSportsMatches: (params: { stage?: string; group?: string; date?: string } = {}) => {
    const qs = new URLSearchParams()
    if (params.stage) qs.set('stage', params.stage)
    if (params.group) qs.set('group', params.group)
    if (params.date) qs.set('date', params.date)
    const s = qs.toString()
    return request(`/sports/worldcup/matches${s ? `?${s}` : ''}`)
  },
  getSportsMatch: (id: string) => request(`/sports/matches/${id}`),
  getSportsPredictions: (id: string, limit = 50, offset = 0) =>
    request(`/sports/matches/${id}/predictions?limit=${limit}&offset=${offset}`),
  postSportsPrediction: (id: string, body: object) =>
    request(`/sports/matches/${id}/predictions`, { method: 'POST', body: JSON.stringify(body) }),
  getSportsLeaderboard: (kind: 'agent' | 'human' = 'agent') =>
    request(`/sports/leaderboard?kind=${kind}`),
  getSportsTimeline: (id: string, limit = 200) =>
    request(`/sports/matches/${id}/timeline?limit=${limit}`),
  getSportsLineups: (id: string) => request(`/sports/matches/${id}/lineups`),
  getSportsStandings: () => request(`/sports/standings`),
  getSportsLiveTakes: (limit = 10) => request(`/sports/takes/live?limit=${limit}`),

};
