// web/src/components/lf/LFPollCard.tsx
'use client'
import { useState, useEffect, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { api } from '../../api/client'
import { useToast } from '../ToastProvider'

// Renders the poll attached to a post (if any) and lets the reader vote.
//
// Why this exists: poll posts were half-wired — the Submit form and the
// MCP create_poll tool persisted polls to the polls/poll_options tables,
// but NOTHING on the web ever fetched or rendered them, so a poll post
// looked like a plain text post and no one could vote. This is the
// missing read/vote surface. It mounts on every post detail page and
// self-erases (returns null) when the post has no poll, mirroring the
// GET /poll endpoint's "200 + null" contract.
//
// Field names are camelCase because api.request() runs transformKeys on
// every response (poll.totalVotes, option.voteCount, poll.userVote).

interface PollOption {
  id: string
  text: string
  voteCount: number
  sortOrder: number
}

interface PollData {
  id: string
  postId: string
  deadline?: string | null
  createdAt: string
  options: PollOption[]
  totalVotes: number
  userVote?: string | null // option id the viewer voted for, if any
}

export interface LFPollCardProps {
  postId: string
}

function deadlineLabel(deadline: string | null | undefined, ended: boolean): string {
  if (!deadline) return 'Open'
  if (ended) return 'Ended'
  const ms = new Date(deadline).getTime() - Date.now()
  const mins = Math.round(ms / 60000)
  if (mins < 60) return `Ends in ${Math.max(1, mins)}m`
  const hrs = Math.round(mins / 60)
  if (hrs < 24) return `Ends in ${hrs}h`
  return `Ends in ${Math.round(hrs / 24)}d`
}

export function LFPollCard({ postId }: LFPollCardProps) {
  const router = useRouter()
  const { addToast } = useToast()
  const [poll, setPoll] = useState<PollData | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [voting, setVoting] = useState(false)

  useEffect(() => {
    let active = true
    api
      .getPoll(postId)
      .then((p: any) => { if (active) setPoll(p ?? null) })
      .catch(() => { if (active) setPoll(null) })
      .finally(() => { if (active) setLoaded(true) })
    return () => { active = false }
  }, [postId])

  const ended = !!(poll?.deadline && new Date(poll.deadline).getTime() < Date.now())
  const hasVoted = !!poll?.userVote
  const showResults = hasVoted || ended

  const handleVote = useCallback(
    async (optionId: string) => {
      if (!poll || voting || hasVoted || ended) return
      const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null
      if (!token) {
        addToast('Login required to vote', 'info')
        router.push('/login')
        return
      }
      setVoting(true)
      // Optimistic: lock in the choice and bump counts so the results
      // reveal instantly; the refetch below reconciles to server truth.
      setPoll((prev) =>
        prev
          ? {
              ...prev,
              userVote: optionId,
              totalVotes: prev.totalVotes + 1,
              options: prev.options.map((o) =>
                o.id === optionId ? { ...o, voteCount: o.voteCount + 1 } : o,
              ),
            }
          : prev,
      )
      try {
        await api.votePoll(postId, optionId)
        const fresh: any = await api.getPoll(postId)
        if (fresh) setPoll(fresh)
      } catch (e: any) {
        addToast(e?.message || 'Failed to record your vote', 'error')
        // Revert the optimistic update by re-reading the real state.
        try {
          const fresh: any = await api.getPoll(postId)
          setPoll(fresh ?? null)
        } catch {
          /* leave optimistic state; next page load reconciles */
        }
      } finally {
        setVoting(false)
      }
    },
    [poll, voting, hasVoted, ended, postId, addToast, router],
  )

  // Nothing until we know; nothing when the post has no poll attached.
  if (!loaded || !poll || poll.options.length === 0) return null

  const total = poll.totalVotes
  const leadCount = poll.options.reduce((m, o) => Math.max(m, o.voteCount), 0)

  return (
    <section
      aria-label="Poll"
      style={{
        border: '1px solid var(--lf-rule-mid)',
        borderRadius: 14,
        padding: '16px 18px',
        margin: '4px 0 16px',
        background: 'var(--lf-paper)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
        <span
          style={{
            font: '700 var(--lf-text-caption) var(--lf-font-mono)',
            letterSpacing: '0.12em',
            textTransform: 'uppercase',
            color: 'var(--lf-ink)',
          }}
        >
          Poll
        </span>
        <span style={{ font: '600 12px var(--lf-font-mono)', color: 'var(--lf-muted)' }}>
          {deadlineLabel(poll.deadline, ended)}
        </span>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {poll.options.map((o) => {
          const pct = total > 0 ? Math.round((o.voteCount / total) * 100) : 0
          const chosen = poll.userVote === o.id
          const isLead = showResults && leadCount > 0 && o.voteCount === leadCount

          if (showResults) {
            return (
              <div
                key={o.id}
                style={{
                  position: 'relative',
                  border: `1px solid ${chosen ? 'var(--lf-ink)' : 'var(--lf-rule-mid)'}`,
                  borderRadius: 10,
                  overflow: 'hidden',
                  padding: '10px 12px',
                }}
              >
                <div
                  aria-hidden
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    bottom: 0,
                    width: `${pct}%`,
                    background: chosen
                      ? 'color-mix(in srgb, var(--lf-accent) 55%, transparent)'
                      : 'var(--lf-gray-100)',
                    transition: 'width .35s ease',
                  }}
                />
                <div style={{ position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
                  <span style={{ font: `${chosen || isLead ? 700 : 500} 14px var(--lf-font-body)`, color: 'var(--lf-ink)' }}>
                    {o.text}
                    {chosen && ' ✓'}
                  </span>
                  <span style={{ font: '700 13px var(--lf-font-mono)', color: 'var(--lf-ink)', flexShrink: 0 }}>
                    {pct}% · {o.voteCount}
                  </span>
                </div>
              </div>
            )
          }

          return (
            <button
              key={o.id}
              type="button"
              disabled={voting}
              onClick={() => handleVote(o.id)}
              style={{
                textAlign: 'left',
                border: '1px solid var(--lf-rule-mid)',
                borderRadius: 10,
                padding: '11px 13px',
                background: 'var(--lf-paper)',
                color: 'var(--lf-ink)',
                font: '500 14px var(--lf-font-body)',
                cursor: voting ? 'default' : 'pointer',
                transition: 'border-color .15s, background .15s',
              }}
              onMouseEnter={(e) => {
                if (voting) return
                e.currentTarget.style.borderColor = 'var(--lf-ink)'
                e.currentTarget.style.background = 'var(--lf-gray-100)'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.borderColor = 'var(--lf-rule-mid)'
                e.currentTarget.style.background = 'var(--lf-paper)'
              }}
            >
              {o.text}
            </button>
          )
        })}
      </div>

      <div style={{ marginTop: 12, font: '600 12px var(--lf-font-mono)', color: 'var(--lf-muted)', letterSpacing: '0.02em' }}>
        {total} {total === 1 ? 'vote' : 'votes'}
        {!showResults && ' · Select an option to vote'}
        {ended && ' · Final'}
      </div>
    </section>
  )
}
