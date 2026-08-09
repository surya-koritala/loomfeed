// web/src/components/lf/LFSurface.tsx
import React from 'react'
import { lfColor } from '../../lib/lf-tokens'

// The base card primitive. Every "card" on the new UI is an LFSurface
// — right-rail panels, profile blocks, arena evidence, onboarding cards.
// Per DESIGN_TOKENS.md, card rest = 1px rule-mid border and NO shadow
// (the 1.5px-ink + 4px hard-offset look is retired; only the lime Create
// CTA keeps the hard shadow). This matches the already-refined feed
// surface so secondary cards stop reading bolder than the posts they
// sit beside. `flat` is kept for API compatibility (now a no-op since
// the shadow is gone everywhere).
//
// `accent` paints a 4px tab on the top edge — used on the "Agent of
// the week" card and the few places we want a category color cue.
// The accent is constrained to the token palette to keep the page
// from accumulating one-off colors.
//
// `inverted` swaps to ink-fill / paper-fg — the hero card pattern
// (sidebar info panel on onboarding, agent of the week card).
export type LFSurfaceAccent = 'lime' | 'tomato' | 'iris' | 'seal' | 'amber'

const ACCENT_COLOR: Record<LFSurfaceAccent, string> = {
  lime:   lfColor.accent,
  tomato: lfColor.accent2,
  iris:   lfColor.accent3,
  seal:   lfColor.seal,
  amber:  lfColor.contested,
}

export interface LFSurfaceProps extends React.HTMLAttributes<HTMLDivElement> {
  padding?: number | string
  flat?: boolean
  accent?: LFSurfaceAccent
  inverted?: boolean
}

export function LFSurface({
  padding = 24,
  flat,
  accent,
  inverted,
  style,
  children,
  ...rest
}: LFSurfaceProps) {
  return (
    <div
      {...rest}
      style={{
        background: inverted ? 'var(--lf-ink)' : 'var(--lf-paper)',
        color: inverted ? 'var(--lf-paper)' : 'var(--lf-ink)',
        // Inverted (ink-fill hero cards) keep an ink border so they read
        // as a solid block; normal cards use the subtle rule-mid rest border.
        border: `var(--lf-border-w) solid ${inverted ? 'var(--lf-ink)' : 'var(--lf-rule-mid)'}`,
        borderRadius: 'var(--lf-radius)',
        padding,
        boxShadow: 'none',
        position: 'relative',
        ...style,
      }}
    >
      {accent && (
        <div
          aria-hidden
          style={{
            position: 'absolute',
            top: 0,
            right: 24,
            width: 4,
            height: 20,
            background: ACCENT_COLOR[accent],
            transform: 'translateY(-1px)',
          }}
        />
      )}
      {children}
    </div>
  )
}
