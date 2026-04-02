import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { SSLRow } from '../components/SSLRow'
import { checks as checksApi, integrations as integrationsApi } from '../api/client'
import type { MonitorCheck, SSLCert, InfraIntegration, TraefikCert } from '../api/types'
import { CheckForm } from '../components/CheckForm'
import {
  type FormFields,
  defaultForm,
  validateForm,
  formToInput,
} from '../components/checkFormHelpers'
import { formatEventTime } from '../utils/formatTime'
import '../styles/Modal.css'
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
        const domain = c.ssl_source === 'traefik'
          ? c.target
          : c.target.replace(/^https?:\/\//, '').split('/')[0]
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

// ── Check type icons ─────────────────────────────────────────────────────────

function CheckTypeIcon({ type }: { type: string }) {
  const s = { width: 15, height: 15, fill: 'none', stroke: 'currentColor', strokeWidth: 1.5, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const, flexShrink: 0 as const, opacity: 0.55 }
  switch (type) {
    case 'url':
      return (
        <svg viewBox="0 0 16 16" style={s}>
          <circle cx="8" cy="8" r="6.5" />
          <path d="M8 1.5S5.5 4 5.5 8 8 14.5 8 14.5M8 1.5S10.5 4 10.5 8 8 14.5 8 14.5" />
          <path d="M1.5 8h13" />
        </svg>
      )
    case 'ssl':
      return (
        <svg viewBox="0 0 16 16" style={s}>
          <rect x="3" y="7" width="10" height="7.5" rx="1.5" />
          <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" />
          <circle cx="8" cy="10.5" r="1" fill="currentColor" stroke="none" opacity="1" />
        </svg>
      )
    case 'dns':
      return (
        <svg viewBox="0 0 16 16" style={s}>
          <path d="M8 2v12M5 5l3-3 3 3M5 11l3 3 3-3" />
          <path d="M2 8h3M11 8h3" />
        </svg>
      )
    case 'ping':
      return (
        <svg viewBox="0 0 16 16" style={s}>
          <path d="M1 9h2.5L5 5l3 8 2-5 1.5 2.5H15" />
        </svg>
      )
    default:
      return null
  }
}

// ── Check card ────────────────────────────────────────────────────────────────

interface CheckCardProps {
  check: MonitorCheck
  runningIds: Set<string>
  onToggleEnabled: () => void
  onRun: () => void
  onClick: () => void
}

function CheckCard({ check, runningIds, onToggleEnabled, onRun, onClick }: CheckCardProps) {
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
        {!check.source_component_id && check.ssl_source === 'traefik' && <span className="check-card-target-tag">Traefik</span>}
        {check.skip_tls_verify && <span className="check-card-target-tag warn">self-signed</span>}
      </div>

      {/* Footer: interval · last checked · run · pause · status · type */}
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
          <div className={statusClass(check.last_status)}>
            {statusLabel(check)}
          </div>
          <span className={`check-type-badge check-type-${check.type}`}>
            {check.type.toUpperCase()}
          </span>
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

  const [traefikIntegrations, setTraefikIntegrations] = useState<InfraIntegration[]>([])
  const [traefikCerts, setTraefikCerts] = useState<TraefikCert[]>([])

  // Add form
  const [showAddForm, setShowAddForm] = useState(false)
  const [addForm, setAddForm] = useState<FormFields>(defaultForm)
  const [addError, setAddError] = useState<string | null>(null)
  const [addSubmitting, setAddSubmitting] = useState(false)

  // Action state
  const [runningIds, setRunningIds] = useState<Set<string>>(new Set())

  function closeAddForm() {
    setShowAddForm(false)
    setAddForm(defaultForm)
    setAddError(null)
  }

  useEffect(() => {
    checksApi.list()
      .then(res => setCheckList(res.data))
      .catch(() => {})
      .finally(() => setLoading(false))

    integrationsApi.list()
      .then(res => {
        const traefik = res.data.filter(i => i.type === 'traefik' && i.enabled)
        setTraefikIntegrations(traefik)
        if (traefik.length > 0) {
          return integrationsApi.certs(traefik[0].id)
            .then(certsRes => setTraefikCerts(certsRes.data))
            .catch(() => {})
        }
      })
      .catch(() => {})
  }, [tick])

  useEffect(() => {
    if (!showAddForm) return
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') closeAddForm() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [showAddForm])

  const sslCerts = extractSSLCerts(checkList)

  function handleIntegrationChange(integrationId: string) {
    if (!integrationId) return
    integrationsApi.certs(integrationId)
      .then(res => setTraefikCerts(res.data))
      .catch(() => {})
  }

  // ── Add form ──

  function handleAddChange(field: keyof FormFields, value: string) {
    setAddForm(prev => {
      const next = { ...prev, [field]: value }
      if (field === 'ssl_source' && value === 'traefik' && traefikIntegrations.length > 0 && !next.integration_id) {
        next.integration_id = traefikIntegrations[0].id
      }
      return next
    })
    setAddError(null)
  }

  async function handleAddSubmit() {
    const err = validateForm(addForm)
    if (err) { setAddError(err); return }
    setAddSubmitting(true)
    try {
      const integrationId = addForm.ssl_source === 'traefik' ? addForm.integration_id : undefined
      const created = await checksApi.create(formToInput(addForm, integrationId))
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

        {showAddForm && (
          <div className="modal-backdrop">
            <div className="modal" style={{ width: 560 }}>
              <CheckForm
                form={addForm}
                onChange={handleAddChange}
                onSubmit={handleAddSubmit}
                onCancel={closeAddForm}
                error={addError}
                submitting={addSubmitting}
                title="New Check"
                submitLabel="Add Check"
                traefikIntegrations={traefikIntegrations}
                traefikCerts={traefikCerts}
                onIntegrationChange={handleIntegrationChange}
              />
            </div>
          </div>
        )}

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
    </>
  )
}
