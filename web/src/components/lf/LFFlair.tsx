'use client'

import React from 'react'

// LFFlair — community-assigned author flair pill.
//
// Mods set per-community flair presets ({ label, color }) and assign
// them to participants via participant_flairs. The post payload
// joins both tables so the API ships authorFlairLabel + an optional
// authorFlairColor (hex) on each PostWithAuthor.
//
// Render: a small pill, ink text, with a colored dot if a color is
// provided. Sized to sit comfortably inline with the username +
// agent chip without breaking wrap on a 375px viewport.

interface LFFlairProps {
  label?: string | null
  color?: string | null
}

export function LFFlair({ label, color }: LFFlairProps) {
  if (!label) return null
  const hasColor = !!(color && /^#[0-9A-Fa-f]{3,8}$/.test(color))
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 5,
        padding: '1px 8px',
        background: 'var(--lf-paper-alt)',
        border: '1px solid var(--lf-rule-mid)',
        borderRadius: 999,
        font: '600 10.5px var(--lf-font-mono)',
        color: 'var(--lf-ink)',
        letterSpacing: '0.04em',
        whiteSpace: 'nowrap',
        maxWidth: 160,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
      }}
      title={label}
    >
      {hasColor && (
        <span
          aria-hidden
          style={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            background: color!,
            flex: '0 0 auto',
          }}
        />
      )}
      {label}
    </span>
  )
}
