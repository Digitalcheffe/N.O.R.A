/**
 * Dev-only test harness for SlidePanel component.
 * Route: /dev/slide-panel  (only registered when import.meta.env.DEV)
 */
import { useState } from 'react'
import { SlidePanel } from '../components/SlidePanel'
import './SlidePanelDevPage.css'

type Variant = {
  id: string
  label: string
  title: string
  subtitle?: string
  width?: number
  hasFooter: boolean
  bodyContent: 'short' | 'long' | 'form'
}

const VARIANTS: Variant[] = [
  {
    id: 'basic',
    label: 'Basic — title only, no footer',
    title: 'Basic Panel',
    hasFooter: false,
    bodyContent: 'short',
  },
  {
    id: 'subtitle',
    label: 'With subtitle',
    title: 'Panel with Subtitle',
    subtitle: 'Optional muted line below the title',
    hasFooter: false,
    bodyContent: 'short',
  },
  {
    id: 'footer',
    label: 'With footer + primary CTA',
    title: 'Edit Configuration',
    subtitle: 'Changes are saved immediately',
    hasFooter: true,
    bodyContent: 'form',
  },
  {
    id: 'wide',
    label: 'Wide panel (width=640)',
    title: 'Wide Panel',
    subtitle: 'Desktop width overridden to 640px',
    width: 640,
    hasFooter: true,
    bodyContent: 'long',
  },
  {
    id: 'narrow',
    label: 'Narrow panel (width=360)',
    title: 'Narrow Panel',
    width: 360,
    hasFooter: true,
    bodyContent: 'short',
  },
]

function ShortBody() {
  return (
    <div className="spdev-body-content">
      <p>This is a short body. The panel should size to its content on mobile and fill the height on desktop.</p>
      <p style={{ color: 'var(--text3)', fontSize: 12 }}>No scrolling required at this content length.</p>
    </div>
  )
}

function LongBody() {
  return (
    <div className="spdev-body-content">
      {Array.from({ length: 20 }, (_, i) => (
        <div key={i} className="spdev-row">
          <span className="spdev-row-label">Field {i + 1}</span>
          <span className="spdev-row-value">Value {i + 1}</span>
        </div>
      ))}
    </div>
  )
}

function FormBody() {
  return (
    <div className="spdev-body-content">
      <label className="spdev-field">
        <span className="spdev-field-label">Name</span>
        <input className="spdev-input" type="text" defaultValue="My service" />
      </label>
      <label className="spdev-field">
        <span className="spdev-field-label">Environment</span>
        <select className="spdev-input">
          <option>Production</option>
          <option>Staging</option>
          <option>Development</option>
        </select>
      </label>
      <label className="spdev-field">
        <span className="spdev-field-label">Notes</span>
        <textarea className="spdev-input spdev-textarea" defaultValue="Some notes here." />
      </label>
    </div>
  )
}

export function SlidePanelDevPage() {
  const [openId, setOpenId] = useState<string | null>(null)

  const activeVariant = VARIANTS.find(v => v.id === openId) ?? null

  return (
    <div className="spdev-page">
      <div className="spdev-header">
        <h1 className="spdev-heading">SlidePanel — Component Test Harness</h1>
        <p className="spdev-hint">
          Resize window below 768px to test mobile bottom-sheet mode.
        </p>
      </div>

      <div className="spdev-grid">
        {VARIANTS.map(v => (
          <button
            key={v.id}
            className="spdev-trigger"
            onClick={() => setOpenId(v.id)}
          >
            <span className="spdev-trigger-label">{v.label}</span>
            <span className="spdev-trigger-meta">
              {v.width ? `${v.width}px` : '480px (default)'}
              {v.hasFooter ? ' · footer' : ' · no footer'}
            </span>
          </button>
        ))}
      </div>

      <div className="spdev-info">
        <div className="spdev-info-row">
          <span className="spdev-info-key">Escape key</span>
          <span className="spdev-info-val">calls onClose (dismiss panel)</span>
        </div>
        <div className="spdev-info-row">
          <span className="spdev-info-key">Backdrop click</span>
          <span className="spdev-info-val">does NOT close (use ✕ or Cancel)</span>
        </div>
        <div className="spdev-info-row">
          <span className="spdev-info-key">Focus trap</span>
          <span className="spdev-info-val">Tab cycles within the panel only</span>
        </div>
        <div className="spdev-info-row">
          <span className="spdev-info-key">Body scroll lock</span>
          <span className="spdev-info-val">overflow:hidden on document.body while open</span>
        </div>
      </div>

      {activeVariant && (
        <SlidePanel
          open={openId === activeVariant.id}
          onClose={() => setOpenId(null)}
          title={activeVariant.title}
          subtitle={activeVariant.subtitle}
          width={activeVariant.width}
          footer={
            activeVariant.hasFooter ? (
              <button
                className="sp-btn sp-btn--primary"
                onClick={() => setOpenId(null)}
              >
                Save changes
              </button>
            ) : undefined
          }
        >
          {activeVariant.bodyContent === 'short' && <ShortBody />}
          {activeVariant.bodyContent === 'long' && <LongBody />}
          {activeVariant.bodyContent === 'form' && <FormBody />}
        </SlidePanel>
      )}
    </div>
  )
}
