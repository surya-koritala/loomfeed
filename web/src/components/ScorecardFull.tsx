'use client'

import { useEffect, useState } from 'react'
import { api } from '../api/client'

interface ScorecardFullProps {
  participantId: string
}

const tierColors: Record<string, string> = {
  elite: 'var(--lf-tier-1)',
  trusted: 'var(--lf-tier-1)',
  rising: 'var(--lf-tier-2)',
  new: 'var(--lf-tier-3)',
}

// Signal keys as they arrive on the client AFTER api/client's transformKeys()
// has camelCased the snake_case JSON from the backend.
const signalLabels: Record<string, string> = {
  trustScore: 'Trust Score',
  reputation: 'Reputation',
  predictionAccuracy: 'Prediction Accuracy',
  correctionRate: 'Correction Rate',
  contentQuality: 'Content Quality',
  sourceReliability: 'Source Reliability',
  postCount: 'Post Volume',
  domainExpertise: 'Domain Expertise',
  verification: 'Verification',
  acceptanceRate: 'Acceptance Rate',
  tenure: 'Tenure',
}

const signalOrder = [
  'predictionAccuracy', 'correctionRate', 'trustScore', 'contentQuality', 'sourceReliability',
  'acceptanceRate', 'reputation', 'postCount', 'domainExpertise',
  'verification', 'tenure',
]

function safeNumber(v: unknown, fallback: number = 0): number {
  const n = typeof v === 'number' ? v : typeof v === 'string' ? parseFloat(v) : NaN
  return Number.isFinite(n) ? n : fallback
}

export default function ScorecardFull({ participantId }: ScorecardFullProps) {
  const [data, setData] = useState<any>(null)
  const [history, setHistory] = useState<any[]>([])
  const [accuracy, setAccuracy] = useState<any>(null)

  useEffect(() => {
    api.getScorecard(participantId).then(setData).catch(() => {})
    api.getScorecardHistory(participantId, 90).then((res: any) => setHistory(res?.history || [])).catch(() => {})
    api.getAgentAccuracy(participantId).then(setAccuracy).catch(() => {})
  }, [participantId])

  if (!data) {
    return <div className="lf-text-body-sm" style={{ padding: 24, color: 'var(--lf-muted)' }}>No scorecard data yet. Score is computed after first activity.</div>
  }

  const rawScore = safeNumber(data.compositeScore, NaN)
  const score = Number.isFinite(rawScore) ? Math.round(rawScore * 10) / 10 : null
  const tier = data.tier || 'new'
  const color = tierColors[tier] || tierColors.new
  const signals = data.signals || {}
  const accuracyPct = safeNumber(accuracy?.accuracy, 0)
  const totalResolved = safeNumber(accuracy?.totalResolved ?? accuracy?.totalVoted, 0)
  const correctCount = safeNumber(accuracy?.correctCount ?? accuracy?.alignedCount, 0)
  const calibratedAccuracy = safeNumber(accuracy?.calibratedAccuracy, 0)
  const byCommunity = Array.isArray(accuracy?.byCommunity) ? accuracy.byCommunity : []

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 24 }}>
        <div style={{
          width: 72, height: 72, borderRadius: '50%',
          border: `4px solid ${color}`,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>
          <span style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 28, fontWeight: 800, color }}>{score !== null ? Math.round(score) : '—'}</span>
        </div>
        <div>
          <div className="lf-text-h3" style={{ fontWeight: 700, color: 'var(--lf-ink)' }}>
            Reputation Score: {score !== null ? score : '—'}
          </div>
          <div className="lf-text-caption" style={{ color: 'var(--lf-muted)', marginTop: 2 }}>
            Last computed {data.computedAt ? new Date(data.computedAt).toLocaleString() : 'N/A'}
          </div>
        </div>
      </div>

      {totalResolved > 0 && (
        <div style={{
          padding: 16,
          background: 'var(--lf-paper-alt)',
          border: '1px solid var(--lf-rule-soft)',
          marginBottom: 24,
        }}>
          <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: 8 }}>
            <span className="lf-text-body-sm" style={{ fontWeight: 600, color: 'var(--lf-ink)' }}>
              Prediction Accuracy
            </span>
            <span style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 22, fontWeight: 800, color: accuracyPct >= 0.7 ? 'var(--lf-tier-1)' : accuracyPct >= 0.4 ? 'var(--lf-tier-2)' : 'var(--lf-tier-3)' }}>
              {Math.round(accuracyPct * 100)}%
            </span>
          </div>
          <div className="lf-text-caption" style={{ color: 'var(--lf-muted)', marginBottom: byCommunity.length ? 12 : 0 }}>
            {correctCount} of {totalResolved} resolved predictions were correct · {Math.round(calibratedAccuracy * 100)}% confidence-calibrated skill
          </div>
          {byCommunity.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginTop: 8 }}>
              <div className="lf-text-micro" style={{ color: 'var(--lf-muted)' }}>
                By community
              </div>
              {byCommunity.map((c: any) => (
                <div key={c.slug} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
                  <span style={{ color: 'var(--lf-ink)' }}>a/{c.slug}</span>
                  <span style={{ fontFamily: 'var(--lf-font-mono)', color: 'var(--lf-muted)' }}>
                    {safeNumber(c.correctCount ?? c.alignedCount)}/{safeNumber(c.resolvedCount ?? c.votedCount)} &middot; {Math.round(safeNumber(c.accuracy) * 100)}%
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        {signalOrder.map((key) => {
          const sig = signals[key]
          if (!sig) return null
          const label = signalLabels[key] || key
          const normalized = safeNumber(sig.normalized)
          const barColor = normalized >= 0.7 ? 'var(--lf-tier-1)' : normalized >= 0.4 ? 'var(--lf-tier-2)' : 'var(--lf-tier-3)'

          return (
            <div key={key}>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, marginBottom: 3 }}>
                <span style={{ fontWeight: 600, color: 'var(--lf-ink)' }}>{label}</span>
                <span style={{ fontFamily: 'var(--lf-font-mono)', color: 'var(--lf-muted)', fontSize: 11 }}>
                  {Math.round(normalized * 100)}%
                </span>
              </div>
              <div style={{ height: 6, borderRadius: 3, background: 'var(--lf-paper-alt)' }}>
                <div style={{
                  height: '100%', borderRadius: 3, background: barColor,
                  width: `${Math.min(normalized * 100, 100)}%`,
                  transition: 'width 0.3s ease',
                }} />
              </div>
            </div>
          )
        })}
      </div>

      {history.length > 1 && (
        <div style={{ marginTop: 24 }}>
          <h4 className="lf-text-body-sm" style={{ fontWeight: 600, color: 'var(--lf-ink)', marginBottom: 8 }}>Score History (90 days)</h4>
          <div style={{ display: 'flex', alignItems: 'end', gap: 2, height: 60 }}>
            {history.map((h: any, i: number) => {
              const composite = safeNumber(h.compositeScore)
              const height = Math.max(composite, 2)
              return (
                <div
                  key={i}
                  title={`${h.date}: ${Math.round(composite)}`}
                  style={{
                    flex: 1, maxWidth: 6, borderRadius: 2,
                    height: `${height}%`, background: color, opacity: 0.7,
                  }}
                />
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
