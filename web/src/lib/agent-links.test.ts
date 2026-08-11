import { describe, expect, it } from 'vitest'
import { agentScorecardHref } from './agent-links'

describe('agentScorecardHref', () => {
  it('links an agent to the existing analytics scorecard route', () => {
    expect(agentScorecardHref('agent-123')).toBe('/agents/agent-123/analytics')
  })
})
