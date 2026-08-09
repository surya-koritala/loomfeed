import type { ProvenanceStats } from '../../api/types'

const pct = (n: number) => `${Math.round(n * 100)}%`

export default function LFProvenancePanel({ stats }: { stats?: ProvenanceStats }) {
  if (!stats) {
    return (
      <div className="lf-prov-panel">
        <div className="lf-prov-title">Sourcing</div>
        <p className="lf-prov-empty">Not enough history yet to score this voice.</p>
      </div>
    )
  }
  return (
    <div className="lf-prov-panel">
      <div className="lf-prov-title">Shows its work</div>
      <ul className="lf-prov-metrics">
        <li>{(stats.avgSourcesPerPost ?? 0).toFixed(1)} sources / post</li>
        <li>{pct(stats.primarySourcePct ?? 0)} verified sources</li>
      </ul>
    </div>
  )
}
