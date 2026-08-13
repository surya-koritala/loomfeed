// @vitest-environment jsdom

import { act, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import axe from 'axe-core'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Dialog from './Dialog'

;(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true

function Harness({ onClose }: { onClose: () => void }) {
  const [open, setOpen] = useState(true)
  return (
    <>
      <button data-testid="opener">Open dialog</button>
      {open && (
        <Dialog
          labelledBy="test-dialog-title"
          describedBy="test-dialog-description"
          onClose={() => {
            onClose()
            setOpen(false)
          }}
        >
          <h2 id="test-dialog-title">Test dialog</h2>
          <p id="test-dialog-description">A dialog used to verify accessible behavior.</p>
          <button data-testid="first">First action</button>
          <button data-testid="last">Last action</button>
        </Dialog>
      )}
    </>
  )
}

describe('Dialog', () => {
  let host: HTMLDivElement
  let root: Root

  beforeEach(() => {
    document.body.innerHTML = ''
    document.body.style.overflow = 'auto'
    host = document.createElement('div')
    host.id = 'app-root'
    document.body.appendChild(host)
    root = createRoot(host)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    document.body.innerHTML = ''
    document.body.style.overflow = ''
  })

  async function renderHarness(onClose = vi.fn()) {
    await act(async () => root.render(<Harness onClose={onClose} />))
    return onClose
  }

  it('labels the modal, moves focus inside, and makes the background inert', async () => {
    await renderHarness()

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')
    const first = document.querySelector<HTMLElement>('[data-testid="first"]')
    expect(dialog?.getAttribute('aria-modal')).toBe('true')
    expect(dialog?.getAttribute('aria-labelledby')).toBe('test-dialog-title')
    expect(dialog?.getAttribute('aria-describedby')).toBe('test-dialog-description')
    expect(document.activeElement).toBe(first)
    expect(host.hasAttribute('inert')).toBe(true)
    expect(document.body.style.overflow).toBe('hidden')
  })

  it('contains forward and reverse tab navigation', async () => {
    await renderHarness()
    const first = document.querySelector<HTMLElement>('[data-testid="first"]')!
    const last = document.querySelector<HTMLElement>('[data-testid="last"]')!
    const opener = document.querySelector<HTMLElement>('[data-testid="opener"]')!

    last.focus()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }))
    expect(document.activeElement).toBe(first)

    first.focus()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true }))
    expect(document.activeElement).toBe(last)

    opener.focus()
    expect(document.activeElement).toBe(first)
  })

  it('closes on Escape and restores focus to the opener', async () => {
    const opener = document.createElement('button')
    opener.textContent = 'Outside opener'
    document.body.insertBefore(opener, host)
    opener.focus()
    const onClose = await renderHarness()

    await act(async () => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })

    expect(onClose).toHaveBeenCalledOnce()
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    expect(document.activeElement).toBe(opener)
    expect(host.hasAttribute('inert')).toBe(false)
    expect(document.body.style.overflow).toBe('auto')
  })

  it('closes only when the backdrop itself is pressed', async () => {
    const onClose = await renderHarness()
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    const backdrop = document.querySelector<HTMLElement>('[data-dialog-backdrop]')!

    dialog.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onClose).not.toHaveBeenCalled()

    await act(async () => {
      backdrop.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('has no automated axe violations', async () => {
    await renderHarness()
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    const results = await axe.run(dialog, {
      rules: { 'color-contrast': { enabled: false } },
    })

    expect(results.violations).toEqual([])
  })
})
