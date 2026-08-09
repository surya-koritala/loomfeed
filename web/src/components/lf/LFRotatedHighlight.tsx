'use client'

import React from 'react'

// LFRotatedHighlight — Brand Guidelines v1.0 motif #2.
// "One word per headline gets a tilted accent block. Never two."
//
// The word renders in front of a lime block tilted ~3deg, with a
// tomato block underneath offset by 6-10px to create the signature
// stacked-shadow effect (cover slide: "Read, argue, verify."; final
// slide: "Now go build it.").
//
// Usage:
//   <h1>Read, argue, <LFRotatedHighlight>verify.</LFRotatedHighlight></h1>
//
// The component sets `display: inline-block` so it doesn't break the
// host headline's natural flow. The shadow blocks are absolutely
// positioned inside the wrapper and ignore pointer events.
//
// **Use sparingly.** One per headline. Never on body text. Don't
// nest two. Reserve for hero moments — landing page, cover decks,
// onboarding splash, occasional campaign headers.

export interface LFRotatedHighlightProps {
  children: React.ReactNode
  /** Tilt angle in degrees. Defaults to the CSS rule (-3deg) when unset. */
  angle?: number
  className?: string
  style?: React.CSSProperties
}

export function LFRotatedHighlight({
  children,
  angle,
  className,
  style,
}: LFRotatedHighlightProps) {
  // Single inline-block element with the .verify-highlight class. All
  // visual styling — lime fill, tomato hard shadow (4px 4px 0), -3deg
  // rotation, 2px 8px padding — lives in index.css under
  // `body.lf-v2 .verify-highlight`. Ports hybrid-front.html line 235
  // verbatim; the previous double-absolute-blocks variant produced a
  // different effect (8px offset shadow + double rotation) and was the
  // reason the highlight looked larger / softer than the reference.
  const cls = 'verify-highlight' + (className ? ' ' + className : '')
  // Allow callers to override the angle via inline style; default is
  // baked into the CSS rule (-3deg).
  const inline: React.CSSProperties = { ...(style ?? {}) }
  if (angle != null) inline.transform = `rotate(${angle}deg)`
  return (
    <span className={cls} style={inline}>
      {children}
    </span>
  )
}
