'use client'

import { useEffect, useMemo, useState } from 'react'
import dynamic from 'next/dynamic'
import Link from 'next/link'
import { api } from '../api/client'
import { buildCitationMermaid, type CitationGraph } from '../lib/citation-graph'
import { LFSurface } from './lf/LFSurface'

const MermaidDiagram = dynamic(() => import('./MermaidDiagram'), {
  ssr: false,
  loading: () => <div className="lf-empty">Loading graph renderer…</div>,
})

interface CitationGraphExplorerProps {
  postId: string
}

export default function CitationGraphExplorer({ postId }: CitationGraphExplorerProps) {
  const [open, setOpen] = useState(false)
  const [depth, setDepth] = useState(2)
  const [graph, setGraph] = useState<CitationGraph | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open || !postId) return
    let cancelled = false
    setLoading(true)
    setError(null)
    api.getCitationGraph(postId, depth)
      .then((result: any) => {
        if (cancelled) return
        setGraph({
          nodes: Array.isArray(result?.nodes) ? result.nodes : [],
          edges: Array.isArray(result?.edges) ? result.edges : [],
        })
      })
      .catch((cause: Error) => {
        if (!cancelled) {
          setGraph(null)
          setError(cause.message || 'Could not load citation graph')
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, postId, depth])

  const chart = useMemo(
    () => graph && graph.edges.length > 0 ? buildCitationMermaid(graph, postId) : '',
    [graph, postId],
  )

  return (
    <section aria-labelledby="citation-graph-heading" style={{ margin: '8px 0 14px' }}>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        style={{
          font: '600 var(--lf-text-caption) var(--lf-font-body)',
          background: 'transparent',
          color: 'var(--lf-ink)',
          border: 'none',
          padding: 0,
          cursor: 'pointer',
          textDecoration: 'underline',
          textDecorationColor: 'var(--lf-accent-3)',
          textDecorationThickness: 2,
          textUnderlineOffset: 4,
        }}
      >
        {open ? 'Hide citation graph' : 'Explore citation graph'}
      </button>

      {open && (
        <LFSurface padding={16} accent="iris" style={{ marginTop: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
            <div>
              <h2 id="citation-graph-heading" className="lf-text-h3" style={{ margin: 0 }}>
                Citation graph
              </h2>
              <div style={{ color: 'var(--lf-muted)', fontSize: 12, marginTop: 3 }}>
                Arrows point from a post to the post it cites.
              </div>
            </div>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12 }}>
              Depth
              <select
                aria-label="Citation graph depth"
                value={depth}
                onChange={(event) => setDepth(Number(event.target.value))}
                style={{
                  border: '1px solid var(--lf-rule-mid)',
                  background: 'var(--lf-paper)',
                  color: 'var(--lf-ink)',
                  padding: '4px 8px',
                  borderRadius: 'var(--lf-radius-sm)',
                }}
              >
                {[1, 2, 3, 4, 5].map((value) => (
                  <option key={value} value={value}>{value}</option>
                ))}
              </select>
            </label>
          </div>

          {loading && <div className="lf-empty">Loading citation graph…</div>}
          {!loading && error && (
            <div role="alert" style={{ color: 'var(--lf-accent-2)', marginTop: 14 }}>
              {error}
            </div>
          )}
          {!loading && !error && graph && graph.edges.length === 0 && (
            <div className="lf-empty">No connected Loomfeed citations yet.</div>
          )}
          {!loading && !error && chart && graph && (
            <>
              <MermaidDiagram chart={chart} />
              <div style={{ marginTop: 12 }}>
                <div style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 11, color: 'var(--lf-muted)', marginBottom: 6 }}>
                  POSTS IN THIS VIEW · {graph.nodes.length}
                </div>
                <div style={{ display: 'grid', gap: 6 }}>
                  {graph.nodes.map((node) => (
                    <Link
                      key={node.id}
                      href={`/post/${node.id}`}
                      style={{ color: 'var(--lf-ink)', fontSize: 13, textDecorationColor: node.id === postId ? 'var(--lf-accent)' : 'var(--lf-rule-mid)' }}
                    >
                      {node.title} · {node.author}{node.id === postId ? ' (current)' : ''}
                    </Link>
                  ))}
                </div>
              </div>
            </>
          )}
        </LFSurface>
      )}
    </section>
  )
}
