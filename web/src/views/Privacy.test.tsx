import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import Privacy from './Privacy'

describe('Privacy policy', () => {
  it('describes the authentication and presence storage used by the browser', () => {
    const html = renderToStaticMarkup(createElement(Privacy))

    expect(html).toContain('lf_access')
    expect(html).toContain('lf_refresh')
    expect(html).toContain('oauth_state_github')
    expect(html).toContain('lf_authed')
    expect(html).toContain('15 minutes')
    expect(html).toContain('7 days')
    expect(html).toContain('10 minutes')
    expect(html).toContain('30 days')
    expect(html).toContain('localStorage')
    expect(html).toContain('HttpOnly')
  })

  it.each([
    {
      label: 'disabled',
      integrations: { clarity: false, googleAds: false, adsense: false },
      expectedStatus: 'not enabled on this deployment',
    },
    {
      label: 'enabled',
      integrations: { clarity: true, googleAds: true, adsense: true },
      expectedStatus: 'enabled on this deployment',
    },
  ])('renders the optional processor status when integrations are $label', ({ integrations, expectedStatus }) => {
    const html = renderToStaticMarkup(createElement(Privacy, { integrations }))

    expect(html).toContain(`Microsoft Clarity — ${expectedStatus}`)
    expect(html).toContain(`Google Ads conversion tracking — ${expectedStatus}`)
    expect(html).toContain(`Google AdSense — ${expectedStatus}`)
    expect(html).toContain('NEXT_PUBLIC_CLARITY_PROJECT_ID')
    expect(html).toContain('NEXT_PUBLIC_GOOGLE_ADS_ID')
    expect(html).toContain('NEXT_PUBLIC_ADSENSE_CLIENT')
    expect(html).toContain('self-hosted by the web application')
    expect(html).not.toContain('load fonts (Inter) from Google Fonts')
  })

  it('marks deployment-specific policy facts that self-hosters must review', () => {
    const html = renderToStaticMarkup(createElement(Privacy, {
      integrations: { clarity: true, googleAds: true, adsense: true },
    }))

    expect(html).toContain('operator-maintained privacy notice')
    expect(html).toContain('hosting providers and regions')
    expect(html).toContain('retention periods')
    expect(html).toContain('contact address')
    expect(html).toContain('consent and opt-out controls')
    expect(html).not.toContain('US Central region')
    expect(html).not.toContain('We do not share personal information for cross-context behavioral advertising')
  })
})
