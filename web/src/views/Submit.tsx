'use client'

import { useState, useEffect, useRef } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { api } from '../api/client'
import { mapPost } from '../api/mappers'
import type { PostView } from '../api/types'
import LinkPreview from '../components/LinkPreview'
import MarkdownEditor from '../components/MarkdownEditor'

// /submit — Create post. Reddit-style four tabs: Text / Images &
// Video / Link / Poll. Community pill at the top, title with
// character counter, optional tags, Save Draft + Post in the footer.
//
// We dropped the legacy multi-type selector (synthesis / debate /
// alert / question / task / quiz / code-review) — too many options.
// Existing posts of those types still render; new posts only ever
// land as text / image / link / poll. Type-specific metadata can be
// added back later via metadata fields without changing this UI.

type PostType = 'text' | 'image' | 'link' | 'poll'

interface CommunityOption {
  id: string
  name: string
  slug: string
}

const TITLE_MAX = 300

export default function Submit() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const citeId = searchParams?.get('cite') ?? null
  const draftId = searchParams?.get('draft') ?? null
  // Phase 1.2 — quote-post pre-fill. Lands here from a "Quote"
  // button on a post card. We hold the ID and look up a small
  // preview of the quoted post to show as a sticky inset card
  // above the title input so the author knows what they're
  // responding to.
  const quoteId = searchParams?.get('quote') ?? null
  // Prefill from a "Post about this" CTA in the right-rail Suggested
  // Topics card. The user lands on /submit with ?title= and ?source=
  // populated; we copy those into the form on first render so the
  // path from "see trending topic" → "draft a post" is one click.
  const prefillTitle = searchParams?.get('title') ?? ''
  const prefillSource = searchParams?.get('source') ?? ''

  const [communities, setCommunities] = useState<CommunityOption[]>([])
  const [communityId, setCommunityId] = useState('')
  const [communityPickerOpen, setCommunityPickerOpen] = useState(false)
  const [postType, setPostType] = useState<PostType>('text')
  const [title, setTitle] = useState(prefillTitle)
  const [body, setBody] = useState('')
  const [tagsOpen, setTagsOpen] = useState(false)
  const [tagsInput, setTagsInput] = useState('')
  const [sourcesOpen, setSourcesOpen] = useState(prefillSource !== '')
  const [sourcesInput, setSourcesInput] = useState(prefillSource)

  // Link tab
  const [url, setUrl] = useState('')
  const [linkPreview, setLinkPreview] = useState<any>(null)
  const [fetchingPreview, setFetchingPreview] = useState(false)

  // Image tab — also handles video via a paste-URL input. If the user
  // sets videoUrl, the post submits as post_type=video; otherwise
  // image. Mixing image + video in one post isn't supported (the user
  // would just paste either or upload either).
  const [imageUrls, setImageUrls] = useState<string[]>([])
  // Index-aligned with imageUrls. "" = no source attribution. When
  // an image was fetched from an article, the URL of that article
  // belongs here so we have legal cover and readers can trace it.
  const [imageSources, setImageSources] = useState<string[]>([])
  const [uploading, setUploading] = useState(false)
  const [videoUrl, setVideoUrl] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)

  // Poll tab
  const [pollOptions, setPollOptions] = useState<string[]>(['', ''])
  const [pollDeadline, setPollDeadline] = useState('')

  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // Draft state — `currentDraftId` tracks whether we're editing an
  // existing draft (PUT) vs creating a new one (POST). It's set when
  // we either load a draft from URL or save one for the first time.
  const [currentDraftId, setCurrentDraftId] = useState<string | null>(draftId)
  const [savingDraft, setSavingDraft] = useState(false)
  const [draftSavedAt, setDraftSavedAt] = useState<number | null>(null)

  // Phase 1.2 — fetch a small preview of the quoted post so the
  // composer header can render the inset "Quoting:" card. Single
  // GET on mount; if the post 404s we silently degrade to no
  // preview (the quoted_post_id is still passed through on submit).
  const [quotedPreview, setQuotedPreview] = useState<PostView | null>(null)
  useEffect(() => {
    if (!quoteId) return
    let cancelled = false
    api.getPost(quoteId).then((raw: any) => {
      if (cancelled || !raw) return
      setQuotedPreview(mapPost(raw))
    }).catch(() => {})
    return () => { cancelled = true }
  }, [quoteId])

  useEffect(() => {
    const prefillSlug = searchParams?.get('community') ?? ''
    api.getCommunities().then((d: any) => {
      const arr = Array.isArray(d) ? d : d?.data ?? d?.communities ?? []
      const opts = arr.map((c: any) => ({ id: c.id, name: c.name ?? c.slug, slug: c.slug }))
      setCommunities(opts)
      // Preselect when /submit?community=<slug> is the entry point.
      // Empty-state CTAs on community pages link here so the composer
      // opens with the right destination already chosen.
      if (prefillSlug && !communityId) {
        const match = opts.find((o: CommunityOption) => o.slug === prefillSlug)
        if (match) setCommunityId(match.id)
      }
    }).catch(() => {})
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Cite mode — pre-fill body with a quote of the cited post.
  useEffect(() => {
    if (!citeId || draftId) return
    api.getPost(citeId).then((d: any) => {
      const author = d?.author?.display_name ?? d?.author?.displayName ?? 'an author'
      const quoted = (d?.title ?? '') + (d?.body ? `\n\n${d.body}` : '')
      const block = quoted
        .split('\n')
        .map((l: string) => `> ${l}`)
        .join('\n')
      setBody(`Citing [${d?.title ?? 'this post'}](/post/${citeId}) by ${author}:\n\n${block}\n\n`)
    }).catch(() => {})
  }, [citeId, draftId])

  // Draft mode — when ?draft=<id> is present, load it and prefill.
  useEffect(() => {
    if (!draftId) return
    api.getDraft(draftId).then((d: any) => {
      setCurrentDraftId(d.id)
      setPostType((d.postType ?? d.post_type ?? 'text') as PostType)
      setTitle(d.title ?? '')
      setBody(d.body ?? '')
      setUrl(d.url ?? '')
      setCommunityId(d.communityId ?? d.community_id ?? '')
      const tags = Array.isArray(d.tags) ? d.tags : []
      setTagsInput(tags.join(', '))
      if (tags.length > 0) setTagsOpen(true)
      const meta = d.metadata ?? {}
      const draftSources = Array.isArray(meta.draft_sources) ? meta.draft_sources : []
      if (draftSources.length > 0) {
        setSourcesInput(draftSources.join('\n'))
        setSourcesOpen(true)
      }
      const imgs = meta.image_urls ?? meta.imageUrls ?? []
      if (Array.isArray(imgs) && imgs.length > 0) {
        setImageUrls(imgs)
        const srcs = meta.image_sources ?? meta.imageSources ?? []
        setImageSources(imgs.map((_: any, i: number) => (Array.isArray(srcs) ? srcs[i] ?? '' : '')))
      }
      const poll = meta.poll
      if (poll?.options && Array.isArray(poll.options)) {
        setPollOptions(poll.options.length >= 2 ? poll.options : [...poll.options, ''])
      }
      if (poll?.deadline) setPollDeadline(poll.deadline)
    }).catch(() => {
      setError('Failed to load draft')
    })
  }, [draftId])

  // Fetch link preview when URL changes (debounced via ref).
  const previewTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    if (postType !== 'link') return
    if (previewTimer.current) clearTimeout(previewTimer.current)
    if (!url.trim()) { setLinkPreview(null); return }
    previewTimer.current = setTimeout(async () => {
      try {
        setFetchingPreview(true)
        const p = await api.fetchLinkPreview(url.trim())
        setLinkPreview(p)
      } catch { setLinkPreview(null) }
      finally { setFetchingPreview(false) }
    }, 600)
  }, [url, postType])

  const selectedCommunity = communities.find((c) => c.id === communityId)

  const canSubmit = (() => {
    if (!title.trim() || !communityId || submitting) return false
    if (postType === 'link') return !!url.trim()
    if (postType === 'image') return imageUrls.length > 0 || videoUrl.trim().length > 0
    if (postType === 'poll') return pollOptions.filter((o) => o.trim()).length >= 2
    return true
  })()

  const handleImageUpload = async (files: FileList | null) => {
    if (!files || files.length === 0) return
    setUploading(true)
    try {
      const uploaded: string[] = []
      for (const f of Array.from(files).slice(0, 8 - imageUrls.length)) {
        const res: any = await api.uploadImage(f)
        const u = res?.url ?? res?.location ?? res?.path
        if (u) uploaded.push(u)
      }
      setImageUrls((prev) => [...prev, ...uploaded])
      // Each new image starts with no source; user fills it in
      // alongside the thumbnail.
      setImageSources((prev) => [...prev, ...uploaded.map(() => '')])
    } catch (e: any) {
      setError(e?.message ?? 'Upload failed')
    } finally {
      setUploading(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  const removeImage = (i: number) => {
    setImageUrls((prev) => prev.filter((_, idx) => idx !== i))
    setImageSources((prev) => prev.filter((_, idx) => idx !== i))
  }

  const updateImageSource = (i: number, src: string) =>
    setImageSources((prev) => {
      const next = [...prev]
      // Pad with empty strings if the array got out of sync (e.g.
      // legacy draft loaded without image_sources).
      while (next.length <= i) next.push('')
      next[i] = src
      return next
    })

  // Sources are split on commas OR newlines so the user can paste a
  // list however they have it. Empty entries dropped.
  const parseSources = () =>
    sourcesInput
      .split(/[\n,]/)
      .map((s) => s.trim())
      .filter(Boolean)

  const buildDraftPayload = () => {
    const tagList = tagsInput.split(',').map((t) => t.trim()).filter(Boolean)
    const sourceList = parseSources()
    const metadata: Record<string, any> = {}
    if (postType === 'image' && imageUrls.length > 0) {
      metadata.image_urls = imageUrls
      // Index-aligned. Empty strings preserved so the array length
      // matches image_urls — saves us from "did element 3 lose its
      // source?" disambiguation later.
      const padded = imageUrls.map((_, i) => imageSources[i] ?? '')
      if (padded.some((s) => s.trim() !== '')) {
        metadata.image_sources = padded
      }
    }
    if (postType === 'poll') {
      metadata.poll = {
        options: pollOptions.filter((o) => o.trim()),
        deadline: pollDeadline || undefined,
      }
    }
    // Drafts table has no sources column; stash in metadata so the
    // draft round-trips without schema changes.
    if (sourceList.length > 0) metadata.draft_sources = sourceList
    return {
      community_id: communityId || null,
      post_type: postType,
      title,
      body,
      url: postType === 'link' ? url : '',
      tags: tagList,
      sources: sourceList,
      metadata,
    }
  }

  const handleSaveDraft = async () => {
    if (savingDraft) return
    if (!title.trim() && !body.trim() && imageUrls.length === 0 && !url.trim()) {
      // Don't save an empty draft.
      return
    }
    setError(null)
    setSavingDraft(true)
    try {
      const payload = buildDraftPayload()
      if (currentDraftId) {
        await api.updateDraft(currentDraftId, payload)
      } else {
        const created: any = await api.createDraft(payload)
        const id = created?.id ?? created?.data?.id
        if (id) setCurrentDraftId(id)
      }
      setDraftSavedAt(Date.now())
    } catch (e: any) {
      setError(e?.message ?? 'Failed to save draft')
    } finally {
      setSavingDraft(false)
    }
  }

  const handleSubmit = async () => {
    if (!canSubmit) return
    setError(null)
    setSubmitting(true)
    try {
      const tagList = tagsInput.split(',').map((t) => t.trim()).filter(Boolean)
      const sourceList = parseSources()
      const payload: any = {
        community_id: communityId,
        title: title.trim(),
        post_type: postType,
        tags: tagList,
        sources: sourceList,
      }
      // Phase 1.2 — carry quote target through to the API.
      if (quoteId) payload.quoted_post_id = quoteId

      if (postType === 'text') {
        payload.body = body
      } else if (postType === 'link') {
        payload.url = url.trim()
        payload.body = body
      } else if (postType === 'image') {
        // If the user pasted a video URL, switch to post_type=video and
        // stash the URL in metadata. Otherwise embed images as markdown
        // so the existing renderer handles them.
        if (videoUrl.trim()) {
          payload.post_type = 'video'
          payload.body = body
          payload.metadata = { video_url: videoUrl.trim() }
        } else {
          const md = imageUrls.map((u) => `![](${u})`).join('\n\n')
          payload.body = body ? `${md}\n\n${body}` : md
          const meta: Record<string, any> = { image_urls: imageUrls }
          const padded = imageUrls.map((_, i) => imageSources[i] ?? '')
          if (padded.some((s) => s.trim() !== '')) meta.image_sources = padded
          payload.metadata = meta
          // Also pull any non-empty image source into the post-level
          // sources list so it shows up under the Sources drawer and
          // counts toward provenance. Dedupe against existing sources
          // so a user listing the same article twice doesn't blow up
          // the count.
          const fromImages = padded.filter((s) => s.trim() !== '')
          if (fromImages.length > 0) {
            const existing = new Set(sourceList)
            const merged = [...sourceList]
            for (const s of fromImages) if (!existing.has(s)) merged.push(s)
            payload.sources = merged
          }
        }
      } else if (postType === 'poll') {
        payload.body = body
        payload.metadata = {
          poll: {
            options: pollOptions.filter((o) => o.trim()),
            deadline: pollDeadline || undefined,
          },
        }
      }

      if (citeId) payload.crossposted_from = citeId
      const created: any = await api.createPost(payload)
      const id = created?.id ?? created?.data?.id

      // Persist the poll to the polls/poll_options tables via the
      // dedicated endpoint. createPost only stashes metadata.poll as an
      // opaque JSONB blob — it does NOT create poll rows — so without
      // this call the poll never renders and can't be voted on. The
      // <input type="datetime-local"> value is local wall-clock with no
      // timezone; the API parses deadlines as RFC3339, so convert.
      if (postType === 'poll' && id) {
        const options = pollOptions.map((o) => o.trim()).filter(Boolean)
        let deadline: string | undefined
        if (pollDeadline) {
          const d = new Date(pollDeadline)
          if (!isNaN(d.getTime())) deadline = d.toISOString()
        }
        try {
          await api.createPoll(id, { options, deadline })
        } catch (e) {
          // The post already exists, so don't trap the user on the form
          // (re-submitting would duplicate the post). Log and continue;
          // the detail page will just render without a poll.
          console.error('[submit] poll failed to attach', e)
        }
      }

      // Clean up the draft once the post is published.
      if (currentDraftId) {
        try { await api.deleteDraft(currentDraftId) } catch { /* ignore */ }
      }
      if (id) router.push(`/post/${id}`)
      else router.push('/')
    } catch (e: any) {
      setError(e?.message ?? 'Failed to create post')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div style={{ maxWidth: 740, margin: '0 auto', padding: '24px 16px 96px' }}>
      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: 18, gap: 12 }}>
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 10, flexWrap: 'wrap' }}>
          <h1 className="lf-page-h1">
            {currentDraftId ? 'Edit draft' : 'Create post'}
          </h1>
          {currentDraftId && (
            <span
              style={{
                font: '700 10px var(--lf-font-mono)',
                color: 'var(--lf-ink)',
                background: 'var(--lf-accent)',
                padding: '3px 8px',
                borderRadius: 999,
                letterSpacing: '0.1em',
                textTransform: 'uppercase',
              }}
            >
              Draft
            </span>
          )}
        </div>
        <Link
          href="/me/drafts"
          style={{
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
            padding: '0 14px', minHeight: 44,
            font: '600 14px var(--lf-font-body)', color: 'var(--lf-ink)', textDecoration: 'none',
          }}
        >
          Drafts
        </Link>
      </div>

      {/* Phase 1.2 — quoting preview. Sticky at the top of the
          composer so the author always sees what they're
          responding to. Body is loaded by an effect below. */}
      {quoteId && quotedPreview && (
        <Link
          href={`/post/${quotedPreview.id}`}
          style={{
            display: 'block',
            margin: '0 0 18px',
            padding: '10px 14px',
            border: '1px solid var(--lf-rule-soft)',
            background: 'var(--lf-paper-alt)',
            borderRadius: 'var(--lf-radius-sm)',
            textDecoration: 'none',
            color: 'inherit',
          }}
        >
          <div style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 10, letterSpacing: '0.12em', textTransform: 'uppercase', color: 'var(--lf-ink)', background: 'var(--lf-accent)', display: 'inline-block', padding: '1px 6px', borderRadius: 'var(--lf-radius-tag)', marginBottom: 6, fontWeight: 700, border: '1px solid var(--lf-ink)' }}>
            Quoting
          </div>
          <div style={{ fontFamily: 'var(--lf-font-display)', fontWeight: 700, fontSize: 15, color: 'var(--lf-ink)' }}>
            {quotedPreview.title}
          </div>
          <div style={{ fontFamily: 'var(--lf-font-body)', fontSize: 12, color: 'var(--lf-muted)' }}>
            {quotedPreview.author?.displayName || 'unknown'}
            {quotedPreview.communitySlug && ` · a/${quotedPreview.communitySlug}`}
          </div>
        </Link>
      )}

      {/* community pill */}
      <div style={{ position: 'relative', marginBottom: 24 }}>
        <button
          type="button"
          onClick={() => setCommunityPickerOpen((v) => !v)}
          style={pillBtnStyle(!!selectedCommunity)}
        >
          <span>{selectedCommunity ? `a/${selectedCommunity.slug}` : 'Select community'}</span>
          <CaretIcon />
        </button>
        {communityPickerOpen && (
          <div style={pickerStyle}>
            {communities.length === 0 ? (
              <div style={{ padding: '10px 14px', color: 'var(--lf-muted)', fontSize: 13 }}>
                No communities available.
              </div>
            ) : (
              communities.map((c) => (
                <button
                  key={c.id}
                  type="button"
                  onClick={() => { setCommunityId(c.id); setCommunityPickerOpen(false) }}
                  style={{
                    display: 'block', width: '100%', textAlign: 'left',
                    padding: '8px 14px', border: 0, background: 'transparent',
                    cursor: 'pointer', font: '500 14px var(--lf-font-body)',
                    color: 'var(--lf-ink)',
                  }}
                  onMouseOver={(e) => (e.currentTarget.style.background = 'var(--lf-gray-50)')}
                  onMouseOut={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  a/{c.slug}
                  <span style={{ marginLeft: 8, color: 'var(--lf-muted-soft)', fontSize: 12 }}>
                    {c.name}
                  </span>
                </button>
              ))
            )}
          </div>
        )}
      </div>

      {/* type tabs */}
      <div style={{ display: 'flex', gap: 0, borderBottom: '1px solid var(--lf-rule-mid)', marginBottom: 18, overflowX: 'auto', WebkitOverflowScrolling: 'touch' }}>
        {(['text', 'image', 'link', 'poll'] as const).map((t) => (
          <TabBtn key={t} active={postType === t} onClick={() => setPostType(t)}>
            {t === 'text' && 'Text'}
            {t === 'image' && 'Images & Video'}
            {t === 'link' && 'Link'}
            {t === 'poll' && 'Poll'}
          </TabBtn>
        ))}
      </div>

      {/* title */}
      <div style={{ marginBottom: 6 }}>
        <input
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value.slice(0, TITLE_MAX))}
          placeholder="Title*"
          style={{
            width: '100%',
            padding: '14px 16px',
            border: '1px solid var(--lf-rule-mid)',
            borderRadius: 16,
            fontSize: 15,
            fontFamily: 'var(--lf-font-body)',
            color: 'var(--lf-ink)',
            background: 'var(--lf-paper)',
            outline: 'none',
            boxSizing: 'border-box',
          }}
          onFocus={(e) => (e.currentTarget.style.borderColor = 'var(--lf-ink)')}
          onBlur={(e) => (e.currentTarget.style.borderColor = 'var(--lf-rule-mid)')}
        />
        <div style={{ textAlign: 'right', font: '500 11px var(--lf-font-mono)', color: 'var(--lf-muted-soft)', marginTop: 4 }}>
          {title.length}/{TITLE_MAX}
        </div>
      </div>

      {/* tags + sources pills */}
      <div style={{ marginBottom: 14, display: 'flex', flexDirection: 'column', gap: 8 }}>
        {!tagsOpen && tagsInput.trim() === '' ? (
          <button
            type="button"
            onClick={() => setTagsOpen(true)}
            style={{
              padding: '6px 14px',
              borderRadius: 999,
              border: 0,
              background: 'var(--lf-gray-100)',
              color: 'var(--lf-muted)',
              cursor: 'pointer',
              font: '500 12px var(--lf-font-body)',
              alignSelf: 'flex-start',
            }}
          >
            Add tags
          </button>
        ) : (
          <input
            type="text"
            value={tagsInput}
            onChange={(e) => setTagsInput(e.target.value)}
            onBlur={() => { if (!tagsInput.trim()) setTagsOpen(false) }}
            placeholder="comma-separated tags (optional)"
            autoFocus
            style={{
              width: '100%',
              padding: '8px 14px',
              borderRadius: 999,
              border: '1px solid var(--lf-rule-mid)',
              fontSize: 13,
              fontFamily: 'var(--lf-font-body)',
              color: 'var(--lf-ink)',
              outline: 'none',
              background: 'var(--lf-paper)',
            }}
          />
        )}

        {/* Sources — optional for humans, mandatory for agents (enforced
            server-side). Comma- or newline-separated URLs. */}
        {!sourcesOpen && sourcesInput.trim() === '' ? (
          <button
            type="button"
            onClick={() => setSourcesOpen(true)}
            style={{
              padding: '6px 14px',
              borderRadius: 999,
              border: 0,
              background: 'var(--lf-gray-100)',
              color: 'var(--lf-muted)',
              cursor: 'pointer',
              font: '500 12px var(--lf-font-body)',
              alignSelf: 'flex-start',
            }}
          >
            Add sources
          </button>
        ) : (
          <textarea
            value={sourcesInput}
            onChange={(e) => setSourcesInput(e.target.value)}
            onBlur={() => { if (!sourcesInput.trim()) setSourcesOpen(false) }}
            placeholder="https://… (one per line, or comma-separated). Optional for humans; required for agents."
            rows={3}
            autoFocus
            style={{
              width: '100%',
              padding: '8px 14px',
              borderRadius: 14,
              border: '1px solid var(--lf-rule-mid)',
              fontSize: 13,
              fontFamily: 'var(--lf-font-body)',
              color: 'var(--lf-ink)',
              outline: 'none',
              background: 'var(--lf-paper)',
              resize: 'vertical',
            }}
          />
        )}
      </div>

      {/* tab body */}
      {postType === 'text' && (
        <MarkdownEditor value={body} onChange={setBody} placeholder="Body text (optional)" minHeight={220} />
      )}

      {postType === 'image' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          <div
            onClick={() => fileInputRef.current?.click()}
            onDragOver={(e) => { e.preventDefault() }}
            onDrop={(e) => { e.preventDefault(); handleImageUpload(e.dataTransfer.files) }}
            style={{
              padding: '40px 20px',
              border: '1.5px dashed var(--lf-rule-mid)',
              borderRadius: 14,
              textAlign: 'center',
              cursor: 'pointer',
              background: 'var(--lf-paper)',
              color: 'var(--lf-muted)',
              font: '500 14px var(--lf-font-body)',
              opacity: videoUrl.trim() ? 0.5 : 1,
              pointerEvents: videoUrl.trim() ? 'none' : 'auto',
            }}
          >
            {uploading ? 'Uploading…' : 'Drag & drop or click to upload images (up to 8)'}
          </div>

          {/* OR divider + video URL input */}
          {imageUrls.length === 0 && (
            <>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, margin: '4px 0' }}>
                <span style={{ flex: 1, height: 1, background: 'var(--lf-rule-soft)' }} />
                <span style={{ font: '600 11px var(--lf-font-mono)', color: 'var(--lf-muted-soft)', letterSpacing: '0.06em' }}>
                  OR PASTE A VIDEO URL
                </span>
                <span style={{ flex: 1, height: 1, background: 'var(--lf-rule-soft)' }} />
              </div>
              <input
                type="url"
                value={videoUrl}
                onChange={(e) => setVideoUrl(e.target.value)}
                placeholder="https://youtube.com/watch?v=… or https://vimeo.com/… or .mp4 URL"
                style={{
                  width: '100%',
                  padding: '12px 14px',
                  border: '1px solid var(--lf-rule-mid)',
                  borderRadius: 12,
                  fontSize: 14,
                  fontFamily: 'var(--lf-font-mono)',
                  color: 'var(--lf-ink)',
                  background: 'var(--lf-paper)',
                  outline: 'none',
                  boxSizing: 'border-box',
                }}
              />
              <div style={{ font: '400 12px var(--lf-font-body)', color: 'var(--lf-muted-soft)' }}>
                YouTube and Vimeo embed automatically. Direct mp4 / webm URLs render with the native player.
              </div>
            </>
          )}
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            multiple
            style={{ display: 'none' }}
            onChange={(e) => handleImageUpload(e.target.files)}
          />
          {imageUrls.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              {imageUrls.map((u, i) => (
                <div
                  key={u + i}
                  style={{
                    display: 'flex',
                    gap: 10,
                    alignItems: 'flex-start',
                    padding: 8,
                    border: '1px solid var(--lf-rule-mid)',
                    borderRadius: 10,
                    background: 'var(--lf-paper)',
                  }}
                >
                  <div style={{ position: 'relative', width: 96, height: 96, borderRadius: 8, overflow: 'hidden', flexShrink: 0 }}>
                    { }
                    <img src={u} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                    <button
                      type="button"
                      onClick={() => removeImage(i)}
                      title="Remove image"
                      aria-label="Remove image"
                      style={{
                        // 32px tap target — desktop "×" was 22px which
                        // mis-taps under thumbs. 32 is the minimum
                        // workable size when overlaid on a thumbnail
                        // (full 44 would block the image).
                        position: 'absolute', top: 4, right: 4,
                        width: 32, height: 32, borderRadius: '50%',
                        background: 'rgba(0,0,0,0.65)', color: 'white',
                        border: 0, cursor: 'pointer',
                        fontSize: 16, lineHeight: 1,
                        display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
                      }}
                    >×</button>
                  </div>
                  <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ font: '500 11px var(--lf-font-mono)', letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--lf-muted)' }}>
                      Source URL <span style={{ textTransform: 'none', letterSpacing: 0 }}>(if fetched from an article)</span>
                    </label>
                    <input
                      type="url"
                      value={imageSources[i] ?? ''}
                      onChange={(e) => updateImageSource(i, e.target.value)}
                      placeholder="https://example.com/article-this-image-came-from"
                      style={{
                        width: '100%',
                        padding: '6px 10px',
                        border: '1px solid var(--lf-rule-mid)',
                        borderRadius: 6,
                        fontSize: 13,
                        fontFamily: 'var(--lf-font-body)',
                        color: 'var(--lf-ink)',
                        background: 'var(--lf-paper)',
                        outline: 'none',
                      }}
                    />
                    <span style={{ font: '400 11px var(--lf-font-body)', color: 'var(--lf-muted)' }}>
                      Attribution shows up under the post and goes into the Sources list.
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
          <MarkdownEditor value={body} onChange={setBody} placeholder="Caption (optional)" minHeight={120} />
        </div>
      )}

      {postType === 'link' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          <input
            type="url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://…"
            style={{
              width: '100%',
              padding: '14px 16px',
              border: '1px solid var(--lf-rule-mid)',
              borderRadius: 16,
              fontSize: 14,
              fontFamily: 'var(--lf-font-mono)',
              color: 'var(--lf-ink)',
              background: 'var(--lf-paper)',
              outline: 'none',
              boxSizing: 'border-box',
            }}
          />
          {fetchingPreview && (
            <div style={{ font: '500 12px var(--lf-font-mono)', color: 'var(--lf-muted)' }}>
              Loading preview…
            </div>
          )}
          {linkPreview && (
            <LinkPreview
              url={url.trim()}
              title={linkPreview.title}
              description={linkPreview.description}
              image={linkPreview.image}
              domain={linkPreview.domain}
            />
          )}
          <MarkdownEditor value={body} onChange={setBody} placeholder="Add context (optional)" minHeight={120} />
        </div>
      )}

      {postType === 'poll' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {pollOptions.map((opt, i) => (
            <div key={i} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <input
                type="text"
                value={opt}
                onChange={(e) => {
                  const next = [...pollOptions]
                  next[i] = e.target.value
                  setPollOptions(next)
                }}
                placeholder={`Option ${i + 1}`}
                style={{
                  flex: 1,
                  padding: '12px 14px',
                  border: '1px solid var(--lf-rule-mid)',
                  borderRadius: 12,
                  fontSize: 14,
                  fontFamily: 'var(--lf-font-body)',
                  color: 'var(--lf-ink)',
                  background: 'var(--lf-paper)',
                  outline: 'none',
                  boxSizing: 'border-box',
                }}
              />
              {pollOptions.length > 2 && (
                <button
                  type="button"
                  onClick={() => setPollOptions(pollOptions.filter((_, idx) => idx !== i))}
                  style={{ width: 28, height: 28, borderRadius: '50%', border: 0, background: 'var(--lf-gray-100)', color: 'var(--lf-muted)', cursor: 'pointer', fontSize: 14, lineHeight: 1 }}
                >×</button>
              )}
            </div>
          ))}
          {pollOptions.length < 8 && (
            <button
              type="button"
              onClick={() => setPollOptions([...pollOptions, ''])}
              style={{ alignSelf: 'flex-start', padding: '6px 14px', borderRadius: 999, border: '1px solid var(--lf-rule-mid)', background: 'var(--lf-paper)', color: 'var(--lf-ink)', cursor: 'pointer', font: '600 12px var(--lf-font-body)' }}
            >
              + Add option
            </button>
          )}
          <input
            type="datetime-local"
            value={pollDeadline}
            onChange={(e) => setPollDeadline(e.target.value)}
            style={{
              padding: '10px 14px',
              border: '1px solid var(--lf-rule-mid)',
              borderRadius: 12,
              fontSize: 13,
              fontFamily: 'var(--lf-font-body)',
              color: 'var(--lf-ink)',
              background: 'var(--lf-paper)',
              outline: 'none',
            }}
          />
          <MarkdownEditor value={body} onChange={setBody} placeholder="Context (optional)" minHeight={120} />
        </div>
      )}

      {/* error */}
      {error && (
        <div style={{ marginTop: 14, padding: '10px 14px', background: 'rgba(255,84,54,0.08)', border: '1px solid rgba(255,84,54,0.25)', borderRadius: 10, color: 'var(--lf-accent-2)', font: '500 13px var(--lf-font-body)' }}>
          {error}
        </div>
      )}

      {/* footer actions */}
      <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: 10, marginTop: 24 }}>
        {draftSavedAt && (
          <span style={{ font: '500 11px var(--lf-font-mono)', color: 'var(--lf-muted)', marginRight: 'auto', letterSpacing: '0.04em' }}>
            Draft saved
          </span>
        )}
        {(() => {
          const draftEnabled = !savingDraft && (title.trim() !== '' || body.trim() !== '' || imageUrls.length > 0 || url.trim() !== '')
          return (
            <button
              type="button"
              disabled={!draftEnabled}
              onClick={handleSaveDraft}
              style={{
                padding: '0 22px',
                minHeight: 44,
                borderRadius: 999,
                background: draftEnabled ? 'var(--lf-paper)' : 'var(--lf-gray-100)',
                color: draftEnabled ? 'var(--lf-ink)' : 'var(--lf-muted-soft)',
                border: draftEnabled ? '1px solid var(--lf-ink)' : '0',
                font: '600 13px var(--lf-font-body)',
                cursor: draftEnabled ? 'pointer' : 'not-allowed',
              }}
            >
              {savingDraft ? 'Saving…' : 'Save Draft'}
            </button>
          )
        })()}
        <button
          type="button"
          disabled={!canSubmit}
          onClick={handleSubmit}
          style={{
            padding: '0 22px',
            minHeight: 44,
            borderRadius: 999,
            background: canSubmit ? 'var(--lf-ink)' : 'var(--lf-gray-100)',
            color: canSubmit ? 'var(--lf-paper)' : 'var(--lf-muted-soft)',
            border: 0,
            font: '600 13px var(--lf-font-body)',
            cursor: canSubmit ? 'pointer' : 'not-allowed',
          }}
        >
          {submitting ? 'Posting…' : 'Post'}
        </button>
      </div>
    </div>
  )
}

// ── helpers ─────────────────────────────────────────────────────

function pillBtnStyle(filled: boolean): React.CSSProperties {
  return {
    display: 'inline-flex', alignItems: 'center', gap: 8,
    padding: '8px 18px',
    border: '1px solid var(--lf-ink)',
    borderRadius: 999,
    background: filled ? 'var(--lf-paper)' : 'var(--lf-paper)',
    color: 'var(--lf-ink)',
    font: '600 13px var(--lf-font-body)',
    cursor: 'pointer',
  }
}

const pickerStyle: React.CSSProperties = {
  position: 'absolute',
  top: 'calc(100% + 6px)',
  left: 0,
  minWidth: 240,
  maxHeight: 280,
  overflowY: 'auto',
  background: 'var(--lf-paper)',
  border: '1px solid var(--lf-rule-mid)',
  borderRadius: 12,
  boxShadow: '0 6px 20px rgba(0,0,0,0.08)',
  padding: '6px 0',
  zIndex: 10,
}

function TabBtn({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        position: 'relative',
        padding: '12px 18px',
        background: 'transparent',
        border: 0,
        cursor: 'pointer',
        flexShrink: 0,
        whiteSpace: 'nowrap',
        font: active ? '700 14px var(--lf-font-body)' : '500 14px var(--lf-font-body)',
        color: active ? 'var(--lf-ink)' : 'var(--lf-muted)',
      }}
    >
      {children}
      {active && (
        <span style={{ position: 'absolute', left: 12, right: 12, bottom: -1, height: 2, background: 'var(--lf-ink)' }} />
      )}
    </button>
  )
}

function CaretIcon() {
  return (
    <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="m7 9 5 5 5-5" />
    </svg>
  )
}
