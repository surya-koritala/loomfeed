import { afterEach, describe, expect, it, vi } from 'vitest'
import { getOptionalPrivacyIntegrations } from './privacy-integrations'

describe('optional privacy integrations', () => {
  afterEach(() => vi.unstubAllEnvs())

  it('reports every integration disabled when public configuration is empty', () => {
    vi.stubEnv('NEXT_PUBLIC_CLARITY_PROJECT_ID', '')
    vi.stubEnv('NEXT_PUBLIC_GOOGLE_ADS_ID', '  ')
    vi.stubEnv('NEXT_PUBLIC_ADSENSE_CLIENT', '')

    expect(getOptionalPrivacyIntegrations()).toEqual({
      clarityProjectId: '',
      googleAdsId: '',
      adsenseClient: '',
      status: { clarity: false, googleAds: false, adsense: false },
    })
  })

  it('reports the integrations enabled from the same values used to load their scripts', () => {
    vi.stubEnv('NEXT_PUBLIC_CLARITY_PROJECT_ID', ' clarity-project ')
    vi.stubEnv('NEXT_PUBLIC_GOOGLE_ADS_ID', ' AW-123 ')
    vi.stubEnv('NEXT_PUBLIC_ADSENSE_CLIENT', ' ca-pub-456 ')

    expect(getOptionalPrivacyIntegrations()).toEqual({
      clarityProjectId: 'clarity-project',
      googleAdsId: 'AW-123',
      adsenseClient: 'ca-pub-456',
      status: { clarity: true, googleAds: true, adsense: true },
    })
  })
})
