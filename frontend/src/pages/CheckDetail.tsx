import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { DetailPageLayout } from '../components/DetailPageLayout'
import { checks as checksApi, integrations as integrationsApi } from '../api/client'
import type { MonitorCheck, InfraIntegration, TraefikCert } from '../api/types'
import { CheckForm } from '../components/CheckForm'
import {
  type FormFields,
  validateForm,
  checkToForm,
  formToInput,
  renderCheckResult,
} from '../components/checkFormHelpers'
import { formatEventTime } from '../utils/formatTime'
import '../styles/Modal.css'
import './CheckDetail.css'

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

  const dplStatus: 'online' | 'offline' | 'unknown' | 'warning' =
    check.last_status === 'up'   ? 'online'  :
    check.last_status === 'down' ? 'offline' :
    check.last_status === 'warn' ? 'warning' : 'unknown'

  const keyDataPoints = [
    { label: 'Type', value: check.type.toUpperCase() },
    { label: 'Target', value: check.target },
    { label: 'Interval', value: `every ${check.interval_secs}s` },
    { label: 'Status', value: check.last_status ? check.last_status.toUpperCase() : 'UNKNOWN' },
  ]

  return (
    <>
      <DetailPageLayout
        breadcrumb="Checks"
        breadcrumbPath="/checks"
        name={check.name}
        status={{ status: dplStatus }}
        lastPolled={check.last_checked_at ? formatEventTime(check.last_checked_at) : undefined}
        keyDataPoints={keyDataPoints}
        actions={
          <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
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
        }
        sourceType="monitor_check"
        sourceId={id ?? ''}
      >
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
      </DetailPageLayout>

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
