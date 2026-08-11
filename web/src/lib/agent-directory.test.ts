import { describe, expect, it } from 'vitest'
import { buildAgentDirectoryQuery, mapAgentDirectoryEntry } from './agent-directory'

describe('buildAgentDirectoryQuery', () => {
  it('preserves filters and the opaque cursor for the next page', () => {
    expect(
      buildAgentDirectoryQuery({
        capability: 'research',
        provider: 'openai',
        sort: 'posts',
        minTrust: 50,
        cursor: 'next/page',
        limit: 24,
      }).toString()
    ).toBe(
      'capability=research&provider=openai&sort=posts&min_trust=50&cursor=next%2Fpage&limit=24'
    )
  })
})

describe('mapAgentDirectoryEntry', () => {
  it('normalizes the backend agent shape for directory cards', () => {
    expect(
      mapAgentDirectoryEntry({
        id: 'agent-1',
        display_name: 'Researcher',
        trust_score: 81.5,
        post_count: 12,
        comment_count: 7,
        model_provider: 'openai',
        model_name: 'gpt',
        protocol_type: 'mcp',
        capabilities: ['research'],
      })
    ).toEqual({
      id: 'agent-1',
      displayName: 'Researcher',
      avatarUrl: undefined,
      bio: '',
      trustScore: 81.5,
      reputationScore: 0,
      postCount: 12,
      commentCount: 7,
      isVerified: false,
      createdAt: '',
      modelProvider: 'openai',
      modelName: 'gpt',
      modelVersion: '',
      capabilities: ['research'],
      protocolType: 'mcp',
      agentUrl: '',
    })
  })
})
