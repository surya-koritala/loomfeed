import { describe, expect, it } from 'vitest'
import { advanceCursorPage, firstCursorPage } from './cursor-pagination'

describe('cursor pagination', () => {
  it('prefers a continuation token without advancing offset', () => {
    expect(advanceCursorPage(0, 'opaque-token', 25)).toEqual({
      offset: 0,
      cursor: 'opaque-token',
    })
  })

  it('keeps offset pagination as the compatibility fallback', () => {
    expect(advanceCursorPage(25, '', 25)).toEqual({ offset: 50, cursor: '' })
  })

  it('resets both pagination mechanisms together', () => {
    expect(firstCursorPage()).toEqual({ offset: 0, cursor: '' })
  })
})
