import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { InfraIntegrations } from './Integrations'
import { appTemplates, digestSettings, smtpSettings } from '../api/client'
import type { AppTemplate, CustomProfile, DigestFrequency, DigestSchedule, SMTPSettings } from '../api/types'

import './Settings.css'

type Tab = 'apps' | 'notifications' | 'metrics'

const TABS: { id: Tab; label: string }[] = [
  { id: 'apps', label: 'Apps' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'metrics', label: 'Instance Metrics' },
]

// ── Delete confirmation modal ─────────────────────────────────────────────────

interface DeleteConfirmModalProps {
  name: string
  onConfirm: () => void
  onCancel: () => void
  deleting: boolean
}

function DeleteConfirmModal({ name, onConfirm, onCancel, deleting }: DeleteConfirmModalProps) {
  return (
    <div className="modal-overlay" onClick={onCancel}>
      <div className="modal modal--destructive" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <span className="modal-title">Delete Custom App Template</span>
          <button className="modal-close" onClick={onCancel}>✕</button>
        </div>
        <div className="modal-body">
          <p className="modal-delete-name">"{name}"</p>
          <p className="modal-delete-warning">
            This will permanently delete the template definition. Any apps using this template will lose their field mappings and severity rules.
          </p>
          <div className="modal-delete-nonrecoverable">
            This action cannot be undone.
          </div>
        </div>
        <div className="modal-footer">
          <button className="settings-btn secondary" onClick={onCancel}>Cancel</button>
          <button
            className="settings-btn danger"
            onClick={onConfirm}
            disabled={deleting}
          >
            {deleting ? 'Deleting…' : 'Delete permanently'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Apps tab ──────────────────────────────────────────────────────────────────

function AppsTab() {
  const navigate = useNavigate()
  const [builtins, setBuiltins] = useState<AppTemplate[]>([])
  const [customs, setCustoms] = useState<CustomProfile[]>([])
  const [loadError, setLoadError] = useState('')
  const [confirmDelete, setConfirmDelete] = useState<CustomProfile | null>(null)
  const [deleting, setDeleting] = useState(false)

  const handleDelete = async () => {
    if (!confirmDelete) return
    setDeleting(true)
    try {
      await appTemplates.deleteCustom(confirmDelete.id)
      setCustoms(prev => prev.filter(c => c.id !== confirmDelete.id))
    } catch {
      // leave list unchanged on error
    } finally {
      setConfirmDelete(null)
      setDeleting(false)
    }
  }

  useEffect(() => {
    Promise.all([appTemplates.list(), appTemplates.listCustom()])
      .then(([bt, ct]) => {
        // Sort built-ins by category then name
        const sorted = [...bt.data].sort((a, b) => a.name.localeCompare(b.name))
        setBuiltins(sorted)
        setCustoms(ct.data ?? [])
      })
      .catch(() => setLoadError('Failed to load app templates'))
  }, [])

  return (
    <div className="tab-content">
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Webhook Ingest</span>
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Ingest URL</label>
          <input className="settings-input" readOnly value="http://localhost:8080/ingest/webhook" />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Ingest Token</label>
          <div className="settings-row-inline">
            <input className="settings-input" readOnly value="••••••••••••••••" />
            <button className="settings-btn secondary">Rotate</button>
          </div>
        </div>
      </section>

      <section className="settings-section">
        <InfraIntegrations />
      </section>

      {/* Built-in app templates */}
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Apps</span>
        </div>
        {loadError ? (
          <div className="settings-placeholder" style={{ color: 'var(--red)' }}>{loadError}</div>
        ) : builtins.length === 0 ? (
          <div className="settings-placeholder">Loading…</div>
        ) : (
          <div className="apps-pills">
            {builtins.map(t => (
              <span key={t.id} className="app-pill">
                {t.name}
                <span className="app-pill-type">{t.category}</span>
              </span>
            ))}
          </div>
        )}
      </section>

      {/* Custom app templates */}
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Custom Apps</span>
          <button
            className="settings-btn primary"
            onClick={() => navigate('/app-templates/new')}
          >
            + Add Custom App
          </button>
        </div>
        {customs.length === 0 ? (
          <div className="settings-placeholder">
            No custom apps yet. Click "+ Add Custom App" to write a YAML template for an app not in the library.
          </div>
        ) : (
          <div className="apps-list">
            {customs.map(cp => (
              <div key={cp.id} className="app-row">
                <span className="app-row-name">{cp.name}</span>
                <div className="app-row-actions">
                  <button
                    className="settings-btn secondary settings-btn--sm"
                    onClick={() => navigate(`/app-templates/${cp.id}/edit`)}
                  >
                    Edit
                  </button>
                  <button
                    className="settings-btn danger settings-btn--sm"
                    onClick={() => setConfirmDelete(cp)}
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {confirmDelete && (
        <DeleteConfirmModal
          name={confirmDelete.name}
          onConfirm={handleDelete}
          onCancel={() => setConfirmDelete(null)}
          deleting={deleting}
        />
      )}
    </div>
  )
}

// ── Helpers ───────────────────────────────────────────────────────────────────

const DAY_OF_WEEK_LABELS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

function ordinal(n: number): string {
  const s = ['th', 'st', 'nd', 'rd']
  const v = n % 100
  return n + (s[(v - 20) % 10] || s[v] || s[0])
}

// ── Notifications tab ─────────────────────────────────────────────────────────

function NotificationsTab() {
  // SMTP state
  const [smtp, setSMTP] = useState<SMTPSettings>({ host: '', port: 587, user: '', pass: '', from: '' })
  const [smtpSaving, setSmtpSaving] = useState(false)
  const [smtpMsg, setSmtpMsg] = useState('')

  // Digest schedule state
  const [schedule, setSchedule] = useState<DigestSchedule>({ frequency: 'monthly', day_of_week: 1, day_of_month: 1, send_hour: 8 })
  const [schedSaving, setSchedSaving] = useState(false)
  const [schedMsg, setSchedMsg] = useState('')
  const [sendingNow, setSendingNow] = useState(false)
  const [sendNowMsg, setSendNowMsg] = useState('')

  useEffect(() => {
    smtpSettings.get().then(setSMTP).catch(() => {/* not yet saved — keep defaults */})
    digestSettings.getSchedule().then(setSchedule).catch(() => {/* keep defaults */})
  }, [])

  const saveSMTP = async () => {
    setSmtpSaving(true)
    setSmtpMsg('')
    try {
      await smtpSettings.put(smtp)
      setSmtpMsg('Saved.')
    } catch (e: unknown) {
      setSmtpMsg(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setSmtpSaving(false)
    }
  }

  const saveSchedule = async () => {
    setSchedSaving(true)
    setSchedMsg('')
    try {
      const saved = await digestSettings.putSchedule(schedule)
      setSchedule(saved)
      setSchedMsg('Saved.')
    } catch (e: unknown) {
      setSchedMsg(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setSchedSaving(false)
    }
  }

  const sendNow = async () => {
    setSendingNow(true)
    setSendNowMsg('')
    try {
      const res = await digestSettings.sendNow()
      setSendNowMsg(`Queued for period ${res.period}`)
    } catch (e: unknown) {
      setSendNowMsg(e instanceof Error ? e.message : 'Failed to send')
    } finally {
      setSendingNow(false)
    }
  }

  return (
    <div className="tab-content">
      {/* SMTP */}
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">SMTP</span>
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Host</label>
          <input
            className="settings-input"
            placeholder="smtp.example.com"
            value={smtp.host}
            onChange={e => setSMTP(s => ({ ...s, host: e.target.value }))}
          />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Port</label>
          <input
            className="settings-input"
            placeholder="587"
            style={{ maxWidth: 120 }}
            type="number"
            value={smtp.port}
            onChange={e => setSMTP(s => ({ ...s, port: Number(e.target.value) }))}
          />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Username</label>
          <input
            className="settings-input"
            placeholder="user@example.com"
            value={smtp.user}
            onChange={e => setSMTP(s => ({ ...s, user: e.target.value }))}
          />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Password</label>
          <input
            className="settings-input"
            type="password"
            placeholder="••••••••"
            value={smtp.pass}
            onChange={e => setSMTP(s => ({ ...s, pass: e.target.value }))}
          />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">From</label>
          <input
            className="settings-input"
            placeholder="nora@example.com"
            value={smtp.from}
            onChange={e => setSMTP(s => ({ ...s, from: e.target.value }))}
          />
        </div>
        <div className="settings-actions">
          <button className="settings-btn primary" onClick={saveSMTP} disabled={smtpSaving}>
            {smtpSaving ? 'Saving…' : 'Save'}
          </button>
          {smtpMsg && <span className="settings-status-msg">{smtpMsg}</span>}
        </div>
      </section>

      {/* Digest Schedule */}
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Digest Email</span>
        </div>

        {/* Frequency — segmented control */}
        <div className="settings-field-row">
          <label className="settings-label">Frequency</label>
          <div className="settings-segmented">
            {(['daily', 'weekly', 'monthly'] as DigestFrequency[]).map(f => (
              <button
                key={f}
                className={`settings-seg-btn${schedule.frequency === f ? ' active' : ''}`}
                onClick={() => setSchedule(s => ({ ...s, frequency: f }))}
              >
                {f.charAt(0).toUpperCase() + f.slice(1)}
              </button>
            ))}
          </div>
        </div>

        {/* Day-of-week dropdown — weekly only */}
        {schedule.frequency === 'weekly' && (
          <div className="settings-field-row">
            <label className="settings-label">Send on</label>
            <select
              className="settings-input settings-select"
              value={schedule.day_of_week}
              onChange={e => setSchedule(s => ({ ...s, day_of_week: Number(e.target.value) }))}
            >
              {DAY_OF_WEEK_LABELS.map((label, i) => (
                <option key={i} value={i}>{label}</option>
              ))}
            </select>
          </div>
        )}

        {/* Day-of-month dropdown — monthly only */}
        {schedule.frequency === 'monthly' && (
          <div className="settings-field-row">
            <label className="settings-label">Send on day</label>
            <select
              className="settings-input settings-select"
              value={schedule.day_of_month}
              onChange={e => setSchedule(s => ({ ...s, day_of_month: Number(e.target.value) }))}
            >
              {Array.from({ length: 28 }, (_, i) => i + 1).map(d => (
                <option key={d} value={d}>{ordinal(d)}</option>
              ))}
            </select>
          </div>
        )}

        <div className="settings-field-row">
          <label className="settings-label">Send time</label>
          <select
            className="settings-input settings-select"
            value={schedule.send_hour ?? 8}
            onChange={e => setSchedule(s => ({ ...s, send_hour: Number(e.target.value) }))}
          >
            {Array.from({ length: 24 }, (_, h) => (
              <option key={h} value={h}>
                {String(h).padStart(2, '0')}:00
              </option>
            ))}
          </select>
        </div>

        <div className="settings-actions">
          <button className="settings-btn primary" onClick={saveSchedule} disabled={schedSaving}>
            {schedSaving ? 'Saving…' : 'Save schedule'}
          </button>
          <button className="settings-btn secondary" onClick={sendNow} disabled={sendingNow}>
            {sendingNow ? 'Sending…' : 'Send test digest now'}
          </button>
          {schedMsg && <span className="settings-status-msg">{schedMsg}</span>}
          {sendNowMsg && <span className="settings-status-msg">{sendNowMsg}</span>}
        </div>
      </section>
    </div>
  )
}

// ── Instance Metrics tab ──────────────────────────────────────────────────────

function MetricsTab() {
  return (
    <div className="tab-content">
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">NORA Process</span>
        </div>
        <div className="settings-kv-grid">
          <span className="settings-kv-key">Version</span><span className="settings-kv-val">v0.1.0</span>
          <span className="settings-kv-key">Uptime</span><span className="settings-kv-val">—</span>
          <span className="settings-kv-key">Go runtime</span><span className="settings-kv-val">go1.22</span>
          <span className="settings-kv-key">Goroutines</span><span className="settings-kv-val">—</span>
        </div>
      </section>

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Database</span>
        </div>
        <div className="settings-kv-grid">
          <span className="settings-kv-key">Engine</span><span className="settings-kv-val">SQLite</span>
          <span className="settings-kv-key">File size</span><span className="settings-kv-val">—</span>
          <span className="settings-kv-key">Last vacuum</span><span className="settings-kv-val">—</span>
        </div>
        <div className="settings-actions" style={{ marginTop: 12 }}>
          <button className="settings-btn secondary">Run Vacuum</button>
          <button className="settings-btn secondary">Export DB</button>
        </div>
      </section>

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Retention Policy</span>
        </div>
        <div className="settings-kv-grid">
          <span className="settings-kv-key">Raw events</span><span className="settings-kv-val">7 days</span>
          <span className="settings-kv-key">Hourly rollups</span><span className="settings-kv-val">90 days</span>
          <span className="settings-kv-key">Daily rollups</span><span className="settings-kv-val">Forever</span>
          <span className="settings-kv-key">Error events</span><span className="settings-kv-val">90 days</span>
        </div>
      </section>

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Resource Usage</span>
        </div>
        <div className="settings-resource-bars">
          <div className="settings-resource-row">
            <span className="settings-resource-label">CPU</span>
            <div className="settings-progress-track">
              <div className="settings-progress-fill" style={{ width: '12%' }} />
            </div>
            <span className="settings-resource-pct">12%</span>
          </div>
          <div className="settings-resource-row">
            <span className="settings-resource-label">MEM</span>
            <div className="settings-progress-track">
              <div className="settings-progress-fill" style={{ width: '34%' }} />
            </div>
            <span className="settings-resource-pct">34%</span>
          </div>
          <div className="settings-resource-row">
            <span className="settings-resource-label">DISK</span>
            <div className="settings-progress-track">
              <div className="settings-progress-fill" style={{ width: '8%' }} />
            </div>
            <span className="settings-resource-pct">8%</span>
          </div>
        </div>
      </section>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function Settings() {
  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = (searchParams.get('tab') as Tab) || 'apps'

  return (
    <>
      <Topbar title="Settings" />
      <div className="settings-tabs-bar">
        {TABS.map(t => (
          <button
            key={t.id}
            className={`settings-tab${activeTab === t.id ? ' active' : ''}`}
            onClick={() => setSearchParams({ tab: t.id })}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div className="content">
        {activeTab === 'apps' && <AppsTab />}
        {activeTab === 'notifications' && <NotificationsTab />}
        {activeTab === 'metrics' && <MetricsTab />}
      </div>
    </>
  )
}
