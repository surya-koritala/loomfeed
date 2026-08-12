export interface QualityGateSettings {
  minTrustScore: number
  minConfidenceScore: number
  requireProvenance: boolean
  requireHumanVerification: boolean
  maxAgentPostsPerHour: number
}

export const defaultQualityGateSettings: QualityGateSettings = {
  minTrustScore: 0,
  minConfidenceScore: 0,
  requireProvenance: false,
  requireHumanVerification: false,
  maxAgentPostsPerHour: 0,
}

export function parseQualityGateSettings(value: any): QualityGateSettings {
  if (!value || typeof value !== 'object') return { ...defaultQualityGateSettings }
  return {
    minTrustScore: numberValue(value.min_trust_score ?? value.minTrustScore),
    minConfidenceScore: numberValue(value.min_confidence_score ?? value.minConfidenceScore),
    requireProvenance: Boolean(value.require_provenance ?? value.requireProvenance),
    requireHumanVerification: Boolean(
      value.require_human_verification ?? value.requireHumanVerification
    ),
    maxAgentPostsPerHour: numberValue(
      value.max_agent_posts_per_hour ?? value.maxAgentPostsPerHour
    ),
  }
}

export function qualityGatePayload(settings: QualityGateSettings) {
  return {
    min_trust_score: settings.minTrustScore,
    min_confidence_score: settings.minConfidenceScore,
    require_provenance: settings.requireProvenance,
    require_human_verification: settings.requireHumanVerification,
    max_agent_posts_per_hour: settings.maxAgentPostsPerHour,
  }
}

function numberValue(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}
