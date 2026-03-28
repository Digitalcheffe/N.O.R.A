import { useState, useEffect } from 'react'
import type { ReactNode } from 'react'
import { Topbar } from '../components/Topbar'
import { SSLRow } from '../components/SSLRow'
import { checks as checksApi, integrations as integrationsApi } from '../api/client'
import type { MonitorCheck, CreateCheckInput, CheckType, SSLCert, InfraIntegration, TraefikCert, SSLSource, Event } from '../api/types'
import './Checks.css'

// ── Types ─────────────────────────────────────────────────────────────────────

type FormFields = {
  type: CheckType
  name: string
  target: string
  interval_secs: string
  expected_status: string
  ssl_warn_days: string
  ssl_crit_days: string
  ssl_source: SSLSource
  integration_id: string
  traefik_domain: string
  skip_tls_verify: string  // 'true' | 'false'
}

const defaultForm: FormFields = {
  type: 'ping',
  name: '',
  target: '',
  interval_secs: '60',
  expected_status: '200',
  ssl_warn_days: '30',
  ssl_crit_days: '7',
  ssl_source: 'standalone',
  integration_id: '',
  traefik_domain: '',
  skip_tls_verify: 'false',
}

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

function validateForm(form: FormFields): string | null {
  if (!form.name.trim()) return 'Name is required'
  const interval = parseInt(form.interval_secs, 10)
  if (isNaN(interval) || interval < 30) return 'Interval must be at least 30 seconds'

  if (form.type === 'url') {
    if (!form.target.trim()) return 'Target is required'
    if (!form.target.startsWith('http://') && !form.target.startsWith('https://')) {
      return 'Target must begin with http:// or https://'
    }
  }

  if (form.type === 'ssl') {
    if (form.ssl_source === 'traefik') {
      if (!form.traefik_domain) return 'Select a domain from the Traefik cert list'
    } else {
      if (!form.target.trim()) return 'Target is required'
      if (!form.target.startsWith('http://') && !form.target.startsWith('https://')) {
        return 'Target must begin with http:// or https://'
      }
    }
  }

  if (form.type === 'ping' && !form.target.trim()) return 'Target is required'

  return null
}

function checkToForm(check: MonitorCheck): FormFields {
  return {
    type: check.type as CheckType,
    name: check.name,
    target: check.type === 'ssl' && check.ssl_source === 'traefik' ? '' : check.target,
    interval_secs: String(check.interval_secs),
    expected_status: String(check.expected_status ?? 200),
    ssl_warn_days: String(check.ssl_warn_days ?? 30),
    ssl_crit_days: String(check.ssl_crit_days ?? 7),
    ssl_source: (check.ssl_source as SSLSource) ?? 'standalone',
    integration_id: check.integration_id ?? '',
    traefik_domain: check.ssl_source === 'traefik' ? check.target : '',
    skip_tls_verify: check.skip_tls_verify ? 'true' : 'false',
  }
}

function formToInput(form: FormFields, integrationID?: string): CreateCheckInput {
  const input: CreateCheckInput = {
    name: form.name.trim(),
    type: form.type,
    target: form.type === 'ssl' && form.ssl_source === 'traefik'
      ? form.traefik_domain
      : form.target.trim(),
    interval_secs: parseInt(form.interval_secs, 10),
  }
  if (form.type === 'url') {
    input.expected_status = parseInt(form.expected_status, 10)
    input.skip_tls_verify = form.skip_tls_verify === 'true'
  }
  if (form.type === 'ssl') {
    input.ssl_warn_days = parseInt(form.ssl_warn_days, 10)
    input.ssl_crit_days = parseInt(form.ssl_crit_days, 10)
    input.ssl_source = form.ssl_source
    if (form.ssl_source === 'traefik' && integrationID) {
      input.integration_id = integrationID
    }
  }
  return input
}

// ── Result display ────────────────────────────────────────────────────────────

function renderCheckResult(check: MonitorCheck): ReactNode {
  if (!check.last_result) return <span className="check-result-empty">No result recorded yet</span>
  try {
    const r = JSON.parse(check.last_result) as Record<string, unknown>
    if (check.type === 'ping') {
      return (
        <div className="check-result-grid">
          <span className="check-result-label">Latency</span>
          <span className="check-result-value">{r.latency_ms != null ? `${r.latency_ms}ms` : '—'}</span>
        </div>
      )
    }
    if (check.type === 'url') {
      return (
        <div className="check-result-grid">
          <span className="check-result-label">HTTP Status</span>
          <span className="check-result-value">{r.status_code != null ? String(r.status_code) : '—'}</span>
          <span className="check-result-label">Latency</span>
          <span className="check-result-value">{r.latency_ms != null ? `${r.latency_ms}ms` : '—'}</span>
          {!!r.error && <>
            <span className="check-result-label">Error</span>
            <span className="check-result-value check-result-error">{String(r.error)}</span>
          </>}
        </div>
      )
    }
    if (check.type === 'ssl') {
      if (r.error) return (
        <div className="check-result-grid">
          <span className="check-result-label">Error</span>
          <span className="check-result-value check-result-error">{String(r.error)}</span>
        </div>
      )
      const expiresStr = r.expires_at
        ? new Date(r.expires_at as string).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
        : '—'
      return (
        <div className="check-result-grid">
          <span className="check-result-label">Days remaining</span>
          <span className="check-result-value">{r.days_remaining != null ? String(r.days_remaining) : '—'}</span>
          <span className="check-result-label">Expires</span>
          <span className="check-result-value">{expiresStr}</span>
          {!!r.issuer && <>
            <span className="check-result-label">Issuer</span>
            <span className="check-result-value">{String(r.issuer)}</span>
          </>}
          {!!r.subject && <>
            <span className="check-result-label">Subject</span>
            <span className="check-result-value">{String(r.subject)}</span>
          </>}
        </div>
      )
    }
  } catch { /* fall through */ }
  return <span className="check-result-empty">{check.last_result}</span>
}

// ── Severity badge ────────────────────────────────────────────────────────────

function severityBadge(severity: string) {
  return <span className={`event-severity-badge ${severity}`}>{severity}</span>
}

// ── CheckForm component ───────────────────────────────────────────────────────

interface CheckFormProps {
  form: FormFields
  onChange: (field: keyof FormFields, value: string) => void
  onSubmit: () => void
  onCancel: () => void
  error: string | null
  submitting: boolean
  title: string
  submitLabel: string
  extraAction?: ReactNode
  traefikIntegrations: InfraIntegration[]
  traefikCerts: TraefikCert[]
  onIntegrationChange: (integrationId: string) => void
}

const CHECK_TYPES: CheckType[] = ['ping', 'url', 'ssl']

function CheckForm({
  form,
  onChange,
  onSubmit,
  onCancel,
  error,
  submitting,
  title,
  submitLabel,
  extraAction,
  traefikIntegrations,
  traefikCerts,
  onIntegrationChange,
}: CheckFormProps) {
  const hasTraefik = traefikIntegrations.length > 0
  const selectedIntegration = traefikIntegrations.find(i => i.id === form.integration_id)

  return (
    <div className="add-form">
      <div className="form-title">{title}</div>
      <div className="type-selector">
        {CHECK_TYPES.map(t => (
          <button
            key={t}
            className={`type-btn${form.type === t ? ' active' : ''}`}
            onClick={() => onChange('type', t)}
          >
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>
      <div className="form-fields">
        <div className="form-field">
          <div className="form-label">Name</div>
          <input
            className="form-input"
            value={form.name}
            onChange={e => onChange('name', e.target.value)}
            placeholder="e.g. Proxmox Web UI"
          />
        </div>

        {/* SSL source toggle */}
        {form.type === 'ssl' && (
          <div className="form-field">
            <div className="form-label">SSL Source</div>
            {!hasTraefik ? (
              <div className="ssl-no-traefik-banner">
                Connect Traefik in Settings → Integrations to enable automatic SSL discovery.
              </div>
            ) : (
              <div className="type-selector">
                <button
                  className={`type-btn${form.ssl_source === 'traefik' ? ' active' : ''}`}
                  onClick={() => onChange('ssl_source', 'traefik')}
                >
                  Traefik
                </button>
                <button
                  className={`type-btn${form.ssl_source === 'standalone' ? ' active' : ''}`}
                  onClick={() => onChange('ssl_source', 'standalone')}
                >
                  Standalone
                </button>
              </div>
            )}
          </div>
        )}

        {/* Traefik SSL: integration selector + domain dropdown */}
        {form.type === 'ssl' && form.ssl_source === 'traefik' && hasTraefik && (
          <>
            {traefikIntegrations.length > 1 && (
              <div className="form-field">
                <div className="form-label">Traefik Integration</div>
                <select
                  className="form-input"
                  value={form.integration_id}
                  onChange={e => {
                    onChange('integration_id', e.target.value)
                    onIntegrationChange(e.target.value)
                  }}
                >
                  <option value="">Select integration…</option>
                  {traefikIntegrations.map(i => (
                    <option key={i.id} value={i.id}>{i.name}</option>
                  ))}
                </select>
              </div>
            )}
            <div className="form-field">
              <div className="form-label">Domain</div>
              {traefikCerts.length === 0 ? (
                <div className="ssl-no-certs-msg">
                  {selectedIntegration
                    ? 'No certs discovered yet — run a sync in Settings → Integrations.'
                    : 'Select an integration to see available domains.'}
                </div>
              ) : (
                <select
                  className="form-input"
                  value={form.traefik_domain}
                  onChange={e => onChange('traefik_domain', e.target.value)}
                >
                  <option value="">Select domain…</option>
                  {traefikCerts.map(c => (
                    <option key={c.id} value={c.domain}>{c.domain}</option>
                  ))}
                </select>
              )}
            </div>
          </>
        )}

        {/* Standalone SSL or other types: target URL/host */}
        {(form.type !== 'ssl' || form.ssl_source === 'standalone' || !hasTraefik) && (
          <div className="form-field">
            <div className="form-label">{form.type === 'ping' ? 'Host / IP' : 'URL'}</div>
            <input
              className="form-input"
              value={form.target}
              onChange={e => onChange('target', e.target.value)}
              placeholder={form.type === 'ping' ? 'e.g. 192.168.1.1' : 'https://example.com'}
            />
            {form.type === 'ssl' && form.ssl_source === 'standalone' && (
              <div className="ssl-standalone-warning">
                ⚠ Standalone SSL checks make a direct TLS connection. This may fail for
                services proxied through Traefik on the same host. Use for external URLs only.
              </div>
            )}
          </div>
        )}

        <div className="form-field">
          <div className="form-label">Interval (seconds)</div>
          <input
            className="form-input"
            type="number"
            min="30"
            value={form.interval_secs}
            onChange={e => onChange('interval_secs', e.target.value)}
          />
        </div>

        {form.type === 'url' && (
          <>
            <div className="form-field">
              <div className="form-label">Expected Status</div>
              <input
                className="form-input"
                type="number"
                value={form.expected_status}
                onChange={e => onChange('expected_status', e.target.value)}
                placeholder="200"
              />
            </div>
            <div className="form-field form-field-full">
              <label className="form-checkbox-label">
                <input
                  type="checkbox"
                  className="form-checkbox"
                  checked={form.skip_tls_verify === 'true'}
                  onChange={e => onChange('skip_tls_verify', e.target.checked ? 'true' : 'false')}
                />
                <span className="form-checkbox-text">
                  Accept self-signed certificates
                  <span className="form-checkbox-hint"> — skips TLS verification; use for internal services only</span>
                </span>
              </label>
            </div>
          </>
        )}

        {form.type === 'ssl' && (
          <>
            <div className="form-field">
              <div className="form-label">Warn Threshold (days)</div>
              <input
                className="form-input"
                type="number"
                min="1"
                value={form.ssl_warn_days}
                onChange={e => onChange('ssl_warn_days', e.target.value)}
                placeholder="30"
              />
            </div>
            <div className="form-field">
              <div className="form-label">Critical Threshold (days)</div>
              <input
                className="form-input"
                type="number"
                min="1"
                value={form.ssl_crit_days}
                onChange={e => onChange('ssl_crit_days', e.target.value)}
                placeholder="7"
              />
            </div>
          </>
        )}
      </div>
      {error && <div className="form-error">{error}</div>}
      <div className="form-actions">
        <button className="form-btn primary" onClick={onSubmit} disabled={submitting}>
          {submitting ? 'Saving…' : submitLabel}
        </button>
        <button className="form-btn secondary" onClick={onCancel}>
          Cancel
        </button>
        {extraAction}
      </div>
    </div>
  )
}

// ── Check card expand panel ───────────────────────────────────────────────────

interface CheckDetailProps {
  check: MonitorCheck
  editForm: FormFields
  editError: string | null
  editSubmitting: boolean
  deletingIds: Set<string>
  runningIds: Set<string>
  events: Event[]
  eventsLoading: boolean
  traefikIntegrations: InfraIntegration[]
  traefikCerts: TraefikCert[]
  onEditChange: (field: keyof FormFields, value: string) => void
  onEditSubmit: () => void
  onClose: () => void
  onDelete: () => void
  onRun: () => void
  onIntegrationChange: (id: string) => void
}

function CheckDetail({
  check,
  editForm,
  editError,
  editSubmitting,
  deletingIds,
  runningIds,
  events,
  eventsLoading,
  traefikIntegrations,
  traefikCerts,
  onEditChange,
  onEditSubmit,
  onClose,
  onDelete,
  onRun,
  onIntegrationChange,
}: CheckDetailProps) {
  const [tab, setTab] = useState<'result' | 'history' | 'edit'>('result')

  return (
    <div className="check-detail-panel">
      <div className="check-detail-tabs">
        <button
          className={`check-detail-tab${tab === 'result' ? ' active' : ''}`}
          onClick={() => setTab('result')}
        >
          Last Result
        </button>
        <button
          className={`check-detail-tab${tab === 'history' ? ' active' : ''}`}
          onClick={() => setTab('history')}
        >
          History
        </button>
        <button
          className={`check-detail-tab${tab === 'edit' ? ' active' : ''}`}
          onClick={() => setTab('edit')}
        >
          Edit
        </button>
        <div className="check-detail-tab-actions">
          <button
            className={`check-run-btn${runningIds.has(check.id) ? ' running' : ''}`}
            title="Run now"
            onClick={e => { e.stopPropagation(); onRun() }}
            disabled={runningIds.has(check.id)}
          >
            {runningIds.has(check.id) ? <span className="check-spinner" /> : '▶'}
          </button>
          <button className="check-detail-close" onClick={onClose} title="Close">✕</button>
        </div>
      </div>

      {tab === 'result' && (
        <div className="check-detail-content">
          <div className="check-result-box">
            {renderCheckResult(check)}
          </div>
          <div className="check-result-meta">
            <span className="check-result-meta-label">Last checked</span>
            <span className="check-result-meta-value">{check.last_checked_at
              ? new Date(check.last_checked_at).toLocaleString()
              : 'Never'}
            </span>
          </div>
        </div>
      )}

      {tab === 'history' && (
        <div className="check-detail-content">
          {eventsLoading ? (
            <div className="check-history-empty">Loading…</div>
          ) : events.length === 0 ? (
            <div className="check-history-empty">
              No status-change events recorded yet. Events are created on first status transition.
            </div>
          ) : (
            <div className="check-history-list">
              {events.map(ev => (
                <div key={ev.id} className="check-history-row">
                  {severityBadge(ev.severity)}
                  <span className="check-history-text">{ev.display_text}</span>
                  <span className="check-history-time">{formatTimeAgo(ev.received_at)}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {tab === 'edit' && (
        <div className="check-detail-content">
          <CheckForm
            form={editForm}
            onChange={onEditChange}
            onSubmit={onEditSubmit}
            onCancel={onClose}
            error={editError}
            submitting={editSubmitting}
            title="Edit Check"
            submitLabel="Save"
            traefikIntegrations={traefikIntegrations}
            traefikCerts={traefikCerts}
            onIntegrationChange={onIntegrationChange}
            extraAction={
              <button
                className="form-btn danger"
                onClick={onDelete}
                disabled={deletingIds.has(check.id)}
                style={{ marginLeft: 'auto' }}
              >
                {deletingIds.has(check.id) ? 'Deleting…' : 'Delete'}
              </button>
            }
          />
        </div>
      )}
    </div>
  )
}

// ── Check card ────────────────────────────────────────────────────────────────

interface CheckCardProps {
  check: MonitorCheck
  expanded: boolean
  editForm: FormFields
  editError: string | null
  editSubmitting: boolean
  deletingIds: Set<string>
  runningIds: Set<string>
  checkEvents: Event[]
  checkEventsLoading: boolean
  traefikIntegrations: InfraIntegration[]
  traefikCerts: TraefikCert[]
  onToggleExpand: () => void
  onToggleEnabled: () => void
  onEditChange: (field: keyof FormFields, value: string) => void
  onEditSubmit: () => void
  onDelete: () => void
  onRun: () => void
  onIntegrationChange: (id: string) => void
}

function CheckCard({
  check,
  expanded,
  editForm,
  editError,
  editSubmitting,
  deletingIds,
  runningIds,
  checkEvents,
  checkEventsLoading,
  traefikIntegrations,
  traefikCerts,
  onToggleExpand,
  onToggleEnabled,
  onEditChange,
  onEditSubmit,
  onDelete,
  onRun,
  onIntegrationChange,
}: CheckCardProps) {
  const disabled = !check.enabled

  return (
    <div className={`check-card-wrapper${expanded ? ' expanded' : ''}${disabled ? ' disabled' : ''}`}>
      <div className={`check-card${expanded ? ' expanded' : ''}`} onClick={onToggleExpand}>

        {/* Top row: status block + type badge */}
        <div className="check-card-top">
          <div className={statusClass(check.last_status)}>
            {statusLabel(check)}
          </div>
          <span className={`check-type-badge check-type-${check.type}`}>
            {check.type.toUpperCase()}
          </span>
          {disabled && <span className="check-paused-badge">PAUSED</span>}
        </div>

        {/* Name */}
        <div className="check-card-name">{check.name}</div>

        {/* Target */}
        <div className="check-card-target" title={check.target}>
          {check.target}
          {check.ssl_source === 'traefik' && <span className="check-card-target-tag">Traefik</span>}
          {check.skip_tls_verify && <span className="check-card-target-tag warn">self-signed</span>}
        </div>

        {/* Footer: interval + last checked + toggle */}
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

      {expanded && (
        <CheckDetail
          check={check}
          editForm={editForm}
          editError={editError}
          editSubmitting={editSubmitting}
          deletingIds={deletingIds}
          runningIds={runningIds}
          events={checkEvents}
          eventsLoading={checkEventsLoading}
          traefikIntegrations={traefikIntegrations}
          traefikCerts={traefikCerts}
          onEditChange={onEditChange}
          onEditSubmit={onEditSubmit}
          onClose={onToggleExpand}
          onDelete={onDelete}
          onRun={onRun}
          onIntegrationChange={onIntegrationChange}
        />
      )}
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function Checks() {
  const [checkList, setCheckList] = useState<MonitorCheck[]>([])
  const [loading, setLoading] = useState(true)

  const [traefikIntegrations, setTraefikIntegrations] = useState<InfraIntegration[]>([])
  const [traefikCerts, setTraefikCerts] = useState<TraefikCert[]>([])

  // Add form
  const [showAddForm, setShowAddForm] = useState(false)
  const [addForm, setAddForm] = useState<FormFields>(defaultForm)
  const [addError, setAddError] = useState<string | null>(null)
  const [addSubmitting, setAddSubmitting] = useState(false)

  // Expand + edit
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [editForm, setEditForm] = useState<FormFields>(defaultForm)
  const [editError, setEditError] = useState<string | null>(null)
  const [editSubmitting, setEditSubmitting] = useState(false)

  // Per-check events (for history tab)
  const [checkEvents, setCheckEvents] = useState<Event[]>([])
  const [checkEventsLoading, setCheckEventsLoading] = useState(false)

  // Action state
  const [runningIds, setRunningIds] = useState<Set<string>>(new Set())
  const [deletingIds, setDeletingIds] = useState<Set<string>>(new Set())

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

  // ── Integration change ──

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

  // ── Expand ──

  function handleToggleExpand(check: MonitorCheck) {
    if (expandedId === check.id) {
      setExpandedId(null)
      setCheckEvents([])
      return
    }
    setExpandedId(check.id)
    setEditForm(checkToForm(check))
    setEditError(null)
    if (check.ssl_source === 'traefik' && check.integration_id) {
      handleIntegrationChange(check.integration_id)
    }
    // Load events for history tab
    setCheckEventsLoading(true)
    setCheckEvents([])
    checksApi.listEvents(check.id)
      .then(res => setCheckEvents(res.data))
      .catch(() => {})
      .finally(() => setCheckEventsLoading(false))
  }

  // ── Edit ──

  function handleEditChange(field: keyof FormFields, value: string) {
    setEditForm(prev => ({ ...prev, [field]: value }))
    setEditError(null)
  }

  async function handleEditSubmit(id: string) {
    const err = validateForm(editForm)
    if (err) { setEditError(err); return }
    setEditSubmitting(true)
    try {
      const integrationId = editForm.ssl_source === 'traefik' ? editForm.integration_id : undefined
      const updated = await checksApi.update(id, formToInput(editForm, integrationId))
      setCheckList(prev => prev.map(c => c.id === id ? updated : c))
      setExpandedId(null)
    } catch (e: unknown) {
      setEditError(e instanceof Error ? e.message : 'Failed to save check')
    } finally {
      setEditSubmitting(false)
    }
  }

  // ── Toggle enabled ──

  async function handleToggleEnabled(check: MonitorCheck) {
    const newEnabled = !check.enabled
    // Optimistic update
    setCheckList(prev => prev.map(c => c.id === check.id ? { ...c, enabled: newEnabled } : c))
    try {
      const updated = await checksApi.update(check.id, { enabled: newEnabled })
      setCheckList(prev => prev.map(c => c.id === check.id ? updated : c))
    } catch {
      // Revert on failure
      setCheckList(prev => prev.map(c => c.id === check.id ? check : c))
    }
  }

  // ── Delete ──

  async function handleDelete(id: string) {
    setDeletingIds(prev => new Set(prev).add(id))
    try {
      await checksApi.delete(id)
      setCheckList(prev => prev.filter(c => c.id !== id))
      setExpandedId(null)
    } catch {
      // keep in list if delete fails
    } finally {
      setDeletingIds(prev => { const next = new Set(prev); next.delete(id); return next })
    }
  }

  // ── Run ──

  async function handleRun(id: string) {
    setRunningIds(prev => new Set(prev).add(id))
    try {
      await checksApi.run(id)
      const updated = await checksApi.get(id)
      setCheckList(prev => prev.map(c => c.id === id ? updated : c))
      // Refresh events for expanded check
      if (expandedId === id) {
        checksApi.listEvents(id)
          .then(res => setCheckEvents(res.data))
          .catch(() => {})
      }
    } catch {
      // noop
    } finally {
      setRunningIds(prev => { const next = new Set(prev); next.delete(id); return next })
    }
  }

  return (
    <>
      <Topbar title="Monitor Checks" onAdd={() => setShowAddForm(prev => !prev)} />
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
                expanded={expandedId === check.id}
                editForm={editForm}
                editError={editError}
                editSubmitting={editSubmitting}
                deletingIds={deletingIds}
                runningIds={runningIds}
                checkEvents={checkEvents}
                checkEventsLoading={checkEventsLoading}
                traefikIntegrations={traefikIntegrations}
                traefikCerts={traefikCerts}
                onToggleExpand={() => handleToggleExpand(check)}
                onToggleEnabled={() => void handleToggleEnabled(check)}
                onEditChange={handleEditChange}
                onEditSubmit={() => void handleEditSubmit(check.id)}
                onDelete={() => void handleDelete(check.id)}
                onRun={() => void handleRun(check.id)}
                onIntegrationChange={handleIntegrationChange}
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
