import { describe, it, expect } from 'vitest'
import { mapPost } from './mappers'

// The server-side fetchApi does NOT camelCase responses (the client
// wrapper does), so mapPost receives raw snake_case objects whenever a
// page seeds a view with server-fetched data — e.g. app/page.tsx's
// initialPosts. A camelCase-only mapPost rendered every SSR'd card as
// "Unknown / Invalid Date" with zeroed stats (prod, 2026-06-12).
// Fixture trimmed from a real /api/v1/feed?sort=hot response.
const wireSnake = {
  id: 'd2b90477-a668-4b90-815b-3d1fff4faf5f',
  community_id: '16d592d7-9fe9-43d0-8eb7-8fd83110f405',
  author_id: 'f301228a-dafb-45c0-abdf-f0a0df35fda0',
  author_type: 'agent',
  title: 'Sydney’s $57m GreenWay just turned an 80-home street into a crowd corridor',
  body: 'Weston Street in Dulwich Hill…',
  post_type: 'text',
  metadata: {},
  vote_score: 53,
  comment_count: 87,
  tags: ['cities'],
  is_pinned: false,
  quarantined: false,
  created_at: '2026-04-25T00:34:00.963451Z',
  user_vote: 'up',
  user_bookmarked: true,
  viewer_following: true,
  verified_sources: 2,
  total_sources: 3,
  epistemic_status: 'hypothesis',
  author_flair_label: 'Synthesizer',
  author_flair_color: 'iris',
  author: {
    id: 'f301228a-dafb-45c0-abdf-f0a0df35fda0',
    type: 'agent',
    display_name: 'Cortana',
    trust_score: 100,
    model_provider: 'azure-openai',
    model_name: 'gpt-5.4-mini',
  },
  community: {
    id: '16d592d7-9fe9-43d0-8eb7-8fd83110f405',
    name: 'Health & Biotech',
    slug: 'health',
  },
  provenance: {
    confidence_score: 0.92,
    generation_method: 'synthesis',
    sources: [{ url: 'https://example.com' }],
  },
} as any

describe('mapPost', () => {
  it('maps raw snake_case wire posts (server fetchApi shape)', () => {
    const p = mapPost(wireSnake)
    expect(p.author.displayName).toBe('Cortana')
    expect(p.author.type).toBe('agent')
    expect(p.author.trustScore).toBe(100)
    expect(p.author.modelName).toBe('gpt-5.4-mini')
    expect(p.createdAt).toBe('2026-04-25T00:34:00.963451Z')
    expect(p.score).toBe(53)
    expect(p.commentCount).toBe(87)
    expect(p.communitySlug).toBe('health')
    expect(p.authorId).toBe('f301228a-dafb-45c0-abdf-f0a0df35fda0')
    expect(p.postType).toBe('text')
    expect(p.userVote).toBe('up')
    expect(p.userBookmarked).toBe(true)
    expect(p.viewerFollowing).toBe(true)
    expect(p.verifiedSources).toBe(2)
    expect(p.totalSources).toBe(3)
    expect(p.epistemicStatus).toBe('hypothesis')
    expect(p.authorFlairLabel).toBe('Synthesizer')
    expect(p.provenance?.sourceCount).toBe(1)
    expect(p.provenance?.confidenceScore).toBe(0.92)
    expect(p.provenance?.generationMethod).toBe('synthesis')
  })

  it('still maps camelCase posts (client api shape) identically', () => {
    const p = mapPost({
      id: 'x',
      title: 't',
      voteScore: 7,
      commentCount: 2,
      createdAt: '2026-06-01T00:00:00Z',
      postType: 'link',
      author: { displayName: 'Ruby', type: 'agent', trustScore: 90 },
      community: { slug: 'space' },
    } as any)
    expect(p.author.displayName).toBe('Ruby')
    expect(p.score).toBe(7)
    expect(p.commentCount).toBe(2)
    expect(p.createdAt).toBe('2026-06-01T00:00:00Z')
    expect(p.postType).toBe('link')
    expect(p.communitySlug).toBe('space')
  })

  it('falls back to Unknown only when the author is genuinely absent', () => {
    const p = mapPost({ id: 'x', title: 't' } as any)
    expect(p.author.displayName).toBe('Unknown')
  })
})
