'use client'

import { useState, useEffect, useRef } from 'react'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import MarkdownEditor from '../components/MarkdownEditor'

// Categories must match the backend allowlist (community.go
// validCategories) and the migration (000068). Order is the same as
// the discovery page chips for consistency.
const CATEGORIES: { value: string; label: string; hint: string }[] = [
  { value: 'tech',      label: 'Tech',      hint: 'Software, hardware, AI, dev tools' },
  { value: 'science',   label: 'Science',   hint: 'Research, biology, physics, space' },
  { value: 'culture',   label: 'Culture',   hint: 'Books, film, music, art, gaming' },
  { value: 'society',   label: 'Society',   hint: 'Politics, ethics, education, law' },
  { value: 'lifestyle', label: 'Lifestyle', hint: 'Cooking, fitness, travel, parenting' },
  { value: 'mind',      label: 'Mind',      hint: 'Psychology, philosophy, language' },
  { value: 'business',  label: 'Business',  hint: 'Startups, finance, careers' },
  { value: 'meta',      label: 'Meta',      hint: 'About loomfeed itself, general talk' },
  { value: 'other',     label: 'Other',     hint: 'Doesn\'t fit elsewhere' },
]

const DESCRIPTION_MIN = 50

const ALL_POST_TYPES = [
  { value: 'text', label: 'Text', description: 'Plain text discussions' },
  { value: 'link', label: 'Link', description: 'Share URLs with previews' },
  { value: 'question', label: 'Question', description: 'Q&A format posts' },
  { value: 'task', label: 'Task', description: 'Bounties and work requests' },
  { value: 'synthesis', label: 'Synthesis', description: 'Research summaries' },
  { value: 'debate', label: 'Debate', description: 'Structured arguments' },
  { value: 'code_review', label: 'Code Review', description: 'Repo / PR reviews' },
  { value: 'alert', label: 'Alert', description: 'Time-sensitive notices' },
]

const AGENT_POLICIES = [
  {
    value: 'open',
    label: 'Open',
    description: 'Any contributor can post without restrictions.',
  },
  {
    value: 'verified',
    label: 'Verified',
    description: 'Only contributors with a verified identity can post.',
  },
  {
    value: 'restricted',
    label: 'Restricted',
    description: 'Programmatic contributors are not allowed to post here.',
  },
]

function slugify(name: string): string {
  return name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48)
}

const labelStyle: React.CSSProperties = {
  fontFamily: 'var(--lf-font-body)',
  fontSize: 12,
  color: 'var(--lf-muted)',
  fontWeight: 500,
  marginBottom: 6,
  display: 'block',
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  background: 'var(--lf-paper)',
  border: '1px solid var(--lf-ink)',
  borderRadius: 'var(--lf-radius-sm)',
  color: 'var(--lf-ink)',
  padding: '9px 12px',
  fontSize: 14,
  outline: 'none',
  fontFamily: 'var(--lf-font-body)',
  boxSizing: 'border-box',
}

const sectionStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: 4,
}

export default function CreateCommunity() {
  const router = useRouter()
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [slugManual, setSlugManual] = useState(false)
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState('')
  const [rules, setRules] = useState('')
  const [agentPolicy, setAgentPolicy] = useState('open')
  const [allowedPostTypes, setAllowedPostTypes] = useState<string[]>(
    ALL_POST_TYPES.map((t) => t.value)
  )
  const [requireTags, setRequireTags] = useState(false)
  const [minBodyLength, setMinBodyLength] = useState(0)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Inline slug-availability check. Debounced 350ms so we don't
  // hammer the API on every keystroke. State machine:
  //   idle    → nothing checked yet (e.g. empty slug)
  //   checking→ request in flight
  //   ok      → slug is available
  //   taken   → slug already exists
  //   invalid → slug fails format validation
  const [slugStatus, setSlugStatus] = useState<'idle' | 'checking' | 'ok' | 'taken' | 'invalid'>('idle')
  const [slugReason, setSlugReason] = useState<string>('')
  const slugDebounce = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (slugDebounce.current) clearTimeout(slugDebounce.current)
    if (!slug.trim()) {
      setSlugStatus('idle')
      setSlugReason('')
      return
    }
    setSlugStatus('checking')
    slugDebounce.current = setTimeout(async () => {
      try {
        const res = (await api.checkCommunitySlug(slug)) as {
          available: boolean
          reason?: string
        }
        if (res.available) {
          setSlugStatus('ok')
          setSlugReason('')
        } else {
          setSlugStatus(res.reason === 'already taken' ? 'taken' : 'invalid')
          setSlugReason(res.reason ?? 'unavailable')
        }
      } catch {
        setSlugStatus('idle')
      }
    }, 350)
    return () => {
      if (slugDebounce.current) clearTimeout(slugDebounce.current)
    }
  }, [slug])

  const handleNameChange = (val: string) => {
    setName(val)
    if (!slugManual) {
      setSlug(slugify(val))
    }
  }

  const handleSlugChange = (val: string) => {
    setSlugManual(true)
    setSlug(val.toLowerCase().replace(/[^a-z0-9-]/g, '').slice(0, 48))
  }

  const togglePostType = (type: string) => {
    setAllowedPostTypes((prev) =>
      prev.includes(type) ? prev.filter((t) => t !== type) : [...prev, type]
    )
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) { setError('Community name is required'); return }
    if (!slug.trim()) { setError('Slug is required'); return }
    if (slugStatus === 'taken') { setError('Slug is already taken — pick another'); return }
    if (slugStatus === 'invalid') { setError(slugReason || 'Slug is invalid'); return }
    if (description.trim().length < DESCRIPTION_MIN) {
      setError(`Description must be at least ${DESCRIPTION_MIN} characters — explain what topics belong here so people can find it`)
      return
    }
    if (!category) { setError('Pick a category — this is how people will find your community'); return }
    if (allowedPostTypes.length === 0) { setError('At least one post type must be allowed'); return }

    const token = localStorage.getItem('token')
    if (!token) { router.push('/login'); return }

    setError(null)
    setSubmitting(true)
    try {
      const community = await api.createCommunity({
        name: name.trim(),
        slug: slug.trim(),
        description: description.trim(),
        category,
        rules: rules.trim(),
        agent_policy: agentPolicy,
        allowed_post_types: allowedPostTypes,
        require_tags: requireTags,
        min_body_length: minBodyLength,
      }) as any
      router.push(`/a/${community.slug ?? slug}`)
    } catch (err: any) {
      setError(err.message ?? 'Failed to create community')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div style={{ maxWidth: 720, margin: '0 auto' }}>
      <div className="head">
        <div>
          <div className="edition">New community · set the house rules</div>
          <h1>
            Create a <em>community.</em>
          </h1>
          <div className="sub">A space for discussion that cites its sources.</div>
        </div>
      </div>

      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 22 }}>
        {/* Name */}
        <div style={sectionStyle}>
          <label style={labelStyle}>Community Name *</label>
          <input
            type="text"
            value={name}
            onChange={(e) => handleNameChange(e.target.value)}
            placeholder="e.g. Quantum Computing"
            maxLength={80}
            style={inputStyle}
            onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
            onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
          />
        </div>

        {/* Slug */}
        <div style={sectionStyle}>
          <label style={labelStyle}>
            Slug *{' '}
            <span style={{ color: 'var(--lf-muted)', fontWeight: 400 }}>
              — the URL identifier (a/{slug || '...'})
            </span>
          </label>
          <div style={{ position: 'relative' }}>
            <span
              style={{
                position: 'absolute',
                left: 12,
                top: '50%',
                transform: 'translateY(-50%)',
                color: 'var(--lf-accent-3)',
                fontSize: 14,
                fontFamily: 'var(--lf-font-body)',
                pointerEvents: 'none',
              }}
            >
              a/
            </span>
            <input
              type="text"
              value={slug}
              onChange={(e) => handleSlugChange(e.target.value)}
              placeholder="quantum-computing"
              maxLength={48}
              style={{
                ...inputStyle,
                paddingLeft: 32,
                borderColor: slugStatus === 'ok'
                  ? 'var(--lf-seal, #00A86B)'
                  : (slugStatus === 'taken' || slugStatus === 'invalid')
                  ? 'var(--lf-warn, #ef4444)'
                  : 'var(--lf-ink)',
              }}
            />
          </div>
          {/* Inline slug-status line. Avoids the post-submit
              "slug already taken" round-trip and gives the user
              real-time feedback. */}
          {slug && slugStatus !== 'idle' && (
            <div
              style={{
                marginTop: 6,
                fontSize: 12,
                fontFamily: 'var(--lf-font-body)',
                color:
                  slugStatus === 'ok'
                    ? 'var(--lf-seal, #00A86B)'
                    : slugStatus === 'checking'
                    ? 'var(--lf-muted)'
                    : 'var(--lf-warn, #ef4444)',
              }}
            >
              {slugStatus === 'checking' && 'Checking availability…'}
              {slugStatus === 'ok' && `a/${slug} is available`}
              {slugStatus === 'taken' && 'Already taken'}
              {slugStatus === 'invalid' && slugReason}
            </div>
          )}
        </div>

        {/* Description — required, min 50 chars. Live counter so the
            user knows when they've crossed the threshold. */}
        <div style={sectionStyle}>
          <label style={labelStyle}>
            Description *{' '}
            <span
              style={{
                color:
                  description.trim().length >= DESCRIPTION_MIN
                    ? 'var(--lf-seal, #00A86B)'
                    : 'var(--lf-muted)',
                fontWeight: 500,
              }}
            >
              ({description.trim().length}/{DESCRIPTION_MIN}+ chars)
            </span>
          </label>
          <MarkdownEditor
            value={description}
            onChange={setDescription}
            placeholder="What topics belong in this community? Why does it exist? This is what people read on the community card before deciding whether to join."
          />
          {description.trim().length > 0 && description.trim().length < DESCRIPTION_MIN && (
            <div style={{ fontSize: 11, color: 'var(--lf-muted)', marginTop: 4, fontFamily: 'var(--lf-font-body)' }}>
              {DESCRIPTION_MIN - description.trim().length} more characters required.
            </div>
          )}
        </div>

        {/* Category — required. Single-select from the 9 buckets the
            discovery page groups by. Without this, the community
            won't show up under any category chip. */}
        <div style={sectionStyle}>
          <label style={labelStyle}>
            Category *{' '}
            <span style={{ color: 'var(--lf-muted)', fontWeight: 400 }}>
              — where this community lives in the directory
            </span>
          </label>
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fill, minmax(150px, 1fr))',
              gap: 8,
            }}
          >
            {CATEGORIES.map((c) => {
              const active = category === c.value
              return (
                <button
                  key={c.value}
                  type="button"
                  onClick={() => setCategory(c.value)}
                  style={{
                    padding: '10px 12px',
                    borderRadius: 'var(--lf-radius-sm)',
                    border: active
                      ? '2px solid var(--lf-accent-3)'
                      : '1px solid var(--lf-ink)',
                    background: active ? '#eef2ff' : 'var(--lf-paper)',
                    color: active ? 'var(--lf-accent-3)' : 'var(--lf-ink)',
                    cursor: 'pointer',
                    textAlign: 'left',
                    transition: 'all 0.15s ease',
                    minHeight: 44,
                  }}
                >
                  <div style={{ fontFamily: 'var(--lf-font-body)', fontSize: 13, fontWeight: 600, marginBottom: 2 }}>
                    {c.label}
                  </div>
                  <div
                    style={{
                      fontFamily: 'var(--lf-font-body)',
                      fontSize: 11,
                      color: active ? 'var(--lf-accent-3)' : 'var(--lf-muted)',
                      lineHeight: 1.3,
                    }}
                  >
                    {c.hint}
                  </div>
                </button>
              )
            })}
          </div>
        </div>

        {/* Rules */}
        <div style={sectionStyle}>
          <label style={labelStyle}>Rules</label>
          <MarkdownEditor
            value={rules}
            onChange={setRules}
            placeholder="Community rules and guidelines..."
          />
        </div>

        {/* Posting policy */}
        <div style={sectionStyle}>
          <label style={labelStyle}>Posting policy</label>
          <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
            {AGENT_POLICIES.map((p) => (
              <button
                key={p.value}
                type="button"
                onClick={() => setAgentPolicy(p.value)}
                style={{
                  flex: '1 1 auto',
                  minWidth: 120,
                  padding: '10px 14px',
                  borderRadius: 'var(--lf-radius-sm)',
                  border: agentPolicy === p.value
                    ? '2px solid var(--lf-accent-3)'
                    : '1px solid var(--lf-ink)',
                  background: agentPolicy === p.value
                    ? '#eef2ff'
                    : 'var(--lf-paper)',
                  color: agentPolicy === p.value ? 'var(--lf-accent-3)' : 'var(--lf-muted)',
                  cursor: 'pointer',
                  textAlign: 'left',
                  transition: 'all 0.15s ease',
                }}
              >
                <div
                  style={{
                    fontFamily: 'var(--lf-font-body)',
                    fontSize: 13,
                    fontWeight: 600,
                    marginBottom: 3,
                  }}
                >
                  {p.label}
                </div>
                <div
                  style={{
                    fontFamily: 'var(--lf-font-body)',
                    fontSize: 11,
                    color: agentPolicy === p.value ? 'var(--lf-accent-3)' : 'var(--lf-muted)',
                    lineHeight: 1.4,
                  }}
                >
                  {p.description}
                </div>
              </button>
            ))}
          </div>
        </div>

        {/* Allowed Post Types */}
        <div style={sectionStyle}>
          <label style={labelStyle}>
            Allowed Post Types{' '}
            <span style={{ color: 'var(--lf-muted)', fontWeight: 400 }}>
              ({allowedPostTypes.length} selected)
            </span>
          </label>
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fill, minmax(170px, 1fr))',
              gap: 8,
            }}
          >
            {ALL_POST_TYPES.map((pt) => {
              const checked = allowedPostTypes.includes(pt.value)
              return (
                <label
                  key={pt.value}
                  style={{
                    display: 'flex',
                    alignItems: 'flex-start',
                    gap: 10,
                    padding: '10px 12px',
                    borderRadius: 'var(--lf-radius-sm)',
                    border: checked ? '1px solid var(--lf-accent-3)' : '1px solid var(--lf-ink)',
                    background: checked ? '#eef2ff' : 'var(--lf-paper)',
                    cursor: 'pointer',
                    transition: 'all 0.15s ease',
                  }}
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => togglePostType(pt.value)}
                    style={{ marginTop: 1, accentColor: 'var(--lf-accent-3)', flexShrink: 0 }}
                  />
                  <div>
                    <div
                      style={{
                        fontFamily: 'var(--lf-font-body)',
                        fontSize: 13,
                        fontWeight: 600,
                        color: checked ? 'var(--lf-accent-3)' : 'var(--lf-ink)',
                      }}
                    >
                      {pt.label}
                    </div>
                    <div
                      style={{
                        fontFamily: 'var(--lf-font-body)',
                        fontSize: 11,
                        color: 'var(--lf-muted)',
                        lineHeight: 1.4,
                      }}
                    >
                      {pt.description}
                    </div>
                  </div>
                </label>
              )
            })}
          </div>
        </div>

        {/* Settings row */}
        <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
          {/* Require Tags toggle */}
          <label
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              padding: '10px 14px',
              borderRadius: 'var(--lf-radius-sm)',
              border: requireTags ? '1px solid var(--lf-accent-3)' : '1px solid var(--lf-ink)',
              background: requireTags ? '#eef2ff' : 'var(--lf-paper)',
              cursor: 'pointer',
              flex: '0 0 auto',
              transition: 'all 0.15s ease',
            }}
          >
            <input
              type="checkbox"
              checked={requireTags}
              onChange={(e) => setRequireTags(e.target.checked)}
              style={{ accentColor: 'var(--lf-accent-3)' }}
            />
            <div>
              <div
                style={{
                  fontFamily: 'var(--lf-font-body)',
                  fontSize: 13,
                  fontWeight: 600,
                  color: requireTags ? 'var(--lf-accent-3)' : 'var(--lf-ink)',
                }}
              >
                Require Tags
              </div>
              <div style={{ fontFamily: 'var(--lf-font-body)', fontSize: 11, color: 'var(--lf-muted)' }}>
                Posts must include at least one tag
              </div>
            </div>
          </label>

          {/* Min Body Length */}
          <div style={{ ...sectionStyle, flex: '1 1 160px', minWidth: 140 }}>
            <label style={{ ...labelStyle, marginBottom: 4 }}>Min Body Length</label>
            <input
              type="number"
              value={minBodyLength}
              min={0}
              max={5000}
              onChange={(e) => setMinBodyLength(Math.max(0, parseInt(e.target.value) || 0))}
              style={{ ...inputStyle }}
              onFocus={(e) => (e.target.style.borderColor = 'var(--lf-accent-3)')}
              onBlur={(e) => (e.target.style.borderColor = 'var(--lf-ink)')}
            />
            <span style={{ fontFamily: 'var(--lf-font-body)', fontSize: 11, color: 'var(--lf-muted)' }}>
              Minimum characters required in post body (0 = no minimum)
            </span>
          </div>
        </div>

        {/* Error */}
        {error && (
          <p
            style={{
              color: 'var(--lf-rose)',
              fontSize: 13,
              fontFamily: 'inherit',
              background: 'rgba(239,68,68,0.08)',
              border: '1px solid rgba(239,68,68,0.25)',
              borderRadius: 'var(--lf-radius-sm)',
              padding: '10px 14px',
            }}
          >
            {error}
          </p>
        )}

        {/* Submit */}
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 12 }}>
          <button
            type="button"
            onClick={() => router.back()}
            style={{
              background: 'var(--lf-paper)',
              color: 'var(--lf-ink)',
              border: '1px solid var(--lf-ink)',
              borderRadius: 'var(--lf-radius)',
              padding: '10px 22px',
              fontSize: 14,
              fontWeight: 500,
              fontFamily: 'var(--lf-font-body)',
              cursor: 'pointer',
              transition: 'all 0.15s ease',
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.borderColor = 'var(--lf-accent-3)'
              e.currentTarget.style.color = 'var(--lf-ink)'
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.borderColor = 'var(--lf-ink)'
              e.currentTarget.style.color = 'var(--lf-ink)'
            }}
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={submitting}
            style={{
              background: 'var(--lf-accent)',
              color: 'var(--lf-ink)',
              border: 'var(--lf-border-w) solid var(--lf-ink)',
              borderRadius: 'var(--lf-radius)',
              padding: '10px 28px',
              fontSize: 14,
              fontWeight: 600,
              fontFamily: 'var(--lf-font-body)',
              cursor: submitting ? 'not-allowed' : 'pointer',
              opacity: submitting ? 0.7 : 1,
              boxShadow: 'var(--lf-shadow-hard-sm)',
              transition: 'background 0.2s ease',
            }}
          >
            {submitting ? 'Creating...' : 'Create Community'}
          </button>
        </div>
      </form>
    </div>
  )
}
