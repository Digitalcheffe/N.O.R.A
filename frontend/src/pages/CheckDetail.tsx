import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { checks as checksApi, integrations as integrationsApi } from '../api/client'
import type { MonitorCheck, Event, InfraIntegration, TraefikCert } from '../api/types'
import {
  CheckForm,
  type FormFields,
  validateForm,
  checkToForm,
  formToInput,
  renderCheckResult,
} from '../components/CheckForm'
import { formatEventTime } from '../utils/formatTime'
import '../styles/Modal.css'
import './CheckDetail.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function severityBadge(severity: string) {
  return <span className={`event-severity-badge ${severity}`}>{severity}</span>
}

function statusLabel(check: MonitorCheck): string {
  if (check.type === 'ssl') {
    if (check.last_result) {
      try {
        const r = JSON.parse(check.last_result) as { days_remaining?: number }
        if (r.days_remaining != null) return `${r.days_remaining}d`
      } catch { /* fallthrough */ }
    }
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

// ── Edit Modal ────────────────────────────────────────────────────────────────

interface EditModalProps {
  check: MonitorCheck
  traefikIntegrations: InfraIntegration[]
  traefikCerts: TraefikCert[]
  onIntegrationChange: (id: string) => void
  onSave: (form: FormFields) => Promise<void>
  onDelete: () => Promise<void>
  onClose: () => void
}

function EditModal({
  check,
  traefikIntegrations,
  traefikCerts,
  onIntegrationChange,
  onSave,
  onDelete,
  onClose,
}: EditModalProps) {
  const [form, setForm] = useState<FormFields>(() => checkToForm(check))
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  function handleChange(field: keyof FormFields, value: string) {
    setForm(prev => ({ ...prev, [field]: value }))
    setError(null)
  }

  async function handleSubmit() {
    const err = validateForm(form)
    if (err) { setError(err); return }
    setSubmitting(true)
    try {
      await onSave(form)
      onClose()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to save')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleDelete() {
    setDeleting(true)
    try {
      await onDelete()
    } catch {
      setDeleting(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <div className="modal-title">Edit Check</div>
          <div className="modal-subtitle">{check.name}</div>
          <button className="modal-close" onClick={onClose}>✕</button>
        </div>
        <div className="modal-body check-edit-modal-body">
          <CheckForm
            form={form}
            onChange={handleChange}
            onSubmit={handleSubmit}
            onCancel={onClose}
            error={error}
            submitting={submitting}
            title=""
            submitLabel="Save Changes"
            traefikIntegrations={traefikIntegrations}
            traefikCerts={traefikCerts}
            onIntegrationChange={onIntegrationChange}
            extraAction={
              check.source_component_id ? (
                <span
                  className="form-btn secondary"
                  style={{ marginLeft: 'auto', opacity: 0.5, cursor: 'default' }}
                  title="This check is managed by a Traefik component. Delete the component to remove it."
                >
                  Managed by Traefik
                </span>
              ) : (
                <button
                  className="form-btn danger"
                  onClick={() => void handleDelete()}
                  disabled={deleting}
                  style={{ marginLeft: 'auto' }}
                >
                  {deleting ? 'Deleting…' : 'Delete Check'}
                </button>
              )
            }
          />
        </div>
      </div>
    </div>
  )
}

// ── Main Page ─────────────────────────────────────────────────────────────────

export function CheckDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()

  const [check, setCheck] = useState<MonitorCheck | null>(null)
  const [loading, setLoading] = useState(true)
  const [events, setEvents] = useState<Event[]>([])
  const [eventsLoading, setEventsLoading] = useState(true)
  const [running, setRunning] = useState(false)
  const [showEdit, setShowEdit] = useState(false)
  const [traefikIntegrations, setTraefikIntegrations] = useState<InfraIntegration[]>([])
  const [traefikCerts, setTraefikCerts] = useState<TraefikCert[]>([])

  useEffect(() => {
    if (!id) return

    checksApi.get(id)
      .then(setCheck)
      .catch(() => navigate('/checks'))
      .finally(() => setLoading(false))

    checksApi.listEvents(id)
      .then(res => setEvents(res.data))
      .catch(() => {})
      .finally(() => setEventsLoading(false))

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
  }, [id, navigate, tick])

  async function handleRun() {
    if (!id || !check) return
    setRunning(true)
    try {
      await checksApi.run(id)
      const updated = await checksApi.get(id)
      setCheck(updated)
      const eventsRes = await checksApi.listEvents(id)
      setEvents(eventsRes.data)
    } catch { /* noop */ }
    finally { setRunning(false) }
  }

  async function handleTogglePause() {
    if (!id || !check) return
    try {
      const updated = await checksApi.update(id, { enabled: !check.enabled })
      setCheck(updated)
    } catch { /* noop */ }
  }

  function handleIntegrationChange(integrationId: string) {
    if (!integrationId) return
    integrationsApi.certs(integrationId)
      .then(res => setTraefikCerts(res.data))
      .catch(() => {})
  }

  async function handleSave(form: FormFields) {
    if (!id) return
    const integrationId = form.ssl_source === 'traefik' ? form.integration_id : undefined
    const updated = await checksApi.update(id, formToInput(form, integrationId))
    setCheck(updated)
  }

  async function handleDelete() {
    if (!id) return
    await checksApi.delete(id)
    navigate('/checks')
  }

  if (loading) {
    return (
      <>
        <Topbar title="Check" />
        <div className="content">
          <div style={{ padding: 40, textAlign: 'center', fontFamily: 'var(--mono)', fontSize: 12, color: 'var(--text3)' }}>
            Loading…
          </div>
        </div>
      </>
    )
  }

  if (!check) return null

  return (
    <>
      <Topbar title={check.name} />
      <div className="content">

        {/* Header */}
        <div className="check-detail-header">
          <div className="check-detail-header-left">
            <button className="check-detail-back-btn" onClick={() => navigate('/checks')}>
              ← Checks
            </button>
            <div className={statusClass(check.last_status)}>
              {statusLabel(check)}
            </div>
            <div className="check-detail-meta">
              <div className="check-detail-name">
                {check.name}
                <span className={`check-type-badge check-type-${check.type}`}>
                  {check.type.toUpperCase()}
                </span>
                {!check.enabled && <span className="check-paused-badge">PAUSED</span>}
              </div>
              <div className="check-detail-target">{check.target}</div>
            </div>
          </div>
          <div className="check-detail-header-actions">
            <button
              className="check-detail-action-btn"
              onClick={() => void handleRun()}
              disabled={running}
            >
              {running ? <span className="check-detail-spinner" /> : '▶'} Run Now
            </button>
            <button
              className={`check-detail-action-btn${check.enabled ? ' pause' : ' paused'}`}
              onClick={() => void handleTogglePause()}
            >
              {check.enabled ? '⏸ Pause' : '▶ Resume'}
            </button>
            <button className="check-detail-action-btn" onClick={() => setShowEdit(true)}>
              ⚙ Settings
            </button>
          </div>
        </div>

        {/* Stats */}
        <div className="check-detail-stats">
          <div className="check-detail-stat">
            <div className="check-detail-stat-label">Status</div>
            <div className={`check-detail-stat-value ${check.last_status ?? 'unknown'}`}>
              {check.last_status ? check.last_status.toUpperCase() : 'UNKNOWN'}
            </div>
          </div>
          <div className="check-detail-stat">
            <div className="check-detail-stat-label">Last Checked</div>
            <div className="check-detail-stat-value">
              {check.last_checked_at
                ? new Date(check.last_checked_at).toLocaleString()
                : '—'}
            </div>
          </div>
          <div className="check-detail-stat">
            <div className="check-detail-stat-label">Interval</div>
            <div className="check-detail-stat-value">every {check.interval_secs}s</div>
          </div>
          <div className="check-detail-stat">
            <div className="check-detail-stat-label">Type</div>
            <div className="check-detail-stat-value">{check.type.toUpperCase()}</div>
          </div>
        </div>

        {/* Last Result */}
        <div className="check-detail-section">
          <div className="check-detail-section-header">
            <div className="check-detail-section-title">Last Result</div>
            {check.last_checked_at && (
              <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--text3)' }}>
                {formatEventTime(check.last_checked_at)}
              </span>
            )}
          </div>
          <div className="check-detail-section-body">
            <div className="check-result-box" style={{ marginBottom: 0 }}>
              {renderCheckResult(check)}
            </div>
          </div>
        </div>

        {/* History */}
        <div className="check-detail-section">
          <div className="check-detail-section-header">
            <div className="check-detail-section-title">History</div>
          </div>
          <div className="check-detail-section-body">
            {eventsLoading ? (
              <div className="check-history-empty">Loading…</div>
            ) : events.length === 0 ? (
              <div className="check-history-empty">
                No status-change events yet. Events are created on first status transition.
              </div>
            ) : (
              <div className="check-history-list">
                {events.map(ev => (
                  <div key={ev.id} className="check-history-row">
                    {severityBadge(ev.severity)}
                    <span className="check-history-text">{ev.display_text}</span>
                    <span className="check-history-time">{formatEventTime(ev.received_at)}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

      </div>

      {showEdit && (
        <EditModal
          check={check}
          traefikIntegrations={traefikIntegrations}
          traefikCerts={traefikCerts}
          onIntegrationChange={handleIntegrationChange}
          onSave={handleSave}
          onDelete={handleDelete}
          onClose={() => setShowEdit(false)}
        />
      )}
    </>
  )
}
