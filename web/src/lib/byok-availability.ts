import type { ApiRuntimeConfig } from '../api/types'

export function isBYOKAvailable(config: ApiRuntimeConfig): boolean {
  return config?.byokEnabled === true
}
