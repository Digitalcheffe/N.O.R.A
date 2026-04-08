import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { SSLRow } from '../components/SSLRow'
import { checks as checksApi, apps as appsApi, appTemplates as appTemplatesApi } from '../api/client'
import type { App, MonitorCheck, SSLCert } from '../api/types'
import { CheckForm } from '../components/CheckForm'
import { SlidePanel } from '../components/SlidePanel'
import {
  type FormFields,
  defaultForm,
  validateForm,
  formToInput,
} from '../components/checkFormHelpers'
import { formatEventTime } from '../utils/formatTime'
import './Checks.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function sslDaysFromResult(lastResult: string | null): number | null {
  if (!lastResult) return null
  try {
    const r = JSON.parse(lastResult) as { days_remaining?: number }
    return r.days_remaining ?? null
  } catch {
    return null
  }
}

function statusLabel(check: MonitorCheck): string {
  if (check.type === 'ssl') {
    const days = sslDaysFromResult(check.last_result)
    if (days != null) return `${days}d`
    return 'SSL'
  }
  if (check.last_status === 'up') return 'UP'
  if (check.last_status === 'down') return 'DOWN'
  if (check.last_status === 'warn') return 'WARN'
  return '—'
}

function statusClass(status: string | null): string {
  if (status === 'up') return 'monitor-status-block up'
  if (status === 'warn') return 'monitor-status-block warn'
  if (status === 'down' || status === 'critical') return 'monitor-status-block down'
  return 'monitor-status-block unknown'
}

function extractSSLCerts(checkList: MonitorCheck[]): SSLCert[] {
  return checkList
    .filter(c => c.type === 'ssl' && c.last_result)
    .flatMap(c => {
      try {
        const result = JSON.parse(c.last_result!) as { days_remaining?: number; expires_at?: string }
        if (result.days_remaining == null) return []
        const domain = c.target.replace(/^https?:\/\//, '').split('/')[0]
        const expiresAt = result.expires_at
          ? new Date(result.expires_at).toISOString().split('T')[0]
          : ''
        return [{
          domain,
          days_remaining: result.days_remaining,
          expires_at: expiresAt,
          status: c.last_status ?? 'unknown',
        } as SSLCert]
      } catch {
        return []
      }
    })
    .sort((a, b) => a.days_remaining - b.days_remaining)
}

import { CheckTypeIcon } from '../components/CheckTypeIcon'

// ── Check card ────────────────────────────────────────────────────────────────

interface CheckCardProps {
  check: MonitorCheck
  runningIds: Set<string>
  onToggleEnabled: () => void
  onRun: () => void
  onClick: () => void
  onSettings: () => void
}

function CheckCard({ check, runningIds, onToggleEnabled, onRun, onClick, onSettings }: CheckCardProps) {
  const disabled = !check.enabled

  return (
    <div className={`check-card${disabled ? ' disabled' : ''}`} onClick={onClick}>

      {/* Top row: type icon + name */}
      <div className="check-card-top">
        <CheckTypeIcon type={check.type} />
        <span className="check-card-name">{check.name}</span>
        {disabled && <span className="check-paused-badge">PAUSED</span>}
      </div>

      {/* Target */}
      <div className="check-card-target" title={check.target}>
        {check.target}
        {check.source_component_id && <span className="check-card-target-tag traefik">Traefik</span>}
        {check.skip_tls_verify && <span className="check-card-target-tag warn">self-signed</span>}
      </div>

      {/* Status + type row (between target and buttons) */}
      <div className="check-card-status-row">
        <div className={statusClass(check.last_status)}>
          {statusLabel(check)}
        </div>
        <span className={`check-type-badge check-type-${check.type}`}>
          {check.type.toUpperCase()}
        </span>
      </div>

      {/* Footer: interval · last checked · run · pause · settings */}
      <div className="check-card-footer">
        <span className="check-card-interval">every {check.interval_secs}s</span>
        <span className="check-card-last">{formatEventTime(check.last_checked_at)}</span>
        <div className="check-card-footer-actions">
          <button
            className={`check-run-btn${runningIds.has(check.id) ? ' running' : ''}`}
            title="Run now"
            onClick={e => { e.stopPropagation(); onRun() }}
            disabled={runningIds.has(check.id)}
          >
            {runningIds.has(check.id) ? <span className="check-spinner" /> : '▶'}
          </button>
          <button
            className={`check-toggle-btn${check.enabled ? ' enabled' : ' paused'}`}
            title={check.enabled ? 'Pause check' : 'Resume check'}
            onClick={e => { e.stopPropagation(); onToggleEnabled() }}
          >
            {check.enabled ? '⏸' : '▶'}
          </button>
          <button
            className="check-settings-btn"
            title="Settings"
            onClick={e => { e.stopPropagation(); onSettings() }}
          >
            ⚙
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function Checks() {
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()
  const [checkList, setCheckList] = useState<MonitorCheck[]>([])
  const [loading, setLoading] = useState(true)

  const [appList, setAppList] = useState<App[]>([])

  // Add form
  const [showAddForm, setShowAddForm] = useState(false)
  const [addForm, setAddForm] = useState<FormFields>(defaultForm)
  const [addError, setAddError] = useState<string | null>(null)
  const [addSubmitting, setAddSubmitting] = useState(false)
  const [targetSuggestion, setTargetSuggestion] = useState<string>('')

  // Action state
  const [runningIds, setRunningIds] = useState<Set<string>>(new Set())

  function closeAddForm() {
    setShowAddForm(false)
    setAddForm(defaultForm)
    setAddError(null)
    setTargetSuggestion('')
  }

  useEffect(() => {
    checksApi.list()
      .then(res => setCheckList(res.data))
      .catch(() => {})
      .finally(() => setLoading(false))

    appsApi.list()
      .then(res => setAppList(res.data))
      .catch(() => {})
  }, [tick])

  const sslCerts = extractSSLCerts(checkList)

  // ── Add form ──

  function handleAddChange(field: keyof FormFields, value: string) {
    setAddForm(prev => ({ ...prev, [field]: value }))
    setAddError(null)

    if (field === 'app_id') {
      setTargetSuggestion('')
      if (!value) return
      const app = appList.find(a => a.id === value)
      if (!app?.profile_id) return
      appTemplatesApi.get(app.profile_id)
        .then(tmpl => {
          if (!tmpl.monitor?.check_url) return
          const baseUrl = typeof app.config?.base_url === 'string' ? app.config.base_url : ''
          const resolved = baseUrl
            ? tmpl.monitor.check_url.replace('{base_url}', baseUrl)
            : tmpl.monitor.check_url
          setTargetSuggestion(resolved)
        })
        .catch(() => {})
    }
  }

  async function handleAddSubmit() {
    const err = validateForm(addForm)
    if (err) { setAddError(err); return }
    setAddSubmitting(true)
    try {
      const created = await checksApi.create(formToInput(addForm))
      setCheckList(prev => [created, ...prev])
      setShowAddForm(false)
      setAddForm(defaultForm)
    } catch (e: unknown) {
      setAddError(e instanceof Error ? e.message : 'Failed to create check')
    } finally {
      setAddSubmitting(false)
    }
  }

  // ── Toggle enabled ──

  async function handleToggleEnabled(check: MonitorCheck) {
    const newEnabled = !check.enabled
    setCheckList(prev => prev.map(c => c.id === check.id ? { ...c, enabled: newEnabled } : c))
    try {
      const updated = await checksApi.update(check.id, { enabled: newEnabled })
      setCheckList(prev => prev.map(c => c.id === check.id ? updated : c))
    } catch {
      setCheckList(prev => prev.map(c => c.id === check.id ? check : c))
    }
  }

  // ── Run ──

  async function handleRun(id: string) {
    setRunningIds(prev => new Set(prev).add(id))
    try {
      await checksApi.run(id)
      const updated = await checksApi.get(id)
      setCheckList(prev => prev.map(c => c.id === id ? updated : c))
    } catch {
      // noop
    } finally {
      setRunningIds(prev => { const next = new Set(prev); next.delete(id); return next })
    }
  }

  return (
    <>
      <Topbar title="Monitor Checks" />
      <div className="content">

        <div className="section-header">
          <span className="section-title">Active Checks</span>
          <button className="section-action" onClick={() => setShowAddForm(prev => !prev)}>
            + Add check
          </button>
        </div>

        {loading ? (
          <div className="checks-empty"><span>Loading…</span></div>
        ) : checkList.length === 0 ? (
          <div className="checks-empty"><span>No monitor checks configured yet.</span></div>
        ) : (
          <div className="checks-grid">
            {checkList.map(check => (
              <CheckCard
                key={check.id}
                check={check}
                runningIds={runningIds}
                onToggleEnabled={() => void handleToggleEnabled(check)}
                onRun={() => void handleRun(check.id)}
                onClick={() => navigate(`/checks/${check.id}`)}
                onSettings={() => navigate(`/checks/${check.id}`)}
              />
            ))}
          </div>
        )}

        {sslCerts.length > 0 && (
          <>
            <div className="section-header" style={{ marginTop: 24 }}>
              <span className="section-title">SSL Certificates</span>
            </div>
            <div className="ssl-panel">
              {sslCerts.map(cert => (
                <SSLRow key={cert.domain} cert={cert} />
              ))}
            </div>
          </>
        )}

      </div>

      <SlidePanel
        open={showAddForm}
        onClose={closeAddForm}
        title="Add Check"
        footer={
          <button
            className="sp-btn sp-btn--primary"
            onClick={() => void handleAddSubmit()}
            disabled={addSubmitting}
          >
            {addSubmitting ? 'Saving…' : 'Add Check'}
          </button>
        }
      >
        <CheckForm
          form={addForm}
          onChange={handleAddChange}
          onSubmit={handleAddSubmit}
          onCancel={closeAddForm}
          error={addError}
          submitting={addSubmitting}
          title=""
          submitLabel="Add Check"
          apps={appList.map(a => ({ id: a.id, name: a.name }))}
          targetSuggestion={targetSuggestion}
          hideActions
        />
      </SlidePanel>
    </>
  )
}
