import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useIsMobile } from '../hooks/useIsMobile'
import './SlidePanel.css'

export interface SlidePanelProps {
  open: boolean
  onClose: () => void
  title: string
  subtitle?: string
  width?: number          // desktop width in px, default 480
  children: React.ReactNode
  footer?: React.ReactNode  // sticky footer — Primary CTA goes here; Cancel is auto-rendered
}

const FOCUSABLE_SELECTORS =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

export function SlidePanel({
  open,
  onClose,
  title,
  subtitle,
  width = 480,
  children,
  footer,
}: SlidePanelProps) {
  const isMobile = useIsMobile()
  const [mounted, setMounted] = useState(false)
  const [closing, setClosing] = useState(false)
  const panelRef = useRef<HTMLDivElement>(null)

  // ── Mount / unmount with exit animation ────────────────────────────────────
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>
    if (open) {
      setMounted(true)
      setClosing(false)
    } else {
      setClosing(true)
      timer = setTimeout(() => {
        setMounted(false)
        setClosing(false)
      }, 150)
    }
    return () => clearTimeout(timer)
  }, [open])

  // ── Prevent body scroll while panel is visible ─────────────────────────────
  useEffect(() => {
    if (!mounted) return
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = ''
    }
  }, [mounted])

  // ── Escape key calls onClose ───────────────────────────────────────────────
  useEffect(() => {
    if (!mounted) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [mounted, onClose])

  // ── Focus trap ─────────────────────────────────────────────────────────────
  useEffect(() => {
    if (!mounted || !panelRef.current) return
    const panel = panelRef.current

    const getFocusable = () =>
      Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS))

    // Auto-focus the first focusable element
    const els = getFocusable()
    els[0]?.focus()

    const handleTab = (e: KeyboardEvent) => {
      if (e.key !== 'Tab') return
      const els = getFocusable()
      if (els.length === 0) return
      const first = els[0]
      const last = els[els.length - 1]
      if (e.shiftKey) {
        if (document.activeElement === first) {
          e.preventDefault()
          last.focus()
        }
      } else {
        if (document.activeElement === last) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    document.addEventListener('keydown', handleTab)
    return () => document.removeEventListener('keydown', handleTab)
  }, [mounted])

  if (!mounted) return null

  const desktopWidth = Math.min(width, Math.round(window.innerWidth * 0.9))

  return createPortal(
    <div className={`sp-overlay${closing ? ' sp-overlay--exit' : ''}`}>
      <div
        ref={panelRef}
        className={[
          'sp-panel',
          isMobile ? 'sp-panel--mobile' : 'sp-panel--desktop',
          closing ? 'sp-panel--exit' : '',
        ]
          .filter(Boolean)
          .join(' ')}
        style={!isMobile ? { width: desktopWidth } : undefined}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        {isMobile && <div className="sp-drag-handle" aria-hidden="true" />}

        <div className="sp-header">
          <div className="sp-header-text">
            <span className="sp-title">{title}</span>
            {subtitle && <span className="sp-subtitle">{subtitle}</span>}
          </div>
          <button className="sp-close-btn" onClick={onClose} aria-label="Close panel">
            ✕
          </button>
        </div>

        <div className="sp-body">{children}</div>

        {footer && (
          <div className="sp-footer">
            <button className="sp-btn sp-btn--secondary" onClick={onClose}>
              Cancel
            </button>
            {footer}
          </div>
        )}
      </div>
    </div>,
    document.body
  )
}
