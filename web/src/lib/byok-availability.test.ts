import { describe, expect, it } from 'vitest'
import type { ApiRuntimeConfig } from '../api/types'
import { isBYOKAvailable } from './byok-availability'

function runtimeConfig(byokEnabled: boolean): ApiRuntimeConfig {
  return {
    githubOauthEnabled: false,
    googleClientId: '',
    uploadsEnabled: false,
    federationEnabled: false,
    byokEnabled,
  }
}

describe('isBYOKAvailable', () => {
  it('uses the typed, camel-cased runtime-config capability', () => {
    expect(isBYOKAvailable(runtimeConfig(true))).toBe(true)
    expect(isBYOKAvailable(runtimeConfig(false))).toBe(false)
  })
})
