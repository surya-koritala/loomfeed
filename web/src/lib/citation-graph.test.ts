import { describe, expect, it } from 'vitest'
import { buildCitationMermaid, type CitationGraph } from './citation-graph'

const graph: CitationGraph = {
  nodes: [
    { id: 'post-root', title: 'Root finding', author: 'Alice', type: 'text', score: 12 },
    { id: 'post-source', title: 'Prior evidence', author: 'ResearchBot', type: 'synthesis', score: 8 },
  ],
  edges: [
    { source: 'post-root', target: 'post-source', type: 'supports' },
  ],
}

describe('buildCitationMermaid', () => {
  it('renders deterministic nodes, typed directed edges, and the root class', () => {
    const chart = buildCitationMermaid(graph, 'post-root')

    expect(chart).toContain('flowchart LR')
    expect(chart).toContain('n0["Root finding<br/>Alice · text · score 12"]')
    expect(chart).toContain('n1["Prior evidence<br/>ResearchBot · synthesis · score 8"]')
    expect(chart).toContain('n0 -->|supports| n1')
    expect(chart).toContain('class n0 root')
    expect(chart).not.toContain('post-root[')
  })

  it('escapes labels and drops edges whose endpoint is not in the graph', () => {
    const chart = buildCitationMermaid({
      nodes: [
        { id: 'root', title: 'A "quoted" <claim>', author: 'A&B', type: 'link', score: 1 },
      ],
      edges: [{ source: 'root', target: 'missing', type: 'contradicts|breakout' }],
    }, 'root')

    expect(chart).toContain('A &quot;quoted&quot; &lt;claim&gt;')
    expect(chart).toContain('A&amp;B')
    expect(chart).not.toContain('breakout')
  })
})
