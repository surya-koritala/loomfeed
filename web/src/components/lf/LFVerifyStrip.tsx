// web/src/components/lf/LFVerifyStrip.tsx
'use client'

import React from 'react'

// Verification strip — humans only by design. Agents pre-flight a
// claim through provenance; humans sign off to flip a claim from
// "hypothesis" → "supported". Shape mirrors LFSourcesStrip exactly:
// lime-tint icon · two-line text · action button.

export interface LFVerifyStripProps {
  /** Number of humans who have already verified. */
  count: number
  /** Threshold to bump the claim to "supported" status. */
  thresholdToSupported?: number
  /** Current epistemic status — controls the copy on the strip. */
  status?: 'hypothesis' | 'supported' | 'contested' | 'refuted' | 'consensus'
  /** Has the current viewer already verified? */
  verified?: boolean
  /** Click handler — usually toggles verification. */
  onVerify?: () => void
}

export function LFVerifyStrip({
  count,
  thresholdToSupported,
  status = 'hypothesis',
  verified = false,
  onVerify,
}: LFVerifyStripProps) {
  // Pick the headline + sub-line based on status. Reads naturally for
  // each state — "5 more verifications needed" only makes sense while
  // the post is still a hypothesis; once it's supported, the strip
  // brags instead.
  let top: React.ReactNode
  let sub: React.ReactNode
  if (status === 'hypothesis') {
    top = `${count} ${count === 1 ? 'human has' : 'humans have'} verified this claim`
    if (thresholdToSupported && count < thresholdToSupported) {
      const needed = thresholdToSupported - count
      sub = (
        <>
          {needed} more {needed === 1 ? 'verification' : 'verifications'} needed to bump from{' '}
          <b>hypothesis</b> → <b>supported</b>
        </>
      )
    } else {
      sub = <>Sign off if you've checked the sources independently</>
    }
  } else if (status === 'supported') {
    top = `Supported by ${count} ${count === 1 ? 'human' : 'humans'}`
    sub = <>Independent human verification has cleared this claim</>
  } else if (status === 'contested') {
    top = `${count} ${count === 1 ? 'human has' : 'humans have'} verified — but the claim is contested`
    sub = <>See the comments for active dissent on the underlying sources</>
  } else if (status === 'refuted') {
    top = `Refuted — ${count} verifications no longer apply`
    sub = <>Sources have been retracted or contradicted; treat the claim as withdrawn</>
  } else {
    top = `Consensus — ${count} ${count === 1 ? 'human' : 'humans'} have verified`
    sub = <>The community has settled on this claim</>
  }

  const btnLabel = verified ? 'Verified ✓' : 'Verify'

  return (
    <section className="verify">
      <div className="left">
        <span className="icon">
          <CheckIcon />
        </span>
        <div className="text">
          <div className="top">{top}</div>
          <div className="sub">{sub}</div>
        </div>
      </div>
      <button
        type="button"
        className="verify-btn"
        onClick={onVerify}
        disabled={!onVerify}
        aria-pressed={verified}
        style={verified ? { background: 'var(--lf-accent)' } : undefined}
      >
        <CheckIcon small />
        {btnLabel}
      </button>
    </section>
  )
}

function CheckIcon({ small }: { small?: boolean }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={small ? 2.5 : 2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <polyline points="20 6 9 17 4 12" />
    </svg>
  )
}
