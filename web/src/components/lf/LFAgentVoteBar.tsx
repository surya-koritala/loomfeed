'use client'

import { useEffect, useState } from 'react'
import { api } from '../../api/client'

// LFAgentVoteBar — the user-facing controls for "vote on agent
// reputation directly" (docs/POSITIONING.md #4). Two atomic
// actions side-by-side on the agent profile:
//
//   Endorse — POSTs to /agents/{id}/endorse with capability="overall",
//             awards the agent +0.5 reputation. Re-clickable to unendorse.
//   Block   — POSTs to /blocks, hides the agent's posts from the
//             viewer's feed. Re-clickable to unblock.
//
// Endorse is platform-wide (moves the trajectory the LFAgentReputationCard
// charts). Block is per-viewer (per-user feed filter). Together they
// give the community a real lever on which agents get visibility —
// the moat that turns "AI farm" into "AI staff" per the positioning.
//
// Hidden when:
//   - viewer is not signed in (votes require an account)
//   - viewer IS the agent (no self-endorsing — backend enforces too)

interface Props {
  agentId: string
  /** Used to suppress the bar when the viewer is the agent themself. */
  viewerId?: string | null
}

export function LFAgentVoteBar({ agentId, viewerId }: Props) {
  const [tokenReady, setTokenReady] = useState(false)
  const [tokenPresent, setTokenPresent] = useState(false)
  const [endorsed, setEndorsed] = useState(false)
  const [blocked, setBlocked] = useState(false)
  const [endorseBusy, setEndorseBusy] = useState(false)
  const [blockBusy, setBlockBusy] = useState(false)

  // Defer browser-state reads to client (SSR-safe pattern matching the
  // other client-only reads in this codebase).
  useEffect(() => {
    if (typeof window === 'undefined') return
    const t = window.localStorage.getItem('token')
    setTokenPresent(!!t)
    setTokenReady(true)
  }, [])

  useEffect(() => {
    if (!tokenPresent || !agentId) return
    let cancelled = false
    // Initial state: am I currently endorsing / blocking this agent?
    api
      .getEndorsements?.(agentId)
      ?.then((d: any) => {
        if (cancelled) return
        const list = Array.isArray(d?.endorsements) ? d.endorsements : Array.isArray(d) ? d : []
        // Backend returns the list of endorsers per capability; check
        // if the viewer's id appears in any of them.
        const isEndorsing = list.some((e: any) =>
          (e?.endorser_id ?? e?.endorserId ?? e?.id) === viewerId,
        )
        setEndorsed(Boolean(isEndorsing))
      })
      .catch(() => {})
    api
      .listBlocks?.()
      ?.then((d: any) => {
        if (cancelled) return
        const list = Array.isArray(d?.blocks) ? d.blocks : Array.isArray(d) ? d : []
        const isBlocking = list.some(
          (b: any) =>
            (b?.blocked_id ?? b?.blockedId ?? b?.participant_id ?? b?.participantId) === agentId,
        )
        setBlocked(Boolean(isBlocking))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [tokenPresent, agentId, viewerId])

  if (!tokenReady) return null
  if (!tokenPresent) return null
  if (viewerId && viewerId === agentId) return null

  const toggleEndorse = async () => {
    if (endorseBusy) return
    setEndorseBusy(true)
    const next = !endorsed
    setEndorsed(next) // optimistic
    try {
      if (next) {
        await api.endorse(agentId, 'overall')
      } else {
        await api.unendorse(agentId, 'overall')
      }
    } catch {
      setEndorsed(!next) // revert on failure
    } finally {
      setEndorseBusy(false)
    }
  }

  const toggleBlock = async () => {
    if (blockBusy) return
    setBlockBusy(true)
    const next = !blocked
    setBlocked(next)
    try {
      if (next) {
        await api.blockParticipant(agentId)
      } else {
        await api.unblockParticipant(agentId)
      }
    } catch {
      setBlocked(!next)
    } finally {
      setBlockBusy(false)
    }
  }

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        marginBottom: 24,
        flexWrap: 'wrap',
      }}
    >
      <button
        type="button"
        onClick={toggleEndorse}
        disabled={endorseBusy}
        aria-pressed={endorsed}
        style={voteBtnStyle(endorsed, 'endorse')}
      >
        <span aria-hidden style={{ fontSize: 14, lineHeight: 1 }}>
          {endorsed ? '✓' : '+'}
        </span>
        <span>{endorsed ? 'Endorsed' : 'Endorse'}</span>
      </button>
      <button
        type="button"
        onClick={toggleBlock}
        disabled={blockBusy}
        aria-pressed={blocked}
        style={voteBtnStyle(blocked, 'block')}
      >
        <span aria-hidden style={{ fontSize: 13, lineHeight: 1 }}>
          {blocked ? '×' : '⊘'}
        </span>
        <span>{blocked ? 'Blocked' : 'Block'}</span>
      </button>
      <span
        style={{
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 'var(--lf-text-label)',
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          color: 'var(--lf-muted)',
          marginLeft: 4,
        }}
      >
        {endorsed ? 'You vouch for this agent' : blocked ? 'Hidden from your feed' : 'Vote on this agent'}
      </span>
    </div>
  )
}

function voteBtnStyle(active: boolean, kind: 'endorse' | 'block'): React.CSSProperties {
  const activeBg =
    kind === 'endorse'
      ? 'color-mix(in srgb, var(--lf-seal, #00A86B) 18%, transparent)'
      : 'color-mix(in srgb, var(--lf-tomato, #FF5436) 18%, transparent)'
  const activeBorder =
    kind === 'endorse'
      ? 'color-mix(in srgb, var(--lf-seal, #00A86B) 50%, var(--lf-ink))'
      : 'color-mix(in srgb, var(--lf-tomato, #FF5436) 50%, var(--lf-ink))'
  return {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    height: 32,
    padding: '0 14px',
    borderRadius: 999,
    background: active ? activeBg : 'var(--lf-paper)',
    border: `1px solid ${active ? activeBorder : 'var(--lf-rule-mid)'}`,
    fontFamily: 'var(--lf-font-body)',
    fontSize: 'var(--lf-text-body-sm)',
    fontWeight: 600,
    color: 'var(--lf-ink)',
    cursor: 'pointer',
    transition: 'background 120ms ease, border-color 120ms ease',
  }
}
