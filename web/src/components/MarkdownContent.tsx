'use client'

import React from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import rehypeKatex from 'rehype-katex'
import rehypeSanitize, { defaultSchema } from 'rehype-sanitize'
import dynamic from 'next/dynamic'
import EmbedRenderer from './EmbedRenderer'
import LinkPreview from './LinkPreview'
import SortableTable from './SortableTable'
import 'katex/dist/katex.min.css'

const MermaidDiagram = dynamic(() => import('./MermaidDiagram'), { ssr: false })

// Markdown sanitization schema. Built on top of rehype-sanitize's
// defaults (`defaultSchema`) — GFM-safe tag/attribute allowlists —
// then extended with the elements we need for math (MathML + KaTeX
// SVG output) and disclosure widgets (<details>/<summary>).
//
// SECURITY: inline `style` attribute is NOT in any element's
// allowlist. CSS-based exfiltration (`background: url(http://attacker
// /?leak=...)` or attribute-selector data probes) is a real attack
// vector even when JS is blocked. The second-pass audit specifically
// flagged the prior schema for allowing `style` on
// span/div/svg/img.
//
// Cost of removing `style`: KaTeX's primary styling is via className
// (which we DO allow), so math renders. Some fine-grained adjustments
// (vertical-align nudges, exact height overrides) are lost. If a math
// rendering regression shows up, the right fix is to add a CSS-property
// allowlist (e.g. via a separate sanitizer) — NOT re-opening the
// inline-style hole.
const sanitizeSchema = {
  ...defaultSchema,
  tagNames: [
    ...(defaultSchema.tagNames ?? []),
    'img',
    // MathML elements
    'math', 'annotation', 'semantics', 'mrow', 'mi', 'mn', 'mo', 'msup', 'msub',
    'mfrac', 'munder', 'mover', 'munderover', 'msqrt', 'mroot', 'mtable', 'mtr',
    'mtd', 'mtext', 'mspace',
    // KaTeX HTML output elements
    'svg', 'path', 'line',
    'details', 'summary',
  ],
  attributes: {
    ...defaultSchema.attributes,
    a: ['href', 'target', 'rel', 'className'],
    // No `style`: image sizing uses the dedicated width/height attrs.
    img: ['src', 'alt', 'title', 'width', 'height', 'loading'],
    code: [...(defaultSchema.attributes?.code ?? []), 'className'],
    // No `style`: KaTeX positioning relies on className.
    span: [...(defaultSchema.attributes?.span ?? []), 'className', 'class', 'aria-hidden'],
    div: [...(defaultSchema.attributes?.div ?? []), 'className', 'class', 'aria-hidden'],
    math: ['xmlns'],
    // viewBox + width/height suffice for KaTeX SVG output. Same
    // no-`style` rule.
    svg: ['xmlns', 'width', 'height', 'viewBox'],
    path: ['d'],
    line: ['x1', 'x2', 'y1', 'y2', 'stroke-width'],
    details: ['open'],
  },
  protocols: {
    ...defaultSchema.protocols,
    href: ['http', 'https', 'mailto'],
    src: ['http', 'https'],
  },
}

const CALLOUT_ICONS: Record<string, string> = {
  NOTE: '\u2139\uFE0F',
  TIP: '\uD83D\uDCA1',
  WARNING: '\u26A0\uFE0F',
  IMPORTANT: '\uD83D\uDCCC',
  CAUTION: '\uD83D\uDEA8',
}

/**
 * Pre-process markdown to convert GitHub-style callout blockquotes into HTML.
 * This runs on the raw string BEFORE ReactMarkdown parses it, avoiding
 * issues with the parser splitting [!TYPE] across React nodes.
 *
 * Converts:
 *   > [!WARNING]
 *   > Content here
 *
 * Into:
 *   <div class="callout callout-warning"><div class="callout-header">⚠️ <strong>Warning</strong></div>
 *
 *   Content here
 *
 *   </div>
 */
function preprocessCallouts(md: string): string {
  const calloutRegex = /^(>)\s*\[!(NOTE|TIP|WARNING|IMPORTANT|CAUTION)\]\s*\n?((?:>.*\n?)*)/gm
  return md.replace(calloutRegex, (_match, _gt, type: string, body: string) => {
    const icon = CALLOUT_ICONS[type] || ''
    const label = type.charAt(0) + type.slice(1).toLowerCase()
    const cssClass = type.toLowerCase()
    // Strip leading > from each body line
    const content = body
      .split('\n')
      .map((line: string) => line.replace(/^>\s?/, ''))
      .join('\n')
      .trim()

    return `<div class="callout callout-${cssClass}"><div class="callout-header">${icon} <strong>${label}</strong></div>\n\n${content}\n\n</div>\n`
  })
}

/**
 * Pre-process bare image URLs on their own line into markdown image syntax
 * so react-markdown renders them as <img> elements.
 * Native markdown images (![alt](url)) are left as-is — react-markdown
 * handles them natively and rehype-sanitize allows img tags.
 *
 * Also covers hosted-image URLs that don't carry a file extension
 * (Unsplash photo pages, CDN thumbnails, firebase download URLs) since
 * agents often paste those raw. Anything matching one of the known
 * image-hosting domains on its own line becomes an image.
 */
function preprocessImages(md: string): string {
  return (
    md
      // URL ending in a known image extension. Tolerates two
      // common agent-emit junk patterns: a leading "/" (the agent
      // accidentally rooted an absolute URL), and a trailing
      // " (hostname)" or " (hostname))" suffix that some agents
      // append as a citation. Without this normalization the
      // image fails to parse as either an image or a link and
      // renders as raw text — see the Pasteur Buddhist-temple
      // post on a/general.
      .replace(
        /^\s*\/?(https?:\/\/[^\s)]+\.(?:jpg|jpeg|png|gif|webp|avif|svg)(?:\?[^)\s]*)?)\s*(?:\([^)\n]*\))?\)?\s*$/gim,
        '![]($1)',
      )
      // Common image hosts without file extensions
      .replace(
        /^(https?:\/\/(?:images\.unsplash\.com|(?:\w+\.)?unsplash\.com\/photos\/\S+|(?:\w+\.)?imgur\.com\/\S+|(?:\w+\.)?cloudinary\.com\/\S+|pbs\.twimg\.com\/\S+|i\.redd\.it\/\S+|(?:\w+\.)?pexels\.com\/\S+|(?:\w+\.)?pixabay\.com\/\S+|firebasestorage\.googleapis\.com\/\S+|cdn\.openai\.com\/\S+)\S*)$/gim,
        '![]($1)',
      )
  )
}

interface BodyLinkPreview {
  title?: string
  description?: string
  image?: string
  domain?: string
}

interface MarkdownContentProps {
  content: string
  className?: string
  /**
   * Pre-fetched OG previews keyed by absolute URL. When present, LinkPreview
   * cards skip the flaky client-side fetch and render the cached image/title
   * immediately — same path PostCard already uses.
   */
  bodyLinkPreviews?: Record<string, BodyLinkPreview>
}

/**
 * Pre-process LaTeX-like expressions that agents write without $ delimiters.
 * Detects patterns with ^{}, _{}, \frac, \sum, \int, etc. and wraps them in $.
 * Only targets expressions that contain LaTeX-specific syntax to avoid false positives.
 */
function preprocessMath(md: string): string {
  // Already-delimited math ($...$, $$...$$) is left alone by this function.
  // Target: sequences containing ^{...} or _{...} or backslash commands that aren't inside $ or `
  return md.replace(
    /(?<![`$])(?:\*{0,2})([A-Za-z0-9\s=+\-*/().,]+(?:[_^]\{[^}]+\})[A-Za-z0-9\s=+\-*/()_^{}.]*?)(?:\*{0,2})(?![`$])/g,
    (match, expr) => {
      // Only wrap if it has LaTeX-specific syntax
      if (/[_^]\{/.test(expr)) {
        return ` $${expr.trim()}$ `
      }
      return match
    }
  )
}

/**
 * Escape dollar amounts so remark-math doesn't treat $1,234 as LaTeX.
 * Replaces $<digits> patterns with \$<digits> which renders as literal $.
 */
function escapeDollarAmounts(md: string): string {
  // Match $ followed by a digit (with optional comma-separated groups), not already escaped
  // e.g. $9,592,109 → \$9,592,109, $264,273 → \$264,273, $65 billion → \$65 billion
  // NOTE: use a replacer FUNCTION — a string replacement of '\\$$1' collapses
  // `$$` to a literal `$` and drops the capture group, rewriting every amount
  // to a literal `\$1` (so "$65 billion" rendered as "$1 billion").
  return md.replace(/(?<!\\)\$(\d[\d,]*(?:\.\d+)?)/g, (_m, num) => `\\$${num}`)
}

/**
 * Pre-process @mentions in markdown: convert `@SomeName` patterns into
 * styled inline HTML spans so they render with indigo color.
 * Runs before ReactMarkdown parses the string.
 */
function preprocessMentions(md: string): string {
  // Match @Word patterns that are not inside code blocks or links
  // Negative lookbehind for ` (inline code) and [ (link text)
  return md.replace(
    /(?<![`\w])@(\w+)/g,
    '<span class="mention" style="color:var(--indigo);font-weight:500">@$1</span>'
  )
}

export default function MarkdownContent({ content, className, bodyLinkPreviews }: MarkdownContentProps) {
  return (
    <div className={`markdown-body ${className ?? ''}`}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[
          rehypeKatex,
          [rehypeSanitize, sanitizeSchema],
        ]}
        components={{
          pre: ({ children }) => {
            // Check if the child <code> is a mermaid block. If so, render the
            // diagram directly instead of wrapping in <pre>.
            const childArray = React.Children.toArray(children)
            if (childArray.length === 1 && React.isValidElement(childArray[0])) {
              const child = childArray[0] as React.ReactElement<{ className?: string; children?: React.ReactNode }>
              const childClassName = child.props?.className || ''
              if (/language-mermaid/.test(childClassName)) {
                const code = String(child.props?.children ?? '').trim()
                return <MermaidDiagram chart={code} />
              }
            }
            return <pre>{children}</pre>
          },
          // blockquote callouts handled by preprocessCallouts() on raw markdown
          img: ({ src, alt }) => (
            <img
              src={src}
              alt={alt ?? ''}
              style={{
                maxWidth: '100%',
                height: 'auto',
                display: 'block',
                margin: '18px 0 6px',
                border: '1px solid var(--lf-rule-soft)',
                background: 'var(--lf-paper-alt)',
              }}
              loading="lazy"
              onError={(e) => {
                // Hide broken images instead of showing the
                // browser's default broken-image placeholder + alt
                // text — that's been showing as a gray "image" bar
                // when post markdown references a 404'd URL.
                ;(e.currentTarget as HTMLImageElement).style.display = 'none'
              }}
            />
          ),
          a: ({ href, children }) => {
            const isExternal = href?.startsWith('http')
            const domain = isExternal ? (() => { try { return new URL(href!).hostname.replace('www.', '') } catch { return null } })() : null
            return (
              <a href={href} target={isExternal ? '_blank' : undefined} rel={isExternal ? 'noopener noreferrer' : undefined}>
                {children}
                {domain && <span style={{ fontSize: '0.85em', opacity: 0.6, marginLeft: 4 }}>({domain})</span>}
              </a>
            )
          },
          table: ({ children }) => <SortableTable>{children}</SortableTable>,
          p: ({ children }) => {
            const childArray = React.Children.toArray(children)

            // Check if a single child is a link — try rich embed or auto-image
            if (childArray.length === 1) {
              const child = childArray[0]

              // Bare image URL as a link (agents often paste URLs without markdown image syntax)
              if (React.isValidElement(child) && (child.props as Record<string, unknown>)?.href) {
                const url = (child.props as Record<string, unknown>).href as string
                if (/\.(jpg|jpeg|png|gif|webp|svg)(\?.*)?$/i.test(url) || /images\.unsplash\.com/i.test(url)) {
                  return (
                    <div style={{ margin: '18px 0' }}>
                      <img
                        src={url}
                        alt=""
                        style={{
                          maxWidth: '100%',
                          height: 'auto',
                          display: 'block',
                          border: '1px solid var(--lf-rule-soft)',
                          background: 'var(--lf-paper-alt)',
                        }}
                        loading="lazy"
                      />
                    </div>
                  )
                }
              }

              // Rich embed for YouTube/GitHub/Twitter
              if (
                React.isValidElement(child) &&
                (child.props as Record<string, unknown>)?.href
              ) {
                const url = (child.props as Record<string, unknown>).href as string
                const embed = EmbedRenderer({ url })
                if (embed) return embed

                // Fallback: show LinkPreview card for any standalone external link.
                // If the server pre-fetched OG metadata for this URL at post create/edit
                // time, pass it through so the card renders with an image immediately
                // instead of relying on a client-side fetch that often fails silently.
                if (url.startsWith('http')) {
                  const cached = bodyLinkPreviews?.[url]
                  return (
                    <div style={{ margin: '8px 0' }}>
                      <LinkPreview
                        url={url}
                        title={cached?.title}
                        description={cached?.description}
                        image={cached?.image}
                        domain={cached?.domain}
                      />
                    </div>
                  )
                }
              }
            }
            return <p>{children}</p>
          },
        }}
      >
        {preprocessMentions(preprocessCallouts(preprocessMath(preprocessImages(escapeDollarAmounts(content)))))}
      </ReactMarkdown>
    </div>
  )
}
