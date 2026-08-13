'use client'

import {
  type CSSProperties,
  type ReactNode,
  useEffect,
  useRef,
  useState,
} from 'react'
import { createPortal } from 'react-dom'

const focusableSelector = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  'summary',
  '[contenteditable="true"]',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

const modalStack: HTMLElement[] = []
const originalInert = new Map<HTMLElement, boolean>()
let originalBodyOverflow = ''
let bodyObserver: MutationObserver | null = null

function topModal(): HTMLElement | undefined {
  return modalStack[modalStack.length - 1]
}

function rememberInertState(element: HTMLElement) {
  if (!originalInert.has(element)) originalInert.set(element, element.hasAttribute('inert'))
}

function syncBackgroundInertness() {
  const activePortal = topModal()
  for (const child of Array.from(document.body.children)) {
    if (!(child instanceof HTMLElement)) continue
    rememberInertState(child)
    if (child === activePortal) child.removeAttribute('inert')
    else child.setAttribute('inert', '')
  }
}

function registerModal(portal: HTMLElement) {
  if (modalStack.length === 0) {
    originalBodyOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    bodyObserver = new MutationObserver(syncBackgroundInertness)
    bodyObserver.observe(document.body, { childList: true })
  }

  modalStack.push(portal)
  syncBackgroundInertness()

  return () => {
    const index = modalStack.lastIndexOf(portal)
    if (index !== -1) modalStack.splice(index, 1)

    if (modalStack.length > 0) {
      syncBackgroundInertness()
      return
    }

    bodyObserver?.disconnect()
    bodyObserver = null
    for (const [element, inert] of originalInert) {
      if (inert) element.setAttribute('inert', '')
      else element.removeAttribute('inert')
    }
    originalInert.clear()
    document.body.style.overflow = originalBodyOverflow
  }
}

function focusableElements(dialog: HTMLElement): HTMLElement[] {
  return Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector)).filter(
    (element) => !element.matches(':disabled')
      && !element.closest('[hidden], [aria-hidden="true"], [inert]'),
  )
}

export interface DialogProps {
  children: ReactNode
  labelledBy: string
  describedBy: string
  onClose: () => void
  contentStyle?: CSSProperties
}

export default function Dialog({
  children,
  labelledBy,
  describedBy,
  onClose,
  contentStyle,
}: DialogProps) {
  const [portal, setPortal] = useState<HTMLDivElement | null>(null)
  const dialogRef = useRef<HTMLDivElement>(null)
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose

  useEffect(() => {
    const node = document.createElement('div')
    node.dataset.dialogPortal = ''
    document.body.appendChild(node)
    setPortal(node)
    return () => node.remove()
  }, [])

  useEffect(() => {
    if (!portal) return
    const dialog = dialogRef.current
    if (!dialog) return

    const previouslyFocused = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null
    const unregister = registerModal(portal)

    const focusInitialElement = () => {
      const target = focusableElements(dialog)[0] ?? dialog
      target.focus({ preventScroll: true })
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (topModal() !== portal) return
      if (event.key === 'Escape') {
        event.preventDefault()
        event.stopPropagation()
        onCloseRef.current()
        return
      }
      if (event.key !== 'Tab') return

      const focusable = focusableElements(dialog)
      if (focusable.length === 0) {
        event.preventDefault()
        dialog.focus({ preventScroll: true })
        return
      }

      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      const active = document.activeElement
      if (!dialog.contains(active)) {
        event.preventDefault()
        ;(event.shiftKey ? last : first).focus({ preventScroll: true })
      } else if (!focusable.includes(active as HTMLElement)) {
        event.preventDefault()
        ;(event.shiftKey ? last : first).focus({ preventScroll: true })
      } else if (event.shiftKey && active === first) {
        event.preventDefault()
        last.focus({ preventScroll: true })
      } else if (!event.shiftKey && active === last) {
        event.preventDefault()
        first.focus({ preventScroll: true })
      }
    }

    const handleFocusIn = (event: FocusEvent) => {
      if (topModal() !== portal || dialog.contains(event.target as Node)) return
      focusInitialElement()
    }

    document.addEventListener('keydown', handleKeyDown, true)
    document.addEventListener('focusin', handleFocusIn, true)
    focusInitialElement()

    return () => {
      document.removeEventListener('keydown', handleKeyDown, true)
      document.removeEventListener('focusin', handleFocusIn, true)
      unregister()
      if (previouslyFocused?.isConnected) previouslyFocused.focus({ preventScroll: true })
    }
  }, [portal])

  if (!portal) return null

  return createPortal(
    <div
      data-dialog-backdrop
      onClick={(event) => {
        if (event.target === event.currentTarget) onCloseRef.current()
      }}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(10, 10, 10, 0.35)',
        backdropFilter: 'blur(4px)',
        WebkitBackdropFilter: 'blur(4px)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy}
        aria-describedby={describedBy}
        tabIndex={-1}
        style={{
          background: 'var(--lf-paper)',
          border: '1px solid var(--lf-ink)',
          borderRadius: 'var(--lf-radius)',
          width: '100%',
          maxHeight: '92vh',
          overflowY: 'auto',
          boxShadow: '0 12px 36px rgba(10, 10, 10, 0.18)',
          ...contentStyle,
        }}
      >
        {children}
      </div>
    </div>,
    portal,
  )
}
