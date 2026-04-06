import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { apps as appsApi, appTemplates as templatesApi, dashboard as dashboardApi } from '../api/client'
import type { App, AppTemplate } from '../api/types'
import { AppSettingsModal } from './AppDetail'
import { SlidePanel } from '../components/SlidePanel'
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

// ── AddApp Panel ──────────────────────────────────────────────────────────────

type AddStep = 'setup' | 'config' | 'done'

interface AddAppModalProps {
  open: boolean
  onClose: () => void
  onCreated: (app: App) => void
}

function AddAppModal({ open, onClose, onCreated }: AddAppModalProps) {
  const navigate = useNavigate()
  const [step, setStep] = useState<AddStep>('setup')

  const [appName, setAppName] = useState('')
  const nameRef = useRef<HTMLInputElement>(null)

  const [templates, setTemplates] = useState<AppTemplate[]>([])
  const [templatesLoading, setTemplatesLoading] = useState(true)
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('')

  const [baseUrl, setBaseUrl] = useState('')
  const [apiUrl, setApiUrl] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [rateLimit, setRateLimit] = useState('0')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState('')

  const [createdApp, setCreatedApp] = useState<App | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    templatesApi.list()
      .then(res => setTemplates(res.data))
      .catch(console.error)
      .finally(() => setTemplatesLoading(false))
  }, [])

  const selectedTemplate = templates.find(t => t.id === selectedTemplateId) ?? null

  const grouped = templates.reduce<Record<string, AppTemplate[]>>((acc, t) => {
    if (!acc[t.category]) acc[t.category] = []
    acc[t.category].push(t)
    return acc
  }, {})

  async function handleCreate() {
    setSubmitError('')
    setSubmitting(true)
    try {
      const config: Record<string, unknown> = {}
      if (baseUrl.trim()) config.base_url = baseUrl.trim()
      if (apiUrl.trim())  config.api_url  = apiUrl.trim()
      if (apiKey.trim())  config.api_key  = apiKey.trim()
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
    setBaseUrl(''); setApiUrl(''); setApiKey(''); setRateLimit('0')
    setCreatedApp(null); setCopied(false); setSubmitError('')
  }

  const panelTitle =
    step === 'done' ? '✓ App Created' :
    step === 'config' ? `Configure${selectedTemplate ? ` ${selectedTemplate.name}` : ' App'}` :
    'New App'

  const panelSubtitle =
    step === 'done' ? "Copy the webhook URL and paste it into your app's notification settings" :
    step === 'config' ? (selectedTemplate?.description ?? 'Set optional details for this app connection') :
    'Name your connection and pick an app template'

  const panelFooter =
    step === 'setup' ? (
      <button
        className="sp-btn sp-btn--primary"
        onClick={() => setStep('config')}
        disabled={!appName.trim()}
      >
        Next →
      </button>
    ) : step === 'config' ? (
      <>
        <button className="sp-btn sp-btn--secondary" onClick={() => setStep('setup')}>
          ← Back
        </button>
        <button
          className="sp-btn sp-btn--primary"
          onClick={() => void handleCreate()}
          disabled={submitting}
        >
          {submitting ? 'Creating…' : 'Create App'}
        </button>
      </>
    ) : createdApp ? (
      <>
        <button className="sp-btn sp-btn--secondary" onClick={handleAddAnother}>
          + Add Another
        </button>
        <button
          className="sp-btn sp-btn--primary"
          onClick={() => navigate(`/apps/${createdApp.id}`)}
        >
          View App →
        </button>
      </>
    ) : null

  return (
    <SlidePanel
      open={open}
      onClose={onClose}
      title={panelTitle}
      subtitle={panelSubtitle}
      footer={panelFooter ?? undefined}
    >
      {step === 'setup' && (
        <>
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
                  {[...items].sort((a, b) => a.name.localeCompare(b.name)).map(t => (
                    <option key={t.id} value={t.id}>
                      {t.name} — {CAPABILITY_LABEL[t.capability] ?? t.capability}
                    </option>
                  ))}
                </optgroup>
              ))
            )}
          </select>
        </>
      )}

      {step === 'config' && (
        <>
          <label className="modal-label">
            App URL <span className="modal-hint">(optional — enables the Launch button)</span>
          </label>
          <input className="modal-input" placeholder="https://sonarr.yourdomain.com"
            value={baseUrl} onChange={e => setBaseUrl(e.target.value)} />

          <label className="modal-label" style={{ marginTop: 16 }}>
            API URL <span className="modal-hint">(optional — overrides App URL for API polling)</span>
          </label>
          <input className="modal-input" placeholder="http://sonarr:8989"
            value={apiUrl} onChange={e => setApiUrl(e.target.value)} />

          <label className="modal-label" style={{ marginTop: 16 }}>
            API Key <span className="modal-hint">(optional — used for API polling widgets)</span>
          </label>
          <input className="modal-input modal-input-mono" placeholder="your-api-key"
            type="password" autoComplete="new-password"
            value={apiKey} onChange={e => setApiKey(e.target.value)} />

          <label className="modal-label" style={{ marginTop: 16 }}>
            Rate limit <span className="modal-hint">(events / minute, 0 = unlimited)</span>
          </label>
          <input className="modal-input modal-input-sm" type="number" min="0"
            value={rateLimit} onChange={e => setRateLimit(e.target.value)} />

          {submitError && <div className="modal-error">{submitError}</div>}
        </>
      )}

      {step === 'done' && createdApp && (
        <>
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
        </>
      )}
    </SlidePanel>
  )
}

// ── Apps Page ─────────────────────────────────────────────────────────────────

export function Apps() {
  const navigate = useNavigate()
  const [appList, setAppList] = useState<App[]>([])
  const [statusMap, setStatusMap] = useState<Record<string, 'online' | 'warn' | 'down'>>({})
  const [loading, setLoading] = useState(true)
  const [showAdd, setShowAdd] = useState(false)

  const { tick } = useAutoRefresh()
  const [addKey, setAddKey] = useState(0)
  const [editingApp, setEditingApp] = useState<App | null>(null)
  const [editingKey, setEditingKey] = useState(0)
  const [showEditPanel, setShowEditPanel] = useState(false)

  useEffect(() => {
    appsApi.list()
      .then(res => setAppList(res.data))
      .catch(console.error)
      .finally(() => setLoading(false))
    dashboardApi.summary()
      .then(res => {
        const map: Record<string, 'online' | 'warn' | 'down'> = {}
        for (const a of res.apps) map[a.id] = a.status
        setStatusMap(map)
      })
      .catch(() => { /* status is best-effort */ })
  }, [tick])

  return (
    <>
      <Topbar title="Apps" />
      <div className="content">
        <div className="section-header">
          <span className="section-title">Configured Apps</span>
          <button className="section-action" onClick={() => { setAddKey(k => k + 1); setShowAdd(true) }}>+ Add app</button>
        </div>

        <div className="apps-page-grid widget-grid">
          {loading ? (
            [0, 1, 2].map(i => (
              <div key={i} className="app-widget skeleton" style={{ height: 100 }} />
            ))
          ) : appList.length === 0 ? (
            <div className="apps-empty">
              No apps configured yet.{' '}
              <button className="apps-empty-link" onClick={() => { setAddKey(k => k + 1); setShowAdd(true) }}>
                Add your first app →
              </button>
            </div>
          ) : (
            appList.map(app => {
              const status = statusMap[app.id]
              const baseUrl = app.config?.base_url as string | undefined
              return (
              <div
                key={app.id}
                className={`app-widget${status === 'warn' ? ' warn' : status === 'down' ? ' down' : ''}`}
                onClick={() => navigate(`/apps/${app.id}`)}
              >
                <div className="app-widget-header">
                  <div className="app-icon"><AppIcon name={app.name} profileId={app.profile_id} /></div>
                  <div className="app-name">{app.name}</div>
                  {status && <span className={`app-status-dot ${status}`} />}
                </div>
                {baseUrl && (
                  <div className="app-url">{baseUrl}</div>
                )}
                {app.profile_id && (
                  <div className="app-profile-badge">{app.profile_id}</div>
                )}
                <div className="app-last-event">
                  Added {formatDate(app.created_at)}
                </div>
                <button
                  className="app-card-menu-btn"
                  title="Settings"
                  onClick={e => {
                    e.stopPropagation()
                    setEditingApp(app)
                    setEditingKey(k => k + 1)
                    setShowEditPanel(true)
                  }}
                >
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="13" height="13">
                    <circle cx="12" cy="12" r="3" />
                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
                  </svg>
                </button>
              </div>
              )
            })
          )}
        </div>
      </div>

      <AddAppModal
        key={addKey}
        open={showAdd}
        onClose={() => setShowAdd(false)}
        onCreated={app => {
          setAppList(prev => [...prev, app])
          // Panel stays open to show the done step with webhook URL
        }}
      />

      {editingApp && (
        <AppSettingsModal
          key={editingKey}
          open={showEditPanel}
          app={editingApp}
          onClose={() => setShowEditPanel(false)}
          onUpdated={updated => setAppList(prev => prev.map(a => a.id === updated.id ? updated : a))}
          onDeleted={() => {
            setAppList(prev => prev.filter(a => a.id !== editingApp.id))
            setShowEditPanel(false)
          }}
        />
      )}
    </>
  )
}
