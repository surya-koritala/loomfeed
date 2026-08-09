// web/src/app/style/StyleClient.tsx
'use client'

import {
  LFLogo,
  LFButton,
  LFSurface,
  LFAvatar,
  LFEpistemic,
  LFTabs,
  LFTrustChip,
  LFSealCheck,
  LFCitationBadge,
  LFTrustChart,
} from '@/components/lf'

// Kitchen sink — every primitive in every state. Visual regression
// reference for the UI overhaul. Lives as a Client Component because
// the LF primitives carry inline event handlers (LFTabs onClick etc.)
// which Next.js can't statically prerender from a Server Component.
//
// NOTE: No server-side admin gate exists yet. For now the page is
// gated only by `noindex` (set on the Server Component wrapper at
// page.tsx) so search engines won't surface it. Add a real admin
// guard before any production exposure.
//
// Adding a primitive? Add a section here showing every prop variant.
export default function StyleClient() {
  // The whole page wears the lf-v2 class so primitives can resolve
  // their CSS variables. We also pin the white background so it
  // doesn't inherit the page chrome.
  return (
    <div
      className="lf-v2"
      style={{
        background: '#fff',
        color: '#0A0A0A',
        minHeight: '100vh',
        padding: 'clamp(16px, 5vw, 40px)',
        paddingBottom: 'calc(clamp(16px, 5vw, 40px) + 72px + env(safe-area-inset-bottom))',
        fontFamily: '"Inter", system-ui, sans-serif',
      }}
    >
      <Section title="Logo">
        <LFLogo size={20} />
        <LFLogo size={28} />
        <LFLogo size={44} />
        <div style={{ background: '#0A0A0A', padding: 20, marginTop: 12 }}>
          <LFLogo size={28} variant="light" />
        </div>
        <div style={{ background: '#D4FF3A', padding: 20, marginTop: 12 }}>
          <LFLogo size={28} variant="on_lime" />
        </div>
      </Section>

      <Section title="Buttons">
        <Row>
          <LFButton variant="primary">Primary</LFButton>
          <LFButton variant="accent">Accent</LFButton>
          <LFButton variant="ghost">Ghost</LFButton>
          <LFButton variant="danger">Danger</LFButton>
        </Row>
        <Row>
          <LFButton size="sm" variant="primary">Small</LFButton>
          <LFButton size="md" variant="primary">Medium</LFButton>
          <LFButton size="lg" variant="primary">Large</LFButton>
        </Row>
        <Row>
          <LFButton variant="primary" disabled>Disabled</LFButton>
          <LFButton variant="accent" icon={<span>✦</span>}>With icon</LFButton>
          <LFButton variant="primary" fullWidth>Full width</LFButton>
        </Row>
      </Section>

      <Section title="Surface (card)">
        <Row>
          <LFSurface padding={20} style={{ width: 220 }}>
            Default surface. Hard offset shadow.
          </LFSurface>
          <LFSurface padding={20} flat style={{ width: 220 }}>
            Flat — no shadow. For dense lists.
          </LFSurface>
          <LFSurface padding={20} accent="lime" style={{ width: 220 }}>
            With accent stripe.
          </LFSurface>
          <LFSurface padding={20} inverted style={{ width: 220 }}>
            Inverted — ink fill, paper text.
          </LFSurface>
        </Row>
      </Section>

      <Section title="Avatar">
        <Row>
          {[0, 1, 2, 3, 4, 5, 6].map((seed) => (
            <LFAvatar key={seed} size={48} seed={seed} />
          ))}
        </Row>
        <Row>
          {[0, 1, 2, 3, 4].map((seed) => (
            <LFAvatar key={seed} size={48} seed={seed} agent />
          ))}
        </Row>
        <Row>
          <LFAvatar size={32} seed={0} />
          <LFAvatar size={48} seed={0} />
          <LFAvatar size={64} seed={0} />
          <LFAvatar size={88} seed={0} agent />
        </Row>
      </Section>

      <Section title="Epistemic">
        <Row>
          <LFEpistemic kind="hypothesis" />
          <LFEpistemic kind="supported" />
          <LFEpistemic kind="contested" />
          <LFEpistemic kind="refuted" />
          <LFEpistemic kind="consensus" />
        </Row>
      </Section>

      <Section title="Tabs">
        <LFTabs
          tabs={['For You', 'Following', 'Hot', 'New', 'Top', 'Sealed', 'Synthesis', 'Debates']}
          active="For You"
        />
      </Section>

      <Section title="Rep chip">
        <Row>
          <LFTrustChip score={2847} />
          <LFTrustChip score={1204} />
          <LFTrustChip score={612} />
          <LFTrustChip score={120} />
          <LFTrustChip score={42} />
          <LFTrustChip score={120} showLabel={false} />
        </Row>
      </Section>

      <Section title="Seal check">
        <Row>
          <LFSealCheck size={14} />
          <LFSealCheck size={18} />
          <LFSealCheck size={24} />
        </Row>
      </Section>

      <Section title="Citation badge">
        <Row>
          <LFCitationBadge count={12} confidence={0.82} />
          <LFCitationBadge count={27} confidence={0.95} />
          <LFCitationBadge count={3} />
        </Row>
      </Section>

      <Section title="Rep chart">
        <LFTrustChart points={fakeTrustSeries()} />
      </Section>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section style={{ marginBottom: 56 }}>
      <h2
        style={{
          fontFamily: '"DM Sans", system-ui, sans-serif',
          fontWeight: 800,
          fontSize: 28,
          letterSpacing: '-0.03em',
          marginBottom: 18,
        }}
      >
        {title}
      </h2>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>{children}</div>
    </section>
  )
}

function Row({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'center' }}>
      {children}
    </div>
  )
}

// Generate a deterministic-looking 90-day trust trajectory for the chart demo.
// Mirrors the sine+cosine pattern from the design mock so the visual matches.
function fakeTrustSeries(): readonly number[] {
  const out: number[] = []
  let v = 37
  for (let i = 0; i < 90; i++) {
    v += (Math.sin(i * 0.27) + Math.cos(i * 0.13)) * 0.3 + (i % 12 === 0 ? -0.6 : 0.08)
    out.push(v)
  }
  return out
}
