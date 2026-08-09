// Loomfeed icon library. Lucide-style stroked SVGs at 24×24 viewBox,
// 1.75px stroke, square caps, round joins. Replace the unicode glyph
// soup (✦ ⚔ ◯ ⌕ ◇ ◐ ↪ ✎ ⊖ ⊕) with these so the chrome reads as a
// designed icon set instead of typographic stand-ins.
//
// Usage:
//   import { IconHome } from './icons'
//   <IconHome size={20} />
//
// All icons accept `size` (sets width+height in px), `color` (defaults
// to currentColor so they inherit text color), and `strokeWidth`.
// `aria-hidden` is on by default — pass aria-label via the `title`
// prop when the icon stands alone (no adjacent text label).

import React from 'react'

export interface LFIconProps {
  size?: number
  color?: string
  strokeWidth?: number
  className?: string
  title?: string
  style?: React.CSSProperties
}

interface IconWrapProps extends LFIconProps {
  children: React.ReactNode
}

function IconWrap({ size = 20, color = 'currentColor', strokeWidth = 1.75, className, title, style, children }: IconWrapProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke={color}
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      style={style}
      role={title ? 'img' : undefined}
      aria-label={title}
      aria-hidden={title ? undefined : true}
    >
      {title && <title>{title}</title>}
      {children}
    </svg>
  )
}

/* ─────────────────────── Navigation ─────────────────────── */

export function IconHome(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M3 11.5 12 4l9 7.5" />
      <path d="M5 10v10h14V10" />
    </IconWrap>
  )
}

export function IconSearch(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <circle cx="11" cy="11" r="6.5" />
      <path d="m20 20-3.5-3.5" />
    </IconWrap>
  )
}

export function IconCommunity(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <circle cx="9" cy="9" r="3.5" />
      <circle cx="17" cy="11" r="2.5" />
      <path d="M3 19c0-3 2.7-5 6-5s6 2 6 5" />
      <path d="M14 19c0-2 1.7-3.5 4-3.5s3 1.5 3 3.5" />
    </IconWrap>
  )
}

export function IconArena(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      {/* Shield (battle/arena). The crossed-swords version read as a
          close "✕" at nav size, which was confusing in the tab bar. */}
      <path d="M12 2 4 6v6c0 5 3.5 9.5 8 10 4.5-.5 8-5 8-10V6z" />
    </IconWrap>
  )
}

export function IconFootball(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      {/* Football: circle + central pentagon + seam spokes. */}
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7.5 16.3 10.6 14.6 15.6 9.4 15.6 7.7 10.6Z" />
      <path d="M12 7.5V3" />
      <path d="M16.3 10.6 20.6 9.2" />
      <path d="M14.6 15.6 17.3 19.3" />
      <path d="M9.4 15.6 6.7 19.3" />
      <path d="M7.7 10.6 3.4 9.2" />
    </IconWrap>
  )
}

export function IconLeaderboard(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <rect x="4" y="13" width="4" height="7" rx="0.5" />
      <rect x="10" y="8" width="4" height="12" rx="0.5" />
      <rect x="16" y="11" width="4" height="9" rx="0.5" />
    </IconWrap>
  )
}

export function IconNotification(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M6 9a6 6 0 0 1 12 0c0 5 2 6 2 6H4s2-1 2-6Z" />
      <path d="M10 19a2 2 0 0 0 4 0" />
    </IconWrap>
  )
}

export function IconConnect(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M9 7a3 3 0 1 0 0 10" />
      <path d="M15 7a3 3 0 1 1 0 10" />
      <path d="M9 12h6" />
    </IconWrap>
  )
}

/* ─────────────────────── Actions ─────────────────────── */

export function IconCompose(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M4 20h4l11-11-4-4L4 16v4Z" />
      <path d="m14 6 4 4" />
    </IconWrap>
  )
}

export function IconUpvote(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M12 4v16" />
      <path d="m6 10 6-6 6 6" />
    </IconWrap>
  )
}

export function IconDownvote(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M12 4v16" />
      <path d="m6 14 6 6 6-6" />
    </IconWrap>
  )
}

export function IconStar(p: LFIconProps & { filled?: boolean }) {
  const { filled, ...rest } = p
  return (
    <IconWrap {...rest}>
      <path
        d="M12 3.5 14.6 9l5.9.6-4.4 4 1.2 5.9L12 16.6 6.7 19.5l1.2-5.9-4.4-4L9.4 9 12 3.5Z"
        fill={filled ? 'currentColor' : 'none'}
      />
    </IconWrap>
  )
}

export function IconReply(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="m9 14-5-5 5-5" />
      <path d="M4 9h11a5 5 0 0 1 5 5v6" />
    </IconWrap>
  )
}

export function IconComment(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M4 6a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2h-7l-4 4v-4H6a2 2 0 0 1-2-2Z" />
    </IconWrap>
  )
}

export function IconShare(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <circle cx="6" cy="12" r="2.5" />
      <circle cx="18" cy="6" r="2.5" />
      <circle cx="18" cy="18" r="2.5" />
      <path d="m8 11 8-4" />
      <path d="m8 13 8 4" />
    </IconWrap>
  )
}

export function IconBookmark(p: LFIconProps & { filled?: boolean }) {
  const { filled, ...rest } = p
  return (
    <IconWrap {...rest}>
      <path
        d="M6 4h12v17l-6-4-6 4V4Z"
        fill={filled ? 'currentColor' : 'none'}
      />
    </IconWrap>
  )
}

export function IconLink(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M10 14a4 4 0 0 0 5.7.3l3-3a4 4 0 0 0-5.7-5.7L11.5 7" />
      <path d="M14 10a4 4 0 0 0-5.7-.3l-3 3a4 4 0 0 0 5.7 5.7l1.5-1.4" />
    </IconWrap>
  )
}

export function IconCite(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M7 7h4v4H7l-2 4V7Z" />
      <path d="M14 7h4v4h-4l-2 4V7Z" />
    </IconWrap>
  )
}

/* ─────────────────────── Affordances ─────────────────────── */

export function IconCheck(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="m5 12 5 5 9-11" />
    </IconWrap>
  )
}

export function IconClose(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M6 6 18 18" />
      <path d="M18 6 6 18" />
    </IconWrap>
  )
}

export function IconPlus(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M12 5v14" />
      <path d="M5 12h14" />
    </IconWrap>
  )
}

export function IconMinus(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M5 12h14" />
    </IconWrap>
  )
}

export function IconChevronUp(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="m6 14 6-6 6 6" />
    </IconWrap>
  )
}

export function IconChevronDown(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="m6 10 6 6 6-6" />
    </IconWrap>
  )
}

export function IconChevronLeft(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="m14 6-6 6 6 6" />
    </IconWrap>
  )
}

export function IconChevronRight(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="m10 6 6 6-6 6" />
    </IconWrap>
  )
}

export function IconArrowRight(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M5 12h14" />
      <path d="m13 5 7 7-7 7" />
    </IconWrap>
  )
}

export function IconExternal(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M14 5h5v5" />
      <path d="M19 5 9 15" />
      <path d="M19 14v5H5V5h5" />
    </IconWrap>
  )
}

export function IconCopy(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <rect x="8" y="8" width="12" height="12" rx="2" />
      <path d="M16 8V5a1 1 0 0 0-1-1H5a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3" />
    </IconWrap>
  )
}

export function IconGlobe(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18" />
      <path d="M12 3a14 14 0 0 1 4 9 14 14 0 0 1-4 9 14 14 0 0 1-4-9 14 14 0 0 1 4-9Z" />
    </IconWrap>
  )
}

export function IconCornerUpRight(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="m15 14 5-5-5-5" />
      <path d="M4 20v-7a4 4 0 0 1 4-4h12" />
    </IconWrap>
  )
}

/* ─────────────────────── Status / verification ─────────────────────── */

export function IconShield(p: LFIconProps & { filled?: boolean }) {
  const { filled, ...rest } = p
  return (
    <IconWrap {...rest}>
      <path
        d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6l-8-3Z"
        fill={filled ? 'currentColor' : 'none'}
      />
    </IconWrap>
  )
}

export function IconAgent(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <rect x="5" y="6" width="14" height="13" rx="2" />
      <path d="M9 11h.01" />
      <path d="M15 11h.01" />
      <path d="M12 3v3" />
      <path d="M9 16h6" />
    </IconWrap>
  )
}

export function IconHuman(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <circle cx="12" cy="8" r="3.5" />
      <path d="M5 20c0-3.5 3-6 7-6s7 2.5 7 6" />
    </IconWrap>
  )
}

export function IconTrending(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="m3 17 6-6 4 4 8-8" />
      <path d="M14 7h7v7" />
    </IconWrap>
  )
}

export function IconSparkle(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M12 4v3" />
      <path d="M12 17v3" />
      <path d="M5 12H2" />
      <path d="M22 12h-3" />
      <path d="m7 7 2 2" />
      <path d="m15 15 2 2" />
      <path d="m17 7-2 2" />
      <path d="m9 15-2 2" />
    </IconWrap>
  )
}

export function IconSettings(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <circle cx="12" cy="12" r="3" />
      <path d="M19 12a7 7 0 0 0-.1-1.2l2-1.5-2-3.4-2.4.8a7 7 0 0 0-2.1-1.2L14 3h-4l-.4 2.5a7 7 0 0 0-2.1 1.2l-2.4-.8-2 3.4 2 1.5A7 7 0 0 0 5 12c0 .4 0 .8.1 1.2l-2 1.5 2 3.4 2.4-.8a7 7 0 0 0 2.1 1.2L10 21h4l.4-2.5a7 7 0 0 0 2.1-1.2l2.4.8 2-3.4-2-1.5c.1-.4.1-.8.1-1.2Z" />
    </IconWrap>
  )
}

export function IconLogOut(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <path d="M9 5H5v14h4" />
      <path d="m16 8 4 4-4 4" />
      <path d="M20 12H10" />
    </IconWrap>
  )
}

export function IconMore(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <circle cx="5" cy="12" r="1.6" fill="currentColor" />
      <circle cx="12" cy="12" r="1.6" fill="currentColor" />
      <circle cx="19" cy="12" r="1.6" fill="currentColor" />
    </IconWrap>
  )
}

/* ─────────────────────── Sports timeline ─────────────────────── */
// Match-event glyphs for the broadcast timeline. Unlike IconFootball
// (nav-sized, stroked pentagon + five spokes) these are tuned to stay
// legible at 14px: fewer strokes, a filled pentagon for the ball.

export function IconBall(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <circle cx="12" cy="12" r="8.5" />
      <path
        d="M12 9 14.85 11.07 13.76 14.43 10.24 14.43 9.15 11.07Z"
        fill="currentColor"
        stroke="none"
      />
      <path d="M9.15 11.07 6.2 9.8" />
      <path d="M14.85 11.07 17.8 9.8" />
    </IconWrap>
  )
}

export function IconCardSwatch(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      {/* Booking card — filled swatch, colored via CSS currentColor. */}
      <rect x="8.5" y="5" width="7" height="14" rx="1.5" fill="currentColor" stroke="none" />
    </IconWrap>
  )
}

export function IconSubArrows(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      {/* Substitution: player off (left), player on (right). */}
      <path d="M19 8H5" />
      <path d="m9 4-4 4 4 4" />
      <path d="M5 16h14" />
      <path d="m15 12 4 4-4 4" />
    </IconWrap>
  )
}

export function IconWhistle(p: LFIconProps) {
  return (
    <IconWrap {...p}>
      <circle cx="9" cy="14" r="5.5" />
      <path d="M9 8.5h11.5v4h-6.2" />
      <circle cx="9" cy="14" r="1" fill="currentColor" stroke="none" />
    </IconWrap>
  )
}
