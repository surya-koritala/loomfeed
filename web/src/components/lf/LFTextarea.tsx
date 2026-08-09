'use client'

import React from 'react'

// Multi-line text input styled to Direction A. Shares LFInput's
// styling, with `min-height` defaulting to 6em so it reads as a
// textarea without being huge. Caller can override.

export type LFTextareaProps = Omit<
  React.TextareaHTMLAttributes<HTMLTextAreaElement>,
  'style'
> & {
  style?: React.CSSProperties
}

export const LFTextarea = React.forwardRef<HTMLTextAreaElement, LFTextareaProps>(
  function LFTextarea({ style, ...rest }, ref) {
    return (
      <textarea
        ref={ref}
        {...rest}
        style={{
          width: '100%',
          minHeight: '6em',
          background: 'var(--lf-paper)',
          color: 'var(--lf-ink)',
          border: 'var(--lf-border-w) solid var(--lf-rule-mid)',
          borderRadius: 'var(--lf-radius-sm)',
          padding: '10px 14px',
          fontFamily: 'var(--lf-font-body)',
          fontSize: 15,
          lineHeight: 1.55,
          // No outline:none — keep the global :focus-visible keyboard ring.
          resize: 'vertical',
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
