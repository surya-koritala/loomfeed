// web/src/lib/lf-tokens.ts

// lf-v2 design tokens — JS mirror. The AUTHORITATIVE values live under
// `body.lf-v2 { ... }` in src/index.css (DESIGN_TOKENS.md is the spec).
// This file is a runtime mirror for the handful of values JS computes
// dynamically (SVG stroke colors, the avatar palette). Keep it in sync
// with index.css — if you change a value there, change it here. (These
// had drifted: Inter/JetBrains fonts, radius 18, and a WCAG-failing
// #8B8E94 muted-2 that index.css already corrected to #6B6E76.)
//
// JS-side use: components that compute styles dynamically (e.g. SVG
// stroke colors, dynamically-toned trust chips) import from this
// module. Static styles can use the CSS variables directly.

export const lfColor = {
  paper:     '#FFFFFF',
  ink:       '#0A0A0A',
  accent:    '#D4FF3A', // lime
  accent2:   '#FF5436', // tomato
  accent3:   '#5B5BFF', // iris
  seal:      '#00A86B', // green
  contested: '#D97706', // amber
  refuted:   '#FF5436',
  muted:     '#6B6E76',
  muted2:    '#6B6E76', // was #8B8E94 (3.28:1, fails WCAG AA on white)
  paperAlt:  '#F6F7F9', // cool gray-50, replaces warm cream
} as const;

export const lfBorder = {
  width: 1,
  color: lfColor.ink,
} as const;

export const lfRadius = {
  base: 12, // matches --lf-radius (the 18px chunky-card radius was retired)
  sm: 8,
  pill: 999,
} as const;

export const lfShadow = {
  hard: `4px 4px 0 ${lfColor.ink}`,
  hardSm: `2px 2px 0 ${lfColor.ink}`,
  none: 'none',
} as const;

export const lfFont = {
  display: '"DM Sans", system-ui, sans-serif',
  body:    '"DM Sans", system-ui, sans-serif',     // was Inter — body+display share DM Sans
  mono:    '"DM Mono", ui-monospace, monospace',   // was JetBrains Mono
} as const;

export const lfWeight = {
  display: 800,
  body: 400,
  bodyBold: 600,
  mono: 500,
} as const;

export const lfTracking = {
  display: '-0.03em',
  displayTight: '-0.04em', // Logo wordmark — slightly tighter than headings
  body: 'normal',
  bodyTight: '-0.01em',    // Button labels, post titles — denser than body
  mono: '0.04em',
} as const;

// Avatar palette — used by LFAvatar to deterministically tone a human
// avatar from an integer seed (typically last 6 hex chars of a UUID
// converted to int). Same palette as the design mocks' `LF_AVATAR_PALETTE`.
export const lfAvatarPalette = [
  lfColor.accent2,    // tomato
  lfColor.accent3,    // iris
  lfColor.ink,        // ink
  '#FFB02E',          // amber
  lfColor.seal,       // green
  '#B33FE5',          // purple
  '#0EA5E9',          // sky
] as const;
