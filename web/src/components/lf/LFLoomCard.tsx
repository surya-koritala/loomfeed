// web/src/components/lf/LFLoomCard.tsx
'use client'

import React from 'react'
import { LFLoomChip } from './LFLoomChip'
import MarkdownContent from '../MarkdownContent'

// LFLoomCard — the per-post Loom summary panel.
//
// Surfaces above the comment thread, not inside it. The Community-
// Notes pattern: a single canonical AI-generated context block per
// post that anyone can summon, refresh, or dismiss — never a chain
// of competing AI comments. Driven by the latest "post-card" summon
// for the post (loom_summons rows where reply_comment_id IS NULL).
//
// Three render states:
//   - pending   → "Loom is thinking…" with pulsing dots
//   - done      → markdown response + disclaimer + actions
//   - error     → friendly error banner with a retry affordance
export interface LFLoomCardProps {
  state: 'pending' | 'done' | 'error'
  intent: string | null
  /** Markdown body. Already cleaned (no inline disclaimers). */
  body: string | null
  /** "true" if served from cache — small visual cue for power
   *  users so they understand why it appeared instantly. */
  cached?: boolean
  onRefresh?: () => void
  onDismiss?: () => void
  /** Disables Refresh while a summon is in flight. */
  refreshing?: boolean
}

export function LFLoomCard({
  state,
  intent,
  body,
  cached,
  onRefresh,
  onDismiss,
  refreshing,
}: LFLoomCardProps) {
  return (
    <section
      className="lf-loom-card"
      aria-label="Loom AI summary"
      role="complementary"
    >
      <header className="lf-loom-card-head">
        <LFLoomChip intent={intent} size="md" />
        {cached && state === 'done' && (
          <span className="lf-loom-card-cached" title="Served from cache">
            cached
          </span>
        )}
        <div className="lf-loom-card-spacer" />
        {state === 'done' && onRefresh && (
          <button
            type="button"
            className="lf-loom-card-action"
            onClick={onRefresh}
            disabled={refreshing}
            aria-label="Re-summon Loom for a fresh summary"
          >
            {refreshing ? 'summoning…' : 'refresh'}
          </button>
        )}
        {onDismiss && (
          <button
            type="button"
            className="lf-loom-card-action"
            onClick={onDismiss}
            aria-label="Hide Loom summary for this session"
          >
            hide
          </button>
        )}
      </header>

      <div className="lf-loom-card-body">
        {state === 'pending' ? (
          <LoomCardThinking />
        ) : state === 'error' ? (
          <span style={{ color: 'var(--lf-muted)' }}>
            Loom couldn&apos;t respond this time. Try refreshing in a moment.
          </span>
        ) : body ? (
          <MarkdownContent content={body} />
        ) : (
          <span style={{ color: 'var(--lf-muted)' }}>
            No summary available.
          </span>
        )}
      </div>

      <footer className="lf-loom-card-foot">
        Loom is the platform AI · responses can be wrong · verify before relying
      </footer>
    </section>
  )
}

function LoomCardThinking() {
  return (
    <span style={{ display: 'inline-flex', alignItems: 'baseline', gap: 6, color: 'var(--lf-muted)' }}>
      <span style={{ fontStyle: 'italic' }}>Loom is thinking</span>
      <span className="loom-thinking-dot" />
      <span className="loom-thinking-dot" style={{ animationDelay: '0.16s' }} />
      <span className="loom-thinking-dot" style={{ animationDelay: '0.32s' }} />
    </span>
  )
}
