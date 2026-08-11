export interface CitationNode {
  id: string
  title: string
  author: string
  type: string
  score: number
}

export interface CitationEdge {
  source: string
  target: string
  type: string
}

export interface CitationGraph {
  nodes: CitationNode[]
  edges: CitationEdge[]
}

function escapeMermaidLabel(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/\s+/g, ' ')
    .trim()
}

function truncate(value: string, max = 72): string {
  const normalized = value.replace(/\s+/g, ' ').trim()
  return normalized.length > max ? `${normalized.slice(0, max - 1)}…` : normalized
}

function safeEdgeType(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9 _-]/g, '').trim().slice(0, 24) || 'references'
}

export function buildCitationMermaid(graph: CitationGraph, rootPostId: string): string {
  const nodes = [...(graph.nodes ?? [])].sort((a, b) => {
    if (a.id === rootPostId) return -1
    if (b.id === rootPostId) return 1
    return a.title.localeCompare(b.title) || a.id.localeCompare(b.id)
  })
  const nodeIDs = new Map(nodes.map((node, index) => [node.id, `n${index}`]))
  const lines = ['flowchart LR']

  for (const node of nodes) {
    const id = nodeIDs.get(node.id)!
    const title = escapeMermaidLabel(truncate(node.title || 'Untitled post'))
    const author = escapeMermaidLabel(truncate(node.author || 'Unknown author', 36))
    const type = escapeMermaidLabel(truncate(node.type || 'post', 24))
    lines.push(`  ${id}["${title}<br/>${author} · ${type} · score ${Number(node.score) || 0}"]`)
  }

  const edges = [...(graph.edges ?? [])].sort(
    (a, b) => a.source.localeCompare(b.source) || a.target.localeCompare(b.target) || a.type.localeCompare(b.type),
  )
  for (const edge of edges) {
    const source = nodeIDs.get(edge.source)
    const target = nodeIDs.get(edge.target)
    if (!source || !target) continue
    lines.push(`  ${source} -->|${safeEdgeType(edge.type)}| ${target}`)
  }

  lines.push('  classDef root fill:#d8ff3e,stroke:#111111,stroke-width:3px,color:#111111')
  const rootNode = nodeIDs.get(rootPostId)
  if (rootNode) lines.push(`  class ${rootNode} root`)

  return lines.join('\n')
}
