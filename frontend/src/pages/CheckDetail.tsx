import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { DetailPageLayout } from '../components/DetailPageLayout'
import { checks as checksApi, apps as appsApi } from '../api/client'
import { CheckTypeIcon } from '../components/CheckTypeIcon'
import type { MonitorCheck, App } from '../api/types'
import { CheckForm } from '../components/CheckForm'
import { SlidePanel } from '../components/SlidePanel'
import {
  type FormFields,
  validateForm,
  checkToForm,
  formToInput,
  renderCheckResult,
} from '../components/checkFormHelpers'
import { formatEventTime } from '../utils/formatTime'
import './CheckDetail.css'

// ── Edit Panel ────────────────────────────────────────────────────────────────

interface EditPanelProps {
  open: boolean
  check: MonitorCheck
  apps: App[]
  onSave: (form: FormFields) => Promise<void>
  onDelete: () => Promise<void>
  onClose: () => void
}

function EditPanel({
  open,
  check,
  apps,
  onSave,
  onDelete,
  onClose,
}: EditPanelProps) {
  const [form, setForm] = useState<FormFields>(() => checkToForm(check))
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [deleting, setDeleting] = useState(false)

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

  const footer = (
    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
      {check.source_component_id ? (
        <span
          className="sp-btn sp-btn--secondary"
          style={{ opacity: 0.5, cursor: 'default' }}
          title="This check is managed by a Traefik component. Delete the component to remove it."
        >
          Managed by Traefik
        </span>
      ) : (
        <button
          className="sp-btn sp-btn--danger"
          onClick={() => void handleDelete()}
          disabled={deleting}
        >
          {deleting ? 'Deleting…' : 'Delete Check'}
        </button>
      )}
      <button
        className="sp-btn sp-btn--primary"
        onClick={() => void handleSubmit()}
        disabled={submitting}
      >
        {submitting ? 'Saving…' : 'Save Changes'}
      </button>
    </div>
  )

  return (
    <SlidePanel
      open={open}
      onClose={onClose}
      title="Edit Check"
      subtitle={check.name}
      footer={footer}
    >
      <CheckForm
        form={form}
        onChange={handleChange}
        onSubmit={handleSubmit}
        onCancel={onClose}
        error={error}
        submitting={submitting}
        title=""
        submitLabel="Save Changes"
        apps={apps}
        hideActions
      />
    </SlidePanel>
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
  const [resetting, setResetting] = useState(false)
  const [showEdit, setShowEdit] = useState(false)
  const [editKey, setEditKey] = useState(0)
  const [appsList, setAppsList] = useState<App[]>([])

  useEffect(() => {
    if (!id) return

    checksApi.get(id)
      .then(setCheck)
      .catch(() => navigate('/checks'))
      .finally(() => setLoading(false))

    appsApi.list()
      .then(res => setAppsList(res.data))
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

  async function handleResetBaseline() {
    if (!id || !check) return
    setResetting(true)
    try {
      const updated = await checksApi.resetBaseline(id)
      setCheck(updated)
    } catch { /* noop */ }
    finally { setResetting(false) }
  }

  async function handleTogglePause() {
    if (!id || !check) return
    try {
      const updated = await checksApi.update(id, { enabled: !check.enabled })
      setCheck(updated)
    } catch (err) {
      console.error('Failed to toggle pause:', err)
    }
  }

  async function handleSave(form: FormFields) {
    if (!id) return
    const updated = await checksApi.update(id, formToInput(form))
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
        icon={<CheckTypeIcon type={check.type} size={20} />}
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
            {check.type === 'dns' && (
              <button
                className="check-detail-action-btn"
                onClick={() => void handleResetBaseline()}
                disabled={resetting}
                title="Re-resolve DNS now and save as the new baseline"
              >
                {resetting ? <span className="check-detail-spinner" /> : '⟳'} Reset Baseline
              </button>
            )}
            <button className="check-detail-action-btn" onClick={() => { setEditKey(k => k + 1); setShowEdit(true) }}>
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

      <EditPanel
        key={editKey}
        open={showEdit}
        check={check}
        apps={appsList}
        onSave={handleSave}
        onDelete={handleDelete}
        onClose={() => setShowEdit(false)}
      />
    </>
  )
}
