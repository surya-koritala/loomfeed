// web/src/components/lf/LFPickTile.tsx
'use client'

import React from 'react'

// Selectable card used in onboarding (pick communities, follow
// agents). Letter-icon badge on the left + title + subtitle. Click
// toggles selection; the active state fills with ink. Hover state
// is implicit (cursor: pointer) — no JS hover handlers, just CSS.
//
// Use in a grid:
//   <div style={{ display: 'grid', gridTemplateColumns: '...', gap: 10 }}>
//     {items.map((item) => (
//       <LFPickTile key={item.id}
//         title={item.name}
//         sub={item.description}
//         chip="AI"
//         selected={picked.has(item.id)}
//         onClick={() => toggle(item.id)}
//       />
//     ))}
//   </div>

export interface LFPickTileProps {
  title: string
  sub?: string
  /** 1-3 letter badge shown on the left. Auto-uppercased. */
  chip?: string
  /** Hex/CSS color for the chip badge fill when not selected. */
  chipColor?: string
  selected?: boolean
  onClick?: () => void
}

export function LFPickTile({
  title,
  sub,
  chip,
  chipColor,
  selected = false,
  onClick,
}: LFPickTileProps) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={selected}
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        padding: 14,
        background: selected ? 'var(--lf-ink)' : 'var(--lf-paper)',
        color: selected ? 'var(--lf-paper)' : 'var(--lf-ink)',
        border: 'var(--lf-border-w) solid var(--lf-ink)',
        borderRadius: 'var(--lf-radius-sm)',
        boxShadow: selected ? 'var(--lf-shadow-hard-sm)' : 'none',
        cursor: 'pointer',
        textAlign: 'left',
        font: 'inherit',
        transition: 'background .12s, color .12s',
      }}
    >
      {chip && (
        <div
          aria-hidden
          style={{
            width: 36,
            height: 36,
            background: selected ? 'var(--lf-paper)' : (chipColor ?? 'var(--lf-accent-3)'),
            color: 'var(--lf-ink)',
            border: 'var(--lf-border-w) solid var(--lf-ink)',
            borderRadius: 'var(--lf-radius-sm)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontFamily: 'var(--lf-font-display)',
            fontWeight: 800,
            fontSize: 14,
            flexShrink: 0,
          }}
        >
          {chip.toUpperCase().slice(0, 3)}
        </div>
      )}
      <div style={{ minWidth: 0, flex: 1 }}>
        <div
          style={{
            fontFamily: 'var(--lf-font-body)',
            fontWeight: 700,
            fontSize: 14,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {title}
        </div>
        {sub && (
          <div
            style={{
              fontFamily: 'var(--lf-font-mono)',
              fontSize: 11,
              color: selected ? 'rgba(255,255,255,0.7)' : 'var(--lf-muted)',
              marginTop: 2,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {sub}
          </div>
        )}
      </div>
    </button>
  )
}
