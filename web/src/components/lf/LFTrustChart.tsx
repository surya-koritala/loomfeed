// web/src/components/lf/LFTrustChart.tsx
import React from 'react'
import { lfColor } from '../../lib/lf-tokens'

// 90-day trust trajectory. Pure SVG — no chart library, no data deps.
// Caller passes 90 numbers (one per day, oldest first). Component
// auto-scales to data range and draws line + lime fill below.
//
// If <90 points, renders what's given. Empty → nothing (no axes).
export interface LFTrustChartProps {
  points: readonly number[]
  height?: number
  className?: string
}

export function LFTrustChart({ points, height = 160, className }: LFTrustChartProps) {
  if (points.length === 0) return null

  const W = 800
  const H = height
  const P = 8

  const min = Math.min(...points)
  const max = Math.max(...points)
  const range = max - min || 1

  const x = (i: number) => P + (i / Math.max(1, points.length - 1)) * (W - 2 * P)
  const y = (v: number) => P + (1 - (v - min) / range) * (H - 2 * P)

  const path = points
    .map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(v).toFixed(1)}`)
    .join(' ')
  const area = `${path} L${x(points.length - 1).toFixed(1)},${H - P} L${x(0).toFixed(1)},${H - P} Z`

  return (
    <svg
      className={className}
      viewBox={`0 0 ${W} ${H}`}
      style={{ width: '100%', height, display: 'block' }}
      role="img"
      aria-label={`Reputation trajectory: ${Math.round(points[0]).toLocaleString()} to ${Math.round(points[points.length - 1]).toLocaleString()}`}
    >
      <path d={area} fill={lfColor.accent} opacity="0.25" />
      <path d={path} fill="none" stroke={lfColor.ink} strokeWidth="2" />
      {points.map((v, i) =>
        i % 30 === 0 ? (
          <circle key={i} cx={x(i)} cy={y(v)} r="3" fill={lfColor.ink} />
        ) : null
      )}
    </svg>
  )
}
