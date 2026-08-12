// KaTeX is one of the larger MarkdownContent dependencies. Keep this detector
// dependency-free so ordinary feed cards can decide whether to request the
// renderer without pulling any math code into their initial chunk.

function withoutCode(content: string): string {
  return content
    .replace(/```[\s\S]*?```/g, '')
    .replace(/~~~[\s\S]*?~~~/g, '')
    .replace(/`+[^`\n]*`+/g, '')
}

export function hasRenderableMath(content: string): boolean {
  const prose = withoutCode(content)

  // Match Markdown math delimiters while respecting escaped dollars. Dollar
  // amounts are intentionally not math: MarkdownContent escapes the same
  // $<digit> shape before remark-math sees it.
  const withoutCurrency = prose.replace(/(?<!\\)\$\d[\d,]*(?:\.\d+)?/g, '')
  const hasDisplayMath = /(^|[^\\])\$\$[\s\S]*?\S[\s\S]*?\$\$/.test(withoutCurrency)
  const hasInlineMath = /(^|[^\\$])\$(?![\s$])(?:\\.|[^$\n\\])+\$/.test(withoutCurrency)

  // preprocessMath() wraps this existing Loomfeed shorthand in $...$ at
  // render time, so it must trigger the same lazy asset path.
  const hasAutoDelimitedMath = /[A-Za-z0-9][A-Za-z0-9\s=+\-*/().,]*[_^]\{[^}\n]+\}/.test(prose)

  return hasDisplayMath || hasInlineMath || hasAutoDelimitedMath
}
