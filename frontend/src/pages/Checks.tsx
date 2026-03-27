import { useState, useEffect } from 'react'
import type { ReactNode } from 'react'
import { Topbar } from '../components/Topbar'
import { SSLRow } from '../components/SSLRow'
import { checks as checksApi } from '../api/client'
import type { MonitorCheck, CreateCheckInput, CheckType, SSLCert } from '../api/types'
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
}

const defaultForm: FormFields = {
  type: 'ping',
  name: '',
  target: '',
  interval_secs: '60',
  expected_status: '200',
  ssl_warn_days: '30',
  ssl_crit_days: '7',
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function statusLabel(check: MonitorCheck): string {
  if (check.type === 'ssl') return 'SSL'
  if (check.last_status === 'up') return 'UP'
  if (check.last_status === 'down') return 'DOWN'
  if (check.last_status === 'warn') return 'WARN'
  return '?'
}

function statusClass(status: string | null, type: string): string {
  if (type === 'ssl' && status === 'warn') return 'monitor-status-block warn'
  if (status === 'up') return 'monitor-status-block up'
  if (status === 'warn') return 'monitor-status-block warn'
  if (status === 'down') return 'monitor-status-block down'
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

function formatResult(lastResult: string | null): string {
  if (!lastResult) return 'No result yet'
  try {
    return JSON.stringify(JSON.parse(lastResult), null, 2)
  } catch {
    return lastResult
  }
}

function validateForm(form: FormFields): string | null {
  if (!form.name.trim()) return 'Name is required'
  if (!form.target.trim()) return 'Target is required'
  if (form.type === 'url' || form.type === 'ssl') {
    if (!form.target.startsWith('http://') && !form.target.startsWith('https://')) {
      return 'Target must begin with http:// or https://'
    }
  }
  const interval = parseInt(form.interval_secs, 10)
  if (isNaN(interval) || interval < 30) return 'Interval must be at least 30 seconds'
  return null
}

function checkToForm(check: MonitorCheck): FormFields {
  return {
    type: check.type as CheckType,
    name: check.name,
    target: check.target,
    interval_secs: String(check.interval_secs),
    expected_status: String(check.expected_status ?? 200),
    ssl_warn_days: String(check.ssl_warn_days ?? 30),
    ssl_crit_days: String(check.ssl_crit_days ?? 7),
  }
}

function formToInput(form: FormFields): CreateCheckInput {
  const input: CreateCheckInput = {
    name: form.name.trim(),
    type: form.type,
    target: form.target.trim(),
    interval_secs: parseInt(form.interval_secs, 10),
  }
  if (form.type === 'url') input.expected_status = parseInt(form.expected_status, 10)
  if (form.type === 'ssl') {
    input.ssl_warn_days = parseInt(form.ssl_warn_days, 10)
    input.ssl_crit_days = parseInt(form.ssl_crit_days, 10)
  }
  return input
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
}: CheckFormProps) {
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
        <div className="form-field">
          <div className="form-label">{form.type === 'ping' ? 'Host / IP' : 'URL'}</div>
          <input
            className="form-input"
            value={form.target}
            onChange={e => onChange('target', e.target.value)}
            placeholder={form.type === 'ping' ? 'e.g. 192.168.1.1' : 'https://example.com'}
          />
        </div>
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

// ── Main page ─────────────────────────────────────────────────────────────────

export function Checks() {
  const [checkList, setCheckList] = useState<MonitorCheck[]>([])
  const [loading, setLoading] = useState(true)

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

  // Run
  const [runningIds, setRunningIds] = useState<Set<string>>(new Set())

  // Delete
  const [deletingIds, setDeletingIds] = useState<Set<string>>(new Set())

  useEffect(() => {
    checksApi.list()
      .then(res => setCheckList(res.data))
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const sslCerts = extractSSLCerts(checkList)

  // ── Add form ──

  function handleAddChange(field: keyof FormFields, value: string) {
    setAddForm(prev => ({ ...prev, [field]: value }))
    setAddError(null)
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

  // ── Expand / edit ──

  function handleToggleExpand(check: MonitorCheck) {
    if (expandedId === check.id) {
      setExpandedId(null)
    } else {
      setExpandedId(check.id)
      setEditForm(checkToForm(check))
      setEditError(null)
    }
  }

  function handleEditChange(field: keyof FormFields, value: string) {
    setEditForm(prev => ({ ...prev, [field]: value }))
    setEditError(null)
  }

  async function handleEditSubmit(id: string) {
    const err = validateForm(editForm)
    if (err) { setEditError(err); return }
    setEditSubmitting(true)
    try {
      const updated = await checksApi.update(id, formToInput(editForm))
      setCheckList(prev => prev.map(c => c.id === id ? updated : c))
      setExpandedId(null)
    } catch (e: unknown) {
      setEditError(e instanceof Error ? e.message : 'Failed to save check')
    } finally {
      setEditSubmitting(false)
    }
  }

  async function handleDelete(id: string) {
    setDeletingIds(prev => new Set(prev).add(id))
    try {
      await checksApi.delete(id)
      setCheckList(prev => prev.filter(c => c.id !== id))
      setExpandedId(null)
    } catch {
      // keep in list if delete fails
    } finally {
      setDeletingIds(prev => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
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
      // noop — status update visible on next poll
    } finally {
      setRunningIds(prev => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
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
          <div className="checks-list">
            {checkList.map(check => (
              <div
                key={check.id}
                className={`check-row-wrapper${expandedId === check.id ? ' expanded' : ''}`}
              >
                <div className="check-row" onClick={() => handleToggleExpand(check)}>
                  <div className={statusClass(check.last_status, check.type)}>
                    {statusLabel(check)}
                  </div>
                  <div className="monitor-info">
                    <div className="monitor-name">{check.name}</div>
                    <div className="monitor-target">
                      {check.target} · {check.type} · every {check.interval_secs}s
                    </div>
                  </div>
                  <div className="monitor-meta">
                    <div className="monitor-last">{formatTimeAgo(check.last_checked_at)}</div>
                  </div>
                  <button
                    className={`check-run-btn${runningIds.has(check.id) ? ' running' : ''}`}
                    title="Run now"
                    onClick={e => { e.stopPropagation(); void handleRun(check.id) }}
                    disabled={runningIds.has(check.id)}
                  >
                    {runningIds.has(check.id) ? <span className="check-spinner" /> : '▶'}
                  </button>
                </div>

                {expandedId === check.id && (
                  <div className="check-expand">
                    <div className="check-expand-details">
                      {formatResult(check.last_result)}
                    </div>
                    <CheckForm
                      form={editForm}
                      onChange={handleEditChange}
                      onSubmit={() => void handleEditSubmit(check.id)}
                      onCancel={() => setExpandedId(null)}
                      error={editError}
                      submitting={editSubmitting}
                      title="Edit Check"
                      submitLabel="Save"
                      extraAction={
                        <button
                          className="form-btn danger"
                          onClick={() => void handleDelete(check.id)}
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
