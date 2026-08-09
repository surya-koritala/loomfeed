// Plain-text rendering of post markdown for meta descriptions, og tags,
// and the embed card. Single source of truth — the post page and embed
// page previously each carried their own copy.

export function stripMarkdown(md: string): string {
  if (!md) return ''
  return md
    .replace(/\|[-:| ]+\|/g, '')
    .replace(/^\|(.+)\|$/gm, (_, row: string) =>
      row.split('|').map((c) => c.trim()).filter(Boolean).join(', ')
    )
    .replace(/#{1,6}\s+/g, '')
    .replace(/\*\*(.+?)\*\*/g, '$1')
    .replace(/\*(.+?)\*/g, '$1')
    .replace(/_(.+?)_/g, '$1')
    .replace(/```[\s\S]*?```/g, '')
    .replace(/`(.+?)`/g, '$1')
    // URL part tolerates one level of nested parens — journal links like
    // cell.com/...S1364-6613(25)00003-3 otherwise leave their tail behind.
    .replace(/!\[.*?\]\((?:[^()]|\([^()]*\))*\)/g, '')
    .replace(/\[(.+?)\]\((?:[^()]|\([^()]*\))*\)/g, '$1')
    .replace(/\[!(NOTE|TIP|WARNING|IMPORTANT|CAUTION)\]\s*/g, '')
    .replace(/<details>[\s\S]*?<\/details>/g, '')
    .replace(/>\s+/g, '')
    .replace(/\n/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

// Excerpt of a markdown body for use in <meta name="description"> etc.
// Strip BEFORE slicing — slicing first can cut a markdown link mid-URL,
// leaving raw `[text](https:/...` syntax that the strip regexes no longer
// match (this leaked into prod meta descriptions).
export function metaExcerpt(body: string, max: number): string {
  return stripMarkdown(body || '').slice(0, max).trim()
}
