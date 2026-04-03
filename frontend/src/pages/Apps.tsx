import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { apps as appsApi, appTemplates as templatesApi } from '../api/client'
import type { App, AppTemplate } from '../api/types'
import '../styles/Modal.css'
import './Apps.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function monogram(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

function AppIcon({ name, profileId }: { name: string; profileId: string | null }) {
  const [failed, setFailed] = useState(false)
  // reset if profileId changes
  useEffect(() => { setFailed(false) }, [profileId])
  if (profileId && !failed) {
    return (
      <img
        src={`/api/v1/icons/${profileId}`}
        alt={name}
        className="app-icon-img"
        onError={() => setFailed(true)}
      />
    )
  }
  return <>{monogram(name)}</>
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

// ── Confirm Delete Modal ──────────────────────────────────────────────────────

interface ConfirmDeleteProps {
  appName: string
  onCancel: () => void
  onConfirm: () => void
  deleting: boolean
  error: string
}

function ConfirmDeleteModal({ appName, onCancel, onConfirm, deleting, error }: ConfirmDeleteProps) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onCancel() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onCancel])

  return (
    <div className="modal-backdrop">
      <div className="modal" style={{ width: 400 }}>
        <div className="modal-header">
          <div className="modal-title modal-title-danger">Delete App</div>
          <div className="modal-subtitle">
            This will permanently delete <strong style={{ color: 'var(--text)' }}>{appName}</strong> and all its events and metrics. This cannot be undone.
          </div>
          <button className="modal-close" onClick={onCancel}>✕</button>
        </div>
        <div className="modal-body">
          {error && <div className="modal-error">{error}</div>}
        </div>
        <div className="modal-footer">
          <button className="modal-btn-ghost" onClick={onCancel}>Cancel</button>
          <button className="modal-btn-danger" onClick={onConfirm} disabled={deleting}>
            {deleting ? 'Deleting…' : 'Delete App'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── AddApp Modal ──────────────────────────────────────────────────────────────

type AddStep = 'setup' | 'config' | 'done'

interface AddAppModalProps {
  onClose: () => void
  onCreated: (app: App) => void
}

function AddAppModal({ onClose, onCreated }: AddAppModalProps) {
  const navigate = useNavigate()
  const [step, setStep] = useState<AddStep>('setup')

  const [appName, setAppName] = useState('')
  const nameRef = useRef<HTMLInputElement>(null)

  const [templates, setTemplates] = useState<AppTemplate[]>([])
  const [templatesLoading, setTemplatesLoading] = useState(true)
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('')

  const [baseUrl, setBaseUrl] = useState('')
  const [monitorUrl, setMonitorUrl] = useState('')
  const [rateLimit, setRateLimit] = useState('0')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState('')

  const [createdApp, setCreatedApp] = useState<App | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => { nameRef.current?.focus() }, [])

  useEffect(() => {
    templatesApi.list()
      .then(res => setTemplates(res.data))
      .catch(console.error)
      .finally(() => setTemplatesLoading(false))
  }, [])

  useEffect(() => {
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const selectedTemplate = templates.find(t => t.id === selectedTemplateId) ?? null

  const grouped = templates.reduce<Record<string, AppTemplate[]>>((acc, t) => {
    if (!acc[t.category]) acc[t.category] = []
    acc[t.category].push(t)
    return acc
  }, {})

  const needsMonitor =
    selectedTemplate?.capability === 'full' ||
    selectedTemplate?.capability === 'monitor_only'

  async function handleCreate() {
    setSubmitError('')
    setSubmitting(true)
    try {
      const config: Record<string, unknown> = {}
      if (baseUrl.trim())    config.base_url    = baseUrl.trim()
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

  function webhookUrl(token: string) {
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
    setStep('setup'); setAppName(''); setSelectedTemplateId('')
    setBaseUrl(''); setMonitorUrl(''); setRateLimit('0')
    setCreatedApp(null); setCopied(false); setSubmitError('')
  }

  return (
    <div className="modal-backdrop">
      <div className="modal">

        {step === 'setup' && (
          <>
            <div className="modal-header">
              <div className="modal-title">New App</div>
              <div className="modal-subtitle">Name your connection and pick an app template</div>
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
                onKeyDown={e => e.key === 'Enter' && appName.trim() && setStep('config')}
              />

              <label className="modal-label" style={{ marginTop: 16 }}>
                App Template <span className="modal-hint">(optional — enables field mapping)</span>
              </label>
              <select
                className="modal-input"
                value={selectedTemplateId}
                onChange={e => setSelectedTemplateId(e.target.value)}
                disabled={templatesLoading}
              >
                <option value="">Generic Webhook — raw JSON, no mapping</option>
                {templatesLoading ? (
                  <option disabled>Loading templates…</option>
                ) : (
                  Object.entries(grouped).sort(([a], [b]) => a.localeCompare(b)).map(([cat, items]) => (
                    <optgroup key={cat} label={cat}>
                      {items.map(t => (
                        <option key={t.id} value={t.id}>
                          {t.name} — {CAPABILITY_LABEL[t.capability] ?? t.capability}
                        </option>
                      ))}
                    </optgroup>
                  ))
                )}
              </select>
            </div>
            <div className="modal-footer">
              <button className="modal-btn-ghost" onClick={onClose}>Cancel</button>
              <button className="modal-btn-primary" onClick={() => setStep('config')} disabled={!appName.trim()}>
                Next →
              </button>
            </div>
          </>
        )}

        {step === 'config' && (
          <>
            <div className="modal-header">
              <div className="modal-title">Configure{selectedTemplate ? ` ${selectedTemplate.name}` : ' App'}</div>
              <div className="modal-subtitle">
                {selectedTemplate ? selectedTemplate.description : 'Set optional details for this app connection'}
              </div>
              <button className="modal-close" onClick={onClose}>✕</button>
            </div>
            <div className="modal-body">
              <label className="modal-label">
                App URL <span className="modal-hint">(optional — enables the Launch button)</span>
              </label>
              <input className="modal-input" placeholder="https://sonarr.yourdomain.com"
                value={baseUrl} onChange={e => setBaseUrl(e.target.value)} />

              {needsMonitor && (
                <>
                  <label className="modal-label" style={{ marginTop: 16 }}>
                    Monitor URL <span className="modal-hint">(NORA will ping this to check uptime)</span>
                  </label>
                  <input className="modal-input" placeholder="https://sonarr.yourdomain.com/ping"
                    value={monitorUrl} onChange={e => setMonitorUrl(e.target.value)} />
                </>
              )}

              <label className="modal-label" style={{ marginTop: 16 }}>
                Rate limit <span className="modal-hint">(events / minute, 0 = unlimited)</span>
              </label>
              <input className="modal-input modal-input-sm" type="number" min="0"
                value={rateLimit} onChange={e => setRateLimit(e.target.value)} />

              {submitError && <div className="modal-error">{submitError}</div>}
            </div>
            <div className="modal-footer">
              <button className="modal-btn-ghost" onClick={() => setStep('setup')}>← Back</button>
              <button className="modal-btn-primary" onClick={handleCreate} disabled={submitting}>
                {submitting ? 'Creating…' : 'Create App'}
              </button>
            </div>
          </>
        )}

        {step === 'done' && createdApp && (
          <>
            <div className="modal-header">
              <div className="modal-title modal-title-success">✓ App Created</div>
              <div className="modal-subtitle">
                Copy the webhook URL and paste it into your app's notification settings
              </div>
              <button className="modal-close" onClick={onClose}>✕</button>
            </div>
            <div className="modal-body">
              <label className="modal-label">Webhook URL</label>
              <div className="webhook-url-row">
                <input className="modal-input modal-input-mono" readOnly
                  value={webhookUrl(createdApp.token)} onFocus={e => e.target.select()} />
                <button className={`webhook-copy-btn${copied ? ' copied' : ''}`} onClick={handleCopy}>
                  {copied ? '✓ Copied' : 'Copy'}
                </button>
              </div>
              <div className="webhook-hint">
                POST a JSON body to this URL to ingest events.
                {selectedTemplate && <> The <strong>{selectedTemplate.name}</strong> template will parse them automatically.</>}
              </div>
            </div>
            <div className="modal-footer">
              <button className="modal-btn-ghost" onClick={handleAddAnother}>+ Add Another</button>
              <button className="modal-btn-primary" onClick={() => navigate(`/apps/${createdApp.id}`)}>
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
  const [showAdd, setShowAdd] = useState(false)

  // card kebab state
  const { tick } = useAutoRefresh()
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')

  useEffect(() => {
    appsApi.list()
      .then(res => setAppList(res.data))
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [tick])

  // Close dropdown on outside click
  useEffect(() => {
    if (!openMenuId) return
    function handler() { setOpenMenuId(null) }
    window.addEventListener('click', handler)
    return () => window.removeEventListener('click', handler)
  }, [openMenuId])

  async function handleDelete() {
    if (!confirmDeleteId) return
    setDeleting(true)
    setDeleteError('')
    try {
      await appsApi.delete(confirmDeleteId)
      setAppList(prev => prev.filter(a => a.id !== confirmDeleteId))
      setConfirmDeleteId(null)
    } catch (err: unknown) {
      setDeleteError(err instanceof Error ? err.message : 'Failed to delete app')
    } finally {
      setDeleting(false)
    }
  }

  const confirmApp = appList.find(a => a.id === confirmDeleteId)

  return (
    <>
      <Topbar title="Apps" />
      <div className="content">
        <div className="section-header">
          <span className="section-title">Configured Apps</span>
          <button className="section-action" onClick={() => setShowAdd(true)}>+ Add app</button>
        </div>

        <div className="apps-page-grid widget-grid">
          {loading ? (
            [0, 1, 2].map(i => (
              <div key={i} className="app-widget skeleton" style={{ height: 100 }} />
            ))
          ) : appList.length === 0 ? (
            <div className="apps-empty">
              No apps configured yet.{' '}
              <button className="apps-empty-link" onClick={() => setShowAdd(true)}>
                Add your first app →
              </button>
            </div>
          ) : (
            appList.map(app => (
              <div
                key={app.id}
                className={`app-widget${openMenuId === app.id ? ' menu-open' : ''}`}
                onClick={() => navigate(`/apps/${app.id}`)}
              >
                {openMenuId === app.id && (
                  <div className="app-card-dropdown" onClick={e => e.stopPropagation()}>
                    <button className="card-dropdown-item" onClick={() => { setOpenMenuId(null); navigate(`/apps/${app.id}`) }}>
                      ⚙ Settings
                    </button>
                    <button className="card-dropdown-item danger" onClick={() => { setOpenMenuId(null); setConfirmDeleteId(app.id) }}>
                      🗑 Delete App
                    </button>
                  </div>
                )}

                <div className="app-widget-header">
                  <div className="app-icon"><AppIcon name={app.name} profileId={app.profile_id} /></div>
                  <div className="app-name-group">
                    <div className="app-name">{app.name}</div>
                    {app.profile_id && (
                      <div className="app-profile-badge">{app.profile_id}</div>
                    )}
                  </div>
                </div>
                <button
                  className={`app-card-menu-btn${openMenuId === app.id ? ' open' : ''}`}
                  title="Settings"
                  onClick={e => {
                    e.stopPropagation()
                    setOpenMenuId(prev => prev === app.id ? null : app.id)
                  }}
                >
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="13" height="13">
                    <circle cx="12" cy="12" r="3" />
                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
                  </svg>
                </button>
                <div className="app-last-event">
                  Added {formatDate(app.created_at)}
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      {showAdd && (
        <AddAppModal
          onClose={() => setShowAdd(false)}
          onCreated={app => {
            setAppList(prev => [...prev, app])
            setShowAdd(false)
          }}
        />
      )}

      {confirmDeleteId && confirmApp && (
        <ConfirmDeleteModal
          appName={confirmApp.name}
          onCancel={() => { setConfirmDeleteId(null); setDeleteError('') }}
          onConfirm={handleDelete}
          deleting={deleting}
          error={deleteError}
        />
      )}
    </>
  )
}
