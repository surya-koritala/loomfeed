// web/src/components/lf/LFButton.tsx
import React from 'react'
import Link from 'next/link'
import { lfWeight, lfTracking } from '../../lib/lf-tokens'

// Primary / accent / ghost / danger button. Every CTA on the new UI
// passes through this. Per DESIGN_TOKENS.md the hard shadow is retired
// except on the lime `accent` (Create) CTA — the one-off "primary
// action" signature — so primary/danger are flat fills and ghost has a
// rule-mid border. Buttons are pills (action-button radius). Primitive
// only: no loading/disabled animation logic; callers add that via props.
export type LFButtonVariant = 'primary' | 'accent' | 'ghost' | 'danger'
export type LFButtonSize = 'sm' | 'md' | 'lg'

export interface LFButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: LFButtonVariant
  size?: LFButtonSize
  icon?: React.ReactNode
  fullWidth?: boolean
  /** When set, render as a Next.js Link instead of <button>. */
  href?: string
  /** target attr when rendering as a link (e.g. "_blank"). */
  target?: string
  /** rel attr when rendering as a link. */
  rel?: string
}

const SIZE_STYLES: Record<LFButtonSize, { padding: string; fontSize: number; gap: number }> = {
  sm: { padding: '6px 12px', fontSize: 13, gap: 6 },
  md: { padding: '10px 18px', fontSize: 14, gap: 8 },
  lg: { padding: '14px 24px', fontSize: 16, gap: 10 },
}

export function LFButton({
  variant = 'primary',
  size = 'md',
  icon,
  fullWidth,
  children,
  style,
  ...rest
}: LFButtonProps) {
  const s = SIZE_STYLES[size]
  const variantStyle = ((): React.CSSProperties => {
    switch (variant) {
      case 'primary':
        return {
          background: 'var(--lf-ink)',
          color: 'var(--lf-paper)',
          borderColor: 'var(--lf-ink)',
          boxShadow: 'none',
        }
      case 'accent':
        // The lime Create CTA — the one surface that keeps the hard
        // offset shadow signature.
        return {
          background: 'var(--lf-accent)',
          color: 'var(--lf-ink)',
          borderColor: 'var(--lf-ink)',
          boxShadow: 'var(--lf-shadow-hard-sm)',
        }
      case 'ghost':
        return {
          background: 'transparent',
          color: 'var(--lf-ink)',
          borderColor: 'var(--lf-rule-mid)',
          boxShadow: 'none',
        }
      case 'danger':
        return {
          background: 'var(--lf-accent-2)',
          color: 'var(--lf-paper)',
          borderColor: 'var(--lf-accent-2)',
          boxShadow: 'none',
        }
    }
  })()

  const mergedStyle: React.CSSProperties = {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: s.gap,
    padding: s.padding,
    fontFamily: 'var(--lf-font-body)',
    fontSize: s.fontSize,
    fontWeight: lfWeight.bodyBold,
    letterSpacing: lfTracking.bodyTight,
    border: `var(--lf-border-w) solid`,
    borderRadius: 'var(--lf-radius-pill)',
    cursor: rest.disabled ? 'not-allowed' : 'pointer',
    opacity: rest.disabled ? 0.5 : 1,
    width: fullWidth ? '100%' : undefined,
    textDecoration: 'none',
    ...variantStyle,
    ...style,
  }

  if (rest.href !== undefined) {
    const { href, target, rel, disabled: _ignored, type: _ignored2, onClick: _ignored3, ...anchorRest } = rest as any
    return (
      <Link
        href={href}
        target={target}
        rel={rel}
        {...anchorRest}
        style={mergedStyle}
      >
        {icon}
        {children}
      </Link>
    )
  }
  return (
    <button {...rest} style={mergedStyle}>
      {icon}
      {children}
    </button>
  )
}
