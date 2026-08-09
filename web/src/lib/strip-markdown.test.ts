import { describe, it, expect } from 'vitest'
import { stripMarkdown, metaExcerpt } from './strip-markdown'

describe('stripMarkdown', () => {
  it('unwraps links, bold, and inline code', () => {
    expect(stripMarkdown('See [the paper](https://example.com) for **bold** `code`.')).toBe(
      'See the paper for bold code.'
    )
  })

  it('handles empty input', () => {
    expect(stripMarkdown('')).toBe('')
  })

  // Regression: journal URLs contain parentheses (e.g. cell.com PII ids
  // like S1364-6613(25)00003-3). A non-greedy `\(.+?\)` stops at the first
  // `)` inside the URL, leaving its tail ("00003-3)") in the output —
  // observed live in prod meta descriptions after #167.
  it('unwraps links whose URL contains parentheses', () => {
    expect(
      stripMarkdown(
        'research in [Trends in Cognitive Sciences](https://www.cell.com/trends/cognitive-sciences/fulltext/S1364-6613(25)00003-3) — as we read'
      )
    ).toBe('research in Trends in Cognitive Sciences — as we read')
  })
})

describe('metaExcerpt', () => {
  // Regression: prod meta descriptions showed raw markdown like
  // "research in [Trends in Cognitive Sciences ...](https:/" because the
  // body was sliced to 160 chars BEFORE stripping — the cut landed inside
  // the link's URL, so the [text](url) pattern no longer matched.
  it('never leaks markdown link syntax when the cut lands inside a link', () => {
    const body =
      'A genuinely unsettling finding: research in ' +
      '[Trends in Cognitive Sciences argues LLMs are homogenizing human expression and thought]' +
      '(https://www.cell.com/trends/cognitive-sciences/fulltext/S1364-6613) — as we read and ' +
      'write more AI-shaped text, our own language converges.'
    const out = metaExcerpt(body, 160)
    expect(out).not.toContain('](')
    expect(out).not.toContain('[')
    expect(out).toContain('Trends in Cognitive Sciences')
    expect(out.length).toBeLessThanOrEqual(160)
  })

  it('keeps short plain text unchanged', () => {
    expect(metaExcerpt('Just a plain sentence.', 160)).toBe('Just a plain sentence.')
  })

  // Card excerpts route through the shared stripper too (LFPostCard) — the
  // same paren-URL artifact ("thought00003-3)") was visible in feed excerpts.
  it('produces clean card excerpts from paren-URL bodies', () => {
    const body =
      'research argues LLMs are homogenizing human expression and thought ' +
      '([Trends in Cognitive Sciences](https://www.cell.com/trends/fulltext/S1364-6613(25)00003-3)) — as we read and write more AI-shaped text our language converges toward the model default, and the effect compounds with exposure over time.'
    const out = metaExcerpt(body, 240)
    expect(out).not.toContain('](')
    expect(out).not.toContain('00003-3)')
    expect(out).toContain('Trends in Cognitive Sciences')
  })
})
