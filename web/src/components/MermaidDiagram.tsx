'use client'

import { useEffect, useRef, useState } from 'react'
import DOMPurify from 'dompurify'

interface Props {
  chart: string
}

export default function MermaidDiagram({ chart }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [svg, setSvg] = useState<string>('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    import('mermaid').then(async (mod) => {
      const mermaid = mod.default
      mermaid.initialize({
        startOnLoad: false,
        theme: 'dark',
        themeVariables: {
          primaryColor: 'var(--lf-accent-3)',
          primaryTextColor: 'var(--lf-ink)',
          primaryBorderColor: 'var(--lf-accent-3)',
          lineColor: 'var(--lf-muted)',
          secondaryColor: 'var(--bg-page)',
          tertiaryColor: 'var(--lf-paper)',
        }
      })

      try {
        const id = `mermaid-${Math.random().toString(36).slice(2)}`
        const { svg: rendered } = await mermaid.render(id, chart)
        if (!cancelled) setSvg(rendered)
      } catch (err: any) {
        if (!cancelled) setError(err?.message || 'Failed to render diagram')
      }
    })

    return () => { cancelled = true }
  }, [chart])

  if (error) {
    return (
      <div style={{
        padding: 12, borderRadius: 'var(--lf-radius-sm)',
        border: '1px solid color-mix(in srgb, var(--lf-rose) 20%, transparent)',
        background: 'color-mix(in srgb, var(--lf-rose) 6%, transparent)',
        fontSize: 12, color: 'var(--lf-accent-2)',
        fontFamily: 'inherit',
      }}>
        Diagram error: {error}
      </div>
    )
  }

  if (!svg) {
    return (
      <div style={{
        padding: 16, borderRadius: 'var(--lf-radius-sm)',
        border: '1px solid var(--lf-rule-soft)',
        background: 'var(--lf-paper)',
        textAlign: 'center', color: 'var(--lf-muted)',
        fontSize: 12,
      }}>
        Rendering diagram...
      </div>
    )
  }

  return (
    <div
      ref={containerRef}
      style={{
        padding: 16, borderRadius: 'var(--lf-radius-sm)',
        border: '1px solid var(--lf-rule-soft)',
        background: 'var(--lf-paper)',
        overflow: 'auto',
        margin: '8px 0',
      }}
      dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true } }) }}
    />
  )
}
