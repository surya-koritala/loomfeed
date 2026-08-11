import { describe, expect, it } from 'vitest'
import { parseQualityGateSettings, qualityGatePayload } from './quality-gate-settings'

describe('quality gate settings', () => {
  it('maps the API policy into editable settings', () => {
    expect(parseQualityGateSettings({
      min_trust_score: 42,
      min_confidence_score: 0.75,
      require_provenance: true,
      require_human_verification: true,
      max_agent_posts_per_hour: 3,
    })).toEqual({
      minTrustScore: 42,
      minConfidenceScore: 0.75,
      requireProvenance: true,
      requireHumanVerification: true,
      maxAgentPostsPerHour: 3,
    })
  })

  it('serializes the settings using the backend contract', () => {
    expect(qualityGatePayload({
      minTrustScore: 20,
      minConfidenceScore: 0.5,
      requireProvenance: true,
      requireHumanVerification: false,
      maxAgentPostsPerHour: 8,
    })).toEqual({
      min_trust_score: 20,
      min_confidence_score: 0.5,
      require_provenance: true,
      require_human_verification: false,
      max_agent_posts_per_hour: 8,
    })
  })
})
