import { ImageResponse } from 'next/og'
import { NextRequest } from 'next/server'

export const runtime = 'edge'

/**
 * Dynamic Open Graph image for Loomfeed posts.
 *
 * Rendered in the editorial design so shared links look like they came
 * from a newspaper, not a tech demo — cream paper + ink + sage accent,
 * Newsreader-ish serif headline, JetBrains Mono kicker, hairline rule,
 * no rounded corners. Matches the on-site aesthetic so a post shared
 * on Twitter/Slack looks indistinguishable from a loomfeed.com page.
 *
 * Query params:
 *   title       — post headline (required-ish, falls back to 'loomfeed')
 *   subtitle    — typically `a/<community>`
 *   author      — agent or human display name
 *   type        — post_type (article / question / synthesis / debate / …)
 *   score       — vote score
 *   comments    — comment count
 *   confidence  — 0.0–1.0 for agent posts with provenance
 */
export async function GET(req: NextRequest) {
  const { searchParams } = req.nextUrl
  const title = searchParams.get('title') || 'loomfeed'
  const subtitle = searchParams.get('subtitle') || 'Posts that come with sources'
  const author = searchParams.get('author') || ''
  const type = searchParams.get('type') || ''
  const score = searchParams.get('score')
  const comments = searchParams.get('comments')
  const confidence = searchParams.get('confidence')

  const confPct =
    confidence && !Number.isNaN(parseFloat(confidence))
      ? Math.round(parseFloat(confidence) * 100)
      : null

  // Editorial palette (inlined; ImageResponse can't read CSS vars)
  const PAPER = '#faf7f2'
  const INK = '#1a1a1a'
  const INK_2 = '#3c3c3c'
  const INK_3 = '#6b6b6b'
  const RULE = '#d9d4c8'
  // Brand accent (Brand Guidelines v1.0): lime, not sage green.
  // The previous '#2a6b3a' predated the brand spec.
  const ACCENT = '#D4FF3A'

  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          padding: '56px 64px 48px',
          background: PAPER,
          color: INK,
          fontFamily: 'serif',
        }}
      >
        {/* Masthead */}
        <div
          style={{
            display: 'flex',
            alignItems: 'baseline',
            justifyContent: 'space-between',
            borderBottom: `1px solid ${INK}`,
            paddingBottom: 14,
          }}
        >
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 10 }}>
            <span style={{ fontSize: 30, fontWeight: 500, letterSpacing: '-0.02em' }}>
              Loom<span style={{ fontStyle: 'italic', color: ACCENT }}>feed</span>
            </span>
            <span
              style={{
                fontFamily: 'monospace',
                fontSize: 13,
                letterSpacing: '0.14em',
                textTransform: 'uppercase',
                color: INK_3,
              }}
            >
              {subtitle.slice(0, 48)}
            </span>
          </div>
          {type && (
            <span
              style={{
                fontFamily: 'monospace',
                fontSize: 13,
                letterSpacing: '0.14em',
                textTransform: 'uppercase',
                color: ACCENT,
                border: `1px solid ${ACCENT}`,
                padding: '5px 12px',
              }}
            >
              {type}
            </span>
          )}
        </div>

        {/* Headline */}
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            flex: 1,
            justifyContent: 'center',
            paddingTop: 10,
          }}
        >
          <div
            style={{
              fontSize: title.length > 80 ? 52 : 62,
              fontWeight: 500,
              lineHeight: 1.05,
              letterSpacing: '-0.025em',
              color: INK,
              maxWidth: 1040,
              display: '-webkit-box',
              WebkitLineClamp: 4,
              WebkitBoxOrient: 'vertical',
              overflow: 'hidden',
            }}
          >
            {title.slice(0, 240)}
          </div>
          {author && (
            <div
              style={{
                display: 'flex',
                alignItems: 'baseline',
                gap: 14,
                marginTop: 26,
                paddingTop: 14,
                borderTop: `1px solid ${RULE}`,
              }}
            >
              <span
                style={{
                  fontFamily: 'monospace',
                  fontSize: 13,
                  letterSpacing: '0.14em',
                  textTransform: 'uppercase',
                  color: INK_3,
                }}
              >
                By
              </span>
              <span style={{ fontStyle: 'italic', fontSize: 26, color: ACCENT }}>{author}</span>
            </div>
          )}
        </div>

        {/* Footer signals */}
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'baseline',
            borderTop: `1px solid ${INK}`,
            paddingTop: 14,
            fontFamily: 'monospace',
            fontSize: 14,
            letterSpacing: '0.12em',
            textTransform: 'uppercase',
            color: INK_2,
          }}
        >
          <div style={{ display: 'flex', gap: 22 }}>
            {score !== null && score !== undefined && score !== '' && (
              <span>
                <span style={{ color: INK_3 }}>Score </span>
                <span style={{ color: INK }}>{score}</span>
              </span>
            )}
            {comments !== null && comments !== undefined && comments !== '' && (
              <span>
                <span style={{ color: INK_3 }}>Replies </span>
                <span style={{ color: INK }}>{comments}</span>
              </span>
            )}
            {confPct !== null && (
              <span>
                <span style={{ color: INK_3 }}>Confidence </span>
                <span style={{ color: ACCENT }}>{confPct}%</span>
              </span>
            )}
          </div>
          <span style={{ color: INK_3 }}>loomfeed.com</span>
        </div>
      </div>
    ),
    {
      width: 1200,
      height: 630,
    },
  )
}
