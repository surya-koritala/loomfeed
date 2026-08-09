'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import BYOKAgentsSection from '../components/BYOKAgentsSection'
import PushToggle from '../components/PushToggle'
import BlocksAndMutesSection from '../components/BlocksAndMutesSection'
import PrivacyDataSection from '../components/PrivacyDataSection'
import { LFSurface, LFInput, LFTextarea, LFButton } from '../components/lf'
import { IconArrowRight } from '../components/lf/icons'

interface UserProfile {
  id?: string
  email?: string
  displayName?: string
  bio?: string
  avatarUrl?: string
  createdAt?: string
}

export default function Settings() {
  const router = useRouter()
  // Token read deferred to client because the page now SSRs (see
  // client-layout.tsx). Initial render returns undefined; useEffect
  // hydrates it to string | null.
  const [token, setToken] = useState<string | null | undefined>(undefined)
  useEffect(() => {
    setToken(typeof window !== 'undefined' ? window.localStorage.getItem('token') : null)
  }, [])

  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [displayName, setDisplayName] = useState('')
  const [bio, setBio] = useState('')
  const [avatarUrl, setAvatarUrl] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  // Digest preferences. Backed by /api/v1/settings/digest.
  // Values: 'weekly' | 'daily' | 'off'. Default is 'weekly' for anyone
  // who signed up before the preference column existed.
  const [digestFrequency, setDigestFrequency] = useState<'weekly' | 'daily' | 'off'>('weekly')
  const [notifLoading, setNotifLoading] = useState(true)
  const [notifSaving, setNotifSaving] = useState(false)

  useEffect(() => {
    if (!token) {
      router.push('/login')
      return
    }
    api.me()
      .then((data: any) => {
        setProfile(data)
        setDisplayName(data?.displayName ?? data?.display_name ?? '')
        setBio(data?.bio ?? '')
        setAvatarUrl(data?.avatarUrl ?? data?.avatar_url ?? '')
        if (data?.id) {
          localStorage.setItem('userId', data.id)
        }
      })
      .catch((err: any) => setError(err.message ?? 'Failed to load profile'))
      .finally(() => setLoading(false))
  }, [token, router])

  // Load digest frequency from /api/v1/settings/digest.
  useEffect(() => {
    if (!token) { setNotifLoading(false); return }
    ;(api as any)
      .getDigestPrefs?.()
      ?.then?.((data: any) => {
        const f = data?.frequency
        if (f === 'weekly' || f === 'daily' || f === 'off') setDigestFrequency(f)
      })
      .catch(() => {})
      .finally(() => setNotifLoading(false))
  }, [token])

  const handleDigestChange = async (value: 'weekly' | 'daily' | 'off') => {
    setNotifSaving(true)
    const prev = digestFrequency
    setDigestFrequency(value)
    try {
      await (api as any).updateDigestPrefs({ frequency: value })
    } catch {
      setDigestFrequency(prev) // Revert on failure
    } finally {
      setNotifSaving(false)
    }
  }

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError(null)
    setSuccess(false)
    try {
      await api.updateProfile({ display_name: displayName, bio, avatar_url: avatarUrl })
      setSuccess(true)
      setTimeout(() => setSuccess(false), 3000)
    } catch (err: any) {
      setError(err.message ?? 'Failed to save settings')
    } finally {
      setSaving(false)
    }
  }

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return '\u2014'
    try {
      return new Date(dateStr).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      })
    } catch {
      return dateStr
    }
  }

  if (!token) return null

  return (
    <div className="lf-narrow">
      <div style={{ marginBottom: 28 }}>
        <div
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            color: 'var(--lf-muted)',
            letterSpacing: '0.06em',
            textTransform: 'uppercase',
            marginBottom: 6,
          }}
        >
          Account
        </div>
        <h1
          style={{
            fontFamily: 'var(--lf-font-display)',
            fontWeight: 800,
            fontSize: 36,
            letterSpacing: '-0.03em',
            color: 'var(--lf-ink)',
            lineHeight: 1.05,
            margin: 0,
          }}
        >
          Settings
        </h1>
      </div>

      {loading && (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '80px 0' }}>
          <div
            className="animate-spin"
            style={{ height: 32, width: 32, borderRadius: 999, borderWidth: 2, borderStyle: 'solid', borderColor: 'var(--lf-rule-soft)', borderTopColor: 'var(--lf-ink)' }}
          />
        </div>
      )}

      {!loading && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--lf-space-6)' }}>
          {/* Profile form */}
          <LFSurface padding={24} style={{ marginBottom: 16 }}>
            <h2
              style={{
                fontFamily: 'var(--lf-font-display)',
                fontWeight: 800,
                fontSize: 18,
                letterSpacing: '-0.02em',
                margin: '0 0 16px',
                color: 'var(--lf-ink)',
              }}
            >
              Profile
            </h2>
            <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--lf-space-4)' }}>
              {/* Avatar preview */}
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--lf-space-4)' }}>
                {avatarUrl ? (
                  <img
                    src={avatarUrl}
                    alt={displayName}
                    style={{ height: 56, width: 56, borderRadius: 999, objectFit: 'cover', border: '1px solid var(--lf-rule-soft)' }}
                  />
                ) : (
                  <div
                    className="lf-text-h3"
                    style={{ display: 'flex', height: 56, width: 56, alignItems: 'center', justifyContent: 'center', borderRadius: 999, fontWeight: 700, color: '#fff', background: 'var(--lf-ink)' }}
                  >
                    {displayName ? displayName[0].toUpperCase() : 'U'}
                  </div>
                )}
                <div style={{ flex: 1 }}>
                  <label
                    htmlFor="avatarUrl"
                    className="lf-text-caption"
                    style={{ color: 'var(--lf-muted)', fontWeight: 500 }}
                  >
                    Avatar URL
                  </label>
                  <LFInput
                    id="avatarUrl"
                    type="url"
                    value={avatarUrl}
                    onChange={(e) => setAvatarUrl(e.target.value)}
                    placeholder="https://example.com/avatar.png"
                    style={{ marginTop: 4 }}
                  />
                </div>
              </div>

              {/* Display Name */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <label
                  htmlFor="displayName"
                  className="lf-text-body-sm"
                  style={{ color: 'var(--lf-muted)', fontWeight: 500 }}
                >
                  Display Name
                </label>
                <LFInput
                  id="displayName"
                  type="text"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder="Your name"
                />
              </div>

              {/* Bio */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <label
                  htmlFor="bio"
                  className="lf-text-body-sm"
                  style={{ color: 'var(--lf-muted)', fontWeight: 500 }}
                >
                  Bio
                </label>
                <LFTextarea
                  id="bio"
                  value={bio}
                  onChange={(e) => setBio(e.target.value)}
                  placeholder="Tell the community about yourself..."
                  rows={4}
                />
              </div>

              {error && (
                <div className="lf-text-body-sm" style={{ borderRadius: 'var(--lf-radius-sm)', padding: '12px 16px', border: '1px solid color-mix(in srgb, var(--lf-rose) 30%, transparent)', background: 'color-mix(in srgb, var(--lf-rose) 10%, transparent)', color: 'var(--lf-rose)' }}>
                  {error}
                </div>
              )}

              {success && (
                <div className="lf-text-body-sm" style={{ borderRadius: 'var(--lf-radius-sm)', padding: '12px 16px', border: '1px solid color-mix(in srgb, var(--lf-seal) 30%, transparent)', background: 'color-mix(in srgb, var(--lf-seal) 10%, transparent)', color: 'var(--lf-seal)' }}>
                  Profile updated successfully.
                </div>
              )}

              <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                <LFButton
                  type="submit"
                  variant="accent"
                  disabled={saving}
                >
                  {saving ? 'Saving…' : <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>Save changes <IconArrowRight size={13} /></span>}
                </LFButton>
              </div>
            </form>
          </LFSurface>

          {/* Connected tools */}
          <LFSurface padding={24} style={{ marginBottom: 16 }}>
            <BYOKAgentsSection />
          </LFSurface>

          {/* Account info (read-only) */}
          <LFSurface padding={24} style={{ marginBottom: 16 }}>
            <h2
              style={{
                fontFamily: 'var(--lf-font-display)',
                fontWeight: 800,
                fontSize: 18,
                letterSpacing: '-0.02em',
                margin: '0 0 16px',
                color: 'var(--lf-ink)',
              }}
            >
              Account Details
            </h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--lf-space-3)' }}>
              {profile?.email && (
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', padding: '8px 0', borderBottom: '1px solid var(--lf-rule-soft)' }}>
                  <span className="lf-text-body-sm" style={{ color: 'var(--lf-muted)' }}>
                    Email
                  </span>
                  <span className="lf-text-body-sm" style={{ color: 'var(--lf-ink)', wordBreak: 'break-all', textAlign: 'right' }}>
                    {profile.email}
                  </span>
                </div>
              )}
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '8px 0', borderBottom: '1px solid var(--lf-rule-soft)' }}>
                <span className="lf-text-body-sm" style={{ color: 'var(--lf-muted)' }}>
                  Member since
                </span>
                <span className="lf-text-body-sm" style={{ color: 'var(--lf-ink)' }}>
                  {formatDate(profile?.createdAt)}
                </span>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '8px 0' }}>
                <span className="lf-text-body-sm" style={{ color: 'var(--lf-muted)' }}>
                  My tools
                </span>
                <Link
                  href="/my-agents"
                  style={{ color: 'var(--lf-accent-3)', fontFamily: 'var(--lf-font-mono)', fontSize: 10, letterSpacing: '0.1em', textTransform: 'uppercase', textDecoration: 'none', display: 'inline-flex', alignItems: 'center', gap: 6 }}
                >
                  Manage tools <IconArrowRight size={12} />
                </Link>
              </div>
            </div>
          </LFSurface>

          {/* Privacy & visibility — blocks + mutes */}
          <LFSurface padding={24} style={{ marginBottom: 16 }}>
            <BlocksAndMutesSection />
          </LFSurface>

          {/* Privacy & data — GDPR export + delete */}
          <LFSurface padding={24} style={{ marginBottom: 16 }}>
            <PrivacyDataSection />
          </LFSurface>

          {/* Notifications */}
          <LFSurface padding={24} style={{ marginBottom: 16 }}>
            <h2
              style={{
                fontFamily: 'var(--lf-font-display)',
                fontWeight: 800,
                fontSize: 18,
                letterSpacing: '-0.02em',
                margin: '0 0 16px',
                color: 'var(--lf-ink)',
              }}
            >
              Notifications
            </h2>
            {notifLoading ? (
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '24px 0' }}>
                <div
                  className="animate-spin"
                  style={{ height: 20, width: 20, borderRadius: 999, borderWidth: 2, borderStyle: 'solid', borderColor: 'var(--lf-rule-soft)', borderTopColor: 'var(--lf-ink)' }}
                />
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--lf-space-3)' }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 'var(--lf-space-16)', padding: '8px 0' }}>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    <span className="lf-text-body-sm" style={{ color: 'var(--lf-ink)', fontWeight: 500 }}>
                      Email digest
                    </span>
                    <span className="lf-text-caption" style={{ color: 'var(--lf-muted)' }}>
                      A summary of the top posts since you last checked in.
                      Pick a cadence or turn it off entirely.
                    </span>
                  </div>
                  <div style={{ display: 'flex', border: 'var(--lf-border-w) solid var(--lf-ink)', borderRadius: 'var(--lf-radius-sm)', overflow: 'hidden', flexShrink: 0 }}>
                    {(['weekly', 'daily', 'off'] as const).map((opt) => {
                      const active = digestFrequency === opt
                      return (
                        <button
                          key={opt}
                          onClick={() => handleDigestChange(opt)}
                          disabled={notifSaving}
                          style={{
                            padding: '6px 12px',
                            fontFamily: 'var(--lf-font-mono)',
                            fontSize: 10,
                            letterSpacing: '0.12em',
                            textTransform: 'uppercase',
                            background: active ? 'var(--lf-ink)' : 'var(--lf-paper)',
                            color: active ? 'var(--lf-paper)' : 'var(--lf-ink)',
                            border: 'none',
                            borderRight: opt !== 'off' ? 'var(--lf-border-w) solid var(--lf-ink)' : 'none',
                            cursor: notifSaving ? 'wait' : 'pointer',
                            opacity: notifSaving && !active ? 0.5 : 1,
                          }}
                        >
                          {opt}
                        </button>
                      )
                    })}
                  </div>
                </div>
                <PushToggle />
              </div>
            )}
          </LFSurface>
        </div>
      )}
    </div>
  )
}
