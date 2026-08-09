// web/src/components/lf/LFLogo.tsx
import React from 'react'

// Loomfeed lockup. Renders the brand asset directly from
// /public/brand/ instead of redrawing it inline — keeps a single
// source of truth (the original SVG export from the brand toolkit)
// and avoids drift if proportions are tuned upstream.
//
// Three variants ship in /public/brand. The brand mark is the ink
// bolt on a lime tile — the SAME mark as the favicon, maskable icon,
// OG image and agent mascot (it replaced an off-system lime pill):
//   logo_dark       — ink wordmark + bolt tile (use on cream/paper)
//   logo_light      — paper wordmark + bolt tile (use on ink surfaces)
//   logo_on_lime    — ink wordmark + ink bolt on the lime field (CTAs)
//
// SVG is preferred; PNG fallbacks ship for the rare environment
// where SVG can't be used (email clients, some screenshot tooling).

export type LFLogoVariant = 'dark' | 'light' | 'on_lime'

export interface LFLogoProps {
  /** Render height in px. Width auto-derives from the brand aspect ratio. */
  size?: number
  /** Which brand asset to use. Defaults to `dark` (ink on cream/paper). */
  variant?: LFLogoVariant
  className?: string
  /** Force PNG. Defaults to false — SVG works in every browser we support. */
  prefersPng?: boolean
  /** Override alt text. Defaults to "loomfeed". */
  alt?: string
}

// Brand asset native dimensions (dark/light lockup): 1736 × 416.64
// (≈ 4.166:1). Used only to reserve width and avoid layout shift —
// the rendered width comes from the SVG's own viewBox (style width:auto).
const BRAND_RATIO = 1736 / 416.64

// Cache-bust token. /brand/* assets are served `immutable, max-age=1yr`
// (next.config.js), so the CDN and browsers pin the old bytes to the
// stable filename — a content change to the same path stays invisible.
// Bump this whenever a brand asset's pixels change to force a fresh
// fetch (a new query string is a new cache key for browser + CDN alike).
// v2 = pill → bolt-tile mark (#150).
const ASSET_V = '2'
const v = (p: string) => `${p}?v=${ASSET_V}`

const FILES: Record<LFLogoVariant, { svg: string; png: string }> = {
  dark:    { svg: v('/brand/logo_dark.svg'),    png: v('/brand/logo_dark.png') },
  light:   { svg: v('/brand/logo_light.svg'),   png: v('/brand/logo_light_on_ink.png') },
  on_lime: { svg: v('/brand/logo_on_lime.svg'), png: v('/brand/logo_on_lime.png') },
}

export function LFLogo({
  size = 28,
  variant = 'dark',
  className,
  prefersPng,
  alt = 'loomfeed',
}: LFLogoProps) {
  const f = FILES[variant]
  const src = prefersPng ? f.png : f.svg
  const width = Math.round(size * BRAND_RATIO)
  return (
    <img
      src={src}
      alt={alt}
      className={className}
      width={width}
      height={size}
      style={{
        display: 'inline-block',
        height: size,
        width: 'auto',
        // Prevent the browser from inverting in dark-mode user-agent
        // styles or shrinking unexpectedly inside a flex parent.
        flexShrink: 0,
      }}
    />
  )
}
