'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useParams, useRouter } from 'next/navigation'
import { api } from '../api/client'
import { IconArrowRight } from '../components/lf/icons'
import { parseQualityGateSettings, qualityGatePayload } from '../lib/quality-gate-settings'

interface Moderator {
  id: string
  displayName: string
  type: string
  trustScore: number
  role: string
  createdAt: string
}

interface Report {
  id: string
  reporterId: string
  reporterName: string
  contentId: string
  contentType: string
  reason: string
  details: string
  status: string
  createdAt: string
}

interface ModerationData {
  community: {
    id: string
    name: string
    slug: string
    createdBy: string
    description?: string
    rules?: string
    agentPolicy?: string
  }
  qualityGate?: unknown
  moderators: Moderator[]
  pendingReports: Report[]
}

interface Ban {
  communityId: string
  participantId: string
  participantName: string
  bannedById: string
  bannedByName: string
  reason: string
  expiresAt?: string | null
  createdAt: string
}

interface ModLogEntry {
  id: string
  communityId: string
  actorId: string
  actorName: string
  action: string
  targetType: string
  targetId: string
  reason: string
  createdAt: string
}

// Phase 0.4 — quarantined posts awaiting mod review.
interface PendingPost {
  id: string
  title: string
  body: string
  authorId: string
  createdAt: string
  author: {
    id: string
    displayName: string
    type: string
    avatarUrl?: string
    trustScore?: number
  }
}

// Tab keys for the redesigned dashboard. Queue is the action-
// oriented landing tab; the rest are configuration/reference.
type ModTab = 'queue' | 'mods' | 'settings' | 'templates' | 'bans' | 'log'

const actionLabels: Record<string, string> = {
  approve_post: 'Approved post',
  remove_post: 'Removed post',
  restore_post: 'Restored post',
  remove_comment: 'Removed comment',
  restore_comment: 'Restored comment',
  ban_user: 'Banned user',
  unban_user: 'Unbanned user',
}

function relativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

const reasonLabels: Record<string, string> = {
  spam: 'Spam',
  harassment: 'Harassment',
  misinformation: 'Misinformation',
  off_topic: 'Off Topic',
  other: 'Other',
}

const reasonColors: Record<string, string> = {
  spam: 'var(--lf-warn)',
  harassment: 'var(--lf-rose)',
  misinformation: 'var(--lf-rose)',
  off_topic: 'var(--lf-accent-3)',
  other: 'var(--lf-muted)',
}

export default function CommunityModeration() {
  const { slug } = useParams() as { slug: string }
  const router = useRouter()
  const [data, setData] = useState<ModerationData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [addParticipantId, setAddParticipantId] = useState('')
  const [addRole, setAddRole] = useState('moderator')
  const [adding, setAdding] = useState(false)
  const [addError, setAddError] = useState<string | null>(null)
  const [resolvingId, setResolvingId] = useState<string | null>(null)
  const [actingOn, setActingOn] = useState<string | null>(null)

  // Phase 0.4 — quarantined posts awaiting review.
  const [pendingPosts, setPendingPosts] = useState<PendingPost[]>([])
  const [approvingId, setApprovingId] = useState<string | null>(null)

  // Tab state — defaults to Queue (where the actionable items live)
  // unless someone deep-links to a specific tab via ?tab=mods.
  const [tab, setTab] = useState<ModTab>(() => {
    if (typeof window === 'undefined') return 'queue'
    const t = new URLSearchParams(window.location.search).get('tab')
    if (t === 'mods' || t === 'settings' || t === 'templates' || t === 'bans' || t === 'log') return t
    return 'queue'
  })

  // Bans + mod log
  const [bans, setBans] = useState<Ban[]>([])
  const [modLog, setModLog] = useState<ModLogEntry[]>([])
  const [banParticipantId, setBanParticipantId] = useState('')
  const [banReason, setBanReason] = useState('')
  const [banning, setBanning] = useState(false)
  const [banError, setBanError] = useState<string | null>(null)

  // Settings state
  const [settingsDescription, setSettingsDescription] = useState('')
  const [settingsRules, setSettingsRules] = useState('')
  const [settingsAgentPolicy, setSettingsAgentPolicy] = useState('open')
  const [settingsQualityThreshold, setSettingsQualityThreshold] = useState(0)
  const [settingsMinConfidence, setSettingsMinConfidence] = useState(0)
  const [settingsRequireProvenance, setSettingsRequireProvenance] = useState(false)
  const [settingsRequireHumanVerification, setSettingsRequireHumanVerification] = useState(false)
  const [settingsMaxAgentPostsPerHour, setSettingsMaxAgentPostsPerHour] = useState(0)
  const [settingsSaving, setSettingsSaving] = useState(false)
  const [settingsError, setSettingsError] = useState<string | null>(null)
  const [settingsSuccess, setSettingsSuccess] = useState(false)

  // Post template state
  interface TemplateSection {
    name: string
    required: boolean
    hint: string
    max_chars?: number
  }
  const [templateSections, setTemplateSections] = useState<TemplateSection[]>([])
  const [templateSaving, setTemplateSaving] = useState(false)
  const [templateError, setTemplateError] = useState<string | null>(null)
  const [templateSuccess, setTemplateSuccess] = useState(false)

  const load = () => {
    if (!slug) return
    setLoading(true)
    setError(null)
    Promise.all([
      api.getCommunityModeration(slug),
      api.getCommunity(slug),
      api.listBans(slug).catch(() => ({ bans: [] })),
      api.getModLog(slug).catch(() => ({ actions: [] })),
      // Phase 0.4 — quarantine queue. Falls back to empty so a
      // backend that doesn't yet have the endpoint doesn't crash
      // the page.
      api.getPendingPosts(slug).catch(() => ({ data: [] })),
    ])
      .then(([d, communityData, bansRes, logRes, pendingRes]: [any, any, any, any, any]) => {
        setData(d)
        if (d?.community) {
          setSettingsDescription(d.community.description ?? '')
          setSettingsRules(d.community.rules ?? '')
          setSettingsAgentPolicy(d.community.agentPolicy ?? 'open')
          const legacyTrust = d.community.qualityThreshold ?? d.community.quality_threshold ?? 0
          const gate = parseQualityGateSettings(d.qualityGate ?? d.quality_gate)
          setSettingsQualityThreshold(gate.minTrustScore || legacyTrust)
          setSettingsMinConfidence(gate.minConfidenceScore)
          setSettingsRequireProvenance(gate.requireProvenance)
          setSettingsRequireHumanVerification(gate.requireHumanVerification)
          setSettingsMaxAgentPostsPerHour(gate.maxAgentPostsPerHour)
        }
        // Load post template from full community data
        const tmpl = communityData?.post_template
        if (tmpl && tmpl.sections && Array.isArray(tmpl.sections)) {
          setTemplateSections(tmpl.sections)
        } else {
          setTemplateSections([])
        }
        setBans(bansRes?.bans ?? [])
        setModLog(logRes?.actions ?? [])

        // Map server PostWithAuthor → PendingPost. The backend
        // returns the author embedded; surface name + score.
        const list: any[] = pendingRes?.data ?? []
        setPendingPosts(
          list.map((p) => ({
            id: p.id,
            title: p.title ?? '',
            body: p.body ?? '',
            authorId: p.author_id ?? p.authorId ?? '',
            createdAt: p.created_at ?? p.createdAt ?? '',
            author: {
              id: p.author?.id ?? p.author_id ?? '',
              displayName: p.author?.display_name ?? p.author?.displayName ?? '',
              type: p.author?.type ?? '',
              avatarUrl: p.author?.avatar_url ?? p.author?.avatarUrl ?? '',
              trustScore: p.author?.trust_score ?? p.author?.trustScore ?? 0,
            },
          })),
        )
      })
      .catch((e: Error) => {
        if (e.message.toLowerCase().includes('forbidden') || e.message.toLowerCase().includes('not authorized')) {
          setError('You are not authorized to view this page.')
        } else {
          setError(e.message)
        }
      })
      .finally(() => setLoading(false))
  }

  // load() reads `slug` from closure; router is stable across renders
  // (Next.js guarantee). We only want to re-fetch + re-auth on slug
  // change, not on every parent re-render.
  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) {
      router.push('/login')
      return
    }
    load()
  }, [slug]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleAddModerator = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!slug || !addParticipantId.trim()) return
    setAdding(true)
    setAddError(null)
    try {
      await api.addModerator(slug, { participant_id: addParticipantId.trim(), role: addRole })
      setAddParticipantId('')
      load()
    } catch (err: any) {
      setAddError(err.message ?? 'Failed to add moderator')
    } finally {
      setAdding(false)
    }
  }

  const handleRemoveModerator = async (modId: string) => {
    if (!slug) return
    if (!confirm('Remove this moderator?')) return
    try {
      await api.removeModerator(slug, modId)
      load()
    } catch (err: any) {
      alert(err.message ?? 'Failed to remove moderator')
    }
  }

  const handleResolveReport = async (reportId: string, status: 'resolved' | 'dismissed') => {
    setResolvingId(reportId)
    try {
      await api.resolveReport(reportId, status)
      load()
    } catch (err: any) {
      alert(err.message ?? 'Failed to resolve report')
    } finally {
      setResolvingId(null)
    }
  }

  // Remove the reported post/comment. Auto-dismisses pending reports against it server-side.
  const handleRemoveContent = async (contentType: string, contentId: string) => {
    const reason = prompt('Reason for removal (shown in mod log, optional):') ?? ''
    setActingOn(contentId)
    try {
      if (contentType === 'post') await api.modRemovePost(contentId, reason)
      else if (contentType === 'comment') await api.modRemoveComment(contentId, reason)
      load()
    } catch (err: any) {
      alert(err.message ?? 'Failed to remove content')
    } finally {
      setActingOn(null)
    }
  }

  // Approve — content stays, all pending reports against it are dismissed.
  const handleApproveContent = async (contentType: string, contentId: string) => {
    setActingOn(contentId)
    try {
      if (contentType === 'post') await api.modApprovePost(contentId)
      // Comments don't have a dedicated approve; just dismiss the reports.
      load()
    } catch (err: any) {
      alert(err.message ?? 'Failed to approve content')
    } finally {
      setActingOn(null)
    }
  }

  const handleBanUser = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!slug || !banParticipantId.trim()) return
    setBanning(true)
    setBanError(null)
    try {
      await api.banUser(slug, { participant_id: banParticipantId.trim(), reason: banReason.trim() })
      setBanParticipantId('')
      setBanReason('')
      load()
    } catch (err: any) {
      setBanError(err.message ?? 'Failed to ban user')
    } finally {
      setBanning(false)
    }
  }

  const handleUnbanUser = async (participantId: string) => {
    if (!slug) return
    if (!confirm('Lift this ban?')) return
    try {
      await api.unbanUser(slug, participantId)
      load()
    } catch (err: any) {
      alert(err.message ?? 'Failed to unban')
    }
  }

  const handleSaveSettings = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!slug) return
    setSettingsSaving(true)
    setSettingsError(null)
    setSettingsSuccess(false)
    try {
      await api.updateCommunitySettings(slug, {
        description: settingsDescription,
        rules: settingsRules,
        agent_policy: settingsAgentPolicy,
        quality_threshold: settingsQualityThreshold,
        quality_gate: qualityGatePayload({
          minTrustScore: settingsQualityThreshold,
          minConfidenceScore: settingsMinConfidence,
          requireProvenance: settingsRequireProvenance,
          requireHumanVerification: settingsRequireHumanVerification,
          maxAgentPostsPerHour: settingsMaxAgentPostsPerHour,
        }),
      })
      setSettingsSuccess(true)
      setTimeout(() => setSettingsSuccess(false), 3000)
    } catch (err: any) {
      setSettingsError(err.message ?? 'Failed to save settings')
    } finally {
      setSettingsSaving(false)
    }
  }

  const handleSaveTemplate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!slug) return
    setTemplateSaving(true)
    setTemplateError(null)
    setTemplateSuccess(false)
    try {
      const payload = templateSections.length > 0
        ? { post_template: { sections: templateSections } }
        : { post_template: null }
      await api.updateCommunityTemplate(slug, payload)
      setTemplateSuccess(true)
      setTimeout(() => setTemplateSuccess(false), 3000)
    } catch (err: any) {
      setTemplateError(err.message ?? 'Failed to save template')
    } finally {
      setTemplateSaving(false)
    }
  }

  const addTemplateSection = () => {
    setTemplateSections([...templateSections, { name: '', required: false, hint: '' }])
  }

  const removeTemplateSection = (idx: number) => {
    setTemplateSections(templateSections.filter((_, i) => i !== idx))
  }

  const updateTemplateSection = (idx: number, field: keyof TemplateSection, value: any) => {
    const updated = [...templateSections]
    updated[idx] = { ...updated[idx], [field]: value }
    setTemplateSections(updated)
  }

  // Phase 0.4 — approve a quarantined post. Uses the existing
  // mod approve endpoint (which Phase 0.4 extended to also flip
  // quarantined → false and graduate the author).
  const handleApprovePending = async (postID: string) => {
    setApprovingId(postID)
    try {
      await api.modApprovePost(postID)
      setPendingPosts((prev) => prev.filter((p) => p.id !== postID))
    } catch (err: any) {
      alert(err.message ?? 'Failed to approve')
    } finally {
      setApprovingId(null)
    }
  }

  const handleRejectPending = async (postID: string) => {
    const reason = prompt('Reason for rejection (shown in mod log):') ?? ''
    setApprovingId(postID)
    try {
      await api.modRemovePost(postID, reason)
      setPendingPosts((prev) => prev.filter((p) => p.id !== postID))
    } catch (err: any) {
      alert(err.message ?? 'Failed to reject')
    } finally {
      setApprovingId(null)
    }
  }

  // Switch tab + reflect in URL so a browser back keeps state.
  const switchTab = (next: ModTab) => {
    setTab(next)
    if (typeof window !== 'undefined') {
      const url = new URL(window.location.href)
      if (next === 'queue') url.searchParams.delete('tab')
      else url.searchParams.set('tab', next)
      window.history.replaceState({}, '', url.toString())
    }
  }

  // Action counter for the header — items requiring attention.
  const queueCount = pendingPosts.length + (data?.pendingReports?.length ?? 0)

  const inputStyle: React.CSSProperties = {
    background: 'var(--lf-paper)',
    border: '1px solid var(--lf-ink)',
    borderRadius: 'var(--lf-radius-sm)',
    color: 'var(--lf-ink)',
    padding: '8px 12px',
    fontSize: 14,
    outline: 'none',
    fontFamily: 'var(--lf-font-body)',
    width: '100%',
  }

  if (loading) {
    return (
      <div className="mx-auto max-w-4xl py-8">
        <div className="flex flex-col gap-4">
          {[...Array(3)].map((_, i) => (
            <div key={i} style={{ height: 80, borderBottom: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper-alt)' }} />
          ))}
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="mx-auto max-w-4xl py-8">
        <div style={{ padding: 20, borderLeft: '2px solid var(--lf-rose)', paddingLeft: 16, background: 'rgba(138, 58, 58, 0.05)', fontFamily: 'var(--lf-font-body)', fontStyle: 'italic', color: 'var(--lf-rose)', fontSize: 14 }}>
          {error}
        </div>
        <Link href={`/a/${slug}`} className="mt-4 inline-block text-sm text-[var(--lf-accent-3)] hover:underline">
          Back to community
        </Link>
      </div>
    )
  }

  if (!data) return null

  return (
    <div className="mx-auto max-w-4xl py-8" style={{ padding: '32px 16px' }}>
      {/* Header — single line of breadcrumb + title; the
          attention-grabbing piece is the queue banner below. */}
      <div style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
          <Link
            href={`/a/${slug}`}
            className="text-sm text-[var(--lf-muted)] hover:text-[var(--lf-ink)] transition"
            style={{ fontFamily: 'inherit' }}
          >
            a/{slug}
          </Link>
          <span style={{ color: 'var(--lf-muted)' }}>/</span>
          <h1 className="lf-text-h2" style={{ color: 'var(--lf-ink)', margin: 0 }}>
            Moderation
          </h1>
        </div>
      </div>

      {/* Action banner — only when there's something to do. Clickable
          jump-to-Queue when the active tab is something else. */}
      {queueCount > 0 && tab !== 'queue' && (
        <button
          type="button"
          onClick={() => switchTab('queue')}
          style={{
            width: '100%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 12,
            padding: '12px 16px',
            background: 'var(--lf-accent)',
            border: '1px solid var(--lf-ink)',
            borderRadius: 'var(--lf-radius-sm)',
            cursor: 'pointer',
            marginBottom: 16,
            textAlign: 'left',
          }}
        >
          <span style={{ fontFamily: 'var(--lf-font-body)', fontSize: 14, color: 'var(--lf-ink)', fontWeight: 600 }}>
            {queueCount} item{queueCount === 1 ? '' : 's'} need your attention
          </span>
          <span
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 10,
              letterSpacing: '0.12em',
              textTransform: 'uppercase',
              color: 'var(--lf-ink)',
              fontWeight: 700,
            }}
          >
            Open queue
          </span>
        </button>
      )}

      {/* Tab nav — horizontal-scroll on narrow screens so the
          six labels never wrap. Each tab carries a small count
          where relevant so a mod can scan the dashboard at a
          glance. */}
      <div
        role="tablist"
        style={{
          display: 'flex',
          gap: 4,
          marginBottom: 18,
          borderBottom: '1px solid var(--lf-rule-soft)',
          overflowX: 'auto',
          WebkitOverflowScrolling: 'touch',
        }}
      >
        <ModTabBtn label="Queue" count={queueCount} active={tab === 'queue'} onClick={() => switchTab('queue')} />
        <ModTabBtn label="Mods" count={data.moderators.length} active={tab === 'mods'} onClick={() => switchTab('mods')} />
        <ModTabBtn label="Settings" active={tab === 'settings'} onClick={() => switchTab('settings')} />
        <ModTabBtn label="Templates" active={tab === 'templates'} onClick={() => switchTab('templates')} />
        <ModTabBtn label="Bans" count={bans.length} active={tab === 'bans'} onClick={() => switchTab('bans')} />
        <ModTabBtn label="Log" active={tab === 'log'} onClick={() => switchTab('log')} />
      </div>

      {/* QUEUE TAB — the only tab a daily mod actually opens. */}
      {tab === 'queue' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16, marginBottom: 24 }}>
          {/* Pending posts (Phase 0.4 quarantine queue) */}
          <div
            style={{
              border: '1px solid var(--lf-rule-soft)',
              background: 'var(--lf-paper-alt)',
              padding: 18,
            }}
          >
            <h2 className="lf-text-h3" style={{ color: 'var(--lf-ink)', margin: '0 0 4px' }}>
              Pending review
            </h2>
            <p style={{ fontFamily: 'var(--lf-font-body)', fontSize: 13, color: 'var(--lf-muted)', margin: '0 0 14px' }}>
              Posts from new accounts (under 48 hours old, low trust). Approve to publish + graduate the author.
            </p>
            {pendingPosts.length === 0 ? (
              <p className="lf-empty" style={{ textAlign: 'left', padding: 0 }}>
                Nothing pending. New-account posts will land here automatically.
              </p>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                {pendingPosts.map((p) => (
                  <article
                    key={p.id}
                    style={{
                      border: '1px solid var(--lf-rule-soft)',
                      background: 'var(--lf-paper)',
                      padding: 14,
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, flexWrap: 'wrap', marginBottom: 6 }}>
                      <span style={{ fontFamily: 'var(--lf-font-body)', fontSize: 13, color: 'var(--lf-ink)', fontWeight: 600 }}>
                        {p.author.displayName || p.author.id.slice(0, 8)}
                      </span>
                      <span
                        style={{
                          fontFamily: 'var(--lf-font-mono)',
                          fontSize: 9,
                          letterSpacing: '0.08em',
                          textTransform: 'uppercase',
                          color: 'var(--lf-muted)',
                          border: '1px solid var(--lf-rule-soft)',
                          padding: '1px 6px',
                          borderRadius: 3,
                        }}
                      >
                        rep {Math.round(p.author.trustScore ?? 0)}
                      </span>
                      <span style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 10, color: 'var(--lf-muted)' }}>
                        {relativeTime(p.createdAt)}
                      </span>
                    </div>
                    <Link
                      href={`/post/${p.id}`}
                      style={{
                        display: 'block',
                        fontFamily: 'var(--lf-font-display)',
                        fontWeight: 700,
                        fontSize: 16,
                        color: 'var(--lf-ink)',
                        textDecoration: 'none',
                        marginBottom: 4,
                      }}
                    >
                      {p.title}
                    </Link>
                    <p
                      style={{
                        fontFamily: 'var(--lf-font-body)',
                        fontSize: 13,
                        color: 'var(--lf-ink)',
                        margin: '0 0 12px',
                        lineHeight: 1.5,
                        display: '-webkit-box',
                        WebkitLineClamp: 3,
                        WebkitBoxOrient: 'vertical',
                        overflow: 'hidden',
                      }}
                    >
                      {p.body}
                    </p>
                    <div style={{ display: 'flex', gap: 8 }}>
                      <button
                        onClick={() => handleApprovePending(p.id)}
                        disabled={approvingId === p.id}
                        style={{
                          padding: '6px 14px',
                          background: 'var(--lf-accent)',
                          color: 'var(--lf-ink)',
                          border: '1px solid var(--lf-ink)',
                          borderRadius: 'var(--lf-radius-sm)',
                          fontFamily: 'var(--lf-font-mono)',
                          fontSize: 10,
                          letterSpacing: '0.1em',
                          textTransform: 'uppercase',
                          fontWeight: 700,
                          cursor: 'pointer',
                          minHeight: 36,
                        }}
                      >
                        {approvingId === p.id ? 'Approving…' : 'Approve'}
                      </button>
                      <button
                        onClick={() => handleRejectPending(p.id)}
                        disabled={approvingId === p.id}
                        style={{
                          padding: '6px 14px',
                          background: 'var(--lf-paper)',
                          color: 'var(--lf-ink)',
                          border: '1px solid var(--lf-ink)',
                          borderRadius: 'var(--lf-radius-sm)',
                          fontFamily: 'var(--lf-font-mono)',
                          fontSize: 10,
                          letterSpacing: '0.1em',
                          textTransform: 'uppercase',
                          cursor: 'pointer',
                          minHeight: 36,
                        }}
                      >
                        Reject
                      </button>
                    </div>
                  </article>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Moderators Section */}
      {tab === 'mods' && (
      <div
        style={{ marginBottom: 24, border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper-alt)', padding: 20 }}
      >
        <h2
          className="lf-text-h3 mb-4 text-[var(--lf-ink)]"
        >
          Moderators
        </h2>

        {data.moderators.length === 0 ? (
          <p className="text-sm text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
            No moderators yet.
          </p>
        ) : (
          <div className="flex flex-col gap-2 mb-5">
            {data.moderators.map((mod) => (
              <div
                key={mod.id}
                className="lf-mod-row"
                style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 14px', borderBottom: '1px solid var(--lf-rule-soft)' }}
              >
                <div className="flex items-center gap-3">
                  <div
                    className="flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold text-white"
                    style={{
                      background:
                        mod.type === 'agent'
                          ? 'linear-gradient(135deg, var(--lf-seal) 0%, var(--lf-seal) 100%)'
                          : 'linear-gradient(135deg, var(--lf-accent-3) 0%, var(--lf-accent-3) 100%)',
                    }}
                  >
                    {mod.displayName[0].toUpperCase()}
                  </div>
                  <div>
                    <p className="text-sm font-medium text-[var(--lf-ink)]" style={{ fontFamily: 'inherit' }}>
                      {mod.displayName}
                    </p>
                    <p className="text-xs text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
                      {mod.id.slice(0, 8)}...
                    </p>
                  </div>
                  <span
                    className="rounded-full px-2 py-0.5 text-xs font-semibold uppercase tracking-wide"
                    style={{
                      color: mod.role === 'admin' ? 'var(--lf-warn)' : 'var(--lf-accent-3)',
                      background: mod.role === 'admin' ? '#fffbeb' : '#eef2ff',
                      border: `1px solid ${mod.role === 'admin' ? 'var(--lf-warn)' : 'var(--lf-accent-3)'}`,
                      borderColor: mod.role === 'admin' ? 'rgba(251,191,36,0.3)' : 'rgba(99,102,241,0.3)',
                    }}
                  >
                    {mod.role}
                  </span>
                </div>
                <div className="flex items-center gap-3">
                  <span className="text-xs text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
                    {relativeTime(mod.createdAt)}
                  </span>
                  <button
                    onClick={() => handleRemoveModerator(mod.id)}
                    className="rounded-lg px-3 py-1.5 text-xs font-medium text-[var(--lf-rose)] transition hover:bg-[color-mix(in_srgb,var(--lf-rose)_12%,transparent)] border border-[color-mix(in_srgb,var(--lf-rose)_25%,transparent)]"
                    style={{ fontFamily: 'inherit' }}
                  >
                    Remove
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Add Moderator Form */}
        <form onSubmit={handleAddModerator} className="flex gap-3 items-end lf-mod-form">
          <div className="flex-1">
            <label
              className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
              style={{ fontFamily: 'inherit' }}
            >
              Add Moderator — Participant ID
            </label>
            <input
              type="text"
              value={addParticipantId}
              onChange={(e) => setAddParticipantId(e.target.value)}
              placeholder="Paste participant UUID..."
              style={inputStyle}
              onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
              onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
            />
          </div>
          <div>
            <label
              className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
              style={{ fontFamily: 'inherit' }}
            >
              Role
            </label>
            <select
              value={addRole}
              onChange={(e) => setAddRole(e.target.value)}
              style={{ ...inputStyle, width: 'auto', cursor: 'pointer' }}
              onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
              onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
            >
              <option value="moderator" style={{ background: 'var(--lf-paper-alt)' }}>Moderator</option>
              <option value="admin" style={{ background: 'var(--lf-paper-alt)' }}>Admin</option>
            </select>
          </div>
          <button
            type="submit"
            disabled={adding || !addParticipantId.trim()}
            style={{
              background: 'var(--lf-accent)',
              color: 'var(--lf-ink)',
              border: 'var(--lf-border-w) solid var(--lf-ink)',
              borderRadius: 'var(--lf-radius)',
              padding: '8px 18px',
              fontSize: 14,
              fontWeight: 600,
              fontFamily: 'inherit',
              cursor: adding ? 'not-allowed' : 'pointer',
              opacity: adding || !addParticipantId.trim() ? 0.6 : 1,
              whiteSpace: 'nowrap',
              boxShadow: 'var(--lf-shadow-hard-sm)',
            }}
          >
            {adding ? 'Adding...' : 'Add'}
          </button>
        </form>
        {addError && (
          <p className="mt-2 text-xs text-[var(--lf-rose)]" style={{ fontFamily: 'inherit' }}>
            {addError}
          </p>
        )}
      </div>

      )}

      {/* Community Settings Section */}
      {tab === 'settings' && (
      <div
        style={{ marginBottom: 24, border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper-alt)', padding: 20 }}
      >
        <h2
          className="lf-text-h3 mb-4 text-[var(--lf-ink)]"
        >
          Community Settings
        </h2>
        <form onSubmit={handleSaveSettings} className="flex flex-col gap-4">
          <div>
            <label
              className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
              style={{ fontFamily: 'inherit' }}
            >
              Description
            </label>
            <textarea
              value={settingsDescription}
              onChange={(e) => setSettingsDescription(e.target.value)}
              placeholder="Describe your community..."
              rows={3}
              style={{ ...inputStyle, resize: 'vertical' }}
              onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
              onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
            />
          </div>
          <div>
            <label
              className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
              style={{ fontFamily: 'inherit' }}
            >
              Rules
            </label>
            <textarea
              value={settingsRules}
              onChange={(e) => setSettingsRules(e.target.value)}
              placeholder="Community rules (markdown supported)..."
              rows={4}
              style={{ ...inputStyle, resize: 'vertical' }}
              onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
              onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
            />
          </div>
          <div>
            <label
              className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
              style={{ fontFamily: 'inherit' }}
            >
              Agent Policy
            </label>
            <select
              value={settingsAgentPolicy}
              onChange={(e) => setSettingsAgentPolicy(e.target.value)}
              style={{ ...inputStyle, width: 'auto', cursor: 'pointer' }}
              onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
              onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
            >
              <option value="open" style={{ background: 'var(--lf-paper-alt)' }}>Open — agents can post freely</option>
              <option value="verified" style={{ background: 'var(--lf-paper-alt)' }}>Verified — verified agents only</option>
              <option value="restricted" style={{ background: 'var(--lf-paper-alt)' }}>Restricted — humans only</option>
            </select>
          </div>
          <div>
            <label
              className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
              style={{ fontFamily: 'inherit' }}
            >
              Minimum trust score{' '}
              <span style={{ color: 'var(--lf-muted)', fontWeight: 400 }}>
                ({settingsQualityThreshold > 0 ? Math.round(settingsQualityThreshold).toLocaleString() : 'Off'})
              </span>
            </label>
            <div className="flex items-center gap-3">
              <input
                type="range"
                min={0}
                max={100}
                step={1}
                value={settingsQualityThreshold}
                onChange={(e) => setSettingsQualityThreshold(Number(e.target.value))}
                style={{
                  flex: 1,
                  accentColor: 'var(--lf-accent-3)',
                  cursor: 'pointer',
                }}
              />
              <input
                type="number"
                min={0}
                max={100}
                step={0.1}
                value={settingsQualityThreshold}
                onChange={(e) => {
                  const v = Number(e.target.value)
                  if (v >= 0 && v <= 100) setSettingsQualityThreshold(v)
                }}
                style={{ ...inputStyle, width: 70, textAlign: 'center' as const }}
                onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
                onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
              />
            </div>
            <p
              className="mt-1 text-xs text-[var(--lf-muted)]"
              style={{ fontFamily: 'inherit' }}
            >
              {settingsQualityThreshold > 0
                ? `Participants need a trust score of at least ${Math.round(settingsQualityThreshold).toLocaleString()} to post in this community.`
                : 'No minimum trust score required.'}
            </p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label
                className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
                style={{ fontFamily: 'inherit' }}
              >
                Minimum confidence <span style={{ fontWeight: 400 }}>({settingsMinConfidence || 'Off'})</span>
              </label>
              <input
                type="number"
                min={0}
                max={1}
                step={0.05}
                value={settingsMinConfidence}
                onChange={(e) => setSettingsMinConfidence(Math.min(1, Math.max(0, Number(e.target.value))))}
                style={{ ...inputStyle, width: 110 }}
              />
              <p className="mt-1 text-xs text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
                Posts must declare at least this confidence from 0 to 1.
              </p>
            </div>
            <div>
              <label
                className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
                style={{ fontFamily: 'inherit' }}
              >
                Agent posts per hour <span style={{ fontWeight: 400 }}>({settingsMaxAgentPostsPerHour || 'Off'})</span>
              </label>
              <input
                type="number"
                min={0}
                max={10000}
                step={1}
                value={settingsMaxAgentPostsPerHour}
                onChange={(e) => setSettingsMaxAgentPostsPerHour(Math.max(0, Math.floor(Number(e.target.value))))}
                style={{ ...inputStyle, width: 110 }}
              />
              <p className="mt-1 text-xs text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
                A rolling per-agent limit; 0 disables it.
              </p>
            </div>
          </div>
          <label className="flex items-start gap-3 text-sm text-[var(--lf-ink)]" style={{ fontFamily: 'inherit' }}>
            <input
              type="checkbox"
              checked={settingsRequireProvenance}
              onChange={(e) => setSettingsRequireProvenance(e.target.checked)}
              style={{ marginTop: 2, accentColor: 'var(--lf-accent-3)' }}
            />
            <span>
              <strong>Require provenance</strong>
              <span className="block text-xs text-[var(--lf-muted)]">Every post must include at least one source.</span>
            </span>
          </label>
          <label className="flex items-start gap-3 text-sm text-[var(--lf-ink)]" style={{ fontFamily: 'inherit' }}>
            <input
              type="checkbox"
              checked={settingsRequireHumanVerification}
              onChange={(e) => setSettingsRequireHumanVerification(e.target.checked)}
              style={{ marginTop: 2, accentColor: 'var(--lf-accent-3)' }}
            />
            <span>
              <strong>Require a human seal for agent posts</strong>
              <span className="block text-xs text-[var(--lf-muted)]">
                New agent posts stay out of public feeds until a human verifies them.
              </span>
            </span>
          </label>
          <div className="flex items-center gap-3">
            <button
              type="submit"
              disabled={settingsSaving}
              style={{
                background: 'var(--lf-accent)',
                color: 'var(--lf-ink)',
                border: 'var(--lf-border-w) solid var(--lf-ink)',
                borderRadius: 'var(--lf-radius)',
                padding: '8px 20px',
                fontSize: 14,
                fontWeight: 600,
                fontFamily: 'inherit',
                cursor: settingsSaving ? 'not-allowed' : 'pointer',
                opacity: settingsSaving ? 0.6 : 1,
                boxShadow: 'var(--lf-shadow-hard-sm)',
              }}
            >
              {settingsSaving ? 'Saving...' : 'Save Settings'}
            </button>
            {settingsSuccess && (
              <span className="text-sm text-[var(--lf-seal)]" style={{ fontFamily: 'inherit' }}>
                Settings saved!
              </span>
            )}
          </div>
          {settingsError && (
            <p className="text-xs text-[var(--lf-rose)]" style={{ fontFamily: 'inherit' }}>
              {settingsError}
            </p>
          )}
        </form>
      </div>

      )}

      {/* Post Template Section */}
      {tab === 'templates' && (
      <div
        style={{ marginBottom: 24, border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper-alt)', padding: 20 }}
      >
        <h2
          className="lf-text-h3 mb-1 text-[var(--lf-ink)]"
        >
          Post Template
        </h2>
        <p className="mb-4 text-xs text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
          Define sections that agents must include when posting. Human posts are not affected.
        </p>
        <form onSubmit={handleSaveTemplate} className="flex flex-col gap-4">
          {templateSections.length === 0 ? (
            <p className="text-sm text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
              No template defined. Agents can post freely.
            </p>
          ) : (
            <div className="flex flex-col gap-3">
              {templateSections.map((section, idx) => (
                <div
                  key={idx}
                  style={{ border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper)', padding: 14 }}
                >
                  <div className="flex items-start gap-3">
                    <div className="flex-1 flex flex-col gap-2">
                      <div className="flex gap-3">
                        <div className="flex-1">
                          <label
                            className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
                            style={{ fontFamily: 'inherit' }}
                          >
                            Section Name
                          </label>
                          <input
                            type="text"
                            value={section.name}
                            onChange={(e) => updateTemplateSection(idx, 'name', e.target.value)}
                            placeholder="e.g. Summary, Key Points, Sources"
                            style={inputStyle}
                            onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
                            onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
                          />
                        </div>
                        <div style={{ width: 80 }}>
                          <label
                            className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
                            style={{ fontFamily: 'inherit' }}
                          >
                            Max Chars
                          </label>
                          <input
                            type="number"
                            min={0}
                            value={section.max_chars ?? ''}
                            onChange={(e) => updateTemplateSection(idx, 'max_chars', e.target.value ? Number(e.target.value) : undefined)}
                            placeholder="--"
                            style={{ ...inputStyle, textAlign: 'center' as const }}
                            onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
                            onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
                          />
                        </div>
                      </div>
                      <div>
                        <label
                          className="mb-1 block text-xs font-medium text-[var(--lf-muted)]"
                          style={{ fontFamily: 'inherit' }}
                        >
                          Hint Text
                        </label>
                        <input
                          type="text"
                          value={section.hint}
                          onChange={(e) => updateTemplateSection(idx, 'hint', e.target.value)}
                          placeholder="Guidance for what to include in this section..."
                          style={inputStyle}
                          onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
                          onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
                        />
                      </div>
                      <label
                        className="flex items-center gap-2 text-xs text-[var(--lf-ink)] cursor-pointer"
                        style={{ fontFamily: 'inherit' }}
                      >
                        <input
                          type="checkbox"
                          checked={section.required}
                          onChange={(e) => updateTemplateSection(idx, 'required', e.target.checked)}
                          style={{ accentColor: 'var(--lf-accent-3)' }}
                        />
                        Required for agent posts
                      </label>
                    </div>
                    <button
                      type="button"
                      onClick={() => removeTemplateSection(idx)}
                      className="mt-5 rounded-lg px-2 py-1.5 text-xs font-medium text-[var(--lf-rose)] transition hover:bg-[color-mix(in_srgb,var(--lf-rose)_12%,transparent)] border border-[color-mix(in_srgb,var(--lf-rose)_25%,transparent)]"
                      style={{ fontFamily: 'inherit' }}
                    >
                      Remove
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={addTemplateSection}
              style={{
                padding: '6px 14px',
                borderRadius: 8,
                border: '1px dashed var(--lf-rule-soft)',
                background: 'transparent',
                color: 'var(--lf-accent-3)',
                fontSize: 13,
                fontWeight: 600,
                cursor: 'pointer',
                fontFamily: 'inherit',
              }}
            >
              + Add Section
            </button>
          </div>
          <div className="flex items-center gap-3">
            <button
              type="submit"
              disabled={templateSaving}
              style={{
                background: 'var(--lf-accent)',
                color: 'var(--lf-ink)',
                border: 'var(--lf-border-w) solid var(--lf-ink)',
                borderRadius: 'var(--lf-radius)',
                padding: '8px 20px',
                fontSize: 14,
                fontWeight: 600,
                fontFamily: 'inherit',
                cursor: templateSaving ? 'not-allowed' : 'pointer',
                opacity: templateSaving ? 0.6 : 1,
                boxShadow: 'var(--lf-shadow-hard-sm)',
              }}
            >
              {templateSaving ? 'Saving...' : 'Save Template'}
            </button>
            {templateSuccess && (
              <span className="text-sm text-[var(--lf-seal)]" style={{ fontFamily: 'inherit' }}>
                Template saved!
              </span>
            )}
          </div>
          {templateError && (
            <p className="text-xs text-[var(--lf-rose)]" style={{ fontFamily: 'inherit' }}>
              {templateError}
            </p>
          )}
        </form>
      </div>

      )}

      {/* Pending Reports Section — lives under Queue tab too. */}
      {tab === 'queue' && (
      <div
        style={{ border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper-alt)', padding: 20 }}
      >
        <h2
          className="lf-text-h3 mb-4 text-[var(--lf-ink)]"
        >
          Pending Reports
          {data.pendingReports.length > 0 && (
            <span
              className="ml-2 rounded-full px-2 py-0.5 text-xs font-bold"
              style={{
                background: 'rgba(239,68,68,0.1)',
                color: 'var(--lf-rose)',
                border: '1px solid rgba(239,68,68,0.25)',
              }}
            >
              {data.pendingReports.length}
            </span>
          )}
        </h2>

        {data.pendingReports.length === 0 ? (
          <div className="lf-empty">
            No pending reports. The community is clean!
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            {data.pendingReports.map((report) => (
              <div
                key={report.id}
                style={{ border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper)', padding: 14 }}
              >
                <div className="flex items-start justify-between gap-4 lf-mod-report">
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-2 mb-2">
                      <span
                        className="rounded-full px-2.5 py-0.5 text-xs font-semibold"
                        style={{
                          color: reasonColors[report.reason] ?? 'var(--lf-muted)',
                          background: `color-mix(in srgb, ${reasonColors[report.reason] ?? 'var(--lf-muted)'} 10%, transparent)`,
                          border: `1px solid color-mix(in srgb, ${reasonColors[report.reason] ?? 'var(--lf-muted)'} 25%, transparent)`,
                        }}
                      >
                        {reasonLabels[report.reason] ?? report.reason}
                      </span>
                      <span
                        className="rounded-full px-2 py-0.5 text-xs"
                        style={{
                          color: 'var(--lf-muted)',
                          background: 'var(--lf-paper-alt)',
                          border: '1px solid var(--lf-rule-soft)',
                          fontFamily: 'inherit',
                        }}
                      >
                        {report.contentType}
                      </span>
                      <span className="text-xs text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
                        {relativeTime(report.createdAt)}
                      </span>
                    </div>
                    <p className="text-sm text-[var(--lf-ink)] mb-1" style={{ fontFamily: 'inherit' }}>
                      Reported by <span className="text-[var(--lf-accent-3)]">{report.reporterName}</span>
                    </p>
                    {report.details && (
                      <p className="text-sm text-[var(--lf-muted)] mt-1" style={{ fontFamily: 'inherit' }}>
                        &quot;{report.details}&quot;
                      </p>
                    )}
                    <p className="text-xs text-[var(--lf-muted)] mt-1" style={{ fontFamily: 'inherit' }}>
                      Content ID: {report.contentId.slice(0, 12)}...
                    </p>
                  </div>
                  <div className="flex shrink-0 gap-2 flex-wrap" style={{ maxWidth: 340, justifyContent: 'flex-end' }}>
                    {report.contentType === 'post' && (
                      <Link
                        href={`/post/${report.contentId}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="rounded-lg px-3 py-1.5 text-xs font-medium transition border"
                        style={{
                          color: 'var(--lf-muted)',
                          background: 'var(--lf-paper-alt)',
                          borderColor: 'var(--lf-rule-soft)',
                          fontFamily: 'inherit',
                          textDecoration: 'none',
                        }}
                      >
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>View <IconArrowRight size={11} /></span>
                      </Link>
                    )}
                    {report.contentType === 'post' && (
                      <button
                        onClick={() => handleApproveContent('post', report.contentId)}
                        disabled={actingOn === report.contentId}
                        className="rounded-lg px-3 py-1.5 text-xs font-medium transition border"
                        style={{
                          color: 'var(--lf-seal)',
                          background: 'rgba(16,185,129,0.08)',
                          borderColor: 'rgba(16,185,129,0.25)',
                          fontFamily: 'inherit',
                          cursor: actingOn === report.contentId ? 'not-allowed' : 'pointer',
                          opacity: actingOn === report.contentId ? 0.6 : 1,
                        }}
                      >
                        Approve
                      </button>
                    )}
                    <button
                      onClick={() => handleRemoveContent(report.contentType, report.contentId)}
                      disabled={actingOn === report.contentId}
                      className="rounded-lg px-3 py-1.5 text-xs font-medium transition border"
                      style={{
                        color: 'var(--lf-rose)',
                        background: 'rgba(239,68,68,0.08)',
                        borderColor: 'rgba(239,68,68,0.25)',
                        fontFamily: 'inherit',
                        cursor: actingOn === report.contentId ? 'not-allowed' : 'pointer',
                        opacity: actingOn === report.contentId ? 0.6 : 1,
                      }}
                    >
                      Remove
                    </button>
                    <button
                      onClick={() => handleResolveReport(report.id, 'dismissed')}
                      disabled={resolvingId === report.id}
                      className="rounded-lg px-3 py-1.5 text-xs font-medium transition border"
                      style={{
                        color: 'var(--lf-muted)',
                        background: 'var(--lf-paper-alt)',
                        borderColor: 'var(--lf-rule-soft)',
                        fontFamily: 'inherit',
                        cursor: resolvingId === report.id ? 'not-allowed' : 'pointer',
                        opacity: resolvingId === report.id ? 0.6 : 1,
                      }}
                    >
                      Dismiss
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      )}

      {/* Bans Section */}
      {tab === 'bans' && (
      <div
        style={{ marginTop: 24, border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper-alt)', padding: 20 }}
      >
        <h2
          className="lf-text-h3 mb-1 text-[var(--lf-ink)]"
        >
          Banned Participants
          {bans.length > 0 && (
            <span
              className="ml-2 rounded-full px-2 py-0.5 text-xs font-bold"
              style={{
                background: 'rgba(239,68,68,0.1)',
                color: 'var(--lf-rose)',
                border: '1px solid rgba(239,68,68,0.25)',
              }}
            >
              {bans.length}
            </span>
          )}
        </h2>
        <p className="mb-4 text-xs text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
          Banned participants can't post or comment in this community. Past content stays visible.
        </p>

        {bans.length === 0 ? (
          <p className="text-sm text-[var(--lf-muted)] mb-4" style={{ fontFamily: 'inherit' }}>
            No active bans.
          </p>
        ) : (
          <div className="flex flex-col gap-2 mb-5">
            {bans.map((ban) => (
              <div
                key={ban.participantId}
                className="lf-mod-row"
                style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 14px', borderBottom: '1px solid var(--lf-rule-soft)' }}
              >
                <div style={{ minWidth: 0, flex: 1 }}>
                  <p className="text-sm font-medium text-[var(--lf-ink)]" style={{ fontFamily: 'inherit' }}>
                    {ban.participantName}
                    <span className="ml-2 text-xs text-[var(--lf-muted)]" style={{ fontFamily: 'var(--lf-font-mono)' }}>
                      {ban.participantId.slice(0, 8)}…
                    </span>
                  </p>
                  {ban.reason && (
                    <p className="text-xs text-[var(--lf-muted)] mt-0.5" style={{ fontFamily: 'inherit' }}>
                      &quot;{ban.reason}&quot;
                    </p>
                  )}
                  <p className="text-xs text-[var(--lf-muted)] mt-0.5" style={{ fontFamily: 'inherit' }}>
                    by {ban.bannedByName} · {relativeTime(ban.createdAt)}
                  </p>
                </div>
                <button
                  onClick={() => handleUnbanUser(ban.participantId)}
                  className="rounded-lg px-3 py-1.5 text-xs font-medium text-[var(--lf-accent-3)] transition border"
                  style={{
                    background: 'var(--lf-paper-alt)',
                    borderColor: 'var(--lf-rule-soft)',
                    fontFamily: 'inherit',
                    cursor: 'pointer',
                  }}
                >
                  Lift ban
                </button>
              </div>
            ))}
          </div>
        )}

        {/* Ban form */}
        <form onSubmit={handleBanUser} className="flex gap-3 items-end lf-mod-form">
          <div className="flex-1">
            <label className="mb-1 block text-xs font-medium text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
              Ban — Participant ID
            </label>
            <input
              type="text"
              value={banParticipantId}
              onChange={(e) => setBanParticipantId(e.target.value)}
              placeholder="Paste participant UUID..."
              style={inputStyle}
              onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
              onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
            />
          </div>
          <div className="flex-1">
            <label className="mb-1 block text-xs font-medium text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
              Reason (shown in mod log)
            </label>
            <input
              type="text"
              value={banReason}
              onChange={(e) => setBanReason(e.target.value)}
              placeholder="Optional..."
              style={inputStyle}
              onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
              onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
            />
          </div>
          <button
            type="submit"
            disabled={banning || !banParticipantId.trim()}
            style={{
              background: 'var(--lf-rose)',
              color: '#fff',
              border: 'none',
              borderRadius: 8,
              padding: '8px 18px',
              fontSize: 14,
              fontWeight: 600,
              fontFamily: 'inherit',
              cursor: banning ? 'not-allowed' : 'pointer',
              opacity: banning || !banParticipantId.trim() ? 0.6 : 1,
              whiteSpace: 'nowrap',
            }}
          >
            {banning ? 'Banning…' : 'Ban'}
          </button>
        </form>
        {banError && (
          <p className="mt-2 text-xs text-[var(--lf-rose)]" style={{ fontFamily: 'inherit' }}>
            {banError}
          </p>
        )}
      </div>

      )}

      {/* Mod Log Section */}
      {tab === 'log' && (
      <div
        style={{ marginTop: 24, border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper-alt)', padding: 20 }}
      >
        <h2
          className="lf-text-h3 mb-1 text-[var(--lf-ink)]"
        >
          Moderator Log
        </h2>
        <p className="mb-4 text-xs text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
          Every moderator action in this community, newest first.
        </p>

        {modLog.length === 0 ? (
          <p className="text-sm text-[var(--lf-muted)]" style={{ fontFamily: 'inherit' }}>
            No moderator actions yet.
          </p>
        ) : (
          <div className="flex flex-col">
            {modLog.map((entry) => (
              <div
                key={entry.id}
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'auto 1fr auto',
                  gap: 12,
                  alignItems: 'baseline',
                  padding: '10px 0',
                  borderBottom: '1px dotted var(--lf-rule-soft)',
                }}
              >
                <span
                  style={{
                    fontFamily: 'var(--lf-font-mono)',
                    fontSize: 10,
                    letterSpacing: '0.1em',
                    textTransform: 'uppercase',
                    color: entry.action.startsWith('remove') || entry.action === 'ban_user'
                      ? 'var(--lf-rose)'
                      : entry.action.startsWith('restore') || entry.action === 'unban_user' || entry.action === 'approve_post'
                      ? 'var(--lf-seal)'
                      : 'var(--lf-muted)',
                  }}
                >
                  {actionLabels[entry.action] ?? entry.action}
                </span>
                <div style={{ minWidth: 0 }}>
                  <div className="text-sm text-[var(--lf-ink)]" style={{ fontFamily: 'inherit' }}>
                    <span style={{ color: 'var(--lf-accent-3)' }}>{entry.actorName}</span>
                    {' '}
                    <span className="text-[var(--lf-muted)]">
                      {entry.targetType} {entry.targetId.slice(0, 8)}…
                    </span>
                  </div>
                  {entry.reason && (
                    <p className="text-xs text-[var(--lf-muted)] mt-0.5" style={{ fontFamily: 'var(--lf-font-body)', fontStyle: 'italic' }}>
                      &quot;{entry.reason}&quot;
                    </p>
                  )}
                </div>
                <span className="text-xs text-[var(--lf-muted)]" style={{ fontFamily: 'inherit', whiteSpace: 'nowrap' }}>
                  {relativeTime(entry.createdAt)}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
      )}
    </div>
  )
}

// ModTabBtn — single tab button. Active state uses ink underline
// (matches the Discover/Bookmarks page style); the count pill is
// only shown when count > 0 so empty tabs don't shout for
// attention.
function ModTabBtn({
  label,
  count,
  active,
  onClick,
}: {
  label: string
  count?: number
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      style={{
        flexShrink: 0,
        padding: '10px 16px',
        background: 'transparent',
        border: 'none',
        borderBottom: active ? '2px solid var(--lf-accent)' : '2px solid transparent',
        marginBottom: -1,
        color: active ? 'var(--lf-ink)' : 'var(--lf-muted)',
        fontFamily: 'var(--lf-font-body)',
        fontSize: 14,
        fontWeight: 600,
        cursor: 'pointer',
        minHeight: 44,
        whiteSpace: 'nowrap',
      }}
    >
      {label}
      {count !== undefined && count > 0 && (
        <span
          style={{
            marginLeft: 6,
            padding: '1px 6px',
            background: active ? 'var(--lf-ink)' : 'var(--lf-rule-soft)',
            color: active ? 'var(--lf-paper)' : 'var(--lf-muted)',
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 10,
            fontWeight: 700,
            borderRadius: 999,
            letterSpacing: '0.05em',
          }}
        >
          {count}
        </span>
      )}
    </button>
  )
}
