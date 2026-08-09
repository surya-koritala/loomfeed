'use client'

import { useState, useEffect } from 'react'
import { safeHref } from '../lib/safe-url'

interface LinkPreviewProps {
  url: string
  title?: string
  description?: string
  image?: string
  domain?: string
}

export default function LinkPreview({ url, title: initialTitle, description: initialDesc, image: initialImage, domain }: LinkPreviewProps) {
  const [title, setTitle] = useState(initialTitle)
  const [description, setDescription] = useState(initialDesc)
  const [image, setImage] = useState(initialImage)
  const [fetched, setFetched] = useState(false)

  const displayDomain = domain || (() => { try { return new URL(url).hostname } catch { return url } })()

  // Extract a readable title from URL path when API returns nothing useful
  function titleFromUrl(rawUrl: string): string {
    try {
      const parsed = new URL(rawUrl)
      // Skip image URLs — they shouldn't show as link previews
      if (/\.(jpg|jpeg|png|gif|webp|svg)(\?|$)/i.test(parsed.pathname)) return ''
      // Skip image hosting domains
      if (['i.redd.it', 'i.imgur.com', 'pbs.twimg.com'].includes(parsed.hostname)) return ''

      const segments = parsed.pathname.split('/').filter(Boolean)

      // Filter out segments that are just IDs, hashes, or short codes
      const meaningful = segments.filter(s =>
        s.length > 3 &&
        !/^[a-f0-9]{6,}$/i.test(s) &&       // pure hex hash
        !/^[a-z0-9]{8,20}$/i.test(s) &&      // alphanumeric ID (reddit IDs etc)
        !/^(r|u|user|comments|s|p|status)$/i.test(s) && // common URL path segments
        /[a-zA-Z]{2,}/.test(s)               // must contain actual letters
      )

      const titleSegment = meaningful.pop() || ''

      return titleSegment
        .replace(/[-_]/g, ' ')
        .replace(/\.\w+$/, '')
        .replace(/\s+/g, ' ')
        .trim()
        .replace(/\b\w/g, c => c.toUpperCase())
    } catch {
      return ''
    }
  }

  useEffect(() => {
    if (fetched || initialTitle || initialDesc || initialImage) return
    setFetched(true)
    fetch(`/api/v1/link-preview?url=${encodeURIComponent(url)}`)
      .then(r => r.ok ? r.json() : null)
      .then(data => {
        if (data) {
          // If title is just the domain or empty, extract from URL path
          const t = data.title
          if (t && t !== displayDomain && t !== `www.${displayDomain}` && !t.startsWith('http')) {
            setTitle(t)
          } else {
            const fallback = titleFromUrl(url)
            if (fallback) setTitle(fallback)
          }
          if (data.description) setDescription(data.description)
          if (data.image) setImage(data.image)
        }
      })
      .catch(() => {})
  }, [url, fetched, initialTitle, initialDesc, initialImage, displayDomain])

  return (
    <a
      href={safeHref(url)}
      target="_blank"
      rel="noopener noreferrer"
      style={{
        display: 'flex',
        alignItems: 'stretch',
        border: '1px solid var(--lf-rule-soft)',
        borderRadius: 'var(--lf-radius-sm)',
        overflow: 'hidden',
        textDecoration: 'none',
        color: 'inherit',
        transition: 'border-color 0.15s, box-shadow 0.15s',
        marginTop: 10,
        background: 'var(--lf-paper)',
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.borderColor = 'var(--lf-ink)'
        e.currentTarget.style.boxShadow = 'var(--shadow-sm)'
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = 'var(--lf-rule-soft)'
        e.currentTarget.style.boxShadow = 'none'
      }}
    >
      {image && (
        <div style={{
          width: 100,
          flexShrink: 0,
          backgroundImage: `url(${image})`,
          backgroundSize: 'cover',
          backgroundPosition: 'center',
          backgroundColor: 'var(--lf-paper-alt)',
        }} />
      )}
      {!image && (
        <div style={{
          width: 48,
          flexShrink: 0,
          background: 'var(--lf-paper-alt)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          borderRight: '1px solid var(--lf-rule-soft)',
          fontSize: 10,
          fontWeight: 700,
          color: 'var(--lf-muted)',
        }}>
          {displayDomain.charAt(0).toUpperCase()}
        </div>
      )}
      <div style={{ padding: '8px 12px', minWidth: 0, flex: 1 }}>
        <div style={{
          fontSize: 10,
          fontWeight: 600,
          color: 'var(--lf-muted)',
          textTransform: 'uppercase' as const,
          letterSpacing: '0.03em',
          marginBottom: 2,
        }}>
          {displayDomain}
        </div>
        {title && (
          <div style={{
            fontSize: 13,
            fontWeight: 500,
            color: 'var(--lf-ink)',
            lineHeight: 1.35,
            marginBottom: 2,
            overflow: 'hidden',
            display: '-webkit-box',
            WebkitLineClamp: 1,
            WebkitBoxOrient: 'vertical' as const,
          }}>
            {title}
          </div>
        )}
        {description && (
          <div style={{
            fontSize: 12,
            color: 'var(--lf-muted)',
            lineHeight: 1.4,
            overflow: 'hidden',
            display: '-webkit-box',
            WebkitLineClamp: 1,
            WebkitBoxOrient: 'vertical' as const,
          }}>
            {description}
          </div>
        )}
      </div>
      <div style={{
        width: 28,
        flexShrink: 0,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: 'var(--lf-muted)',
      }}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
          <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
          <polyline points="15 3 21 3 21 9" />
          <line x1="10" y1="14" x2="21" y2="3" />
        </svg>
      </div>
    </a>
  )
}
