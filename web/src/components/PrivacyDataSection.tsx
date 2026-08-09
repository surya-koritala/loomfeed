'use client'

import { useEffect, useState } from 'react'
import { api } from '../api/client'

interface AccountStatus {
  pending: boolean
  pending_deletion_at?: string
  hard_delete_at?: string
}

function formatLocal(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString(undefined, {
    weekday: 'short',
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export default function PrivacyDataSection() {
  const [status, setStatus] = useState<AccountStatus | null>(null)
  const [exporting, setExporting] = useState(false)
  const [showDelete, setShowDelete] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [info, setInfo] = useState<string | null>(null)

  const refreshStatus = async () => {
    try {
      const s = (await api.getAccountStatus()) as AccountStatus
      setStatus(s)
    } catch {
      // ignore — status surface is best-effort
    }
  }

  useEffect(() => {
    refreshStatus()
  }, [])

  const handleExport = async () => {
    setExporting(true)
    setError(null)
    try {
      const { filename, blob } = await api.exportAccountData()
      // Trigger a browser download.
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
      setInfo(`Downloaded ${filename}.`)
      setTimeout(() => setInfo(null), 4000)
    } catch (e: any) {
      setError(e?.message ?? 'Export failed')
    } finally {
      setExporting(false)
    }
  }

  const handleCancelDelete = async () => {
    setError(null)
    try {
      await api.cancelAccountDelete()
      setStatus({ pending: false })
      setInfo('Deletion cancelled.')
      setTimeout(() => setInfo(null), 4000)
    } catch (e: any) {
      setError(e?.message ?? 'Failed to cancel deletion')
    }
  }

  return (
    <div>
      <h2
        style={{
          fontFamily: 'var(--lf-font-display)',
          fontWeight: 800,
          fontSize: 18,
          letterSpacing: '-0.02em',
          margin: '0 0 4px',
          color: 'var(--lf-ink)',
        }}
      >
        Privacy & data
      </h2>
      <p
        style={{
          fontFamily: 'var(--lf-font-body)',
          fontSize: 13,
          color: 'var(--lf-muted)',
          margin: '0 0 16px',
          lineHeight: 1.5,
        }}
      >
        Download a copy of every post, comment, vote, bookmark, and
        subscription tied to your account, or close it for good.
      </p>

      {error && (
        <div
          style={{
            borderRadius: 'var(--lf-radius-sm)',
            border: '1px solid color-mix(in srgb, var(--lf-rose) 30%, transparent)',
            background: 'color-mix(in srgb, var(--lf-rose) 6%, transparent)',
            padding: '10px 12px',
            color: 'var(--lf-accent-2)',
            marginBottom: 12,
            fontSize: 13,
            fontFamily: 'var(--lf-font-body)',
          }}
        >
          {error}
        </div>
      )}
      {info && (
        <div
          style={{
            borderRadius: 'var(--lf-radius-sm)',
            border: '1px solid color-mix(in srgb, var(--lf-seal) 30%, transparent)',
            background: 'color-mix(in srgb, var(--lf-seal) 6%, transparent)',
            padding: '10px 12px',
            color: 'var(--lf-seal)',
            marginBottom: 12,
            fontSize: 13,
            fontFamily: 'var(--lf-font-body)',
          }}
        >
          {info}
        </div>
      )}

      {/* Pending-deletion banner */}
      {status?.pending && (
        <div
          style={{
            border: '1px solid var(--lf-warn, #D97706)',
            background: 'color-mix(in srgb, var(--lf-warn, #D97706) 8%, transparent)',
            borderRadius: 'var(--lf-radius-sm)',
            padding: '14px 16px',
            marginBottom: 18,
          }}
        >
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.12em',
              textTransform: 'uppercase',
              color: 'var(--lf-warn, #D97706)',
              marginBottom: 6,
              fontWeight: 700,
            }}
          >
            Account scheduled for deletion
          </div>
          <p style={{ fontSize: 14, color: 'var(--lf-ink)', margin: '0 0 10px', lineHeight: 1.5 }}>
            Your account will be permanently anonymized on{' '}
            <strong>{formatLocal(status.hard_delete_at)}</strong>. Posts and
            comments will keep their content but the author will show as{' '}
            <em>[deleted]</em>. Click <strong>Cancel</strong> to keep your
            account, or just log out and log back in within the grace window.
          </p>
          <button onClick={handleCancelDelete} style={primaryBtnStyle}>
            Cancel deletion
          </button>
        </div>
      )}

      {/* Export */}
      <div style={rowStyle}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontFamily: 'var(--lf-font-body)', fontSize: 14, fontWeight: 600, color: 'var(--lf-ink)' }}>
            Export my data
          </div>
          <div style={{ fontFamily: 'var(--lf-font-body)', fontSize: 12, color: 'var(--lf-muted)', marginTop: 2 }}>
            JSON file with everything you&apos;ve created on the platform.
          </div>
        </div>
        <button onClick={handleExport} disabled={exporting} style={ghostBtnStyle}>
          {exporting ? 'Preparing…' : 'Download'}
        </button>
      </div>

      {/* Delete */}
      <div style={rowStyle}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontFamily: 'var(--lf-font-body)', fontSize: 14, fontWeight: 600, color: 'var(--lf-ink)' }}>
            Delete my account
          </div>
          <div style={{ fontFamily: 'var(--lf-font-body)', fontSize: 12, color: 'var(--lf-muted)', marginTop: 2 }}>
            7-day grace period. Logging back in cancels.
          </div>
        </div>
        {!status?.pending && (
          <button onClick={() => setShowDelete(true)} style={dangerBtnStyle}>
            Delete account
          </button>
        )}
      </div>

      {showDelete && (
        <DeleteAccountModal
          onClose={() => setShowDelete(false)}
          onSuccess={(s) => {
            setStatus(s)
            setShowDelete(false)
          }}
        />
      )}
    </div>
  )
}

function DeleteAccountModal({
  onClose,
  onSuccess,
}: {
  onClose: () => void
  onSuccess: (s: AccountStatus) => void
}) {
  const [confirm, setConfirm] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const canSubmit = confirm.trim().toUpperCase() === 'DELETE' && password.length > 0 && !submitting

  const handleSubmit = async () => {
    if (!canSubmit) return
    setSubmitting(true)
    setError(null)
    try {
      const resp = (await api.scheduleAccountDelete({ password, confirm: 'DELETE' })) as AccountStatus
      onSuccess({ ...resp, pending: true })
    } catch (e: any) {
      setError(e?.message ?? 'Failed to schedule deletion')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(10, 10, 10, 0.35)',
        backdropFilter: 'blur(4px)',
        WebkitBackdropFilter: 'blur(4px)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: 'var(--lf-paper)',
          border: '1px solid var(--lf-ink)',
          borderRadius: 'var(--lf-radius)',
          maxWidth: 460,
          width: '100%',
          padding: 20,
          boxShadow: 'var(--lf-shadow-hard-md, 0 8px 32px rgba(0,0,0,0.2))',
        }}
      >
        <h3
          style={{
            fontFamily: 'var(--lf-font-display)',
            fontWeight: 800,
            fontSize: 20,
            letterSpacing: '-0.02em',
            margin: '0 0 6px',
            color: 'var(--lf-ink)',
          }}
        >
          Delete account
        </h3>
        <p
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontSize: 13,
            color: 'var(--lf-muted)',
            margin: '0 0 14px',
            lineHeight: 1.5,
          }}
        >
          7 days from now, your profile, posts, and comments will show as
          <em> [deleted]</em>. Until then, logging in cancels the request.
          Type <strong>DELETE</strong> in the box below and re-enter your
          password to proceed.
        </p>

        <label
          style={{
            display: 'block',
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            letterSpacing: '0.12em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
            marginBottom: 4,
          }}
        >
          Type DELETE to confirm
        </label>
        <input
          type="text"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          autoFocus
          style={inputStyle}
          placeholder="DELETE"
        />

        <label
          style={{
            display: 'block',
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            letterSpacing: '0.12em',
            textTransform: 'uppercase',
            color: 'var(--lf-muted)',
            marginBottom: 4,
            marginTop: 12,
          }}
        >
          Re-enter your password
        </label>
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          style={inputStyle}
        />

        {error && (
          <div
            style={{
              marginTop: 10,
              fontSize: 12,
              color: 'var(--lf-accent-2)',
              fontFamily: 'var(--lf-font-body)',
            }}
          >
            {error}
          </div>
        )}

        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 18 }}>
          <button onClick={onClose} disabled={submitting} style={ghostBtnStyle}>
            Cancel
          </button>
          <button onClick={handleSubmit} disabled={!canSubmit} style={dangerBtnStyle}>
            {submitting ? 'Scheduling…' : 'Schedule deletion'}
          </button>
        </div>
      </div>
    </div>
  )
}

const rowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 12,
  flexWrap: 'wrap',
  padding: '12px 14px',
  border: '1px solid var(--lf-rule-soft)',
  borderRadius: 'var(--lf-radius-sm)',
  background: 'var(--lf-paper-alt)',
  marginBottom: 8,
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '9px 12px',
  border: '1px solid var(--lf-ink)',
  borderRadius: 'var(--lf-radius-sm)',
  background: 'var(--lf-paper)',
  color: 'var(--lf-ink)',
  fontFamily: 'var(--lf-font-body)',
  fontSize: 14,
  outline: 'none',
  boxSizing: 'border-box',
}

const ghostBtnStyle: React.CSSProperties = {
  padding: '8px 14px',
  fontFamily: 'var(--lf-font-mono)',
  fontSize: 11,
  letterSpacing: '0.1em',
  textTransform: 'uppercase',
  background: 'var(--lf-paper)',
  color: 'var(--lf-ink)',
  border: '1px solid var(--lf-ink)',
  borderRadius: 'var(--lf-radius-sm)',
  cursor: 'pointer',
  fontWeight: 600,
}

const primaryBtnStyle: React.CSSProperties = {
  padding: '8px 14px',
  fontFamily: 'var(--lf-font-mono)',
  fontSize: 11,
  letterSpacing: '0.1em',
  textTransform: 'uppercase',
  background: 'var(--lf-accent)',
  color: 'var(--lf-ink)',
  border: '1px solid var(--lf-ink)',
  borderRadius: 'var(--lf-radius-sm)',
  cursor: 'pointer',
  fontWeight: 700,
}

const dangerBtnStyle: React.CSSProperties = {
  padding: '8px 14px',
  fontFamily: 'var(--lf-font-mono)',
  fontSize: 11,
  letterSpacing: '0.1em',
  textTransform: 'uppercase',
  background: 'var(--lf-warn, #D97706)',
  color: 'var(--lf-ink)',
  border: '1px solid var(--lf-ink)',
  borderRadius: 'var(--lf-radius-sm)',
  cursor: 'pointer',
  fontWeight: 700,
}
