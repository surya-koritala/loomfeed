import Link from 'next/link'

// Shared 404 / not-found shell. All three not-found boundaries — root
// (app/not-found.tsx), /a/[slug], and /profile/[id] — render this so the
// brand mark, tokens, and layout can't drift apart (root + profile were
// still on legacy --gray-* tokens; only /a/[slug] had been migrated).
//
// Each boundary file stays a thin physical not-found.tsx because Next.js
// 15 (+ standalone output) needs one present at the segment to commit the
// 404 status header — this component is only what they render. Keeping the
// brand mark in ONE place also means the icon swap only happens here.

export interface LFNotFoundProps {
  /** Big headline — usually "404". */
  title?: string
  /** Explanatory line under the title. */
  message: string
  /** Primary (filled) action. */
  primary: { label: string; href: string }
  /** Secondary (outline) action — defaults to Home. */
  secondary?: { label: string; href: string }
}

const btnBase: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  padding: '10px 24px',
  borderRadius: 'var(--lf-radius)',
  fontSize: 14,
  fontWeight: 600,
  textDecoration: 'none',
}

export function LFNotFound({
  title = '404',
  message,
  primary,
  secondary = { label: 'Go home', href: '/' },
}: LFNotFoundProps) {
  return (
    <div style={{ maxWidth: 600, margin: '0 auto', padding: '80px 24px', textAlign: 'center' }}>
      <img
        src="/favicon.svg"
        alt=""
        aria-hidden="true"
        style={{ width: 96, height: 96, margin: '0 auto 24px', display: 'block' }}
      />
      <h1
        style={{
          fontFamily: 'var(--lf-font-display)',
          fontSize: 48,
          fontWeight: 800,
          color: 'var(--lf-ink)',
          margin: '0 0 8px',
          letterSpacing: '-0.04em',
        }}
      >
        {title}
      </h1>
      <p style={{ fontSize: 18, color: 'var(--lf-muted)', margin: '0 0 32px', lineHeight: 1.5 }}>
        {message}
      </p>
      <div style={{ display: 'flex', gap: 12, justifyContent: 'center', flexWrap: 'wrap' }}>
        <Link
          href={primary.href}
          style={{ ...btnBase, background: 'var(--lf-ink)', color: 'var(--lf-paper)' }}
        >
          {primary.label}
        </Link>
        {secondary && (
          <Link
            href={secondary.href}
            style={{
              ...btnBase,
              background: 'transparent',
              color: 'var(--lf-muted)',
              border: '1px solid var(--lf-rule-mid)',
            }}
          >
            {secondary.label}
          </Link>
        )}
      </div>
    </div>
  )
}
