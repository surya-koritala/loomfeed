'use client'

import { useState, useRef, useEffect, useCallback } from 'react'
import MarkdownContent from './MarkdownContent'
import { api } from '../api/client'

interface MarkdownEditorProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  minHeight?: number
}

const CALLOUT_TYPES = ['NOTE', 'TIP', 'WARNING', 'IMPORTANT', 'CAUTION'] as const

export default function MarkdownEditor({ value, onChange, placeholder, minHeight = 200 }: MarkdownEditorProps) {
  const [showPreview, setShowPreview] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [showCalloutMenu, setShowCalloutMenu] = useState(false)
  // Assume uploads are available until /config tells us otherwise.
  // Hiding the Img button optimistically would flash it off on most
  // page loads; erring the other way (show, then hide) is the less
  // disruptive flicker if server config changes mid-session.
  const [uploadsEnabled, setUploadsEnabled] = useState(true)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const calloutRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    api
      .getConfig()
      .then((data: any) => {
        if (data && typeof data.uploads_enabled === 'boolean') {
          setUploadsEnabled(data.uploads_enabled)
        }
      })
      .catch(() => {
        // Non-fatal; leave the button visible. A blocked upload will
        // surface the 503 from the backend.
      })
  }, [])

  // Close callout dropdown when clicking outside
  useEffect(() => {
    if (!showCalloutMenu) return
    const handleClickOutside = (e: MouseEvent) => {
      if (calloutRef.current && !calloutRef.current.contains(e.target as Node)) {
        setShowCalloutMenu(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [showCalloutMenu])

  // Insert text wrapping the current selection (for bold, italic, strikethrough, code)
  const insertWrap = useCallback((wrapper: string) => {
    const textarea = textareaRef.current
    if (!textarea) {
      onChange(value + wrapper + wrapper)
      return
    }
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    const selected = value.substring(start, end)
    const replacement = wrapper + (selected || 'text') + wrapper
    const newValue = value.substring(0, start) + replacement + value.substring(end)
    onChange(newValue)
    setTimeout(() => {
      if (selected) {
        // Select the wrapped text
        textarea.selectionStart = start + wrapper.length
        textarea.selectionEnd = start + wrapper.length + selected.length
      } else {
        // Select the placeholder word "text"
        textarea.selectionStart = start + wrapper.length
        textarea.selectionEnd = start + wrapper.length + 4
      }
      textarea.focus()
    }, 0)
  }, [value, onChange])

  // Insert a block of text at cursor position
  const insertBlock = useCallback((block: string) => {
    const textarea = textareaRef.current
    if (!textarea) {
      onChange(value + '\n' + block)
      return
    }
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    // Add a newline before if cursor is not at the start of a line
    const needsNewlineBefore = start > 0 && value[start - 1] !== '\n'
    const prefix = needsNewlineBefore ? '\n' : ''
    const insertion = prefix + block
    const newValue = value.substring(0, start) + insertion + value.substring(end)
    onChange(newValue)
    setTimeout(() => {
      const cursorPos = start + insertion.length
      textarea.selectionStart = cursorPos
      textarea.selectionEnd = cursorPos
      textarea.focus()
    }, 0)
  }, [value, onChange])

  // Insert footnote: [^N] at cursor and append definition at end
  const insertFootnote = useCallback(() => {
    const textarea = textareaRef.current
    // Find the next available footnote number
    const existingFootnotes = value.match(/\[\^(\d+)\]/g) || []
    const usedNumbers = existingFootnotes.map(f => parseInt(f.replace(/\[\^|\]/g, '')))
    const nextNum = usedNumbers.length > 0 ? Math.max(...usedNumbers) + 1 : 1
    const ref = `[^${nextNum}]`
    const definition = `\n\n[^${nextNum}]: Source`

    if (!textarea) {
      onChange(value + ref + definition)
      return
    }
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    const newValue = value.substring(0, start) + ref + value.substring(end) + definition
    onChange(newValue)
    setTimeout(() => {
      const cursorPos = start + ref.length
      textarea.selectionStart = cursorPos
      textarea.selectionEnd = cursorPos
      textarea.focus()
    }, 0)
  }, [value, onChange])

  const insertCallout = useCallback((type: string) => {
    const block = `> [!${type}]\n> Content here`
    insertBlock(block)
    setShowCalloutMenu(false)
  }, [insertBlock])

  const handleImageUpload = async (file: File) => {
    setUploading(true)
    try {
      const result = await api.uploadImage(file) as { url: string }
      const imageMarkdown = `![image](${result.url})`
      const textarea = textareaRef.current
      if (textarea) {
        const start = textarea.selectionStart
        const end = textarea.selectionEnd
        const newValue = value.substring(0, start) + imageMarkdown + value.substring(end)
        onChange(newValue)
        // Restore cursor position after the inserted text
        setTimeout(() => {
          textarea.selectionStart = start + imageMarkdown.length
          textarea.selectionEnd = start + imageMarkdown.length
          textarea.focus()
        }, 0)
      } else {
        onChange(value + '\n' + imageMarkdown)
      }
    } catch {
      // silent fail — user can try again
    } finally {
      setUploading(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  const toolbarBtnClass = "px-1.5 py-1 text-xs"
  const toolbarBtnStyle: React.CSSProperties = { color: 'var(--lf-muted)', fontFamily: 'inherit' }
  const handleMouseEnter = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.currentTarget.style.background = 'var(--lf-paper-alt)'
    e.currentTarget.style.color = 'var(--lf-ink)'
  }
  const handleMouseLeave = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.currentTarget.style.background = 'transparent'
    e.currentTarget.style.color = 'var(--lf-muted)'
  }

  const divider = <div className="mx-2 h-4 w-px" style={{ background: 'var(--lf-rule-soft)' }} />

  return (
    <div>
      <div className="lf-md-toolbar flex flex-wrap items-center gap-1 px-2 py-1.5" style={{ border: '1px solid var(--lf-rule-soft)', borderBottom: 'none', background: 'var(--lf-paper)' }}>
        {/* Write/Preview toggle */}
        <button type="button" onClick={() => setShowPreview(false)}
          className="px-2 py-1 text-xs font-medium"
          style={{ color: !showPreview ? 'var(--lf-accent)' : 'var(--lf-muted)', background: !showPreview ? 'var(--lf-paper-alt)' : 'transparent' }}
        >Write</button>
        <button type="button" onClick={() => setShowPreview(true)}
          className="px-2 py-1 text-xs font-medium"
          style={{ color: showPreview ? 'var(--lf-accent)' : 'var(--lf-muted)', background: showPreview ? 'var(--lf-paper-alt)' : 'transparent' }}
        >Preview</button>

        {divider}

        {/* Text group: Bold, Italic, Strikethrough */}
        <button type="button" title="Bold" className={toolbarBtnClass} style={toolbarBtnStyle}
          onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}
          onClick={() => insertWrap('**')}
        >B</button>
        <button type="button" title="Italic" className={toolbarBtnClass} style={toolbarBtnStyle}
          onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}
          onClick={() => insertWrap('_')}
        >I</button>
        <button type="button" title="Strikethrough" className={toolbarBtnClass} style={toolbarBtnStyle}
          onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}
          onClick={() => insertWrap('~~')}
        >S</button>

        {divider}

        {/* Structure group: Code, Blockquote, Collapsible */}
        <button type="button" title="Inline code" className={toolbarBtnClass} style={toolbarBtnStyle}
          onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}
          onClick={() => insertWrap('`')}
        >`</button>
        <button type="button" title="Blockquote" className={toolbarBtnClass} style={toolbarBtnStyle}
          onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}
          onClick={() => insertBlock('> text')}
        >&gt;</button>
        <button type="button" title="Collapsible section" className={toolbarBtnClass} style={toolbarBtnStyle}
          onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}
          onClick={() => insertBlock('<details><summary>Click to expand</summary>\n\nContent here...\n</details>')}
        >...</button>

        {divider}

        {/* Data group: Table */}
        <button type="button" title="Insert table" className={toolbarBtnClass} style={toolbarBtnStyle}
          onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}
          onClick={() => insertBlock('| Header 1 | Header 2 | Header 3 |\n|----------|----------|----------|\n| Cell 1   | Cell 2   | Cell 3   |')}
        >Table</button>

        {divider}

        {/* Rich group: Callout, Footnote, Mermaid */}
        <div ref={calloutRef} className="relative">
          <button type="button" title="Insert callout" className={toolbarBtnClass} style={toolbarBtnStyle}
            onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}
            onClick={() => setShowCalloutMenu(!showCalloutMenu)}
          >Callout ▾</button>
          {showCalloutMenu && (
            <div className="absolute left-0 top-full z-50 mt-1 py-1 shadow-lg"
              style={{ border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper)', minWidth: '140px' }}
            >
              {CALLOUT_TYPES.map((type) => (
                <button key={type} type="button"
                  className="block w-full px-3 py-1.5 text-left text-xs"
                  style={{ color: 'var(--lf-muted)', fontFamily: 'inherit' }}
                  onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--lf-paper-alt)'; e.currentTarget.style.color = 'var(--lf-ink)' }}
                  onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; e.currentTarget.style.color = 'var(--lf-muted)' }}
                  onClick={() => insertCallout(type)}
                >{type.charAt(0) + type.slice(1).toLowerCase()}</button>
              ))}
            </div>
          )}
        </div>
        <button type="button" title="Insert footnote" className={toolbarBtnClass} style={toolbarBtnStyle}
          onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}
          onClick={insertFootnote}
        >Fn</button>
        <button type="button" title="Insert Mermaid diagram" className={toolbarBtnClass} style={toolbarBtnStyle}
          onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}
          onClick={() => insertBlock('```mermaid\ngraph LR\n  A --> B\n```')}
        >Mermaid</button>

        {uploadsEnabled && divider}

        {/* Media group: Image Upload. Hidden when the server reports
            uploads are disabled — the backend also gates the POST, but
            hiding the button prevents user confusion from failed
            clicks. */}
        {uploadsEnabled && (
          <button
            type="button"
            title="Upload image (jpg, png, gif, webp)"
            disabled={uploading}
            className="px-1.5 py-1 text-xs disabled:opacity-50"
            style={{ color: 'var(--lf-muted)' }}
            onMouseEnter={handleMouseEnter}
            onMouseLeave={handleMouseLeave}
            onClick={() => fileInputRef.current?.click()}
          >
            {uploading ? '...' : 'Img'}
          </button>
        )}
        <input
          ref={fileInputRef}
          type="file"
          accept="image/jpeg,image/png,image/gif,image/webp"
          style={{ display: 'none' }}
          onChange={(e) => {
            const file = e.target.files?.[0]
            if (file) handleImageUpload(file)
          }}
        />
      </div>
      {showPreview ? (
        <div className="p-4 text-sm"
          style={{ minHeight, fontFamily: 'inherit', border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper-alt)', color: 'var(--lf-ink)' }}>
          {value ? <MarkdownContent content={value} /> : <span style={{ color: 'var(--lf-muted)' }}>Nothing to preview</span>}
        </div>
      ) : (
        <textarea
          ref={textareaRef}
          value={value} onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder ?? 'Write using Markdown...'}
          className="w-full p-4 text-sm outline-none"
          style={{ minHeight, fontFamily: 'inherit', resize: 'vertical', border: '1px solid var(--lf-rule-soft)', background: 'var(--lf-paper-alt)', color: 'var(--lf-ink)' }}
          onFocus={(e) => (e.currentTarget.style.borderColor = 'var(--lf-accent)')}
          onBlur={(e) => (e.currentTarget.style.borderColor = 'var(--lf-rule-soft)')}
          onKeyDown={(e) => {
            if (e.key === 'Tab') {
              e.preventDefault()
              const start = e.currentTarget.selectionStart
              const end = e.currentTarget.selectionEnd
              onChange(value.substring(0, start) + '  ' + value.substring(end))
            }
          }}
        />
      )}
    </div>
  )
}
