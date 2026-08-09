// web/src/components/lf/LFQuickCompose.tsx
'use client'

import React from 'react'
import Link from 'next/link'
import { LFAvatar } from './LFAvatar'

// Class-based markup mirroring hybrid-front.html `.compose`. Sizes,
// borders, and pill styling all live in index.css under `body.lf-v2`
// — keep this file structural only.

const TYPES: { label: string; type: string }[] = [
  { label: 'Text',     type: 'discussion' },
  { label: 'Link',     type: 'link' },
  { label: 'Question', type: 'question' },
]

export interface LFQuickComposeProps {
  /** Seed for the leading avatar (current user's avatar color). */
  seed?: number
  /** Whether the avatar should render the agent variant. */
  isAgent?: boolean
  /** Avatar image URL if available. */
  avatarUrl?: string
  /** Placeholder text in the prompt. Defaults to a copy-friendly question. */
  placeholder?: string
}

export function LFQuickCompose({
  seed = 0,
  isAgent = false,
  avatarUrl,
  placeholder = 'What did you read or build today?',
}: LFQuickComposeProps) {
  return (
    <div className="compose">
      <span className="av">
        <LFAvatar size={32} seed={seed} agent={isAgent} imageUrl={avatarUrl} />
      </span>
      <Link href="/submit" className="placeholder">
        {placeholder}
      </Link>
      {TYPES.map((t) => (
        <Link key={t.label} href={`/submit?type=${t.type}`} className="type-pill">
          {t.label}
        </Link>
      ))}
    </div>
  )
}
