import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { apps as appsApi, appTemplates as templatesApi } from '../api/client'
import type { App, AppTemplate } from '../api/types'
import './Apps.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function monogram(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

const CAPABILITY_LABEL: Record<string, string> = {
  full:         'Webhook + Monitor',
  webhook_only: 'Webhook',
  monitor_only: 'Monitor',
  docker_only:  'Docker',
  limited:      'Limited',
}

// ── AddApp Modal ──────────────────────────────────────────────────────────────

type Step = 'name' | 'template' | 'config' | 'done'

interface AddAppModalProps {
  onClose: () => void
  onCreated: (app: App) => void
}

function AddAppModal({ onClose, onCreated }: AddAppModalProps) {
  const navigate = useNavigate()
  const [step, setStep] = useState<Step>('name')

  // Step 1 — name
  const [appName, setAppName] = useState('')
  const nameRef = useRef<HTMLInputElement>(null)

  // Step 2 — template
  const [templates, setTemplates] = useState<AppTemplate[]>([])
  const [templatesLoading, setTemplatesLoading] = useState(false)
  const [selectedTemplate, setSelectedTemplate] = useState<AppTemplate | null>(null)

  // Step 3 — config
  const [baseUrl, setBaseUrl] = useState('')
  const [monitorUrl, setMonitorUrl] = useState('')
  const [rateLimit, setRateLimit] = useState('0')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState('')

  // Step 4 — done
  const [createdApp, setCreatedApp] = useState<App | null>(null)
  const [copied, setCopied] = useState(false)

  // Focus name input on mount
  useEffect(() => {
    nameRef.current?.focus()
  }, [])

  // Load templates when moving to step 2
  useEffect(() => {
    if (step !== 'template') return
    setTemplatesLoading(true)
    templatesApi.list()
      .then(res => setTemplates(res.data))
      .catch(console.error)
      .finally(() => setTemplatesLoading(false))
  }, [step])

  // Close on Escape
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  function handleNameNext() {
    if (!appName.trim()) return
    setStep('template')
  }

  function handleTemplateSelect(tmpl: AppTemplate | null) {
    setSelectedTemplate(tmpl)
    setStep('config')
  }

  async function handleCreate() {
    setSubmitError('')
    setSubmitting(true)
    try {
      const config: Record<string, unknown> = {}
      if (baseUrl.trim()) config.base_url = baseUrl.trim()
      if (monitorUrl.trim()) config.monitor_url = monitorUrl.trim()

      const app = await appsApi.create({
        name: appName.trim(),
        profile_id: selectedTemplate?.id,
        config: Object.keys(config).length > 0 ? config : undefined,
        rate_limit: parseInt(rateLimit, 10) || 0,
      })
      setCreatedApp(app)
      onCreated(app)
      setStep('done')
    } catch (err: unknown) {
      setSubmitError(err instanceof Error ? err.message : 'Failed to create app')
    } finally {
      setSubmitting(false)
    }
  }

  function webhookUrl(token: string): string {
    return `${window.location.origin}/api/v1/ingest/${token}`
  }

  function handleCopy() {
    if (!createdApp) return
    navigator.clipboard.writeText(webhookUrl(createdApp.token)).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  function handleAddAnother() {
    setStep('name')
    setAppName('')
    setSelectedTemplate(null)
    setBaseUrl('')
    setMonitorUrl('')
    setRateLimit('0')
    setCreatedApp(null)
    setCopied(false)
    setSubmitError('')
  }

  // Group templates by category
  const grouped = templates.reduce<Record<string, AppTemplate[]>>((acc, t) => {
    if (!acc[t.category]) acc[t.category] = []
    acc[t.category].push(t)
    return acc
  }, {})

  const needsMonitor =
    selectedTemplate?.capability === 'full' ||
    selectedTemplate?.capability === 'monitor_only'

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>

        {/* ── Step: name ── */}
        {step === 'name' && (
          <>
            <div className="modal-header">
              <div className="modal-title">New App</div>
              <div className="modal-subtitle">Give this connection a name you'll recognise</div>
              <button className="modal-close" onClick={onClose}>✕</button>
            </div>
            <div className="modal-body">
              <label className="modal-label">App Name</label>
              <input
                ref={nameRef}
                className="modal-input"
                placeholder="e.g. Sonarr, Home Assistant…"
                value={appName}
                onChange={e => setAppName(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleNameNext()}
              />
            </div>
            <div className="modal-footer">
              <button className="modal-btn-ghost" onClick={onClose}>Cancel</button>
              <button
                className="modal-btn-primary"
                onClick={handleNameNext}
                disabled={!appName.trim()}
              >
                Next →
              </button>
            </div>
          </>
        )}

        {/* ── Step: template ── */}
        {step === 'template' && (
          <>
            <div className="modal-header">
              <div className="modal-title">Choose a Template</div>
              <div className="modal-subtitle">Select the app you're connecting — or skip for a generic webhook</div>
              <button className="modal-close" onClick={onClose}>✕</button>
            </div>
            <div className="modal-body modal-body-templates">
              {/* Generic / no-template option */}
              <div className="tmpl-group-label">Generic</div>
              <div className="tmpl-grid">
                <button className="tmpl-card" onClick={() => handleTemplateSelect(null)}>
                  <div className="tmpl-icon">⚡</div>
                  <div className="tmpl-name">Generic Webhook</div>
                  <div className="tmpl-desc">Raw JSON ingest, no field mapping</div>
                </button>
              </div>

              {templatesLoading ? (
                <div className="tmpl-loading">Loading templates…</div>
              ) : (
                Object.entries(grouped).sort(([a], [b]) => a.localeCompare(b)).map(([cat, items]) => (
                  <div key={cat}>
                    <div className="tmpl-group-label">{cat}</div>
                    <div className="tmpl-grid">
                      {items.map(t => (
                        <button key={t.id} className="tmpl-card" onClick={() => handleTemplateSelect(t)}>
                          <div className="tmpl-icon">{monogram(t.name)}</div>
                          <div className="tmpl-name">{t.name}</div>
                          <div className="tmpl-cap">{CAPABILITY_LABEL[t.capability] ?? t.capability}</div>
                          <div className="tmpl-desc">{t.description}</div>
                        </button>
                      ))}
                    </div>
                  </div>
                ))
              )}
            </div>
            <div className="modal-footer">
              <button className="modal-btn-ghost" onClick={() => setStep('name')}>← Back</button>
            </div>
          </>
        )}

        {/* ── Step: config ── */}
        {step === 'config' && (
          <>
            <div className="modal-header">
              <div className="modal-title">
                Configure{selectedTemplate ? ` ${selectedTemplate.name}` : ' App'}
              </div>
              <div className="modal-subtitle">
                {selectedTemplate
                  ? selectedTemplate.description
                  : 'Set optional details for this app connection'}
              </div>
              <button className="modal-close" onClick={onClose}>✕</button>
            </div>
            <div className="modal-body">
              <label className="modal-label">
                App URL <span className="modal-hint">(optional — enables the Launch button)</span>
              </label>
              <input
                className="modal-input"
                placeholder="https://sonarr.yourdomain.com"
                value={baseUrl}
                onChange={e => setBaseUrl(e.target.value)}
              />

              {needsMonitor && (
                <>
                  <label className="modal-label" style={{ marginTop: 16 }}>
                    Monitor URL <span className="modal-hint">(NORA will ping this to check uptime)</span>
                  </label>
                  <input
                    className="modal-input"
                    placeholder="https://sonarr.yourdomain.com/ping"
                    value={monitorUrl}
                    onChange={e => setMonitorUrl(e.target.value)}
                  />
                </>
              )}

              <label className="modal-label" style={{ marginTop: 16 }}>
                Rate limit <span className="modal-hint">(events / minute, 0 = unlimited)</span>
              </label>
              <input
                className="modal-input modal-input-sm"
                type="number"
                min="0"
                value={rateLimit}
                onChange={e => setRateLimit(e.target.value)}
              />

              {submitError && <div className="modal-error">{submitError}</div>}
            </div>
            <div className="modal-footer">
              <button className="modal-btn-ghost" onClick={() => setStep('template')}>← Back</button>
              <button
                className="modal-btn-primary"
                onClick={handleCreate}
                disabled={submitting}
              >
                {submitting ? 'Creating…' : 'Create App'}
              </button>
            </div>
          </>
        )}

        {/* ── Step: done ── */}
        {step === 'done' && createdApp && (
          <>
            <div className="modal-header">
              <div className="modal-title modal-title-success">✓ App Created</div>
              <div className="modal-subtitle">
                Copy the webhook URL below and paste it into your app's notification settings
              </div>
              <button className="modal-close" onClick={onClose}>✕</button>
            </div>
            <div className="modal-body">
              <label className="modal-label">Webhook URL</label>
              <div className="webhook-url-row">
                <input
                  className="modal-input modal-input-mono"
                  readOnly
                  value={webhookUrl(createdApp.token)}
                  onFocus={e => e.target.select()}
                />
                <button
                  className={`webhook-copy-btn${copied ? ' copied' : ''}`}
                  onClick={handleCopy}
                >
                  {copied ? '✓ Copied' : 'Copy'}
                </button>
              </div>
              <div className="webhook-hint">
                Send a POST request with a JSON body to this URL to ingest events.
                {selectedTemplate && (
                  <> The <strong>{selectedTemplate.name}</strong> template will parse and display them automatically.</>
                )}
              </div>
            </div>
            <div className="modal-footer">
              <button className="modal-btn-ghost" onClick={handleAddAnother}>+ Add Another</button>
              <button
                className="modal-btn-primary"
                onClick={() => navigate(`/apps/${createdApp.id}`)}
              >
                View App →
              </button>
            </div>
          </>
        )}

      </div>
    </div>
  )
}

// ── Apps Page ─────────────────────────────────────────────────────────────────

export function Apps() {
  const navigate = useNavigate()
  const [appList, setAppList] = useState<App[]>([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)

  useEffect(() => {
    appsApi.list()
      .then(res => setAppList(res.data))
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  function handleCreated(app: App) {
    setAppList(prev => [...prev, app])
  }

  return (
    <>
      <Topbar title="Apps" onAdd={() => setShowModal(true)} />
      <div className="content">
        <div className="section-header">
          <span className="section-title">Configured Apps</span>
          <button className="section-action" onClick={() => setShowModal(true)}>+ Add app</button>
        </div>

        <div className="widget-grid">
          {loading ? (
            [0, 1, 2].map(i => (
              <div key={i} className="app-widget skeleton" style={{ height: 100 }} />
            ))
          ) : appList.length === 0 ? (
            <div className="apps-empty">
              No apps configured yet.{' '}
              <button className="apps-empty-link" onClick={() => setShowModal(true)}>
                Add your first app →
              </button>
            </div>
          ) : (
            appList.map(app => (
              <div
                key={app.id}
                className="app-widget"
                onClick={() => navigate(`/apps/${app.id}`)}
              >
                <div className="app-widget-header">
                  <div className="app-icon">{monogram(app.name)}</div>
                  <div className="app-name">{app.name}</div>
                  {app.profile_id && (
                    <div className="app-profile-badge">{app.profile_id}</div>
                  )}
                </div>
                <div className="app-last-event">
                  Added {formatDate(app.created_at)}
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      {showModal && (
        <AddAppModal
          onClose={() => setShowModal(false)}
          onCreated={app => {
            handleCreated(app)
            setShowModal(false)
          }}
        />
      )}
    </>
  )
}
