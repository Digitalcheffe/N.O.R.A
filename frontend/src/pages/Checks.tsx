import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { SSLRow } from '../components/SSLRow'
import { checks as checksApi, integrations as integrationsApi } from '../api/client'
import type { MonitorCheck, SSLCert, InfraIntegration, TraefikCert } from '../api/types'
import {
  CheckForm,
  type FormFields,
  defaultForm,
  validateForm,
  formToInput,
} from '../components/CheckForm'
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

function formatTimeAgo(iso?: string | null): string {
  if (!iso) return '—'
  const diffMs = Date.now() - new Date(iso).getTime()
  const mins = Math.floor(diffMs / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
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

      {/* Top row: status block + type badge */}
      <div className="check-card-top">
        <div className={statusClass(check.last_status)}>
          {statusLabel(check)}
        </div>
        <span className={`check-type-badge check-type-${check.type}`}>
          {check.type.toUpperCase()}
        </span>
        {disabled && <span className="check-paused-badge">PAUSED</span>}
        <button
          className={`check-run-btn${runningIds.has(check.id) ? ' running' : ''}`}
          title="Run now"
          onClick={e => { e.stopPropagation(); onRun() }}
          disabled={runningIds.has(check.id)}
          style={{ marginLeft: 'auto' }}
        >
          {runningIds.has(check.id) ? <span className="check-spinner" /> : '▶'}
        </button>
      </div>

      {/* Name */}
      <div className="check-card-name">{check.name}</div>

      {/* Target */}
      <div className="check-card-target" title={check.target}>
        {check.target}
        {check.ssl_source === 'traefik' && <span className="check-card-target-tag">Traefik</span>}
        {check.skip_tls_verify && <span className="check-card-target-tag warn">self-signed</span>}
      </div>

      {/* Footer: interval + last checked + pause toggle */}
      <div className="check-card-footer">
        <span className="check-card-interval">every {check.interval_secs}s</span>
        <span className="check-card-last">{formatTimeAgo(check.last_checked_at)}</span>
        <button
          className={`check-toggle-btn${check.enabled ? ' enabled' : ' paused'}`}
          title={check.enabled ? 'Pause check' : 'Resume check'}
          onClick={e => { e.stopPropagation(); onToggleEnabled() }}
        >
          {check.enabled ? '⏸' : '▶'}
        </button>
      </div>
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function Checks() {
  const navigate = useNavigate()
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
  }, [])

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
          <CheckForm
            form={addForm}
            onChange={handleAddChange}
            onSubmit={handleAddSubmit}
            onCancel={() => { setShowAddForm(false); setAddForm(defaultForm); setAddError(null) }}
            error={addError}
            submitting={addSubmitting}
            title="New Check"
            submitLabel="Add Check"
            traefikIntegrations={traefikIntegrations}
            traefikCerts={traefikCerts}
            onIntegrationChange={handleIntegrationChange}
          />
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
