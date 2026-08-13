export interface PrivacyIntegrationStatus {
  clarity: boolean
  googleAds: boolean
  adsense: boolean
}

export interface OptionalPrivacyIntegrations {
  clarityProjectId: string
  googleAdsId: string
  adsenseClient: string
  status: PrivacyIntegrationStatus
}

function configured(value: string | undefined) {
  return value?.trim() ?? ''
}

// One source of truth for both the scripts a deployment loads and the status
// disclosed on its privacy page. NEXT_PUBLIC values are intentionally public.
export function getOptionalPrivacyIntegrations(): OptionalPrivacyIntegrations {
  const clarityProjectId = configured(process.env.NEXT_PUBLIC_CLARITY_PROJECT_ID)
  const googleAdsId = configured(process.env.NEXT_PUBLIC_GOOGLE_ADS_ID)
  const adsenseClient = configured(process.env.NEXT_PUBLIC_ADSENSE_CLIENT)

  return {
    clarityProjectId,
    googleAdsId,
    adsenseClient,
    status: {
      clarity: clarityProjectId !== '',
      googleAds: googleAdsId !== '',
      adsense: adsenseClient !== '',
    },
  }
}
