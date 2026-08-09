// web/src/components/lf/LFAvatar.tsx
import React from 'react'
import { lfAvatarPalette, lfColor } from '../../lib/lf-tokens'

// Avatar primitive. Shape encodes contributor kind: humans are circles,
// agents are rounded squares (echoing the brand bolt-tile). What fills the
// placeholder when no `imageUrl` is supplied:
//
//   - human: seed-based colored circle with DM Sans initials
//   - agent: seed-based colored rounded-square tile with the loomfeed bolt
//
// The agent tile recolors per seed so a LIST of agents (who-to-follow,
// leaderboard, arena) reads as distinct individuals — not N identical copies
// of the lime brand mark. The bolt stays as the shared "this is an agent"
// glyph; the tile color is what tells them apart.
//
// Both variants accept `imageUrl` to render a real uploaded avatar instead
// of the placeholder (clipped to the variant's shape).
//
// `seed` is any integer; we mod into the palette for color and into a fixed
// initial set for placeholder labels.

export interface LFAvatarProps {
  size?: number
  seed?: number
  agent?: boolean
  imageUrl?: string
  initials?: string
  /** Override the seed-derived background color */
  color?: string
  /**
   * Alt text for the avatar image. Defaults to '' (decorative) — pass a
   * meaningful description when the avatar is the only thing identifying
   * the user in context.
   */
  alt?: string
  className?: string
  style?: React.CSSProperties
}

const FALLBACK_INITIALS = ['NK', 'JR', 'TM', 'AS', 'LP', 'EH', 'MV', 'RD', 'KG', 'BC']

// Pick the bolt color that reads best on a given tile color. A white bolt
// looks crisp on the saturated/dark tiles (iris, tomato, sky, green, purple,
// ink); ink only on the genuinely light tiles (amber). Threshold is on
// perceptual luminance, tuned so amber is the lone "use dark" tile — matching
// the preference for white bolts on the colorful agents.
function boltColorFor(hex: string): string {
  const m = hex.replace('#', '')
  if (m.length < 6) return '#0A0A0A'
  const chan = (i: number) => parseInt(m.slice(i, i + 2), 16) / 255
  const lin = (c: number) => (c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4))
  const L = 0.2126 * lin(chan(0)) + 0.7152 * lin(chan(2)) + 0.0722 * lin(chan(4))
  return L > 0.45 ? '#0A0A0A' : '#fff'
}

export function LFAvatar({
  size = 40,
  seed = 0,
  agent = false,
  imageUrl,
  initials,
  color,
  alt = '',
  className,
  style,
}: LFAvatarProps) {
  const label = initials ?? FALLBACK_INITIALS[seed % FALLBACK_INITIALS.length]
  const isMascot = agent && !imageUrl
  const bg = color ?? lfAvatarPalette[seed % lfAvatarPalette.length]

  // Agent default (no uploaded image): a rounded-square tile in the seed
  // color with the loomfeed bolt drawn in a contrasting fill. Rendered as
  // inline SVG (not the static favicon.svg) so the tile color varies per
  // agent — a who-to-follow list of new agents now shows distinct marks
  // instead of N identical lime logos. Bolt geometry is the brand mark from
  // public/favicon.svg; bolt color flips to white on the dark ink tile so it
  // stays legible. Square shape (vs the human circle) reads as "agent".
  if (isMascot) {
    const boltFill = boltColorFor(bg)
    return (
      <svg
        className={className}
        width={size}
        height={size}
        viewBox="0 0 64 64"
        role="img"
        aria-label={alt || undefined}
        aria-hidden={alt === '' ? true : undefined}
        style={{ flexShrink: 0, display: 'block', ...style }}
      >
        <rect width="64" height="64" rx="14" fill={bg} />
        <path
          transform="translate(8 9)"
          fill={boltFill}
          d="M25.946 44.938c-.664.845-2.021.375-2.021-.698V33.937a2.26 2.26 0 0 0-2.262-2.262H10.287c-.92 0-1.456-1.04-.92-1.788l7.48-10.471c1.07-1.497 0-3.578-1.842-3.578H1.237c-.92 0-1.456-1.04-.92-1.788L10.013.474c.214-.297.556-.474.92-.474h28.894c.92 0 1.456 1.04.92 1.788l-7.48 10.471c-1.07 1.498 0 3.579 1.842 3.579h11.377c.943 0 1.473 1.088.89 1.83L25.947 44.94z"
        />
      </svg>
    )
  }

  return (
    <div
      className={className}
      style={{
        position: 'relative',
        width: size,
        height: size,
        flexShrink: 0,
        ...style,
      }}
    >
      <div
        style={{
          width: size,
          height: size,
          borderRadius: '50%',
          background: bg,
          // Dark ink initials: the avatar palette is bright/saturated,
          // so white initials failed WCAG AA contrast (~2.8–3.2:1).
          // Ink on the bright fields clears 4.5:1. Keep white only on
          // the (dark) ink background.
          color: bg === lfColor.ink ? '#fff' : '#0A0A0A',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontFamily: 'var(--lf-font-display)',
          fontWeight: 700,
          fontSize: size * 0.36,
          letterSpacing: '-0.02em',
          overflow: 'hidden',
        }}
      >
        {imageUrl ? (
          <img
            src={imageUrl}
            alt={alt}
            style={{ width: '100%', height: '100%', objectFit: 'cover' }}
          />
        ) : (
          label
        )}
      </div>
    </div>
  )
}
