'use client'

import React from 'react'

// Big bordered search input. Used on /search and (later) anywhere we
// need a prominent search affordance. The trailing ⌘K hint is purely
// decorative — actual keyboard wiring is the caller's job (typically
// a global useEffect listening for Cmd+K to focus this input via ref).
//
// Submits on Enter via `onSubmit` callback. Controlled value/change
// pattern so the URL sync in /search stays the source of truth.

export interface LFSearchInputProps {
  value: string
  onChange: (value: string) => void
  /** Fires when the user presses Enter. */
  onSubmit?: (value: string) => void
  placeholder?: string
  /** Show the trailing ⌘K hint on the right. Defaults true. */
  showKbdHint?: boolean
  /** Forwarded autoFocus. */
  autoFocus?: boolean
  /** Ref to the underlying input element — for global Cmd+K wiring. */
  inputRef?: React.RefObject<HTMLInputElement | null>
}

export function LFSearchInput({
  value,
  onChange,
  onSubmit,
  placeholder = 'Search posts, contributors, communities…',
  showKbdHint = true,
  autoFocus,
  inputRef,
}: LFSearchInputProps) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '14px 18px',
        background: 'var(--lf-paper)',
        border: 'var(--lf-border-w) solid var(--lf-ink)',
        borderRadius: 'var(--lf-radius)',
        boxShadow: 'var(--lf-shadow-hard-sm)',
      }}
    >
      <span
        aria-hidden
        style={{
          fontFamily: 'var(--lf-font-mono)',
          fontSize: 16,
          color: 'var(--lf-muted)',
          flexShrink: 0,
        }}
      >
        ⌕
      </span>
      <input
        ref={inputRef}
        type="search"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            onSubmit?.(value)
          }
        }}
        placeholder={placeholder}
        aria-label={placeholder || 'Search'}
        autoFocus={autoFocus}
        style={{
          flex: 1,
          border: 'none',
          // No outline:none — keep the global :focus-visible keyboard ring.
          background: 'transparent',
          fontFamily: 'var(--lf-font-body)',
          fontSize: 16,
          color: 'var(--lf-ink)',
          minWidth: 0,
        }}
      />
      {showKbdHint && (
        <span
          aria-hidden
          style={{
            fontFamily: 'var(--lf-font-mono)',
            fontSize: 11,
            color: 'var(--lf-muted)',
            padding: '3px 6px',
            border: '1px solid var(--lf-ink)',
            borderRadius: 3,
            flexShrink: 0,
          }}
        >
          ⌘K
        </span>
      )}
    </div>
  )
}
