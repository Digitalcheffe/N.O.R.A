import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { DetailPageLayout } from '../components/DetailPageLayout'
import { apps as appsApi, dashboard as dashboardApi, appTemplates as templatesApi, checks as checksApi, events as eventsApi, infrastructure as infraApi } from '../api/client'
import type { App, AppAuthType, AppChainResponse, AppMetricSnapshot, AppSummary, AppTemplate, MonitorCheck } from '../api/types'
import { AppChain } from '../components/AppChain'
import { AppMetricCard, AppMetricCardSkeleton } from '../components/AppMetricCard'
import { CheckTypeIcon } from '../components/CheckTypeIcon'
import { CheckForm } from '../components/CheckForm'
import { SlidePanel } from '../components/SlidePanel'
import {
  type FormFields,
  defaultForm,
  validateForm,
  formToInput,
} from '../components/checkFormHelpers'
import '../components/AppMetricsGrid.css'
import './AppDetail.css'

type TimeFilter = 'day' | 'week' | 'month'


// ── App Settings Modal ────────────────────────────────────────────────────────

export interface AppSettingsModalProps {
  open: boolean
  app: App
  onClose: () => void
  onUpdated: (app: App) => void
  onDeleted: () => void
}

const CAPABILITY_LABEL: Record<string, string> = {
  full:         'Webhook + API',
  webhook_only: 'Webhook',
  api_only:     'API',
  monitor_only: 'Monitor',
}

export function AppSettingsModal({ open, app, onClose, onUpdated, onDeleted }: AppSettingsModalProps) {
  const [name, setName] = useState(app.name)
  const [profileId, setProfileId] = useState(app.profile_id ?? '')
  const [baseUrl, setBaseUrl] = useState((app.config?.base_url as string) ?? '')
  const [apiUrl, setApiUrl] = useState((app.config?.api_url as string) ?? '')
  const [apiKey, setApiKey] = useState((app.config?.api_key as string) ?? '')
  const [showApiKey, setShowApiKey] = useState(false)
  const [authType, setAuthType] = useState<AppAuthType>(
    (app.config?.auth_type as AppAuthType) || 'none'
  )
  const [authHeader, setAuthHeader] = useState(
    (app.config?.auth_header as string) ?? ''
  )
  const [authEverSetByUser, setAuthEverSetByUser] = useState(
    Boolean(app.config?.auth_type) || Boolean(app.config?.auth_header)
  )
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
    templatesApi.list()
      .then(res => setTemplates(res.data))
      .catch(() => {})
  }, [])

  // Pull auth defaults from the selected profile so built-in apps arrive with
  // auth_type/auth_header pre-populated. User-set values always win.
  useEffect(() => {
    if (!profileId) return
    templatesApi.get(profileId)
      .then(t => {
        if (authEverSetByUser) return
        if (t.auth_type) setAuthType(t.auth_type)
        if (t.auth_header) setAuthHeader(t.auth_header)
      })
      .catch(() => {})
  }, [profileId, authEverSetByUser])

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
      if (apiUrl.trim()) config.api_url = apiUrl.trim()
      else delete config.api_url
      if (apiKey.trim()) config.api_key = apiKey.trim()
      else delete config.api_key
      if (authType && authType !== 'none') config.auth_type = authType
      else delete config.auth_type
      if ((authType === 'apikey_header' || authType === 'apikey_query') && authHeader.trim()) {
        config.auth_header = authHeader.trim()
      } else {
        delete config.auth_header
      }

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
    <SlidePanel
      open={open}
      onClose={onClose}
      title="App Settings"
      subtitle={app.name}
      footer={
        <button
          className="sp-btn sp-btn--primary"
          onClick={() => void handleSave()}
          disabled={saving || !name.trim()}
        >
          {saveOk ? '✓ Saved' : saving ? 'Saving…' : 'Save Changes'}
        </button>
      }
    >
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
        API URL <span className="modal-hint">(optional — overrides App URL for API polling)</span>
      </label>
      <input className="modal-input" placeholder="http://app:8989"
        value={apiUrl} onChange={e => setApiUrl(e.target.value)} />

      {/* ── API Polling Auth ── */}
      <div className="app-auth-block">
        <div className="app-auth-title">API Polling Auth</div>

        <label className="modal-label">
          Auth Type <span className="modal-hint">(how to send credentials)</span>
        </label>
        <select
          className="modal-input"
          value={authType}
          onChange={e => {
            setAuthType(e.target.value as AppAuthType)
            setAuthEverSetByUser(true)
          }}
        >
          <option value="none">None</option>
          <option value="apikey_header">API Key — Header</option>
          <option value="apikey_query">API Key — Query param</option>
          <option value="bearer">Bearer Token</option>
          <option value="basic">Basic (user:pass)</option>
        </select>

        {(authType === 'apikey_header' || authType === 'apikey_query') && (
          <>
            <label className="modal-label" style={{ marginTop: 12 }}>
              {authType === 'apikey_header' ? 'Header Name' : 'Query Parameter'}
            </label>
            <input
              className="modal-input modal-input-mono"
              placeholder={authType === 'apikey_header' ? 'X-Api-Key' : 'apikey'}
              value={authHeader}
              onChange={e => {
                setAuthHeader(e.target.value)
                setAuthEverSetByUser(true)
              }}
            />
          </>
        )}

        {authType !== 'none' && (
          <>
            <label className="modal-label" style={{ marginTop: 12 }}>
              {authType === 'basic' ? 'Credentials' : 'API Key'}
              <span className="modal-hint">
                {authType === 'basic' ? '(format: user:pass)' : '(stored encrypted at rest)'}
              </span>
            </label>
            <div className="app-auth-key-row">
              <input
                className="modal-input modal-input-mono"
                placeholder={authType === 'basic' ? 'user:pass' : 'your-api-key'}
                type={showApiKey ? 'text' : 'password'}
                autoComplete="new-password"
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
              />
              <button
                type="button"
                className="app-auth-show-btn"
                onClick={() => setShowApiKey(s => !s)}
              >
                {showApiKey ? 'hide' : 'show'}
              </button>
            </div>
          </>
        )}
      </div>

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
              Back
            </button>
            <button className="modal-btn-danger" style={{ fontSize: 12, padding: '5px 12px' }}
              onClick={() => void handleRegen()} disabled={regening}>
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
              Back
            </button>
            <button className="modal-btn-danger" style={{ flex: 1 }}
              onClick={() => void handleDelete()} disabled={deleting}>
              {deleting ? 'Deleting…' : 'Confirm Delete'}
            </button>
          </div>
        </div>
      )}
    </SlidePanel>
  )
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatIngestTime(dateStr: string): string {
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return ''
  const diff = Date.now() - d.getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'updated just now'
  if (mins < 60) return `updated ${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `updated ${hrs}h ago`
  return `updated ${Math.floor(hrs / 24)}d ago`
}

function statusColor(s: string | null) {
  if (s === 'up')   return 'var(--green)'
  if (s === 'warn') return 'var(--yellow)'
  if (s === 'down') return 'var(--red)'
  return 'var(--text3)'
}

// ── AppDetail ─────────────────────────────────────────────────────────────────

export function AppDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()

  const [timeFilter, setTimeFilter] = useState<TimeFilter>('week')

  const [app, setApp] = useState<App | null>(null)
  const [appSummary, setAppSummary] = useState<AppSummary | null>(null)
  const [appChecks, setAppChecks] = useState<MonitorCheck[]>([])
  const [appTemplate, setAppTemplate] = useState<AppTemplate | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  const [settingsKey, setSettingsKey] = useState(0)

  const [showAddCheck, setShowAddCheck] = useState(false)
  const [addCheckKey, setAddCheckKey] = useState(0)
  const [addCheckForm, setAddCheckForm] = useState<FormFields>({ ...defaultForm })
  const [addCheckError, setAddCheckError] = useState<string | null>(null)
  const [addCheckSubmitting, setAddCheckSubmitting] = useState(false)
  const [appMetrics, setAppMetrics] = useState<AppMetricSnapshot[]>([])
  const [metricsLoading, setMetricsLoading] = useState(true)
  const [polling, setPolling] = useState(false)
  const [pollError, setPollError] = useState<string | null>(null)
  const [appChain, setAppChain] = useState<AppChainResponse | null>(null)
  const [eventCounts, setEventCounts] = useState<Record<string, number>>({})

  const appId = id ?? ''

  useEffect(() => {
    if (!appId) return
    appsApi.get(appId).then(a => {
      setApp(a)
      if (a.profile_id) {
        templatesApi.get(a.profile_id)
          .then(setAppTemplate)
          .catch(() => {})
      }
    }).catch(console.error)
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
    if (!appId) return
    checksApi.list()
      .then(res => setAppChecks(res.data.filter(c => c.app_id === appId)))
      .catch(() => {})
  }, [appId, tick])

  useEffect(() => {
    if (!appId) return
    setMetricsLoading(true)
    appsApi.metrics(appId)
      .then(res => setAppMetrics(res.data))
      .catch(() => setAppMetrics([]))
      .finally(() => setMetricsLoading(false))
  }, [appId, tick])

  useEffect(() => {
    if (!appId) return
    appsApi.chain(appId)
      .then(setAppChain)
      .catch(() => {})
  }, [appId, tick])

  useEffect(() => {
    if (!appId) return
    const ms = timeFilter === 'day' ? 86400000 : timeFilter === 'month' ? 30 * 86400000 : 7 * 86400000
    const since = new Date(Date.now() - ms).toISOString()
    Promise.all(
      (['info', 'warn', 'error', 'critical'] as const).map(level =>
        eventsApi.list({ source_id: appId, source_type: 'app', level, from: since, limit: 1 })
          .then(res => [level, res.total] as const)
          .catch(() => [level, 0] as const)
      )
    ).then(results => {
      setEventCounts(Object.fromEntries(results))
    })
  }, [appId, timeFilter, tick])

  useEffect(() => {
    if (!id) navigate('/apps')
  }, [id, navigate])

  function openAddCheck() {
    setAddCheckForm({ ...defaultForm, app_id: appId })
    setAddCheckError(null)
    setAddCheckKey(k => k + 1)
    setShowAddCheck(true)
  }

  async function handleAddCheckSubmit() {
    const err = validateForm(addCheckForm)
    if (err) { setAddCheckError(err); return }
    setAddCheckSubmitting(true)
    try {
      const created = await checksApi.create(formToInput(addCheckForm))
      setAppChecks(prev => [...prev, created])
      setShowAddCheck(false)
      setAddCheckForm({ ...defaultForm })
    } catch (e: unknown) {
      setAddCheckError(e instanceof Error ? e.message : 'Failed to create check')
    } finally {
      setAddCheckSubmitting(false)
    }
  }

  async function handlePollNow() {
    setPolling(true)
    setPollError(null)
    try {
      const tasks: Promise<unknown>[] = []

      // 1. API polling — skip gracefully if the profile has no api_polling config
      tasks.push(
        appsApi.pollNow(appId).catch((err: unknown) => {
          const msg = err instanceof Error ? err.message : ''
          if (msg.includes('no api_polling') || msg.includes('no profile')) return
          throw err
        })
      )

      // 2. Scan each infra node in the chain (skip app node itself)
      const infraTypes = new Set([
        'portainer', 'docker_engine', 'vm_linux', 'vm_windows', 'vm_other',
        'proxmox_node', 'synology', 'linux_host', 'windows_host', 'generic_host', 'traefik',
      ])
      for (const node of appChain?.chain ?? []) {
        if (infraTypes.has(node.type)) {
          tasks.push(infraApi.scan(node.id).catch(() => {}))
        }
      }

      // 3. Run all linked monitor checks
      for (const check of appChecks) {
        tasks.push(checksApi.run(check.id).catch(() => {}))
      }

      await Promise.all(tasks)

      // Refresh metrics and checks after
      setMetricsLoading(true)
      const [metricsRes, checksRes] = await Promise.all([
        appsApi.metrics(appId).catch(() => ({ data: [] as typeof appMetrics })),
        checksApi.list().catch(() => ({ data: [] as MonitorCheck[] })),
      ])
      setAppMetrics(metricsRes.data)
      setAppChecks(checksRes.data.filter(c => c.app_id === appId))
    } catch (err: unknown) {
      setPollError(err instanceof Error ? err.message : 'Discover failed')
    } finally {
      setPolling(false)
      setMetricsLoading(false)
    }
  }

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

  const capability = appSummary?.capability
  const keyDataPoints = [
    { label: 'Profile', value: app?.profile_id || 'Generic' },
    ...(capability ? [{ label: 'Type', value: CAPABILITY_LABEL[capability] ?? capability }] : []),
    { label: 'Rate limit', value: app?.rate_limit ? `${app.rate_limit}/min` : 'Unlimited' },
    ...(baseUrl ? [{ label: 'URL', value: baseUrl }] : []),
  ]

  // Icon shown next to the name
  const appIcon = appSummary?.icon_url ? (
    <img
      src={appSummary.icon_url}
      alt={appName}
      onError={e => { (e.target as HTMLImageElement).style.display = 'none' }}
    />
  ) : undefined

  // Description + homepage shown below the KDP badges
  const AppExtraHeader = (appTemplate?.description || appTemplate?.homepage) ? (
    <div className="detail-app-icon-row">
      {appTemplate.description && (
        <span className="detail-app-description">{appTemplate.description}</span>
      )}
      {appTemplate.homepage && (
        <a className="detail-app-homepage" href={appTemplate.homepage} target="_blank" rel="noopener noreferrer">
          ↗ {appTemplate.homepage.replace(/^https?:\/\//, '')}
        </a>
      )}
    </div>
  ) : undefined

  return (
    <>
      <DetailPageLayout
        breadcrumb="Apps"
        breadcrumbPath="/apps"
        name={appName}
        status={{ status: dplStatus }}
        lastPolled={lastEvent ? `Last event: ${lastEvent}` : undefined}
        keyDataPoints={keyDataPoints}
        icon={appIcon}
        headerExtra={AppExtraHeader}
        timeFilter={timeFilter}
        onTimeFilter={setTimeFilter}
        actions={
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 6 }}>
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
              {app?.profile_id && (
                <button
                  className="detail-settings-btn"
                  onClick={() => void handlePollNow()}
                  disabled={polling}
                  title="Run API polling now"
                >
                  {polling ? 'Polling…' : 'Discover Now'}
                </button>
              )}
              <button className="detail-settings-btn" onClick={() => { setSettingsKey(k => k + 1); setShowSettings(true) }} title="App Settings">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <circle cx="12" cy="12" r="3" />
                  <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
                </svg>
                Settings
              </button>
            </div>
            {pollError && (
              <div style={{ fontSize: 11, color: 'var(--red)', textAlign: 'right' }}>{pollError}</div>
            )}
          </div>
        }
      >
        {/* ── Top row: events + metrics (left) | monitor checks (right) ── */}
        <div className="appdetail-two-col">

          <div className="appdetail-col-left">

            {/* Live metrics — driven entirely by dashboard/summary stats, which
                are gated by the digest_registry. Categories render as counts,
                api-source widgets render the latest snapshot value, webhook
                widgets render event counts. Legacy api_polling snapshots are
                no longer rendered independently — if a metric isn't in the
                registry it doesn't appear here. */}
            {(metricsLoading || (appSummary?.stats ?? []).length > 0) && (
              <div className="amg-section">
                {!metricsLoading && <div className="amg-label">Live Metrics</div>}
                <div className="amg-grid">
                  {metricsLoading ? (
                    [0, 1, 2].map(i => <AppMetricCardSkeleton key={i} />)
                  ) : (
                    (appSummary?.stats ?? []).map(stat => (
                      <div key={`stat-${stat.label}`} className="amc-card">
                        <div className={`amc-value${stat.color ? ` color-${stat.color}` : ''}`}>{stat.value}</div>
                        <div className="amc-label">{stat.label}</div>
                        {appSummary?.last_event_at && (
                          <div className="amc-timestamp">{formatIngestTime(appSummary.last_event_at)}</div>
                        )}
                      </div>
                    ))
                  )}
                </div>
              </div>
            )}

            {/* Event summary */}
            <div className="amg-section">
              <div className="amg-label">Events ({timeFilter === 'day' ? 'last 24h' : timeFilter === 'month' ? 'last month' : 'last week'})</div>
              <div className="appdetail-check-list">
                {([
                  { level: 'info',     label: 'Info',     colorVar: 'var(--accent)',
                    icon: <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><circle cx="10" cy="10" r="8"/><path d="M10 9v5M10 7h.01"/></svg> },
                  { level: 'warn',     label: 'Warnings', colorVar: 'var(--yellow)',
                    icon: <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M10 3L18.5 17H1.5L10 3z"/><path d="M10 9v4M10 15h.01"/></svg> },
                  { level: 'error',    label: 'Errors',   colorVar: 'var(--red)',
                    icon: <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><circle cx="10" cy="10" r="8"/><path d="M7 7l6 6M13 7l-6 6"/></svg> },
                  { level: 'critical', label: 'Critical', colorVar: 'var(--red)',
                    icon: <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M10 2c0 4-4 5-4 9a4 4 0 0 0 8 0c0-4-4-5-4-9z"/><path d="M8 17.5a2 2 0 0 0 4 0"/></svg> },
                ] as const).map(({ level, label, colorVar, icon }) => (
                  <div
                    key={level}
                    className="appdetail-event-row"
                    onClick={() => navigate(`/events?source_type=app&source_id=${appId}&level=${level}`)}
                  >
                    <span className="appdetail-event-icon" style={{ color: colorVar }}>{icon}</span>
                    <span className="appdetail-event-level" style={{ color: colorVar }}>{label}</span>
                    <span className="appdetail-event-count" style={{ color: colorVar }}>
                      {eventCounts[level] ?? '—'}
                    </span>
                    <span className="appdetail-event-link">View →</span>
                  </div>
                ))}
              </div>
            </div>

            {/* Monitor checks as cards */}
            <div className="amg-section">
              <div className="appdetail-section-header">
                <span className="amg-label" style={{ marginBottom: 0 }}>Monitor Checks</span>
                <button className="service-add-check-btn" onClick={openAddCheck}>+ Add check</button>
              </div>
              {appChecks.length === 0 ? (
                <div className="service-checks-empty" style={{ background: 'none', border: 'none', padding: '8px 0', textAlign: 'left' }}>
                  No monitor checks linked to this app yet.
                </div>
              ) : (
                <div className="appdetail-check-list">
                  {appChecks.map(c => (
                    <div key={c.id} className="appdetail-check-row" onClick={() => navigate(`/checks/${c.id}`)}>
                      <CheckTypeIcon type={c.type} size={14} />
                      <span className="appdetail-check-name" title={c.name}>{c.name}</span>
                      <span className="appdetail-check-target" title={c.target}>{c.target}</span>
                      <span className="appdetail-check-status" style={{ color: statusColor(c.last_status) }}>
                        {c.last_status?.toUpperCase() ?? '—'}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>

          </div>

          {/* Right column: infra chain (vertical) only */}
          <div className="appdetail-col-right">
            {appChain && appChain.chain.length > 0 && (
              <AppChain
                chain={appChain.chain}
                appStatus={appSummary?.status}
                traefik={appChain.traefik ?? []}
                vertical
              />
            )}
          </div>

        </div>

      </DetailPageLayout>

      {app && (
        <AppSettingsModal
          key={settingsKey}
          open={showSettings}
          app={app}
          onClose={() => setShowSettings(false)}
          onUpdated={updated => { setApp(updated); setShowSettings(false) }}
          onDeleted={() => navigate('/apps')}
        />
      )}

      <SlidePanel
        key={addCheckKey}
        open={showAddCheck}
        onClose={() => setShowAddCheck(false)}
        title={`Add Check${app ? ` — ${app.name}` : ''}`}
        footer={
          <button
            className="sp-btn sp-btn--primary"
            onClick={() => void handleAddCheckSubmit()}
            disabled={addCheckSubmitting}
          >
            {addCheckSubmitting ? 'Saving…' : 'Add Check'}
          </button>
        }
      >
        <CheckForm
          form={addCheckForm}
          onChange={(field, value) => {
            setAddCheckForm(prev => ({ ...prev, [field]: value }))
            setAddCheckError(null)
          }}
          onSubmit={() => void handleAddCheckSubmit()}
          onCancel={() => setShowAddCheck(false)}
          error={addCheckError}
          submitting={addCheckSubmitting}
          title=""
          submitLabel="Add Check"
          hideActions
        />
      </SlidePanel>
    </>
  )
}
