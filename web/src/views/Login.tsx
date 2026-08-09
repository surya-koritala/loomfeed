'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { api } from '../api/client'
import { setAuthHintCookie } from '../lib/auth-hint'
import GoogleSignInButton from '../components/GoogleSignInButton'
import { LFSurface, LFButton, LFInput } from '../components/lf'
import { IconArrowRight } from '../components/lf/icons'

export default function Login() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [githubEnabled, setGithubEnabled] = useState(false)
  const [googleClientId, setGoogleClientId] = useState('')

  useEffect(() => {
    fetch('/api/v1/config')
      .then((r) => r.json())
      .then((d) => {
        setGithubEnabled(!!d.githubOauthEnabled)
        setGoogleClientId(d.googleClientId || '')
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    const token = searchParams.get('token')
    if (token) {
      localStorage.setItem('token', token)
      setAuthHintCookie()
      router.push('/')
    }
  }, [searchParams, router])

  const handleGoogleAuth = async (credential: string) => {
    setLoading(true)
    setError(null)
    try {
      const data = (await api.googleAuth(credential)) as {
        token?: string
        accessToken?: string
        refreshToken?: string
      }
      const token = data.accessToken ?? data.token
      if (token) {
        localStorage.setItem('token', token)
        setAuthHintCookie()
      }
      if (data.refreshToken) localStorage.setItem('refresh_token', data.refreshToken)
      router.push('/')
    } catch (err: any) {
      setError(err.message ?? 'Google sign-in failed')
    } finally {
      setLoading(false)
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError(null)
    try {
      const data = (await api.login({ email, password })) as {
        token?: string
        accessToken?: string
        refreshToken?: string
      }
      const token = data.accessToken ?? data.token
      if (token) {
        localStorage.setItem('token', token)
        setAuthHintCookie()
      }
      if (data.refreshToken) localStorage.setItem('refresh_token', data.refreshToken)
      router.push('/')
    } catch (err: any) {
      setError(err.message ?? 'Login failed')
    } finally {
      setLoading(false)
    }
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
            Welcome back
          </div>
          <h1>Sign in.</h1>
        </div>

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

            <label className="lf-field">
              <span className="lf-field-label-row">
                <span>Password</span>
                <Link href="/forgot-password" className="lf-field-aside">
                  Forgot?
                </Link>
              </span>
              <LFInput
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                placeholder="••••••••"
                autoComplete="current-password"
              />
            </label>

            {error && <div className="lf-auth-error">{error}</div>}

            <LFButton variant="primary" size="lg" type="submit" disabled={loading} fullWidth>
              {loading ? 'Signing in…' : 'Sign in'}
            </LFButton>
          </form>

          {(googleClientId || githubEnabled) && (
            <>
              <div className="lf-auth-divider">
                <span className="line" />
                <span className="label">or</span>
                <span className="line" />
              </div>
              <div className="lf-auth-oauth">
                {googleClientId && (
                  <GoogleSignInButton clientId={googleClientId} onCredential={handleGoogleAuth} />
                )}
                {githubEnabled && (
                  <a href="/api/v1/auth/github" className="lf-auth-oauth-btn">
                    <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
                      <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
                    </svg>
                    Continue with GitHub
                  </a>
                )}
              </div>
            </>
          )}
        </LFSurface>

        <p className="lf-auth-foot">
          Don't have an account?{' '}
          <Link href="/register">Register <IconArrowRight size={13} /></Link>
        </p>
      </div>
    </div>
  )
}
