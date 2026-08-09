'use client'

import { useEffect, useState } from 'react'
import { useSearchParams } from 'next/navigation'
import { LFSurface, LFButton } from '../components/lf'

export default function VerifyEmail() {
  const searchParams = useSearchParams()
  const token = searchParams.get('token')
  const [status, setStatus] = useState<'loading' | 'success' | 'error' | 'no-token'>('loading')
  const [message, setMessage] = useState('')

  useEffect(() => {
    if (!token) {
      setStatus('no-token')
      setMessage('No verification token provided.')
      return
    }

    fetch(`/api/v1/auth/verify-email?token=${encodeURIComponent(token)}`)
      .then(async (res) => {
        const data = await res.json()
        if (res.ok) {
          setStatus('success')
          setMessage(data.message || 'Your email has been verified.')
        } else {
          setStatus('error')
          setMessage(data.error || 'Verification failed. The token may be invalid or expired.')
        }
      })
      .catch(() => {
        setStatus('error')
        setMessage('Something went wrong. Please try again later.')
      })
  }, [token])

  const title =
    status === 'loading'
      ? 'Verifying your email…'
      : status === 'success'
      ? 'Email verified.'
      : status === 'no-token'
      ? 'Missing token.'
      : 'Verification failed.'

  const dotColor =
    status === 'success' ? 'var(--lf-pos)' : status === 'loading' ? 'var(--lf-muted)' : 'var(--lf-rose)'

  return (
    <div className="lf-auth">
      <div className="lf-auth-card">
        <div className="lf-auth-head">
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 'var(--lf-text-caption)',
              color: 'var(--lf-muted)',
              letterSpacing: '0.06em',
              textTransform: 'uppercase',
              marginBottom: 6,
              display: 'inline-flex',
              alignItems: 'center',
              gap: 8,
            }}
          >
            <span style={{ width: 8, height: 8, borderRadius: 4, background: dotColor, display: 'inline-block' }} />
            Email verification
          </div>
          <h1>{title}</h1>
        </div>

        <LFSurface padding={28}>
          <p
            style={{
              fontFamily: 'var(--lf-font-body)',
              fontSize: 'var(--lf-text-h3)',
              lineHeight: 1.55,
              color: 'var(--lf-muted)',
              margin: '0 0 18px',
            }}
          >
            {status === 'loading'
              ? 'Please wait while we verify your email address.'
              : status === 'success'
              ? "You're all set. Your account is fully verified and ready to use."
              : message}
          </p>

          {status === 'success' && (
            <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              <LFButton variant="primary" size="lg" href="/" style={{ flex: '1 1 auto' }}>
                Go to feed
              </LFButton>
              <LFButton variant="ghost" size="lg" href="/submit" style={{ flex: '1 1 auto' }}>
                Create a post
              </LFButton>
            </div>
          )}

          {status === 'error' && (
            <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              <LFButton variant="primary" size="lg" href="/settings" style={{ flex: '1 1 auto' }}>
                Go to settings
              </LFButton>
              <LFButton variant="ghost" size="lg" href="/" style={{ flex: '1 1 auto' }}>
                Go home
              </LFButton>
            </div>
          )}

          {status === 'no-token' && (
            <LFButton variant="primary" size="lg" href="/" fullWidth>
              Go home
            </LFButton>
          )}
        </LFSurface>
      </div>
    </div>
  )
}
