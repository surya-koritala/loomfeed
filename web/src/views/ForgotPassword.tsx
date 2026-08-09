'use client'

import { useState } from 'react'
import Link from 'next/link'
import { LFSurface, LFButton, LFInput } from '../components/lf'
import { IconArrowRight } from '../components/lf/icons'

export default function ForgotPassword() {
  const [email, setEmail] = useState('')
  const [submitted, setSubmitted] = useState(false)

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitted(true)
  }

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
            }}
          >
            Account recovery
          </div>
          <h1>Forgot password?</h1>
        </div>

        {submitted ? (
          <LFSurface padding={28}>
            <p style={{ margin: 0, fontSize: 14, lineHeight: 1.55 }}>
              Password reset via email is not yet configured. Contact us at{' '}
              <a
                href="mailto:contact@loomfeed.com"
                style={{ color: 'var(--lf-ink)', textDecoration: 'underline' }}
              >
                contact@loomfeed.com
              </a>{' '}
              and we'll reset it for you.
            </p>
          </LFSurface>
        ) : (
          <LFSurface padding={28}>
            <form onSubmit={handleSubmit} className="lf-auth-form">
              <label className="lf-field">
                <span>Email</span>
                <LFInput
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                  placeholder="you@example.com"
                  autoComplete="email"
                />
              </label>
              <LFButton variant="primary" size="lg" type="submit" fullWidth>
                Send reset link
              </LFButton>
            </form>
          </LFSurface>
        )}

        <p className="lf-auth-foot">
          Remember your password?{' '}
          <Link href="/login">Sign in <IconArrowRight size={13} /></Link>
        </p>
      </div>
    </div>
  )
}
