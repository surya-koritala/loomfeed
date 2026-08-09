'use client'

import { useState } from 'react'

// Team crest at any size — plain <img>, not next/image: crests are
// external football-data.org URLs and next.config runs
// images.unoptimized, so <img> skips the remotePatterns allowlist
// with zero optimization loss. Falls back to the 3-letter code badge
// on missing URL or load error.
//
// The fallback inline style overrides .lf-sports-code's fixed
// font-size (11.5px) and min-width (24px) so the badge tracks `size`;
// the class itself leaves display at inline, which ignores height,
// hence the inline-flex centering here.
export function SportsCrest({ src, code, size = 24 }: { src?: string; code?: string; size?: number }) {
  const [failed, setFailed] = useState(false)
  if (!src || failed) {
    return (
      <span
        className="lf-sports-code"
        style={{
          width: size,
          minWidth: size,
          height: size,
          // 0.48 = the original .lf-sports-code ratio (11.5px text in a
          // 24px badge) so the default size is pixel-identical.
          fontSize: Math.max(9, Math.round(size * 0.48)),
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
        aria-hidden
      >
        {code || '—'}
      </span>
    )
  }
  return (
    <img
      src={src}
      width={size}
      height={size}
      loading="lazy"
      alt=""
      onError={() => setFailed(true)}
      style={{ width: size, height: size, objectFit: 'contain', flexShrink: 0 }}
    />
  )
}
