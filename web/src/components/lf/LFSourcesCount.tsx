'use client'

import React from 'react'

// LFSourcesCount — the "12 verified sources" badge that surfaces
// loomfeed's central promise (per docs/POSITIONING.md: "every post
// links back to its sources") as a primary visual signal in feed
// cards and post detail. Distinct from the epistemic chip
// ("Supported · 5 sources"), which mixes the count into a status
// label and was easy to miss.
//
// Visual: small left-bordered strip with a checkmark icon + count.
// Loud enough to be the first thing a reader notices, calm enough
// to sit inline without dominating the title.
//
// Hidden when sourceCount === 0 — a sourceless post should render
// without a fake "0 sources" badge.

interface Props {
  sourceCount: number
  verifiedCount?: number
  /** Compact variant for tight surfaces (right rail, comment lists). */
  size?: 'sm' | 'md'
}

export function LFSourcesCount({ sourceCount, verifiedCount, size = 'md' }: Props) {
  if (!sourceCount || sourceCount <= 0) return null
  const sm = size === 'sm'
  const hasVerified = typeof verifiedCount === 'number' && verifiedCount > 0
  return (
    <span
      role="status"
      aria-label={
        hasVerified
          ? `${sourceCount} sources, ${verifiedCount} verified`
          : `${sourceCount} ${sourceCount === 1 ? 'source' : 'sources'}`
      }
      title={
        hasVerified
          ? `${sourceCount} sources cited · ${verifiedCount} verified`
          : `${sourceCount} ${sourceCount === 1 ? 'source' : 'sources'} cited`
      }
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        height: sm ? 18 : 22,
        padding: sm ? '0 8px' : '0 10px',
        borderRadius: 4,
        background: hasVerified ? 'color-mix(in srgb, var(--lf-seal, #00A86B) 12%, transparent)' : 'var(--lf-paper-alt)',
        border: `1px solid ${hasVerified ? 'color-mix(in srgb, var(--lf-seal, #00A86B) 40%, var(--lf-rule-soft))' : 'var(--lf-rule-soft)'}`,
        fontFamily: 'var(--lf-font-mono)',
        fontSize: sm ? 'var(--lf-text-label)' : 'var(--lf-text-caption)',
        fontWeight: 600,
        letterSpacing: '0.04em',
        color: 'var(--lf-ink)',
        whiteSpace: 'nowrap',
        userSelect: 'none',
      }}
    >
      <CheckIcon size={sm ? 10 : 12} verified={hasVerified} />
      <span>
        <strong style={{ fontWeight: 700 }}>{sourceCount}</strong>
        {' '}
        {sourceCount === 1 ? 'source' : 'sources'}
        {hasVerified && (
          <span style={{ color: 'var(--lf-muted)', fontWeight: 500 }}>
            {' · '}
            {verifiedCount} verified
          </span>
        )}
      </span>
    </span>
  )
}

function CheckIcon({ size, verified }: { size: number; verified: boolean }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke={verified ? 'var(--lf-seal, #00A86B)' : 'currentColor'}
      strokeWidth="2.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      style={{ flexShrink: 0 }}
    >
      <polyline points="20 6 9 17 4 12" />
    </svg>
  )
}
