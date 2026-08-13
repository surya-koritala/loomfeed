// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import axe from 'axe-core'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PostReceipt from './PostReceipt'
import RevisionModal from './RevisionModal'

vi.mock('../api/client', () => ({
  api: {
    getPostReceipt: vi.fn().mockResolvedValue({
      postId: 'post-1',
      postCreatedAt: '2026-08-13T00:00:00Z',
      sources: [],
    }),
    getPostRevisions: vi.fn().mockResolvedValue([]),
    getCommentRevisions: vi.fn().mockResolvedValue([]),
  },
}))

;(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true

describe('accessible modal consumers', () => {
  let host: HTMLDivElement
  let root: Root

  beforeEach(() => {
    document.body.innerHTML = ''
    host = document.createElement('div')
    document.body.appendChild(host)
    root = createRoot(host)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    document.body.innerHTML = ''
  })

  async function expectAccessibleDialog(title: string, description: string) {
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    const titleElement = document.getElementById(dialog.getAttribute('aria-labelledby')!)
    const descriptionElement = document.getElementById(dialog.getAttribute('aria-describedby')!)
    expect(titleElement?.textContent).toContain(title)
    expect(descriptionElement?.textContent).toContain(description)

    const results = await axe.run(dialog, {
      rules: { 'color-contrast': { enabled: false } },
    })
    expect(results.violations).toEqual([])
  }

  it('gives the post receipt an accessible name and description', async () => {
    await act(async () => {
      root.render(<PostReceipt postId="post-1" onClose={vi.fn()} />)
    })

    await expectAccessibleDialog('Auditable claim', 'Provenance and source verification')
  })

  it('gives the revision history an accessible name and description', async () => {
    await act(async () => {
      root.render(
        <RevisionModal
          target={{ kind: 'post', id: 'post-1', current: { title: 'Post', body: 'Current' } }}
          onClose={vi.fn()}
        />,
      )
    })

    await expectAccessibleDialog('Edit history', 'Compare saved revisions')
  })
})
