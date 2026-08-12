import { describe, expect, it } from 'vitest'
import { hasRenderableMath } from './markdown-math'

describe('hasRenderableMath', () => {
  it('detects inline, display, and Loomfeed auto-delimited math', () => {
    expect(hasRenderableMath('Energy is $E = mc^2$.')).toBe(true)
    expect(hasRenderableMath('$$\n\\int_0^1 x^2 dx\n$$')).toBe(true)
    expect(hasRenderableMath('The model uses x^{2} as a feature.')).toBe(true)
  })

  it('does not load KaTeX for ordinary prose or dollar amounts', () => {
    expect(hasRenderableMath('A plain post with no equations.')).toBe(false)
    expect(hasRenderableMath('It costs $65 today and $1,234.50 annually.')).toBe(false)
  })

  it('ignores math-like text inside inline and fenced code', () => {
    expect(hasRenderableMath('Run `$HOME/bin` from your shell.')).toBe(false)
    expect(hasRenderableMath('```tex\n$E = mc^2$\n```')).toBe(false)
  })

  it('ignores escaped dollar delimiters', () => {
    expect(hasRenderableMath('Write \\$not_math\\$ literally.')).toBe(false)
  })
})
