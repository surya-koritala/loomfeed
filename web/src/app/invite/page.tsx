'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../../api/client'

/**
 * /invite — the user's personal invite page.
 *
 * Shows:
 *   - The invite link + plaintext code to share
 *   - A count of people who joined with the code
 *   - First-degree invitees (up to 50, newest first)
 *
 * Editorial style: .head masthead, mono-caps kickers, hairline rule,
 * ink/paper/accent tokens only. No new design surface.
 */

interface Invitee {
  participantId: string
  displayName: string
  joinedAt: string
  verified: boolean
}

interface InviteSummary {
  code: string
  invitedBy?: string | null
  acceptCount: number
  invitees: Invitee[]
}

function relTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const m = Math.floor(diff / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  if (d < 30) return `${d}d ago`
  return new Date(iso).toLocaleDateString()
}

export default function InvitePage() {
  const router = useRouter()
  const [summary, setSummary] = useState<InviteSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [copied, setCopied] = useState<'link' | 'code' | null>(null)

  useEffect(() => {
    if (typeof window === 'undefined') return
    if (!localStorage.getItem('token')) {
      router.replace('/login?next=/invite')
      return
    }
    api
      .getMyInvite()
      .then((d: any) => setSummary(d))
      .catch(() => setSummary(null))
      .finally(() => setLoading(false))
  }, [router])

  const siteUrl =
    typeof window !== 'undefined'
      ? `${window.location.protocol}//${window.location.host}`
      : 'http://localhost:3000'
  const inviteLink = summary ? `${siteUrl}/register?invite=${summary.code}` : ''

  const copy = async (text: string, kind: 'link' | 'code') => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(kind)
      setTimeout(() => setCopied(null), 1800)
    } catch {
      // silent — not worth a toast for a common denial
    }
  }

  return (
    <div className="lf-narrow">
      <div className="head">
        <div>
          <div className="edition">Invites · Bring someone in</div>
          <h1>
            Your <em>invite.</em>
          </h1>
          <div className="sub">
            Share the link. Each signup that uses it is credited to you — and
            gives your reputation a bump.
          </div>
        </div>
      </div>

      {loading ? (
        <div className="lf-empty">Loading…</div>
      ) : !summary ? (
        <div
          style={{
            padding: '24px 0',
            fontFamily: 'var(--lf-font-body)',
            fontStyle: 'italic',
            color: 'var(--neg)',
            fontSize: 15,
          }}
        >
          Couldn&apos;t load your invite info. Try refreshing — or check your
          sign-in on{' '}
          <Link href="/settings" style={{ color: 'var(--accent)' }}>
            Settings
          </Link>
          .
        </div>
      ) : (
        <>
          {/* Share block */}
          <section
            style={{
              border: '1px solid var(--lf-ink)',
              padding: '18px 18px 16px',
              marginBottom: 24,
              background: 'var(--lf-paper-alt)',
            }}
          >
            <div
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 10,
                letterSpacing: '0.14em',
                textTransform: 'uppercase',
                color: 'var(--lf-muted)',
                marginBottom: 10,
              }}
            >
              Your invite link
            </div>
            <div
              style={{
                display: 'flex',
                gap: 8,
                alignItems: 'stretch',
                marginBottom: 14,
                flexWrap: 'wrap',
              }}
            >
              <input
                readOnly
                value={inviteLink}
                onClick={(e) => (e.target as HTMLInputElement).select()}
                style={{
                  flex: 1,
                  minWidth: 220,
                  fontFamily: 'var(--lf-font-mono)',
                  fontSize: 12,
                  padding: '10px 12px',
                  background: 'var(--lf-paper)',
                  border: '1px solid var(--lf-rule-soft)',
                  color: 'var(--lf-ink)',
                  outline: 'none',
                }}
              />
              <button
                onClick={() => copy(inviteLink, 'link')}
                style={{
                  fontFamily: 'var(--lf-font-mono)',
                  fontSize: 10,
                  letterSpacing: '0.12em',
                  textTransform: 'uppercase',
                  background: 'var(--lf-ink)',
                  color: 'var(--lf-paper)',
                  border: '1px solid var(--lf-ink)',
                  padding: '10px 14px',
                  cursor: 'pointer',
                }}
              >
                {copied === 'link' ? 'Copied' : 'Copy link'}
              </button>
            </div>

            <div
              style={{
                display: 'flex',
                alignItems: 'baseline',
                gap: 14,
                paddingTop: 10,
                borderTop: '1px dotted var(--lf-rule-soft)',
                flexWrap: 'wrap',
              }}
            >
              <span
                style={{
                  fontFamily: 'var(--lf-font-mono)',
                  fontSize: 10,
                  letterSpacing: '0.14em',
                  textTransform: 'uppercase',
                  color: 'var(--lf-muted)',
                }}
              >
                Or the code
              </span>
              <code
                style={{
                  fontFamily: 'var(--lf-font-mono)',
                  fontSize: 18,
                  letterSpacing: '0.12em',
                  color: 'var(--lf-ink)',
                  background: 'var(--lf-paper)',
                  border: '1px solid var(--lf-rule-soft)',
                  padding: '4px 10px',
                }}
              >
                {summary.code}
              </code>
              <button
                onClick={() => copy(summary.code, 'code')}
                style={{
                  fontFamily: 'var(--lf-font-mono)',
                  fontSize: 10,
                  letterSpacing: '0.12em',
                  textTransform: 'uppercase',
                  background: 'transparent',
                  color: 'var(--lf-ink)',
                  border: '1px solid var(--lf-rule-soft)',
                  padding: '6px 10px',
                  cursor: 'pointer',
                }}
              >
                {copied === 'code' ? 'Copied' : 'Copy'}
              </button>
            </div>
          </section>

          {/* Stats row */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr 1fr',
              border: '1px solid var(--lf-rule-soft)',
              marginBottom: 24,
            }}
          >
            <div style={{ padding: '14px 16px', borderRight: '1px solid var(--lf-rule-soft)' }}>
              <div
                style={{
                  fontFamily: 'var(--lf-font-mono)',
                  fontSize: 10,
                  letterSpacing: '0.14em',
                  textTransform: 'uppercase',
                  color: 'var(--lf-muted)',
                  marginBottom: 6,
                }}
              >
                Joined with your code
              </div>
              <div
                style={{
                  fontFamily: 'var(--lf-font-body)',
                  fontSize: 32,
                  fontWeight: 500,
                  letterSpacing: '-0.02em',
                  lineHeight: 1,
                  color: 'var(--lf-ink)',
                }}
              >
                {summary.acceptCount}
              </div>
            </div>
            <div style={{ padding: '14px 16px' }}>
              <div
                style={{
                  fontFamily: 'var(--lf-font-mono)',
                  fontSize: 10,
                  letterSpacing: '0.14em',
                  textTransform: 'uppercase',
                  color: 'var(--lf-muted)',
                  marginBottom: 6,
                }}
              >
                Rep earned
              </div>
              <div
                style={{
                  fontFamily: 'var(--lf-font-body)',
                  fontSize: 32,
                  fontWeight: 500,
                  letterSpacing: '-0.02em',
                  lineHeight: 1,
                  color: 'var(--accent)',
                }}
              >
                +{(summary.acceptCount * 1.0).toFixed(1)}
              </div>
            </div>
          </div>

          {/* Invitees list */}
          <section>
            <div
              style={{
                fontFamily: 'var(--lf-font-mono)',
                fontSize: 10,
                letterSpacing: '0.14em',
                textTransform: 'uppercase',
                color: 'var(--lf-muted)',
                padding: '10px 0',
                borderBottom: '1px solid var(--lf-ink)',
                marginBottom: 0,
                display: 'flex',
                justifyContent: 'space-between',
              }}
            >
              <span>People you invited</span>
              <span style={{ color: 'var(--ink-4)' }}>{summary.invitees.length}</span>
            </div>
            {summary.invitees.length === 0 ? (
              <p
                style={{
                  padding: '22px 0',
                  fontFamily: 'var(--lf-font-body)',
                  fontStyle: 'italic',
                  color: 'var(--lf-muted)',
                  fontSize: 15,
                }}
              >
                No one yet. Share your link — the first signup shows up here
                within minutes.
              </p>
            ) : (
              <div>
                {summary.invitees.map((iv) => (
                  <Link
                    key={iv.participantId}
                    href={`/profile/${iv.participantId}`}
                    className="lf-invite-row"
                    style={{
                      display: 'grid',
                      gridTemplateColumns: '1fr auto auto',
                      gap: 14,
                      padding: '12px 0',
                      borderBottom: '1px solid var(--lf-rule-soft)',
                      alignItems: 'baseline',
                      textDecoration: 'none',
                      color: 'var(--lf-ink)',
                    }}
                  >
                    <span
                      style={{
                        fontFamily: 'var(--lf-font-body)',
                        fontSize: 16,
                        fontWeight: 500,
                        letterSpacing: '-0.005em',
                      }}
                    >
                      {iv.displayName}
                    </span>
                    <span
                      style={{
                        fontFamily: 'var(--lf-font-mono)',
                        fontSize: 10,
                        letterSpacing: '0.12em',
                        textTransform: 'uppercase',
                        color: iv.verified ? 'var(--accent)' : 'var(--lf-muted)',
                      }}
                    >
                      {iv.verified ? '✓ Verified' : 'Pending verification'}
                    </span>
                    <span
                      style={{
                        fontFamily: 'var(--lf-font-mono)',
                        fontSize: 10,
                        letterSpacing: '0.08em',
                        textTransform: 'uppercase',
                        color: 'var(--lf-muted)',
                      }}
                    >
                      {relTime(iv.joinedAt)}
                    </span>
                  </Link>
                ))}
              </div>
            )}
          </section>
        </>
      )}
    </div>
  )
}
