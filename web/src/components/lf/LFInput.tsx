'use client'

import React from 'react'

// Plain text input. White paper bg, 1px rule-mid rest border, 8px
// radius; on focus the border goes ink (DESIGN_TOKENS: input focus =
// 1px ink — the hard-shadow-on-focus is retired). Single-line fields.
//
// Inherits all native <input> props except `style` (we own visuals).
// Pass `style` only if you need to override layout (width, etc.) —
// avoid overriding visual tokens.

export type LFInputProps = Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  'style'
> & {
  /** Layout-only style overrides (width, margin). Visual tokens locked. */
  style?: React.CSSProperties
}

export const LFInput = React.forwardRef<HTMLInputElement, LFInputProps>(
  function LFInput({ style, ...rest }, ref) {
    return (
      <input
        ref={ref}
        {...rest}
        style={{
          width: '100%',
          background: 'var(--lf-paper)',
          color: 'var(--lf-ink)',
          border: 'var(--lf-border-w) solid var(--lf-rule-mid)',
          borderRadius: 'var(--lf-radius-sm)',
          padding: '10px 14px',
          fontFamily: 'var(--lf-font-body)',
          fontSize: 15,
          lineHeight: 1.45,
          // No outline:none — keep the global :focus-visible keyboard ring.
          ...style,
        }}
        onFocus={(e) => {
          e.currentTarget.style.borderColor = 'var(--lf-ink)'
          rest.onFocus?.(e)
        }}
        onBlur={(e) => {
          e.currentTarget.style.borderColor = 'var(--lf-rule-mid)'
          rest.onBlur?.(e)
        }}
      />
    )
  }
)
