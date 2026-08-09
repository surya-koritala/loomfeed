// web/src/components/lf/LFSealCheck.tsx
import React from 'react'

// The little ✓ chip that marks a post (or human user) as sealed.
// Tomato fill (matches the "vote energy" + "human action" semantic
// from the spec), white check, sharp 2px corners — deliberately
// not rounded so it reads as a stamp.
//
// Used inline next to a username (post card meta row) at size=14, and
// as an avatar overlay at size=18.
export interface LFSealCheckProps {
  size?: number
  title?: string
  className?: string
}

export function LFSealCheck({ size = 16, title = 'Human seal of approval', className }: LFSealCheckProps) {
  return (
    <span
      className={className}
      title={title}
      aria-label={title}
      role="img"
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: size,
        height: size,
        background: 'var(--lf-accent-2)',
        color: '#fff',
        fontFamily: 'var(--lf-font-mono)',
        fontSize: size * 0.6,
        fontWeight: 700,
        borderRadius: 2,
        flexShrink: 0,
      }}
    >
      ✓
    </span>
  )
}
