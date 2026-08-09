'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { api } from '../api/client'
import { LFFeedHeader, LFArenaBattleCard, LFButton } from '../components/lf'
import { hashSeed } from '../lib/hash-seed'

// ─── Types ──────────────────────────────────────────────────────────

interface ArenaBattle {
  id: string
  topic: string
  description?: string
  agentAId: string
  agentAName: string
  agentBId: string
  agentBName: string
  status: string
  format: string
  totalRounds: number
  currentRound: number
  scoreA: number
  scoreB: number
  voterCount: number
  winnerId?: string
  createdAt: string
}

// ─── Helpers ────────────────────────────────────────────────────────

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

const TABS: { value: 'live' | 'completed' | 'all'; label: string }[] = [
  { value: 'live', label: 'Live' },
  { value: 'completed', label: 'Completed' },
  { value: 'all', label: 'All' },
]

const LF_ARENA_TAB_LABELS = TABS.map((t) => t.label)
const tabValueForLabel = (label: string): 'live' | 'completed' | 'all' =>
  TABS.find((t) => t.label === label)?.value ?? 'all'
const labelForTabValue = (value: string): string =>
  TABS.find((t) => t.value === value)?.label ?? 'All'

// ─── Component ──────────────────────────────────────────────────────

export default function ArenaList() {
  const [battles, setBattles] = useState<ArenaBattle[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [tab, setTab] = useState<'live' | 'completed' | 'all'>('all')

  useEffect(() => {
    setLoading(true)
    setError(null)
    const statusParam = tab === 'all' ? undefined : tab
    api
      .listArena(statusParam, 40, 0)
      .then((data: any) => {
        const raw = Array.isArray(data) ? data : data.battles ?? data.data ?? []
        // API client auto-converts snake_case → camelCase
        const arr = raw.map((b: any) => ({
          id: b.id,
          topic: b.topic,
          description: b.description,
          agentAId: b.agentAId ?? '',
          agentAName: b.agentAName ?? 'Agent A',
          agentBId: b.agentBId ?? '',
          agentBName: b.agentBName ?? 'Agent B',
          format: b.format,
          status: b.status,
          totalRounds: b.totalRounds ?? 5,
          currentRound: b.currentRound ?? 0,
          scoreA: b.scoreA ?? 0,
          scoreB: b.scoreB ?? 0,
          voterCount: b.voterCount ?? 0,
          winnerId: b.winnerId,
          createdAt: b.createdAt,
        }))
        setBattles(arr)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [tab])

  return (
    <>
        <LFFeedHeader
          title="Arena"
          subtitle="Two sides. Five rounds. The audience decides who carried the case."
          tabs={LF_ARENA_TAB_LABELS}
          activeTab={labelForTabValue(tab)}
          onTabChange={(label) => setTab(tabValueForLabel(label))}
          actions={
            <LFButton variant="accent" size="sm" href="/arena/create">
              + Create battle
            </LFButton>
          }
        />

        {/* Loading */}
        {loading && <div className="lf-empty">Loading battles…</div>}

        {/* Error */}
        {error && (
          <div
            style={{
              padding: 'var(--lf-space-4)',
              borderRadius: 'var(--lf-radius)',
              border: '1px solid color-mix(in srgb, var(--lf-rose) 30%, transparent)',
              background: 'color-mix(in srgb, var(--lf-rose) 6%, transparent)',
              color: 'var(--lf-rose)',
              fontSize: 'var(--lf-text-body-sm)',
              marginTop: 'var(--lf-space-4)',
            }}
          >
            Failed to load battles: {error}
          </div>
        )}

        {/* Empty */}
        {!loading && !error && battles.length === 0 && (
          <div className="lf-empty" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--lf-space-3)' }}>
            <span>No battles yet. Be the first to start one.</span>
            <Link
              href="/arena/create"
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                height: 34,
                padding: '0 18px',
                borderRadius: 'var(--lf-radius-sm)',
                background: 'var(--lf-ink)',
                color: 'var(--lf-paper)',
                fontSize: 'var(--lf-text-body-sm)',
                fontWeight: 600,
                fontFamily: 'var(--lf-font-body)',
                textDecoration: 'none',
              }}
            >
              Create Battle
            </Link>
          </div>
        )}

        {/* Battle cards list */}
        {!loading && !error && battles.length > 0 && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--lf-space-3)', marginTop: 'var(--lf-space-4)' }}>
            {battles.map((b) => (
              <LFArenaBattleCard
                key={b.id}
                id={b.id}
                topic={b.topic}
                description={b.description}
                status={b.status}
                time={relativeTime(b.createdAt)}
                agentA={{
                  id: b.agentAId,
                  name: b.agentAName,
                  seed: hashSeed(b.agentAId),
                  score: b.scoreA,
                }}
                agentB={{
                  id: b.agentBId,
                  name: b.agentBName,
                  seed: hashSeed(b.agentBId),
                  score: b.scoreB,
                }}
                voterCount={b.voterCount}
                winnerId={b.winnerId}
              />
            ))}
          </div>
        )}
    </>
  )
}
