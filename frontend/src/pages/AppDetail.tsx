import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { DetailPageLayout } from '../components/DetailPageLayout'
import { apps as appsApi, dashboard as dashboardApi, appTemplates as templatesApi } from '../api/client'
import type { App, AppSummary, AppTemplate } from '../api/types'
import '../styles/Modal.css'
import './AppDetail.css'

type TimeFilter = 'day' | 'week' | 'month'

// ── Sparkline ─────────────────────────────────────────────────────────────────

function Sparkline({ data, color = 'var(--accent)' }: { data: number[]; color?: string }) {
  if (!data || data.length < 2) return null
  const w = 80, h = 20
  const max = Math.max(...data, 1)
  const pts = data.map((v, i) => {
    const x = ((i / (data.length - 1)) * w).toFixed(1)
    const y = (h - 2 - (v / max) * (h - 4)).toFixed(1)
    return `${x},${y}`
  }).join(' ')
  const closed = `${pts} ${w},${h} 0,${h}`
  return (
    <svg className="detail-sparkline" viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none">
      <polyline points={pts} fill="none" stroke={color} strokeWidth="1.5" opacity="0.8" />
      <polyline points={closed} fill={color} stroke="none" opacity="0.08" />
    </svg>
  )
}

// ── App Settings Modal ────────────────────────────────────────────────────────

interface AppSettingsModalProps {
  app: App
  onClose: () => void
  onUpdated: (app: App) => void
  onDeleted: () => void
}

const CAPABILITY_LABEL: Record<string, string> = {
  full:         'Webhook + Monitor',
  webhook_only: 'Webhook',
  monitor_only: 'Monitor',
  docker_only:  'Docker',
  limited:      'Limited',
}

function AppSettingsModal({ app, onClose, onUpdated, onDeleted }: AppSettingsModalProps) {
  const [name, setName] = useState(app.name)
  const [profileId, setProfileId] = useState(app.profile_id ?? '')
  const [baseUrl, setBaseUrl] = useState((app.config?.base_url as string) ?? '')
  const [monitorUrl, setMonitorUrl] = useState((app.config?.monitor_url as string) ?? '')
  const [apiKey, setApiKey] = useState((app.config?.api_key as string) ?? '')
  const [rateLimit, setRateLimit] = useState(String(app.rate_limit ?? 0))

  const [templates, setTemplates] = useState<AppTemplate[]>([])

  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [saveOk, setSaveOk] = useState(false)

  const [copiedUrl, setCopiedUrl] = useState(false)
  const [regenConfirm, setRegenConfirm] = useState(false)
  const [regening, setRegening] = useState(false)
  const [currentToken, setCurrentToken] = useState(app.token)
  const [newTokenCopied, setNewTokenCopied] = useState(false)

  const [deleteConfirm, setDeleteConfirm] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')

  useEffect(() => {
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  useEffect(() => {
    templatesApi.list()
      .then(res => setTemplates(res.data))
      .catch(() => {})
  }, [])

  const grouped = templates.reduce<Record<string, AppTemplate[]>>((acc, t) => {
    if (!acc[t.category]) acc[t.category] = []
    acc[t.category].push(t)
    return acc
  }, {})

  function webhookUrl(token: string) {
    return `${window.location.origin}/api/v1/ingest/${token}`
  }

  async function handleSave() {
    if (!name.trim()) return
    setSaving(true); setSaveError(''); setSaveOk(false)
    try {
      const config: Record<string, unknown> = { ...app.config }
      if (baseUrl.trim()) config.base_url = baseUrl.trim()
      else delete config.base_url
      if (monitorUrl.trim()) config.monitor_url = monitorUrl.trim()
      else delete config.monitor_url
      if (apiKey.trim()) config.api_key = apiKey.trim()
      else delete config.api_key

      const updated = await appsApi.update(app.id, {
        name: name.trim(),
        profile_id: profileId,
        config,
        rate_limit: parseInt(rateLimit, 10) || 0,
      })
      onUpdated(updated)
      setSaveOk(true)
      setTimeout(() => setSaveOk(false), 2000)
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  async function handleRegen() {
    setRegening(true)
    try {
      const res = await appsApi.regenerateToken(app.id)
      setCurrentToken(res.token)
      setRegenConfirm(false)
      setNewTokenCopied(false)
    } catch (err: unknown) {
      console.error(err)
    } finally {
      setRegening(false)
    }
  }

  async function handleDelete() {
    setDeleting(true); setDeleteError('')
    try {
      await appsApi.delete(app.id)
      onDeleted()
    } catch (err: unknown) {
      setDeleteError(err instanceof Error ? err.message : 'Failed to delete')
      setDeleting(false)
    }
  }

  function copyUrl() {
    navigator.clipboard.writeText(webhookUrl(currentToken)).then(() => {
      setCopiedUrl(true)
      setTimeout(() => setCopiedUrl(false), 2000)
    })
  }

  function copyNewToken() {
    navigator.clipboard.writeText(webhookUrl(currentToken)).then(() => {
      setNewTokenCopied(true)
      setTimeout(() => setNewTokenCopied(false), 2000)
    })
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <div className="modal-title">App Settings</div>
          <div className="modal-subtitle">{app.name}</div>
          <button className="modal-close" onClick={onClose}>✕</button>
        </div>

        <div className="modal-body">

          {/* ── Basic settings ── */}
          <label className="modal-label">Name</label>
          <input className="modal-input" value={name} onChange={e => setName(e.target.value)} />

          <label className="modal-label" style={{ marginTop: 16 }}>
            App Template <span className="modal-hint">(controls field mapping and severity)</span>
          </label>
          <select
            className="modal-input"
            value={profileId}
            onChange={e => setProfileId(e.target.value)}
          >
            <option value="">Generic Webhook — raw JSON, no mapping</option>
            {Object.entries(grouped).sort(([a], [b]) => a.localeCompare(b)).map(([cat, items]) => (
              <optgroup key={cat} label={cat}>
                {items.map(t => (
                  <option key={t.id} value={t.id}>
                    {t.name} — {CAPABILITY_LABEL[t.capability] ?? t.capability}
                  </option>
                ))}
              </optgroup>
            ))}
          </select>

          <label className="modal-label" style={{ marginTop: 16 }}>
            App URL <span className="modal-hint">(optional — enables the Launch button)</span>
          </label>
          <input className="modal-input" placeholder="https://app.yourdomain.com"
            value={baseUrl} onChange={e => setBaseUrl(e.target.value)} />

          <label className="modal-label" style={{ marginTop: 16 }}>
            Monitor URL <span className="modal-hint">(optional — NORA pings this for uptime)</span>
          </label>
          <input className="modal-input" placeholder="https://app.yourdomain.com/ping"
            value={monitorUrl} onChange={e => setMonitorUrl(e.target.value)} />

          <label className="modal-label" style={{ marginTop: 16 }}>
            API Key <span className="modal-hint">(optional — used for active monitor auth)</span>
          </label>
          <input className="modal-input modal-input-mono" placeholder="your-api-key"
            type="password" autoComplete="new-password"
            value={apiKey} onChange={e => setApiKey(e.target.value)} />

          <label className="modal-label" style={{ marginTop: 16 }}>
            Rate limit <span className="modal-hint">(events / minute, 0 = unlimited)</span>
          </label>
          <input className="modal-input modal-input-sm" type="number" min="0"
            value={rateLimit} onChange={e => setRateLimit(e.target.value)} />

          {saveError && <div className="modal-error">{saveError}</div>}

          {/* ── Webhook URL ── */}
          <hr className="modal-section-divider" />

          <label className="modal-label">Webhook URL</label>
          <div className="webhook-url-row">
            <input className="modal-input modal-input-mono" readOnly
              value={webhookUrl(currentToken)} onFocus={e => e.target.select()} />
            <button className={`webhook-copy-btn${copiedUrl ? ' copied' : ''}`} onClick={copyUrl}>
              {copiedUrl ? '✓ Copied' : 'Copy'}
            </button>
          </div>

          {/* Regenerate token */}
          {!regenConfirm ? (
            <button className="detail-regen-btn" onClick={() => setRegenConfirm(true)}>
              Regenerate Token
            </button>
          ) : (
            <div className="detail-regen-confirm">
              <span className="detail-regen-warn">
                ⚠ This will invalidate the current token. Any app sending to the old URL will stop working.
              </span>
              <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                <button className="modal-btn-ghost" style={{ fontSize: 12, padding: '5px 12px' }}
                  onClick={() => setRegenConfirm(false)}>
                  Cancel
                </button>
                <button className="modal-btn-danger" style={{ fontSize: 12, padding: '5px 12px' }}
                  onClick={handleRegen} disabled={regening}>
                  {regening ? 'Regenerating…' : 'Yes, regenerate'}
                </button>
              </div>
              {newTokenCopied && (
                <div style={{ marginTop: 8 }}>
                  <label className="modal-label">New Webhook URL</label>
                  <div className="webhook-url-row">
                    <input className="modal-input modal-input-mono" readOnly
                      value={webhookUrl(currentToken)} onFocus={e => e.target.select()} />
                    <button className={`webhook-copy-btn${newTokenCopied ? ' copied' : ''}`} onClick={copyNewToken}>
                      {newTokenCopied ? '✓ Copied' : 'Copy'}
                    </button>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* ── Danger zone ── */}
          <hr className="modal-section-divider" />
          <div className="modal-danger-label">Danger Zone</div>

          {!deleteConfirm ? (
            <button className="modal-btn-danger" style={{ width: '100%' }}
              onClick={() => setDeleteConfirm(true)}>
              Delete App
            </button>
          ) : (
            <div className="detail-delete-confirm">
              <p className="detail-delete-warn">
                Permanently delete <strong>{app.name}</strong> and all its events, metrics, and monitor checks? This cannot be undone.
              </p>
              {deleteError && <div className="modal-error">{deleteError}</div>}
              <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
                <button className="modal-btn-ghost" style={{ flex: 1 }}
                  onClick={() => { setDeleteConfirm(false); setDeleteError('') }}>
                  Cancel
                </button>
                <button className="modal-btn-danger" style={{ flex: 1 }}
                  onClick={handleDelete} disabled={deleting}>
                  {deleting ? 'Deleting…' : 'Confirm Delete'}
                </button>
              </div>
            </div>
          )}

        </div>

        <div className="modal-footer">
          <button className="modal-btn-ghost" onClick={onClose}>Cancel</button>
          <button className="modal-btn-primary" onClick={handleSave} disabled={saving || !name.trim()}>
            {saveOk ? '✓ Saved' : saving ? 'Saving…' : 'Save Changes'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── AppDetail ─────────────────────────────────────────────────────────────────

export function AppDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()

  const timeFilter: TimeFilter = 'week'

  const [app, setApp] = useState<App | null>(null)
  const [appSummary, setAppSummary] = useState<AppSummary | null>(null)
  const [showSettings, setShowSettings] = useState(false)

  const appId = id ?? ''

  useEffect(() => {
    if (!appId) return
    appsApi.get(appId).then(setApp).catch(console.error)
  }, [appId, tick])

  useEffect(() => {
    if (!appId) return
    dashboardApi.summary(timeFilter)
      .then(res => {
        setAppSummary(res.apps.find(a => a.id === appId) ?? null)
      })
      .catch(console.error)
  }, [appId, timeFilter, tick])

  useEffect(() => {
    if (!id) navigate('/apps')
  }, [id, navigate])

  if (!id) return null

  const appName = app?.name ?? appId
  const baseUrl = app?.config?.base_url as string | undefined
  const rawStatus = appSummary?.status ?? 'online'
  const dplStatus: 'online' | 'offline' | 'unknown' | 'warning' =
    rawStatus === 'online' ? 'online' : rawStatus === 'down' ? 'offline' : 'warning'
  const lastEvent = appSummary?.last_event_at
    ? new Date(appSummary.last_event_at).toLocaleString('en-US', {
        month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', hour12: true,
      })
    : null

  const keyDataPoints = [
    { label: 'Profile', value: app?.profile_id || 'Generic' },
    { label: 'Rate limit', value: app?.rate_limit ? `${app.rate_limit}/min` : 'Unlimited' },
    ...(baseUrl ? [{ label: 'URL', value: baseUrl }] : []),
  ]

  return (
    <>
      <DetailPageLayout
        breadcrumb="Apps"
        breadcrumbPath="/apps"
        name={appName}
        status={{ status: dplStatus }}
        lastPolled={lastEvent ? `Last event: ${lastEvent}` : undefined}
        keyDataPoints={keyDataPoints}
        actions={
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            {baseUrl && (
              <a className="detail-launch-btn" href={baseUrl} target="_blank" rel="noopener noreferrer">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                  <polyline points="15 3 21 3 21 9" />
                  <line x1="10" y1="14" x2="21" y2="3" />
                </svg>
                Launch
              </a>
            )}
            <button className="detail-settings-btn" onClick={() => setShowSettings(true)} title="App Settings">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="12" cy="12" r="3" />
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
              </svg>
              Settings
            </button>
          </div>
        }
        sourceType="app"
        sourceId={appId}
      >
        {/* ── Stats row ── */}
        {appSummary && (
          <div className="detail-stats-row">
            {(appSummary.stats ?? []).map(stat => (
              <div key={stat.label} className="detail-stat-card">
                <div className="detail-stat-label">{stat.label}</div>
                <div className={`detail-stat-value${stat.color ? ` color-${stat.color}` : ''}`}>
                  {stat.value}
                </div>
              </div>
            ))}
            {appSummary.sparkline.some(v => v > 0) && (
              <div className="detail-stat-card detail-stat-sparkline-card">
                <div className="detail-stat-label">Activity</div>
                <Sparkline data={Array.from(appSummary.sparkline)} />
              </div>
            )}
          </div>
        )}
      </DetailPageLayout>

      {showSettings && app && (
        <AppSettingsModal
          app={app}
          onClose={() => setShowSettings(false)}
          onUpdated={updated => { setApp(updated); setShowSettings(false) }}
          onDeleted={() => navigate('/apps')}
        />
      )}
    </>
  )
}
