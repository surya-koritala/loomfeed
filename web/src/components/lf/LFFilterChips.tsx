'use client'

import React from 'react'

// Horizontal row of filter chips. Single-select OR multi-select via
// the `mode` prop. Each chip is a small button with a label + optional
// count. Active chip is ink-filled (inverted); inactive is white.
//
// Single-select usage:
//   <LFFilterChips mode="single" value={tab} onChange={setTab}
//                  options={[{ key: 'posts', label: 'Posts', count: 124 }]} />
//
// Multi-select usage:
//   <LFFilterChips mode="multi" value={selected} onChange={setSelected}
//                  options={[{ key: 'sealed', label: 'Sealed only' }]} />

export interface LFFilterChipOption {
  key: string
  label: string
  count?: number
}

interface BaseProps {
  options: readonly LFFilterChipOption[]
  className?: string
}

interface SingleProps extends BaseProps {
  mode: 'single'
  value: string
  onChange: (key: string) => void
}

interface MultiProps extends BaseProps {
  mode: 'multi'
  value: readonly string[]
  onChange: (keys: string[]) => void
}

export type LFFilterChipsProps = SingleProps | MultiProps

export function LFFilterChips(props: LFFilterChipsProps) {
  const { options, className } = props

  function isActive(key: string): boolean {
    if (props.mode === 'single') return key === props.value
    return props.value.includes(key)
  }

  function toggle(key: string) {
    if (props.mode === 'single') {
      props.onChange(key)
    } else {
      const set = new Set(props.value)
      if (set.has(key)) set.delete(key)
      else set.add(key)
      props.onChange(Array.from(set))
    }
  }

  return (
    <div
      className={className}
      role={props.mode === 'single' ? 'radiogroup' : 'group'}
      style={{
        display: 'flex',
        gap: 8,
        flexWrap: 'wrap',
      }}
    >
      {options.map((opt) => {
        const active = isActive(opt.key)
        return (
          <button
            key={opt.key}
            type="button"
            role={props.mode === 'single' ? 'radio' : 'checkbox'}
            aria-checked={active}
            onClick={() => toggle(opt.key)}
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 6,
              padding: '7px 14px',
              background: active ? 'var(--lf-ink)' : 'var(--lf-paper)',
              color: active ? 'var(--lf-paper)' : 'var(--lf-ink)',
              border: '1px solid ' + (active ? 'var(--lf-ink)' : 'var(--lf-rule-mid)'),
              borderRadius: 999,
              fontFamily: 'var(--lf-font-body)',
              fontSize: 13,
              fontWeight: 600,
              cursor: 'pointer',
              whiteSpace: 'nowrap',
              transition: 'border-color 0.15s, background 0.15s',
            }}
          >
            {opt.label}
            {opt.count != null && (
              <span style={{ opacity: active ? 0.7 : 0.6 }}>{opt.count}</span>
            )}
          </button>
        )
      })}
    </div>
  )
}
