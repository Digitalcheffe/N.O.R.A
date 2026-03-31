import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { InfraIntegrations } from './Integrations'
import { appTemplates, digestSettings, digestReport, smtpSettings, metrics, users } from '../api/client'
import type {
  AppTemplate,
  CustomProfile,
  DigestFrequency,
  DigestSchedule,
  InstanceMetrics,
  SMTPSettings,
  User,
} from '../api/types'

import './Settings.css'

type Tab = 'apps' | 'notifications' | 'metrics' | 'users'

const TABS: { id: Tab; label: string }[] = [
  { id: 'apps', label: 'Apps' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'metrics', label: 'Instance Metrics' },
  { id: 'users', label: 'Users' },
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
  const [smtp, setSMTP] = useState<SMTPSettings>({ host: '', port: 587, user: '', pass: '', from: '', to: '' })
  const [smtpSaving, setSmtpSaving] = useState(false)
  const [smtpMsg, setSmtpMsg] = useState('')
  const [smtpTesting, setSmtpTesting] = useState(false)
  const [smtpTestMsg, setSmtpTestMsg] = useState('')
  const [smtpConfigured, setSmtpConfigured] = useState(false)

  // Digest schedule state
  const [schedule, setSchedule] = useState<DigestSchedule>({ frequency: 'monthly', day_of_week: 1, day_of_month: 1, send_hour: 8 })
  const [schedSaving, setSchedSaving] = useState(false)
  const [schedMsg, setSchedMsg] = useState('')
  const [sendingNow, setSendingNow] = useState(false)
  const [sendNowMsg, setSendNowMsg] = useState('')

  useEffect(() => {
    smtpSettings.get()
      .then(s => { setSMTP(s); setSmtpConfigured(!!s.host) })
      .catch(() => {/* not yet saved — keep defaults */})
    digestSettings.getSchedule().then(setSchedule).catch(() => {/* keep defaults */})
  }, [])

  const saveSMTP = async () => {
    setSmtpSaving(true)
    setSmtpMsg('')
    try {
      const saved = await smtpSettings.put(smtp)
      setSmtpConfigured(!!saved.host)
      setSmtpMsg('Saved.')
    } catch (e: unknown) {
      setSmtpMsg(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setSmtpSaving(false)
    }
  }

  const testSMTP = async () => {
    setSmtpTesting(true)
    setSmtpTestMsg('')
    try {
      const res = await smtpSettings.test()
      setSmtpTestMsg(`Test email sent to ${res.to}`)
    } catch (e: unknown) {
      setSmtpTestMsg(e instanceof Error ? e.message : 'Test failed')
    } finally {
      setSmtpTesting(false)
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
        <div className="settings-field-row">
          <label className="settings-label">To</label>
          <input
            className="settings-input"
            placeholder="admin@example.com"
            value={smtp.to}
            onChange={e => setSMTP(s => ({ ...s, to: e.target.value }))}
          />
        </div>
        <div className="settings-actions">
          <button className="settings-btn primary" onClick={saveSMTP} disabled={smtpSaving}>
            {smtpSaving ? 'Saving…' : 'Save'}
          </button>
          <button
            className="settings-btn secondary"
            onClick={testSMTP}
            disabled={smtpTesting || !smtpConfigured}
            title={!smtpConfigured ? 'Configure and save SMTP first' : 'Send a test email to the configured to address'}
          >
            {smtpTesting ? 'Sending…' : 'Send test email'}
          </button>
          {smtpMsg && <span className="settings-status-msg">{smtpMsg}</span>}
          {smtpTestMsg && <span className="settings-status-msg">{smtpTestMsg}</span>}
        </div>
      </section>

      {/* Web Push */}
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Web Push</span>
        </div>
        <div className="settings-placeholder" style={{ fontStyle: 'italic' }}>
          Push notifications — coming soon
        </div>
      </section>

      {/* Digest Schedule */}
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Digest Email</span>
        </div>

        {!smtpConfigured && (
          <div className="settings-smtp-warning">
            SMTP is not configured. Set up SMTP above before enabling the digest schedule.
          </div>
        )}

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
          <button
            className="settings-btn primary"
            onClick={saveSchedule}
            disabled={schedSaving || !smtpConfigured}
            title={!smtpConfigured ? 'Configure SMTP first' : undefined}
          >
            {schedSaving ? 'Saving…' : 'Save schedule'}
          </button>
          <button
            className="settings-btn secondary"
            onClick={sendNow}
            disabled={sendingNow || !smtpConfigured}
            title={!smtpConfigured ? 'Configure SMTP first' : undefined}
          >
            {sendingNow ? 'Sending…' : 'Send test digest now'}
          </button>
          <button
            className="settings-btn secondary"
            onClick={() => window.open(digestReport.url(), '_blank')}
          >
            View Report
          </button>
          {schedMsg && <span className="settings-status-msg">{schedMsg}</span>}
          {sendNowMsg && <span className="settings-status-msg">{sendNowMsg}</span>}
        </div>
      </section>
    </div>
  )
}

// ── Instance Metrics tab ──────────────────────────────────────────────────────

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return `${h}h ${m}m`
}

function MetricsTab() {
  const [data, setData] = useState<InstanceMetrics | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    void (async () => {
      setLoading(true)
      try {
        const data = await metrics.instance()
        setData(data)
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Failed to load metrics')
      } finally {
        setLoading(false)
      }
    })()
  }, [])

  return (
    <div className="tab-content">
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Instance</span>
        </div>
        {loading ? (
          <div className="settings-placeholder">Loading…</div>
        ) : error ? (
          <div className="settings-placeholder" style={{ color: 'var(--red)' }}>{error}</div>
        ) : data ? (
          <div className="settings-kv-grid">
            <span className="settings-kv-key">DB size</span>
            <span className="settings-kv-val">{formatBytes(data.db_size_bytes)}</span>
            <span className="settings-kv-key">Events last 24h</span>
            <span className="settings-kv-val">{data.events_last_24h.toLocaleString()}</span>
            <span className="settings-kv-key">Uptime</span>
            <span className="settings-kv-val">{formatUptime(data.uptime_seconds)}</span>
          </div>
        ) : null}
      </section>

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Retention Policy</span>
        </div>
        <div className="settings-kv-grid">
          <span className="settings-kv-key">Debug events</span><span className="settings-kv-val">24 hours</span>
          <span className="settings-kv-key">Info events</span><span className="settings-kv-val">7 days</span>
          <span className="settings-kv-key">Warn events</span><span className="settings-kv-val">30 days</span>
          <span className="settings-kv-key">Error / Critical</span><span className="settings-kv-val">90 days</span>
          <span className="settings-kv-key">Hourly rollups</span><span className="settings-kv-val">90 days</span>
          <span className="settings-kv-key">Daily rollups</span><span className="settings-kv-val">Forever</span>
        </div>
      </section>

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Events per App (last 24h)</span>
        </div>
        {loading ? (
          <div className="settings-placeholder">Loading…</div>
        ) : data && data.app_events_24h.length === 0 ? (
          <div className="settings-placeholder">No events in the last 24 hours.</div>
        ) : data ? (
          <table className="settings-metrics-table">
            <thead>
              <tr>
                <th>App</th>
                <th className="settings-metrics-num">Events</th>
              </tr>
            </thead>
            <tbody>
              {data.app_events_24h.map(row => (
                <tr key={row.app_id}>
                  <td>{row.app_name}</td>
                  <td className="settings-metrics-num">{row.count.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : null}
      </section>
    </div>
  )
}

// ── Users tab ─────────────────────────────────────────────────────────────────

function UsersTab() {
  const [userList, setUserList] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // Create form state
  const [newEmail, setNewEmail] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [newRole, setNewRole] = useState<'admin' | 'member'>('member')
  const [creating, setCreating] = useState(false)
  const [createMsg, setCreateMsg] = useState('')

  const [deletingId, setDeletingId] = useState<string | null>(null)

  const load = () => {
    setLoading(true)
    users.list()
      .then(res => setUserList(res.data ?? []))
      .catch(e => setError(e instanceof Error ? e.message : 'Failed to load users'))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleCreate = async () => {
    if (!newEmail || !newPassword) {
      setCreateMsg('Email and password are required.')
      return
    }
    setCreating(true)
    setCreateMsg('')
    try {
      await users.create({ email: newEmail, password: newPassword, role: newRole })
      setNewEmail('')
      setNewPassword('')
      setNewRole('member')
      setCreateMsg('User created.')
      load()
    } catch (e: unknown) {
      setCreateMsg(e instanceof Error ? e.message : 'Create failed')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    setDeletingId(id)
    try {
      await users.delete(id)
      setUserList(prev => prev.filter(u => u.id !== id))
    } catch {
      // ignore — keep list unchanged
    } finally {
      setDeletingId(null)
    }
  }

  return (
    <div className="tab-content">
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Users</span>
        </div>
        {loading ? (
          <div className="settings-placeholder">Loading…</div>
        ) : error ? (
          <div className="settings-placeholder" style={{ color: 'var(--red)' }}>{error}</div>
        ) : userList.length === 0 ? (
          <div className="settings-placeholder">No users yet.</div>
        ) : (
          <div className="apps-list">
            {userList.map(u => (
              <div key={u.id} className="app-row">
                <div>
                  <span className="app-row-name">{u.email}</span>
                  <span className="app-pill-type" style={{ marginLeft: 8 }}>{u.role}</span>
                </div>
                <div className="app-row-actions">
                  <span className="settings-kv-val" style={{ fontSize: '0.8em', marginRight: 8 }}>
                    {new Date(u.created_at).toLocaleDateString()}
                  </span>
                  <button
                    className="settings-btn danger settings-btn--sm"
                    onClick={() => handleDelete(u.id)}
                    disabled={deletingId === u.id}
                  >
                    {deletingId === u.id ? '…' : 'Remove'}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Invite User</span>
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Email</label>
          <input
            className="settings-input"
            placeholder="user@example.com"
            value={newEmail}
            onChange={e => setNewEmail(e.target.value)}
          />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Password</label>
          <input
            className="settings-input"
            type="password"
            placeholder="Initial password"
            value={newPassword}
            onChange={e => setNewPassword(e.target.value)}
          />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Role</label>
          <div className="settings-segmented">
            {(['member', 'admin'] as const).map(r => (
              <button
                key={r}
                className={`settings-seg-btn${newRole === r ? ' active' : ''}`}
                onClick={() => setNewRole(r)}
              >
                {r.charAt(0).toUpperCase() + r.slice(1)}
              </button>
            ))}
          </div>
        </div>
        <div className="settings-actions">
          <button className="settings-btn primary" onClick={handleCreate} disabled={creating}>
            {creating ? 'Creating…' : 'Add User'}
          </button>
          {createMsg && <span className="settings-status-msg">{createMsg}</span>}
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
        {activeTab === 'users' && <UsersTab />}
      </div>
    </>
  )
}
