'use client'

import { useState, useEffect } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import { api } from '../api/client'
import {
  LFArenaHeader,
  LFArenaSideColumn,
  LFArenaVoteBar,
} from '../components/lf'
import { IconArrowRight } from '../components/lf/icons'
import { hashSeed } from '../lib/hash-seed'

// ─── Types ──────────────────────────────────────────────────────────

interface Agent {
  id: string
  displayName: string
  trustScore: number
  avatarUrl?: string
}

interface RoundArgument {
  agentId: string
  argument: string
  submittedAt: string
}

interface RoundVoteTally {
  agentAVotes: number
  agentBVotes: number
  totalVotes: number
}

interface Round {
  roundNumber: number
  roundType: string
  argumentA?: RoundArgument
  argumentB?: RoundArgument
  voteTally: RoundVoteTally
  userVote?: string
}

interface ArenaComment {
  id: string
  authorId: string
  authorName: string
  authorType: string
  body: string
  createdAt: string
}

interface Battle {
  id: string
  topic: string
  description?: string
  agentA: Agent
  agentB: Agent
  status: string
  format: string
  totalRounds: number
  currentRound: number
  scoreA: number
  scoreB: number
  voterCount: number
  rounds: Round[]
  winnerId?: string
  rules?: string
  createdAt: string
}

interface BattleResults {
  winnerId: string
  winnerName: string
  totalVotes: number
  scoreA: number
  scoreB: number
  roundBreakdown: {
    round: number
    votesA: number
    votesB: number
  }[]
}

// ─── Constants ──────────────────────────────────────────────────────

const ROUND_LABELS: Record<number, string> = {
  1: 'Opening',
  2: 'Rebuttal',
  3: 'Evidence',
  4: 'Cross-Exam',
  5: 'Closing',
  6: 'Bonus Round',
  7: 'Final Round',
}

function getRoundLabel(roundNumber: number, totalRounds: number): string {
  if (totalRounds === 3) {
    if (roundNumber === 1) return 'Opening'
    if (roundNumber === 2) return 'Evidence'
    if (roundNumber === 3) return 'Closing'
  }
  if (totalRounds === 7) {
    if (roundNumber === 6) return 'Rebuttal II'
    if (roundNumber === 7) return 'Final Closing'
  }
  return ROUND_LABELS[roundNumber] || `Round ${roundNumber}`
}

// ─── Helpers ────────────────────────────────────────────────────────

function getInitials(name: string): string {
  return name
    .split(/\s+/)
    .map((w) => w[0])
    .join('')
    .toUpperCase()
    .slice(0, 2)
}

function scorePercent(a: number, b: number): number {
  const total = a + b
  if (total === 0) return 50
  return Math.round((a / total) * 100)
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

// ─── Shimmer styles ─────────────────────────────────────────────────

const shimmerStyle: React.CSSProperties = {
  background:
    'linear-gradient(90deg, var(--lf-paper-alt) 25%, var(--lf-paper) 50%, var(--lf-paper-alt) 75%)',
  backgroundSize: '200% 100%',
  animation: 'shimmer 1.5s infinite',
  borderRadius: 'var(--lf-radius-sm)',
}

// ─── Star rating component ──────────────────────────────────────────

function StarRating({
  value,
  onChange,
  label,
}: {
  value: number
  onChange: (v: number) => void
  label: string
}) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <span
        style={{
          fontSize: 12,
          fontWeight: 500,
          color: 'var(--lf-muted)',
          minWidth: 64,
        }}
      >
        {label}
      </span>
      <div style={{ display: 'flex', gap: 4 }}>
        {[1, 2, 3, 4, 5].map((n) => (
          <button
            key={n}
            type="button"
            onClick={() => onChange(n)}
            className="lf-rating-dot"
            style={{
              width: 18,
              height: 18,
              border: 'none',
              background: 'transparent',
              cursor: 'pointer',
              padding: 0,
              display: 'grid',
              placeItems: 'center',
            }}
            aria-label={`${label} ${n} of 5`}
          >
            <span
              style={{
                width: 18,
                height: 18,
                borderRadius: '50%',
                background: n <= value ? 'var(--lf-ink)' : 'var(--lf-rule-soft)',
                transition: 'all 0.12s ease',
              }}
            />
          </button>
        ))}
      </div>
      <span style={{ fontSize: 11, color: 'var(--lf-muted)', minWidth: 16 }}>
        {value}/5
      </span>
    </div>
  )
}

// ─── Component ──────────────────────────────────────────────────────

export default function ArenaBattle() {
  const params = useParams()
  const router = useRouter()
  const battleId = params?.id as string

  const [battle, setBattle] = useState<Battle | null>(null)
  const [results, setResults] = useState<BattleResults | null>(null)
  const [comments, setComments] = useState<ArenaComment[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Round expansion state
  const [expandedRounds, setExpandedRounds] = useState<Set<number>>(new Set())

  // Voting state per round
  const [votingRound, setVotingRound] = useState<number | null>(null)
  const [voteFor, setVoteFor] = useState<string>('')
  const [argScore, setArgScore] = useState(3)
  const [srcScore, setSrcScore] = useState(3)
  const [clarityScore, setClarityScore] = useState(3)
  const [submittingVote, setSubmittingVote] = useState(false)
  const [votedRounds, setVotedRounds] = useState<Set<number>>(new Set())

  // Comment state
  const [commentBody, setCommentBody] = useState('')
  const [submittingComment, setSubmittingComment] = useState(false)

  // Fetch battle data
  useEffect(() => {
    if (!battleId) return
    setLoading(true)
    setError(null)

    Promise.all([
      api.getArena(battleId),
      api.getArenaComments(battleId).catch(() => []),
    ])
      .then(([raw, commentsData]: [any, any]) => {
        // API client auto-converts snake_case to camelCase via transformKeys()
        // So agent_a_name → agentAName, agent_a_argument → agentAArgument, etc.
        const battleData: Battle = {
          id: raw.id,
          topic: raw.topic,
          description: raw.description,
          agentA: {
            id: raw.agentAId ?? '',
            displayName: raw.agentAName ?? 'Agent A',
            trustScore: raw.agentA?.trustScore ?? 0,
          },
          agentB: {
            id: raw.agentBId ?? '',
            displayName: raw.agentBName ?? 'Agent B',
            trustScore: raw.agentB?.trustScore ?? 0,
          },
          format: raw.format,
          status: raw.status,
          totalRounds: raw.totalRounds ?? 5,
          currentRound: raw.currentRound ?? 0,
          voterCount: raw.voterCount ?? 0,
          winnerId: raw.winnerId,
          scoreA: raw.scoreA ?? 0,
          scoreB: raw.scoreB ?? 0,
          rounds: (raw.rounds ?? []).map((r: any) => ({
            roundNumber: r.roundNumber,
            roundType: r.roundType ?? 'argument',
            argumentA: r.agentAArgument ? {
              agentId: raw.agentAId,
              argument: r.agentAArgument,
              submittedAt: r.agentASubmittedAt ?? '',
            } : undefined,
            argumentB: r.agentBArgument ? {
              agentId: raw.agentBId,
              argument: r.agentBArgument,
              submittedAt: r.agentBSubmittedAt ?? '',
            } : undefined,
            voteTally: {
              agentAVotes: r.agentATotalVotes ?? 0,
              agentBVotes: r.agentBTotalVotes ?? 0,
              totalVotes: (r.agentATotalVotes ?? 0) + (r.agentBTotalVotes ?? 0),
            },
          })),
          createdAt: raw.createdAt,
        }
        setBattle(battleData)
        setComments(
          Array.isArray(commentsData) ? commentsData : commentsData.comments ?? commentsData.data ?? []
        )

        // Expand current and first round by default
        const rounds = battleData.rounds ?? []
        const expanded = new Set<number>()
        // Only expand the current active round (or last round if completed)
        const currentRound = battleData.currentRound || rounds.length
        if (currentRound > 0) expanded.add(currentRound)
        setExpandedRounds(expanded)

        // Track rounds already voted on
        const alreadyVoted = new Set<number>()
        rounds.forEach((r: Round) => {
          if (r.userVote) alreadyVoted.add(r.roundNumber)
        })
        setVotedRounds(alreadyVoted)

        // Fetch results if completed
        if (battleData.status === 'completed') {
          api.getArenaResults(battleId).then((r: any) => setResults(r)).catch(() => {})
        }
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [battleId])

  const toggleRound = (roundNumber: number) => {
    setExpandedRounds((prev) => {
      const next = new Set(prev)
      if (next.has(roundNumber)) next.delete(roundNumber)
      else next.add(roundNumber)
      return next
    })
  }

  const handleVote = async (roundNumber: number) => {
    if (!voteFor || !battle) return
    setSubmittingVote(true)
    try {
      await api.voteArenaRound(battleId, roundNumber, {
        voted_for: voteFor,
        argument_score: argScore,
        source_score: srcScore,
        clarity_score: clarityScore,
      })
      setVotedRounds((prev) => new Set(prev).add(roundNumber))
      setVotingRound(null)
      setVoteFor('')
      setArgScore(3)
      setSrcScore(3)
      setClarityScore(3)
      // Refresh battle data
      api.getArena(battleId).then(() => { /* reload page to re-map */ window.location.reload() }).catch(() => {})
    } catch {
      // silently fail
    } finally {
      setSubmittingVote(false)
    }
  }

  const handleComment = async () => {
    if (!commentBody.trim()) return
    setSubmittingComment(true)
    try {
      const newComment: any = await api.addArenaComment(battleId, commentBody.trim())
      setComments((prev) => [newComment, ...prev])
      setCommentBody('')
    } catch {
      // silently fail
    } finally {
      setSubmittingComment(false)
    }
  }

  if (loading) {
    return (
      <>
        <style>{`
          @keyframes shimmer {
            0% { background-position: 200% 0; }
            100% { background-position: -200% 0; }
          }
        `}</style>
        <div className="lf-arena-page lf-narrow" style={{ padding: '32px 24px 60px' }}>
          <div style={{ ...shimmerStyle, height: 28, width: '60%', marginBottom: 12 }} />
          <div style={{ ...shimmerStyle, height: 14, width: '40%', marginBottom: 32 }} />
          <div style={{ ...shimmerStyle, height: 120, marginBottom: 24 }} />
          <div style={{ ...shimmerStyle, height: 200, marginBottom: 16 }} />
          <div style={{ ...shimmerStyle, height: 200 }} />
        </div>
      </>
    )
  }

  if (error || !battle) {
    return (
      <div className="lf-arena-page lf-narrow" style={{ padding: '32px 24px 60px' }}>
        <div
          style={{
            padding: 'var(--lf-space-4)',
            borderRadius: 'var(--lf-radius)',
            border: '1px solid color-mix(in srgb, var(--lf-rose) 30%, transparent)',
            background: 'color-mix(in srgb, var(--lf-rose) 6%, transparent)',
            color: 'var(--lf-rose)',
            fontSize: 'var(--lf-text-body-sm)',
          }}
        >
          {error || 'Battle not found.'}
        </div>
        <button
          onClick={() => router.push('/arena')}
          style={{
            marginTop: 16,
            background: 'none',
            border: 'none',
            color: 'var(--lf-muted)',
            fontSize: 13,
            fontWeight: 500,
            cursor: 'pointer',
            fontFamily: 'inherit',
            padding: 0,
          }}
        >
          Back to Arena
        </button>
      </div>
    )
  }

  // Compute scores from round vote tallies (not API scoreA/scoreB which don't exist)
  const totalVotesA = battle.rounds.reduce((sum, r) => sum + (r.voteTally?.agentAVotes ?? 0), 0)
  const totalVotesB = battle.rounds.reduce((sum, r) => sum + (r.voteTally?.agentBVotes ?? 0), 0)
  const pctA = scorePercent(totalVotesA, totalVotesB)
  const pctB = 100 - pctA
  const isLive = battle.status === 'live'
  const isCompleted = battle.status === 'completed'
  const winner =
    isCompleted && battle.winnerId
      ? battle.winnerId === battle.agentA.id
        ? battle.agentA
        : battle.agentB
      : null

  return (
    <>
      <style>{`
        @keyframes shimmer {
          0% { background-position: 200% 0; }
          100% { background-position: -200% 0; }
        }
        @keyframes pulseLive {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.4; }
        }
        @keyframes fadeInUp {
          from { opacity: 0; transform: translateY(10px); }
          to { opacity: 1; transform: translateY(0); }
        }
        .round-card:hover {
          border-color: var(--lf-ink) !important;
        }
      `}</style>

      <div className="lf-arena-page lf-narrow" style={{ padding: '32px 24px 60px' }}>
        {/* Back link */}
        <button
          onClick={() => router.push('/arena')}
          style={{
            background: 'none',
            border: 'none',
            color: 'var(--lf-muted)',
            fontSize: 13,
            fontWeight: 500,
            cursor: 'pointer',
            fontFamily: 'inherit',
            padding: 0,
            marginBottom: 20,
            display: 'flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          <svg
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <polyline points="15 18 9 12 15 6" />
          </svg>
          Back to Arena
        </button>

        {/* Header */}
        <LFArenaHeader
          topic={battle.topic}
          scope="The Arena"
          phase={battle.status}
          phaseDetail={isLive ? `Round ${battle.currentRound}` : undefined}
          roundInfo={`Round ${battle.currentRound} of ${battle.totalRounds}`}
          totalVotes={battle.voterCount}
        />
        {battle.description && (
          <p
            style={{
              fontSize: 14,
              color: 'var(--lf-muted)',
              margin: '-8px 0 24px',
              lineHeight: 1.6,
            }}
          >
            {battle.description}
          </p>
        )}

        {/* Side columns — A vs B */}
        {(() => {
          const lastRound = battle.rounds?.[battle.rounds.length - 1]
          const argA = lastRound?.argumentA?.argument
          const argB = lastRound?.argumentB?.argument
          const truncate = (s: string | undefined): string =>
            s ? (s.length > 220 ? s.slice(0, 220).trimEnd() + '…' : s) : ''
          const claimA = truncate(argA) || 'No argument yet.'
          const claimB = truncate(argB) || 'No argument yet.'
          // Voting in this view happens per-round inside the round
          // cards below, so we surface the top-of-page Vote button
          // only as a navigation cue when a round vote is actionable.
          const activeRound = battle.rounds?.find(
            (r) => r.roundNumber === battle.currentRound,
          )
          const userVoteSide = activeRound?.userVote as 'A' | 'B' | undefined
          const voteLabelA =
            userVoteSide === 'A'
              ? 'You voted A'
              : isCompleted
                ? 'Voting closed'
                : 'Vote for A in round below'
          const voteLabelB =
            userVoteSide === 'B'
              ? 'You voted B'
              : isCompleted
                ? 'Voting closed'
                : 'Vote for B in round below'
          return (
            <div className="lf-arena-sides">
              <LFArenaSideColumn
                side="A"
                agentName={battle.agentA.displayName}
                agentTrust={battle.agentA.trustScore}
                agentAvatarSeed={hashSeed(battle.agentA.id)}
                agentAvatarUrl={battle.agentA.avatarUrl}
                votePct={pctA}
                claim={claimA}
                voteDisabled
                voteLabel={voteLabelA}
              />
              <LFArenaSideColumn
                side="B"
                agentName={battle.agentB.displayName}
                agentTrust={battle.agentB.trustScore}
                agentAvatarSeed={hashSeed(battle.agentB.id)}
                agentAvatarUrl={battle.agentB.avatarUrl}
                votePct={pctB}
                claim={claimB}
                voteDisabled
                voteLabel={voteLabelB}
              />
            </div>
          )
        })()}

        {/* Bottom vote distribution bar */}
        <LFArenaVoteBar pctA={pctA} pctB={pctB} />

        {/* Winner announcement */}
        {isCompleted && winner && (
          <div
            style={{
              background: 'var(--lf-paper-alt)',
              border: '2px solid var(--lf-rule-soft)',
              borderRadius: 12,
              padding: 24,
              marginBottom: 24,
              textAlign: 'center',
              animation: 'fadeInUp 0.4s ease both',
            }}
          >
            <div
              style={{
                fontSize: 11,
                fontWeight: 700,
                color: 'var(--lf-muted)',
                textTransform: 'uppercase',
                letterSpacing: '0.1em',
                marginBottom: 8,
              }}
            >
              Winner
            </div>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 10,
              }}
            >
              <div
                style={{
                  width: 36,
                  height: 36,
                  borderRadius: 8,
                  background:
                    winner.id === battle.agentA.id ? 'var(--indigo)' : 'var(--emerald)',
                  color: '#fff',
                  fontSize: 13,
                  fontWeight: 700,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                }}
              >
                {getInitials(winner.displayName)}
              </div>
              <span
                style={{
                  fontSize: 20,
                  fontWeight: 800,
                  color: 'var(--lf-ink)',
                  letterSpacing: '-0.02em',
                }}
              >
                {winner.displayName}
              </span>
            </div>
            {results && (
              <div
                style={{
                  marginTop: 12,
                  fontSize: 13,
                  color: 'var(--lf-muted)',
                }}
              >
                {results.totalVotes} total vote{results.totalVotes !== 1 ? 's' : ''} across{' '}
                {results.roundBreakdown?.length ?? battle.totalRounds} rounds
              </div>
            )}
          </div>
        )}

        {/* Round-by-round results breakdown */}
        {isCompleted && results?.roundBreakdown && results.roundBreakdown.length > 0 && (
          <div
            style={{
              background: 'var(--white)',
              border: '1px solid var(--lf-rule-soft)',
              borderRadius: 12,
              padding: 20,
              marginBottom: 24,
            }}
          >
            <h3
              style={{
                fontSize: 13,
                fontWeight: 700,
                color: 'var(--lf-ink)',
                margin: '0 0 14px',
                textTransform: 'uppercase',
                letterSpacing: '0.05em',
              }}
            >
              Round Breakdown
            </h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {results.roundBreakdown.map((rb) => {
                const totalRb = rb.votesA + rb.votesB
                const rbPctA = totalRb === 0 ? 50 : Math.round((rb.votesA / totalRb) * 100)
                return (
                  <div key={rb.round}>
                    <div
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        fontSize: 12,
                        color: 'var(--lf-muted)',
                        marginBottom: 3,
                      }}
                    >
                      <span>{getRoundLabel(rb.round, battle.totalRounds)}</span>
                      <span>
                        {rb.votesA} - {rb.votesB}
                      </span>
                    </div>
                    <div
                      style={{
                        display: 'flex',
                        height: 4,
                        borderRadius: 2,
                        overflow: 'hidden',
                        background: 'var(--lf-rule-soft)',
                      }}
                    >
                      <div
                        style={{
                          width: `${rbPctA}%`,
                          background: 'var(--indigo)',
                        }}
                      />
                      <div
                        style={{
                          width: `${100 - rbPctA}%`,
                          background: 'var(--emerald)',
                        }}
                      />
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        )}

        {/* Rounds */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {(battle.rounds ?? []).map((round) => {
            const isExpanded = expandedRounds.has(round.roundNumber)
            const hasArgA = !!round.argumentA?.argument
            const hasArgB = !!round.argumentB?.argument
            const hasVoted = votedRounds.has(round.roundNumber) || !!round.userVote
            const isVoting = votingRound === round.roundNumber

            return (
              <div
                key={round.roundNumber}
                className="round-card"
                style={{
                  background: 'var(--lf-paper)',
                  border: 'var(--lf-border-w) solid var(--lf-ink)',
                  borderRadius: 'var(--lf-radius)',
                  boxShadow: 'var(--lf-shadow-hard-sm)',
                  overflow: 'hidden',
                }}
              >
                {/* Round header */}
                <button
                  onClick={() => toggleRound(round.roundNumber)}
                  style={{
                    width: '100%',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    padding: '14px 20px',
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    fontFamily: 'inherit',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                    <span
                      style={{
                        fontFamily: 'var(--lf-font-display)',
                        fontWeight: 800,
                        fontSize: 16,
                        letterSpacing: '-0.02em',
                        color: 'var(--lf-ink)',
                      }}
                    >
                      Round {round.roundNumber}
                    </span>
                    <span
                      style={{
                        fontFamily: 'var(--lf-font-mono)',
                        fontSize: 10,
                        textTransform: 'uppercase',
                        letterSpacing: '0.08em',
                        color: 'var(--lf-muted)',
                        fontWeight: 600,
                      }}
                    >
                      {getRoundLabel(round.roundNumber, battle.totalRounds)}
                    </span>
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                    {round.voteTally.totalVotes > 0 && (
                      <span style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 11, color: 'var(--lf-muted)', fontWeight: 600 }}>
                        {round.voteTally.totalVotes} vote{round.voteTally.totalVotes !== 1 ? 's' : ''}
                      </span>
                    )}
                    <svg
                      width="14"
                      height="14"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="var(--lf-ink)"
                      strokeWidth="2.5"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      style={{
                        transition: 'transform 0.2s ease',
                        transform: isExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
                      }}
                    >
                      <polyline points="6 9 12 15 18 9" />
                    </svg>
                  </div>
                </button>

                {/* Expanded content */}
                {isExpanded && (
                  <div style={{ padding: '0 18px 18px', animation: 'fadeInUp 0.2s ease both' }}>
                    {/* Arguments columns — A and B side by side on
                        desktop, stacked on mobile via the shared
                        .lf-arena-pair-grid class. */}
                    <div
                      className="lf-arena-pair-grid"
                      style={{ marginBottom: 16 }}
                    >
                      {/* Agent A argument */}
                      <div>
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 8,
                            marginBottom: 10,
                          }}
                        >
                          <div
                            style={{
                              width: 22,
                              height: 22,
                              borderRadius: 4,
                              background: 'var(--lf-accent)',
                              color: 'var(--lf-ink)',
                              border: 'var(--lf-border-w) solid var(--lf-ink)',
                              fontSize: 10,
                              fontWeight: 800,
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                            }}
                          >
                            {getInitials(battle.agentA.displayName)}
                          </div>
                          <span
                            style={{
                              fontFamily: 'var(--lf-font-body)',
                              fontSize: 13,
                              fontWeight: 700,
                              color: 'var(--lf-ink)',
                            }}
                          >
                            {battle.agentA.displayName}
                          </span>
                        </div>
                        {hasArgA ? (
                          <div
                            style={{
                              fontFamily: 'var(--lf-font-body)',
                              fontSize: 14,
                              color: 'var(--lf-ink)',
                              lineHeight: 1.6,
                              background: 'color-mix(in srgb, var(--lf-accent) 14%, var(--lf-paper))',
                              borderRadius: 'var(--lf-radius-sm)',
                              padding: 14,
                              border: 'var(--lf-border-w) solid var(--lf-ink)',
                              whiteSpace: 'pre-wrap',
                            }}
                          >
                            {round.argumentA!.argument}
                          </div>
                        ) : (
                          <div
                            style={{
                              ...shimmerStyle,
                              height: 80,
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              borderRadius: 'var(--lf-radius-sm)',
                            }}
                          >
                            <span style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 11, color: 'var(--lf-muted)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
                              Waiting for {battle.agentA.displayName}…
                            </span>
                          </div>
                        )}
                      </div>

                      {/* Agent B argument */}
                      <div>
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 8,
                            marginBottom: 10,
                          }}
                        >
                          <div
                            style={{
                              width: 22,
                              height: 22,
                              borderRadius: 4,
                              background: 'var(--lf-accent-2)',
                              color: 'var(--lf-paper)',
                              border: 'var(--lf-border-w) solid var(--lf-ink)',
                              fontSize: 10,
                              fontWeight: 800,
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                            }}
                          >
                            {getInitials(battle.agentB.displayName)}
                          </div>
                          <span
                            style={{
                              fontFamily: 'var(--lf-font-body)',
                              fontSize: 13,
                              fontWeight: 700,
                              color: 'var(--lf-ink)',
                            }}
                          >
                            {battle.agentB.displayName}
                          </span>
                        </div>
                        {hasArgB ? (
                          <div
                            style={{
                              fontFamily: 'var(--lf-font-body)',
                              fontSize: 14,
                              color: 'var(--lf-ink)',
                              lineHeight: 1.6,
                              background: 'color-mix(in srgb, var(--lf-accent-2) 12%, var(--lf-paper))',
                              borderRadius: 'var(--lf-radius-sm)',
                              padding: 14,
                              border: 'var(--lf-border-w) solid var(--lf-ink)',
                              whiteSpace: 'pre-wrap',
                            }}
                          >
                            {round.argumentB!.argument}
                          </div>
                        ) : (
                          <div
                            style={{
                              ...shimmerStyle,
                              height: 80,
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              borderRadius: 'var(--lf-radius-sm)',
                            }}
                          >
                            <span style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 11, color: 'var(--lf-muted)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
                              Waiting for {battle.agentB.displayName}…
                            </span>
                          </div>
                        )}
                      </div>
                    </div>

                    {/* Voting section */}
                    {hasArgA && hasArgB && !hasVoted && !isVoting && (
                      <button
                        onClick={() => setVotingRound(round.roundNumber)}
                        style={{
                          width: '100%',
                          padding: '12px 0',
                          borderRadius: 'var(--lf-radius)',
                          background: 'var(--lf-accent)',
                          border: 'var(--lf-border-w) solid var(--lf-ink)',
                          boxShadow: 'var(--lf-shadow-hard-sm)',
                          color: 'var(--lf-ink)',
                          fontFamily: 'var(--lf-font-body)',
                          fontSize: 14,
                          fontWeight: 700,
                          letterSpacing: '-0.01em',
                          cursor: 'pointer',
                        }}
                      >
                        ⚖ Vote on this round
                      </button>
                    )}

                    {/* Voting form */}
                    {isVoting && (
                      <div
                        style={{
                          background: 'var(--lf-paper-alt)',
                          border: 'var(--lf-border-w) solid var(--lf-ink)',
                          borderRadius: 'var(--lf-radius)',
                          padding: 18,
                          animation: 'fadeInUp 0.2s ease both',
                        }}
                      >
                        <div
                          style={{
                            fontFamily: 'var(--lf-font-display)',
                            fontWeight: 800,
                            fontSize: 16,
                            letterSpacing: '-0.02em',
                            color: 'var(--lf-ink)',
                            marginBottom: 14,
                          }}
                        >
                          Who won this round?
                        </div>

                        {/* Agent selection — pair grid, collapses
                            to single column on narrow viewports. */}
                        <div
                          className="lf-arena-pair-grid"
                          style={{ gap: 10, marginBottom: 16 }}
                        >
                          <button
                            type="button"
                            onClick={() => setVoteFor(battle.agentA.id)}
                            style={{
                              padding: '12px 14px',
                              borderRadius: 'var(--lf-radius-sm)',
                              border: 'var(--lf-border-w) solid var(--lf-ink)',
                              background:
                                voteFor === battle.agentA.id
                                  ? 'var(--lf-accent)'
                                  : 'var(--lf-paper)',
                              boxShadow:
                                voteFor === battle.agentA.id ? 'var(--lf-shadow-hard-sm)' : 'none',
                              cursor: 'pointer',
                              fontFamily: 'inherit',
                              display: 'flex',
                              alignItems: 'center',
                              gap: 10,
                              transition: 'background 0.12s, box-shadow 0.12s',
                            }}
                          >
                            <div
                              style={{
                                width: 24,
                                height: 24,
                                borderRadius: 4,
                                background: 'var(--lf-accent)',
                                color: 'var(--lf-ink)',
                                border: 'var(--lf-border-w) solid var(--lf-ink)',
                                fontSize: 10,
                                fontWeight: 800,
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                flexShrink: 0,
                              }}
                            >
                              {getInitials(battle.agentA.displayName)}
                            </div>
                            <span
                              style={{
                                fontFamily: 'var(--lf-font-body)',
                                fontSize: 13,
                                fontWeight: 700,
                                color: 'var(--lf-ink)',
                                overflow: 'hidden',
                                textOverflow: 'ellipsis',
                                whiteSpace: 'nowrap',
                              }}
                            >
                              {battle.agentA.displayName}
                            </span>
                          </button>

                          <button
                            type="button"
                            onClick={() => setVoteFor(battle.agentB.id)}
                            style={{
                              padding: '12px 14px',
                              borderRadius: 'var(--lf-radius-sm)',
                              border: 'var(--lf-border-w) solid var(--lf-ink)',
                              background:
                                voteFor === battle.agentB.id
                                  ? 'var(--lf-accent-2)'
                                  : 'var(--lf-paper)',
                              boxShadow:
                                voteFor === battle.agentB.id ? 'var(--lf-shadow-hard-sm)' : 'none',
                              cursor: 'pointer',
                              fontFamily: 'inherit',
                              display: 'flex',
                              alignItems: 'center',
                              gap: 10,
                              transition: 'background 0.12s, box-shadow 0.12s',
                            }}
                          >
                            <div
                              style={{
                                width: 24,
                                height: 24,
                                borderRadius: 4,
                                background: 'var(--lf-accent-2)',
                                color: 'var(--lf-paper)',
                                border: 'var(--lf-border-w) solid var(--lf-ink)',
                                fontSize: 10,
                                fontWeight: 800,
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                flexShrink: 0,
                              }}
                            >
                              {getInitials(battle.agentB.displayName)}
                            </div>
                            <span
                              style={{
                                fontFamily: 'var(--lf-font-body)',
                                fontSize: 13,
                                fontWeight: 700,
                                color: voteFor === battle.agentB.id ? 'var(--lf-paper)' : 'var(--lf-ink)',
                                overflow: 'hidden',
                                textOverflow: 'ellipsis',
                                whiteSpace: 'nowrap',
                              }}
                            >
                              {battle.agentB.displayName}
                            </span>
                          </button>
                        </div>

                        {/* Rating sliders */}
                        <div
                          style={{
                            display: 'flex',
                            flexDirection: 'column',
                            gap: 10,
                            marginBottom: 16,
                          }}
                        >
                          <StarRating value={argScore} onChange={setArgScore} label="Argument" />
                          <StarRating value={srcScore} onChange={setSrcScore} label="Sources" />
                          <StarRating
                            value={clarityScore}
                            onChange={setClarityScore}
                            label="Clarity"
                          />
                        </div>

                        {/* Submit vote */}
                        <div style={{ display: 'flex', gap: 10 }}>
                          <button
                            onClick={() => handleVote(round.roundNumber)}
                            disabled={!voteFor || submittingVote}
                            style={{
                              flex: 1,
                              height: 42,
                              borderRadius: 'var(--lf-radius)',
                              background:
                                voteFor && !submittingVote
                                  ? 'var(--lf-accent)'
                                  : 'var(--lf-paper)',
                              color: 'var(--lf-ink)',
                              fontFamily: 'var(--lf-font-body)',
                              fontSize: 14,
                              fontWeight: 700,
                              letterSpacing: '-0.01em',
                              border: 'var(--lf-border-w) solid var(--lf-ink)',
                              boxShadow:
                                voteFor && !submittingVote ? 'var(--lf-shadow-hard-sm)' : 'none',
                              cursor:
                                voteFor && !submittingVote ? 'pointer' : 'not-allowed',
                              opacity: !voteFor ? 0.55 : 1,
                              transition: 'all 0.15s ease',
                            }}
                          >
                            {submittingVote ? 'Submitting…' : <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>Submit vote <IconArrowRight size={14} /></span>}
                          </button>
                          <button
                            onClick={() => {
                              setVotingRound(null)
                              setVoteFor('')
                            }}
                            style={{
                              height: 42,
                              padding: '0 18px',
                              borderRadius: 'var(--lf-radius)',
                              background: 'var(--lf-paper)',
                              border: 'var(--lf-border-w) solid var(--lf-ink)',
                              color: 'var(--lf-ink)',
                              fontFamily: 'var(--lf-font-body)',
                              fontSize: 13,
                              fontWeight: 600,
                              cursor: 'pointer',
                            }}
                          >
                            Cancel
                          </button>
                        </div>
                      </div>
                    )}

                    {/* Vote tally (after voting) */}
                    {hasVoted && round.voteTally.totalVotes > 0 && (
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: 10,
                          padding: '10px 14px',
                          background: 'var(--lf-paper-alt)',
                          borderRadius: 8,
                          border: '1px solid var(--lf-rule-soft)',
                        }}
                      >
                        <span style={{ fontSize: 11, color: 'var(--lf-muted)', fontWeight: 500 }}>
                          Results:
                        </span>
                        <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--indigo)' }}>
                          {round.voteTally.agentAVotes}
                        </span>
                        <div
                          style={{
                            flex: 1,
                            display: 'flex',
                            height: 4,
                            borderRadius: 2,
                            overflow: 'hidden',
                            background: 'var(--lf-rule-soft)',
                          }}
                        >
                          {(() => {
                            const total =
                              round.voteTally.agentAVotes + round.voteTally.agentBVotes
                            const pct =
                              total === 0
                                ? 50
                                : Math.round((round.voteTally.agentAVotes / total) * 100)
                            return (
                              <>
                                <div
                                  style={{ width: `${pct}%`, background: 'var(--indigo)' }}
                                />
                                <div
                                  style={{
                                    width: `${100 - pct}%`,
                                    background: 'var(--emerald)',
                                  }}
                                />
                              </>
                            )
                          })()}
                        </div>
                        <span
                          style={{ fontSize: 12, fontWeight: 600, color: 'var(--emerald)' }}
                        >
                          {round.voteTally.agentBVotes}
                        </span>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>

        {/* Comments Section */}
        <div style={{ marginTop: 32 }}>
          <h3
            style={{
              fontSize: 15,
              fontWeight: 700,
              color: 'var(--lf-ink)',
              margin: '0 0 16px',
            }}
          >
            Comments
          </h3>

          {/* Comment input */}
          {localStorage.getItem('token') && (
            <div style={{ marginBottom: 20 }}>
              <textarea
                value={commentBody}
                onChange={(e) => setCommentBody(e.target.value)}
                placeholder="Share your thoughts on this battle..."
                rows={3}
                style={{
                  width: '100%',
                  background: 'var(--lf-paper-alt)',
                  border: '1px solid var(--lf-rule-soft)',
                  borderRadius: 8,
                  color: 'var(--lf-ink)',
                  padding: '10px 12px',
                  fontSize: 13,
                  outline: 'none',
                  fontFamily: 'inherit',
                  boxSizing: 'border-box',
                  resize: 'vertical',
                  minHeight: 60,
                  transition: 'border-color 0.15s ease',
                }}
                onFocus={(e) => {
                  e.currentTarget.style.borderColor = 'var(--lf-muted)'
                }}
                onBlur={(e) => {
                  e.currentTarget.style.borderColor = 'var(--lf-rule-soft)'
                }}
              />
              <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 8 }}>
                <button
                  onClick={handleComment}
                  disabled={!commentBody.trim() || submittingComment}
                  style={{
                    height: 32,
                    padding: '0 16px',
                    borderRadius: 7,
                    background:
                      commentBody.trim() && !submittingComment
                        ? 'var(--lf-ink)'
                        : 'var(--lf-rule-soft)',
                    color:
                      commentBody.trim() && !submittingComment
                        ? '#fff'
                        : 'var(--lf-muted)',
                    fontSize: 12,
                    fontWeight: 600,
                    border: 'none',
                    cursor:
                      commentBody.trim() && !submittingComment
                        ? 'pointer'
                        : 'not-allowed',
                    fontFamily: 'inherit',
                    transition: 'all 0.15s ease',
                  }}
                >
                  {submittingComment ? 'Posting...' : 'Post Comment'}
                </button>
              </div>
            </div>
          )}

          {/* Comments list */}
          {comments.length === 0 ? (
            <div className="lf-empty">
              No comments yet. Be the first to share your thoughts.
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
              {comments.map((comment) => (
                <div
                  key={comment.id}
                  style={{
                    padding: '14px 0',
                    borderBottom: '1px solid var(--lf-rule-soft)',
                  }}
                >
                  <div
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 6,
                      marginBottom: 6,
                    }}
                  >
                    <Link
                      href={`/profile/${comment.authorId}`}
                      style={{
                        fontSize: 12,
                        fontWeight: 600,
                        color:
                          comment.authorType === 'agent'
                            ? 'var(--indigo)'
                            : 'var(--lf-ink)',
                        textDecoration: 'none',
                      }}
                    >
                      {comment.authorName}
                    </Link>
                    {comment.authorType === 'agent' && (
                      <span
                        style={{
                          fontSize: 9,
                          fontWeight: 700,
                          color: 'var(--indigo)',
                          background: 'color-mix(in srgb, var(--indigo) 10%, transparent)',
                          padding: '1px 5px',
                          borderRadius: 4,
                          textTransform: 'uppercase',
                          letterSpacing: '0.04em',
                        }}
                      >
                        Agent
                      </span>
                    )}
                    <span style={{ fontSize: 11, color: 'var(--lf-muted)' }}>
                      {relativeTime(comment.createdAt)}
                    </span>
                  </div>
                  <p
                    style={{
                      fontSize: 13,
                      color: 'var(--lf-ink)',
                      lineHeight: 1.6,
                      margin: 0,
                      whiteSpace: 'pre-wrap',
                    }}
                  >
                    {comment.body}
                  </p>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </>
  )
}
