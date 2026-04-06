import { useState, useEffect, useRef, Fragment } from 'react'
import pkgJson from '../../package.json'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { InfraIntegrations } from './Integrations'
import { appTemplates, digestSettings, digestReport, smtpSettings, metrics, users, push, notifyRules, jobsApi, passwordPolicy as passwordPolicyApi, mfaSettings, totp as totpApi, digestRegistry } from '../api/client'
import { SlidePanel } from '../components/SlidePanel'
import { useAuth } from '../context/AuthContext'
import { usePushSubscription } from '../hooks/usePushSubscription'
import type {
  AppTemplate,
  CreateRuleInput,
  CustomProfile,
  DigestFrequency,
  DigestRegistryEntry,
  DigestSchedule,
  InstanceMetrics,
  Job,
  JobRunResult,
  Rule,
  RuleCondition,
  RuleConditionLogic,
  RuleSource,
  Severity,
  PasswordPolicy,
  SMTPSettings,
  User,
} from '../api/types'

import './Settings.css'
import '../styles/Modal.css'

// ── App template icon (tries CDN, falls back to initial letter) ───────────────

function AppTemplateIcon({ id, icon, name }: { id: string; icon?: string; name: string }) {
  const [svgFailed, setSvgFailed] = useState(false)
  const [pngFailed, setPngFailed] = useState(false)
  const cdnName = icon ?? id
  const CDN = 'https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons'

  if (!svgFailed) {
    return (
      <img
        src={`${CDN}/svg/${cdnName}.svg`}
        alt={name}
        style={{ width: 20, height: 20, flexShrink: 0 }}
        onError={() => setSvgFailed(true)}
      />
    )
  }
  if (!pngFailed) {
    return (
      <img
        src={`${CDN}/png/${cdnName}.png`}
        alt={name}
        style={{ width: 20, height: 20, flexShrink: 0 }}
        onError={() => setPngFailed(true)}
      />
    )
  }
  return (
    <span style={{
      width: 20, height: 20, flexShrink: 0,
      borderRadius: 4,
      background: 'var(--bg4)',
      border: '1px solid var(--border)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      fontSize: 10, fontFamily: 'var(--mono)', color: 'var(--text3)',
    }}>
      {name.charAt(0).toUpperCase()}
    </span>
  )
}

type Tab = 'apps' | 'notifications' | 'notify_rules' | 'metrics' | 'users' | 'jobs' | 'digest_registry'

const TABS: { id: Tab; label: string }[] = [
  { id: 'apps', label: 'Apps' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'notify_rules', label: 'Notify Rules' },
  { id: 'digest_registry', label: 'Digest Registry' },
  { id: 'jobs', label: 'Jobs' },
  { id: 'users', label: 'Users' },
  { id: 'metrics', label: 'Instance' },
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
  const [reloading, setReloading] = useState(false)

  const handleReload = async () => {
    setReloading(true)
    setLoadError('')
    try {
      await appTemplates.reload()
      const [bt, ct] = await Promise.all([appTemplates.list(), appTemplates.listCustom()])
      setBuiltins([...bt.data].sort((a, b) => a.name.localeCompare(b.name)))
      setCustoms(ct.data ?? [])
    } catch {
      setLoadError('Template reload failed')
    } finally {
      setReloading(false)
    }
  }

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
          <button
            className="settings-btn secondary"
            onClick={handleReload}
            disabled={reloading}
          >
            {reloading ? 'Reloading…' : 'Reload Templates'}
          </button>
        </div>
        {loadError ? (
          <div className="settings-placeholder" style={{ color: 'var(--red)' }}>{loadError}</div>
        ) : builtins.length === 0 ? (
          <div className="settings-placeholder">Loading…</div>
        ) : (
          <div className="apps-pills">
            {builtins.map(t => (
              <span key={t.id} className="app-pill">
                <AppTemplateIcon id={t.id} icon={t.icon} name={t.name} />
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

// ── Web Push section ──────────────────────────────────────────────────────────

function WebPushSection() {
  const { isSupported, isSubscribed, isLoading, subscribe, unsubscribe } = usePushSubscription()
  const [permission, setPermission] = useState<NotificationPermission | 'unknown'>('unknown')
  const [testMsg, setTestMsg] = useState('')
  const [testing, setTesting] = useState(false)
  const [actionError, setActionError] = useState('')
  const [subscriptions, setSubscriptions] = useState<import('../api/client').PushSubscription[]>([])
  const [removingId, setRemovingId] = useState<string | null>(null)

  useEffect(() => {
    if (typeof Notification !== 'undefined') {
      setPermission(Notification.permission)
    }
  }, [])

  const loadSubscriptions = () => {
    push.listSubscriptions()
      .then(res => setSubscriptions(res.data))
      .catch(() => {/* not critical */})
  }

  useEffect(() => {
    loadSubscriptions()
  }, [])

  // Reload the list whenever subscription state changes (subscribe/unsubscribe on this device).
  useEffect(() => {
    loadSubscriptions()
  }, [isSubscribed])

  const handleSubscribe = async () => {
    setActionError('')
    try {
      // Request notification permission first if needed
      if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
        const result = await Notification.requestPermission()
        setPermission(result)
        if (result !== 'granted') return
      }
      await subscribe()
      setPermission('granted')
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : 'Failed to subscribe')
    }
  }

  const handleUnsubscribe = async () => {
    setActionError('')
    try {
      await unsubscribe()
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : 'Failed to unsubscribe')
    }
  }

  const handleRemove = async (id: string) => {
    setRemovingId(id)
    try {
      await push.removeSubscription(id)
      setSubscriptions(prev => prev.filter(s => s.id !== id))
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : 'Failed to remove subscription')
    } finally {
      setRemovingId(null)
    }
  }

  const handleTest = async () => {
    setTesting(true)
    setTestMsg('')
    try {
      await push.test()
      setTestMsg('Test notification sent.')
    } catch (e: unknown) {
      setTestMsg(e instanceof Error ? e.message : 'Test failed')
    } finally {
      setTesting(false)
    }
  }

  const formatEndpoint = (endpoint: string) => {
    try {
      const url = new URL(endpoint)
      return url.hostname
    } catch {
      return endpoint.slice(0, 40) + '…'
    }
  }

  return (
    <section className="settings-section">
      <div className="section-header">
        <span className="section-title">Web Push Notifications</span>
      </div>

      {!isSupported ? (
        <div className="push-status push-status--unsupported">
          <span className="push-status-dot push-status-dot--off">✕</span>
          Not supported in this browser. Web Push requires HTTPS or localhost.
        </div>
      ) : permission === 'denied' ? (
        <div className="push-status push-status--denied">
          <span className="push-status-dot push-status-dot--off">✕</span>
          Notifications are blocked. Reset permissions in your browser settings, then reload.
        </div>
      ) : (
        <div className="push-status-row">
          <div className={`push-status ${isSubscribed ? 'push-status--on' : 'push-status--off'}`}>
            <span className={`push-status-dot ${isSubscribed ? 'push-status-dot--on' : 'push-status-dot--off'}`}>
              {isSubscribed ? '●' : '○'}
            </span>
            {isSubscribed ? 'Subscribed on this device' : 'Not subscribed'}
          </div>
          <div className="settings-actions" style={{ marginTop: 12 }}>
            {isSubscribed ? (
              <button className="settings-btn secondary" onClick={handleUnsubscribe} disabled={isLoading}>
                {isLoading ? 'Working…' : 'Unsubscribe this device'}
              </button>
            ) : (
              <button className="settings-btn primary" onClick={handleSubscribe} disabled={isLoading}>
                {isLoading ? 'Working…' : 'Enable Notifications'}
              </button>
            )}
            {isSubscribed && (
              <button className="settings-btn secondary" onClick={handleTest} disabled={testing}>
                {testing ? 'Sending…' : 'Send test notification'}
              </button>
            )}
            {actionError && <span className="settings-status-msg" style={{ color: 'var(--red)' }}>{actionError}</span>}
            {testMsg && <span className="settings-status-msg">{testMsg}</span>}
          </div>
        </div>
      )}

      {subscriptions.length > 0 && (
        <div style={{ marginTop: 20 }}>
          <div style={{ fontSize: 12, color: 'var(--text2)', marginBottom: 8, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            Registered devices ({subscriptions.length})
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {subscriptions.map(sub => (
              <div key={sub.id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', background: 'var(--surface2)', borderRadius: 6, padding: '8px 12px', fontSize: 13 }}>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                  <span style={{ color: 'var(--text)', fontFamily: 'monospace', fontSize: 12 }}>
                    {formatEndpoint(sub.endpoint)}
                  </span>
                  <span style={{ color: 'var(--text2)', fontSize: 11 }}>
                    Added {new Date(sub.created_at).toLocaleDateString()}
                  </span>
                </div>
                <button
                  className="settings-btn secondary"
                  style={{ padding: '4px 10px', fontSize: 12 }}
                  disabled={removingId === sub.id}
                  onClick={() => handleRemove(sub.id)}
                >
                  {removingId === sub.id ? '…' : 'Remove'}
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </section>
  )
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
  const [schedule, setSchedule] = useState<DigestSchedule>({ frequency: 'weekly', day_of_week: 5, day_of_month: 1, send_hour: 17 })
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

  // Compute period string from the currently selected frequency so both
  // Send now and Preview report always reflect what's shown in the UI.
  // All calculations use local (browser) date parts — never toISOString() which is UTC.
  const currentPeriod = (): string => {
    const now = new Date()
    const y = now.getFullYear()
    const mo = now.getMonth()
    const day = now.getDate()
    if (schedule.frequency === 'daily') {
      return `${y}-${String(mo + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`
    }
    if (schedule.frequency === 'weekly') {
      // ISO week number using local date parts
      const d = new Date(y, mo, day)
      const dow = d.getDay() || 7           // Mon=1 … Sun=7
      d.setDate(d.getDate() + 4 - dow)      // nearest Thursday
      const yearStart = new Date(d.getFullYear(), 0, 1)
      const week = Math.ceil(((d.getTime() - yearStart.getTime()) / 86400000 + 1) / 7)
      return `${d.getFullYear()}-W${String(week).padStart(2, '0')}`
    }
    return `${y}-${String(mo + 1).padStart(2, '0')}` // YYYY-MM
  }

  const sendNow = async () => {
    setSendingNow(true)
    setSendNowMsg('')
    try {
      const res = await digestSettings.sendNow(currentPeriod())
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
      <WebPushSection />

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
            value={schedule.send_hour ?? 17}
            onChange={e => setSchedule(s => ({ ...s, send_hour: Number(e.target.value) }))}
          >
            {Array.from({ length: 24 }, (_, h) => (
              <option key={h} value={h}>
                {String(h).padStart(2, '0')}:00
              </option>
            ))}
          </select>
          {schedule.timezone && (
            <span className="settings-hint">{schedule.timezone}</span>
          )}
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
            title={!smtpConfigured ? 'Configure SMTP first' : 'Email the digest for the current period'}
          >
            {sendingNow ? 'Sending…' : 'Send now'}
          </button>
          <button
            className="settings-btn secondary"
            onClick={() => window.open(digestReport.url(currentPeriod()), '_blank')}
            title="Preview the digest report for the current period"
          >
            Preview report
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
      <div className="instance-top-row">
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
              <span className="settings-kv-key">Version</span>
              <span className="settings-kv-val">v{data.version}</span>
              <span className="settings-kv-key">Go</span>
              <span className="settings-kv-val">{data.go_version}</span>
              <span className="settings-kv-key">SQLite</span>
              <span className="settings-kv-val">{data.sqlite_version}</span>
              <span className="settings-kv-key">DB size</span>
              <span className="settings-kv-val">{formatBytes(data.db_size_bytes)}</span>
              <span className="settings-kv-key">Events last 24h</span>
              <span className="settings-kv-val">{data.events_last_24h.toLocaleString()}</span>
              <span className="settings-kv-key">Uptime</span>
              <span className="settings-kv-val">{formatUptime(data.uptime_seconds)}</span>
              <span className="settings-kv-key">GitHub</span>
              <span className="settings-kv-val">
                <a href="https://github.com/Digitalcheffe/N.O.R.A" target="_blank" rel="noopener noreferrer">Digitalcheffe/N.O.R.A</a>
              </span>
              <span className="settings-kv-key">Wiki</span>
              <span className="settings-kv-val">
                <a href="https://github.com/Digitalcheffe/N.O.R.A/wiki" target="_blank" rel="noopener noreferrer">N.O.R.A Wiki</a>
              </span>
            </div>
          ) : null}
        </section>

        <section className="settings-section">
          <div className="section-header">
            <span className="section-title">Tech Stack</span>
          </div>
          <div className="settings-kv-grid">
            {/* Frontend deps — read live from package.json at build time */}
            <span className="settings-kv-key">React</span>
            <span className="settings-kv-val">Frontend UI ({pkgJson.dependencies['react']})</span>
            <span className="settings-kv-key">React Router</span>
            <span className="settings-kv-val">Client-side routing ({pkgJson.dependencies['react-router-dom']})</span>
            <span className="settings-kv-key">Vite</span>
            <span className="settings-kv-val">Build tool ({pkgJson.devDependencies['vite']})</span>
            <span className="settings-kv-key">TypeScript</span>
            <span className="settings-kv-val">Type safety ({pkgJson.devDependencies['typescript']})</span>
            {/* Backend deps — read live from binary via debug.ReadBuildInfo() */}
            {data?.deps.map(dep => (
              <Fragment key={dep.name}>
                <span className="settings-kv-key">{dep.label}</span>
                <span className="settings-kv-val">{dep.version}</span>
              </Fragment>
            ))}
          </div>
        </section>
      </div>

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

const DEFAULT_POLICY: PasswordPolicy = { min_length: 8, require_uppercase: false, require_number: false, require_special: false }

function checkPasswordPolicy(pw: string, policy: PasswordPolicy): string | null {
  if (pw.length < policy.min_length) return `Password must be at least ${policy.min_length} characters`
  if (policy.require_uppercase && !/[A-Z]/.test(pw)) return 'Password must contain at least one uppercase letter'
  if (policy.require_number && !/[0-9]/.test(pw)) return 'Password must contain at least one number'
  if (policy.require_special && !/[^A-Za-z0-9]/.test(pw)) return 'Password must contain at least one special character'
  return null
}

function UsersTab() {
  const { user: currentUser } = useAuth()
  const [userList, setUserList] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // Create form state
  const [newEmail, setNewEmail] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [newRole, setNewRole] = useState<'admin' | 'member'>('member')
  const [creating, setCreating] = useState(false)
  const [createMsg, setCreateMsg] = useState('')

  // Edit user modal state
  const [editUser, setEditUser] = useState<User | null>(null)
  const [editEmail, setEditEmail] = useState('')
  const [editRole, setEditRole] = useState<'admin' | 'member'>('member')
  const [editSaving, setEditSaving] = useState(false)
  const [editMsg, setEditMsg] = useState('')
  const [editPw, setEditPw] = useState('')
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)

  // Password policy state
  const [policy, setPolicy] = useState<PasswordPolicy>(DEFAULT_POLICY)
  const [savingPolicy, setSavingPolicy] = useState(false)
  const [policyMsg, setPolicyMsg] = useState('')

  // Global MFA toggle
  const [mfaRequired, setMfaRequired] = useState(false)
  const [savingMfa, setSavingMfa] = useState(false)
  const [mfaMsg, setMfaMsg] = useState('')

  // Per-user TOTP actions in edit modal
  const [totpActionMsg, setTotpActionMsg] = useState('')
  const [totpActionSaving, setTotpActionSaving] = useState(false)

  const load = () => {
    setLoading(true)
    users.list()
      .then(res => setUserList(res.data ?? []))
      .catch(e => setError(e instanceof Error ? e.message : 'Failed to load users'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
    passwordPolicyApi.get().then(setPolicy).catch(() => {})
    mfaSettings.get().then(r => setMfaRequired(r.required)).catch(() => {})
  }, [])

  const handleCreate = async () => {
    if (!newEmail || !newPassword) {
      setCreateMsg('Email and password are required.')
      return
    }
    const policyErr = checkPasswordPolicy(newPassword, policy)
    if (policyErr) {
      setCreateMsg(policyErr)
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
      setConfirmDeleteId(null)
    } catch {
      // ignore — keep list unchanged
    } finally {
      setDeletingId(null)
    }
  }

  const handleToggleMFA = async () => {
    setSavingMfa(true)
    setMfaMsg('')
    try {
      const res = await mfaSettings.put(!mfaRequired)
      setMfaRequired(res.required)
      setMfaMsg(res.required ? 'MFA requirement enabled.' : 'MFA requirement disabled.')
    } catch (e: unknown) {
      setMfaMsg(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setSavingMfa(false)
    }
  }

  const handleAdminDisableTOTP = async (userId: string) => {
    setTotpActionSaving(true)
    setTotpActionMsg('')
    try {
      await totpApi.adminDisable(userId)
      setTotpActionMsg('TOTP disabled.')
      load()
    } catch (e: unknown) {
      setTotpActionMsg(e instanceof Error ? e.message : 'Failed')
    } finally {
      setTotpActionSaving(false)
    }
  }

  const handleAdminResetGrace = async (userId: string) => {
    setTotpActionSaving(true)
    setTotpActionMsg('')
    try {
      await totpApi.adminResetGrace(userId)
      setTotpActionMsg('Grace login restored.')
      load()
    } catch (e: unknown) {
      setTotpActionMsg(e instanceof Error ? e.message : 'Failed')
    } finally {
      setTotpActionSaving(false)
    }
  }

  const handleUpdateUser = async () => {
    if (!editUser || !editEmail) return
    if (editPw) {
      const policyErr = checkPasswordPolicy(editPw, policy)
      if (policyErr) { setEditMsg(policyErr); return }
    }
    setEditSaving(true)
    setEditMsg('')
    try {
      const updated = await users.update(editUser.id, { email: editEmail, role: editRole })
      if (editPw) {
        await users.setPassword(editUser.id, editPw)
      }
      setUserList(prev => prev.map(u => u.id === updated.id ? updated : u))
      setEditUser(null)
    } catch (e: unknown) {
      setEditMsg(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setEditSaving(false)
    }
  }

  const handleSetTOTPExempt = async (exempt: boolean) => {
    if (!editUser) return
    setTotpActionSaving(true)
    setTotpActionMsg('')
    try {
      await users.setTOTPExempt(editUser.id, exempt)
      setTotpActionMsg(exempt ? 'Marked as MFA exempt.' : 'Exemption removed.')
      load()
    } catch (e: unknown) {
      setTotpActionMsg(e instanceof Error ? e.message : 'Failed')
    } finally {
      setTotpActionSaving(false)
    }
  }

  const handleSavePolicy = async () => {

    setSavingPolicy(true)
    setPolicyMsg('')
    try {
      const saved = await passwordPolicyApi.put(policy)
      setPolicy(saved)
      setPolicyMsg('Policy saved.')
    } catch (e: unknown) {
      setPolicyMsg(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setSavingPolicy(false)
    }
  }

  return (
    <>
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
                  {u.totp_enabled && (
                    <span className="totp-enabled-badge" style={{ marginLeft: 6 }}>MFA</span>
                  )}
                  {u.totp_exempt && (
                    <span className="totp-enabled-badge" style={{ marginLeft: 6, background: 'var(--bg4)', color: 'var(--text3)', border: '1px solid var(--border2)' }}>Exempt</span>
                  )}
                </div>
                <div className="app-row-actions">
                  <span className="settings-kv-val" style={{ fontSize: '0.8em', marginRight: 8 }}>
                    {new Date(u.created_at).toLocaleDateString()}
                  </span>
                  {u.id === currentUser?.id ? (
                    <span className="settings-kv-val" style={{ fontSize: '0.8em' }}>you</span>
                  ) : (
                    <button
                      className="settings-btn settings-btn--sm settings-btn--edit"
                      onClick={() => { setEditUser(u); setEditEmail(u.email); setEditRole(u.role); setEditPw(''); setEditMsg(''); setTotpActionMsg('') }}
                    >
                      Edit
                    </button>
                  )}
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

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Password Policy</span>
        </div>
        <div className="settings-field-row">
          <span className="settings-label">Min length</span>
          <input
            className="pw-policy-length-input"
            type="number"
            min={1}
            max={128}
            value={policy.min_length}
            onChange={e => setPolicy(p => ({ ...p, min_length: parseInt(e.target.value, 10) || 8 }))}
          />
          <div className="pw-policy-requirements">
            <label className="pw-policy-req-item">
              <input
                type="checkbox"
                checked={policy.require_uppercase}
                onChange={e => setPolicy(p => ({ ...p, require_uppercase: e.target.checked }))}
              />
              Uppercase
            </label>
            <label className="pw-policy-req-item">
              <input
                type="checkbox"
                checked={policy.require_number}
                onChange={e => setPolicy(p => ({ ...p, require_number: e.target.checked }))}
              />
              Number
            </label>
            <label className="pw-policy-req-item">
              <input
                type="checkbox"
                checked={policy.require_special}
                onChange={e => setPolicy(p => ({ ...p, require_special: e.target.checked }))}
              />
              Special character
            </label>
          </div>
        </div>
        <div className="settings-actions">
          <button className="settings-btn primary" onClick={handleSavePolicy} disabled={savingPolicy}>
            {savingPolicy ? 'Saving…' : 'Save Policy'}
          </button>
          {policyMsg && <span className="settings-status-msg">{policyMsg}</span>}
        </div>
      </section>

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Multi-Factor Authentication</span>
        </div>
        <p className="settings-placeholder" style={{ marginBottom: 12 }}>
          When enabled, all non-exempt users must enroll in TOTP. Each user gets one grace login to complete enrollment.
        </p>
        <div className="settings-field-row">
          <label className="settings-label">Require MFA</label>
          <label className="pw-policy-req-item">
            <input
              type="checkbox"
              checked={mfaRequired}
              onChange={handleToggleMFA}
              disabled={savingMfa}
            />
            {mfaRequired ? 'Enabled' : 'Disabled'}
          </label>
        </div>
        {mfaMsg && <span className="settings-status-msg" style={{ marginTop: 4, display: 'block' }}>{mfaMsg}</span>}
      </section>
    </div>

    {/* ── Edit User modal ── */}

    {editUser && (
      <div className="modal-backdrop">
        <div className="modal">
          <div className="modal-header">
            <span className="modal-title">Edit User</span>
            <button className="modal-close" onClick={() => setEditUser(null)}>✕</button>
          </div>
          <div className="modal-body">
            <label className="modal-label">Email</label>
            <input
              className="modal-input"
              type="email"
              value={editEmail}
              onChange={e => setEditEmail(e.target.value)}
              style={{ marginBottom: 10 }}
            />

            <label className="modal-label">Role</label>
            <select
              className="modal-input"
              value={editRole}
              onChange={e => setEditRole(e.target.value as 'admin' | 'member')}
              style={{ marginBottom: 10 }}
            >
              <option value="member">member</option>
              <option value="admin">admin</option>
            </select>

            <label className="modal-label">New Password <span style={{ color: 'var(--text3)', fontWeight: 400 }}>(leave blank to keep current)</span></label>
            <input
              className="modal-input"
              type="password"
              placeholder="••••••••"
              value={editPw}
              onChange={e => setEditPw(e.target.value)}
              style={{ marginBottom: 4 }}
            />

            <hr className="modal-section-divider" />

            <label className="modal-label">Two-Factor Authentication</label>
            {totpActionMsg && (
              <div style={{ fontSize: '0.8rem', color: 'var(--accent)', marginBottom: 8 }}>{totpActionMsg}</div>
            )}
            {editUser.totp_enabled ? (
              <button
                className="modal-btn-danger"
                onClick={() => handleAdminDisableTOTP(editUser.id)}
                disabled={totpActionSaving}
              >
                {totpActionSaving ? '…' : 'Disable MFA'}
              </button>
            ) : (
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
                <span style={{ fontSize: '0.8rem', color: 'var(--text3)' }}>
                  {editUser.totp_grace ? 'Not enrolled — grace login available' : 'Not enrolled — grace used'}
                </span>
                {!editUser.totp_grace && (
                  <button
                    className="modal-btn-ghost"
                    onClick={() => handleAdminResetGrace(editUser.id)}
                    disabled={totpActionSaving}
                  >
                    {totpActionSaving ? '…' : 'Reset Grace'}
                  </button>
                )}
              </div>
            )}
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 10 }}>
              <span style={{ fontSize: '0.8rem', color: 'var(--text2)' }}>MFA Exempt:</span>
              <button
                className={editUser.totp_exempt ? 'modal-btn-danger' : 'modal-btn-ghost'}
                onClick={() => handleSetTOTPExempt(!editUser.totp_exempt)}
                disabled={totpActionSaving}
              >
                {editUser.totp_exempt ? 'Remove Exemption' : 'Mark Exempt'}
              </button>
              <span style={{ fontSize: '0.75rem', color: 'var(--text3)' }}>
                {editUser.totp_exempt ? 'Never blocked by global MFA' : 'Subject to global MFA policy'}
              </span>
            </div>

            <hr className="modal-section-divider" />

            <div className="modal-danger-label">Danger Zone</div>
            {confirmDeleteId === editUser.id ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <span style={{ fontSize: '0.85rem', color: 'var(--text2)' }}>Delete {editUser.email}?</span>
                <button
                  className="modal-btn-danger"
                  onClick={() => handleDelete(editUser.id)}
                  disabled={deletingId === editUser.id}
                >
                  {deletingId === editUser.id ? '…' : 'Yes, delete'}
                </button>
                <button className="modal-btn-ghost" onClick={() => setConfirmDeleteId(null)}>Cancel</button>
              </div>
            ) : (
              <button
                className="modal-btn-danger"
                onClick={() => setConfirmDeleteId(editUser.id)}
              >
                Delete User
              </button>
            )}

          </div>
          <div className="modal-footer">
            <button className="modal-btn-primary" onClick={handleUpdateUser} disabled={editSaving || !editEmail}>
              {editSaving ? 'Saving…' : 'Save'}
            </button>
            <button className="modal-btn-ghost" onClick={() => setEditUser(null)}>Cancel</button>
            {editMsg && <span style={{ fontSize: '0.8rem', color: 'var(--red)', marginLeft: 4 }}>{editMsg}</span>}
          </div>
        </div>
      </div>
    )}
    </>
  )
}

// ── Notify Rules tab ──────────────────────────────────────────────────────────

const SEVERITY_OPTIONS: Severity[] = ['debug', 'info', 'warn', 'error', 'critical']
const FIELD_OPTIONS = ['display_text', 'severity', 'source_name', 'event_type']
const OPERATOR_OPTIONS = [
  { value: 'contains', label: 'contains' },
  { value: 'does_not_contain', label: 'does not contain' },
  { value: 'is', label: 'is' },
  { value: 'is_not', label: 'is not' },
]

function emptyRule(): CreateRuleInput {
  return {
    name: '',
    enabled: true,
    source_id: null,
    source_type: null,
    severity: null,
    conditions: [],
    condition_logic: 'AND',
    delivery_email: false,
    delivery_push: false,
    delivery_webhook: false,
    webhook_url: null,
    notif_title: '{display_text}',
    notif_body: 'Severity: {severity}\nSource: {source_name}',
  }
}

interface RulePanelProps {
  rule: CreateRuleInput | null  // null = new rule
  editingId: string | null
  sources: RuleSource[]
  smtpConfigured: boolean
  hasPushSubscription: boolean
  onSave: (input: CreateRuleInput) => Promise<void>
  onClose: () => void
  saving: boolean
  saveError: string
}

function RulePanel({ rule, editingId, sources, smtpConfigured, hasPushSubscription, onSave, onClose, saving, saveError }: RulePanelProps) {
  const [form, setForm] = useState<CreateRuleInput>(rule ?? emptyRule())

  // Sync form when rule prop changes (e.g. open different rule).
  useEffect(() => { setForm(rule ?? emptyRule()) }, [rule])

  const panelRef = useRef<HTMLDivElement>(null)

  function addCondition() {
    setForm(f => ({ ...f, conditions: [...f.conditions, { field: 'display_text', operator: 'contains', value: '' }] }))
  }

  function removeCondition(i: number) {
    setForm(f => ({ ...f, conditions: f.conditions.filter((_, idx) => idx !== i) }))
  }

  function updateCondition(i: number, patch: Partial<RuleCondition>) {
    setForm(f => ({
      ...f,
      conditions: f.conditions.map((c, idx) => idx === i ? { ...c, ...patch } : c),
    }))
  }

  function handleSourceChange(val: string) {
    if (val === '') {
      setForm(f => ({ ...f, source_id: null, source_type: null }))
    } else {
      const src = sources.find(s => (s.id ?? '') === val)
      if (!src) return
      if (src.type === 'app') {
        setForm(f => ({ ...f, source_id: src.id, source_type: 'app' }))
      } else {
        setForm(f => ({ ...f, source_id: null, source_type: src.type }))
      }
    }
  }

  const sourceValue = form.source_type === 'app' ? (form.source_id ?? '') : (form.source_type ?? '')

  return (
    <div className="rule-panel-overlay" onClick={onClose}>
      <div className="rule-panel" ref={panelRef} onClick={e => e.stopPropagation()}>
        <div className="rule-panel-header">
          <span className="rule-panel-title">{editingId ? 'Edit Rule' : 'New Rule'}</span>
          <button className="modal-close" onClick={onClose}>✕</button>
        </div>
        <div className="rule-panel-body">
          <div className="settings-field-row">
            <label className="settings-label">Rule Name</label>
            <input className="settings-input" placeholder="e.g. Sonarr download failures"
              value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} />
          </div>

          <div className="settings-field-row">
            <label className="settings-label">Source</label>
            <select className="settings-input settings-select" value={sourceValue}
              onChange={e => handleSourceChange(e.target.value)}>
              {sources.map(s => (
                <option key={s.id ?? ''} value={s.id ?? ''}>{s.label}</option>
              ))}
            </select>
          </div>

          <div className="settings-field-row">
            <label className="settings-label">Severity</label>
            <select className="settings-input settings-select"
              value={form.severity ?? ''}
              onChange={e => setForm(f => ({ ...f, severity: (e.target.value as Severity) || null }))}>
              <option value="">Any severity</option>
              {SEVERITY_OPTIONS.map(s => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
          </div>

          <div className="settings-field-row">
            <label className="settings-label">Conditions</label>
            <button className="settings-btn secondary settings-btn--sm" onClick={addCondition}>+ Add condition</button>
          </div>
          {form.conditions.length > 0 && (
            <div className="rule-conditions-list">
              {form.conditions.map((c, i) => (
                <div key={i} className="rule-condition-row">
                  <select className="settings-input settings-select rule-cond-field"
                    value={c.field} onChange={e => updateCondition(i, { field: e.target.value })}>
                    {FIELD_OPTIONS.map(f => <option key={f} value={f}>{f}</option>)}
                  </select>
                  <select className="settings-input settings-select rule-cond-op"
                    value={c.operator} onChange={e => updateCondition(i, { operator: e.target.value as RuleCondition['operator'] })}>
                    {OPERATOR_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
                  </select>
                  <input className="settings-input rule-cond-value" placeholder="value"
                    value={c.value} onChange={e => updateCondition(i, { value: e.target.value })} />
                  <button className="settings-btn danger settings-btn--sm" onClick={() => removeCondition(i)}>✕</button>
                </div>
              ))}
            </div>
          )}
          {form.conditions.length > 1 && (
            <div className="rule-logic-row">
              <span className="settings-label">Match:</span>
              <label className="rule-logic-option">
                <input type="radio" name="logic" value="AND"
                  checked={form.condition_logic === 'AND'}
                  onChange={() => setForm(f => ({ ...f, condition_logic: 'AND' as RuleConditionLogic }))} />
                ALL conditions (AND)
              </label>
              <label className="rule-logic-option">
                <input type="radio" name="logic" value="OR"
                  checked={form.condition_logic === 'OR'}
                  onChange={() => setForm(f => ({ ...f, condition_logic: 'OR' as RuleConditionLogic }))} />
                ANY condition (OR)
              </label>
            </div>
          )}

          <div className="settings-field-row">
            <label className="settings-label">Delivery</label>
            <div className="rule-delivery-pills">
            <button
              type="button"
              className={`rule-delivery-pill${form.delivery_email ? ' rule-delivery-pill--on' : ''}${!smtpConfigured ? ' rule-delivery-pill--disabled' : ''}`}
              disabled={!smtpConfigured}
              title={!smtpConfigured ? 'Configure SMTP on the Notifications tab to enable email delivery.' : undefined}
              onClick={() => smtpConfigured && setForm(f => ({ ...f, delivery_email: !f.delivery_email }))}>
              Email
            </button>
            <button
              type="button"
              className={`rule-delivery-pill${form.delivery_push ? ' rule-delivery-pill--on' : ''}${!hasPushSubscription ? ' rule-delivery-pill--disabled' : ''}`}
              disabled={!hasPushSubscription}
              title={!hasPushSubscription ? 'No active push subscriptions. Subscribe from a browser first.' : undefined}
              onClick={() => hasPushSubscription && setForm(f => ({ ...f, delivery_push: !f.delivery_push }))}>
              Web Push
            </button>
            <button
              type="button"
              className={`rule-delivery-pill${form.delivery_webhook ? ' rule-delivery-pill--on' : ''}`}
              onClick={() => setForm(f => ({ ...f, delivery_webhook: !f.delivery_webhook }))}>
              Webhook
            </button>
            </div>
          </div>
          {form.delivery_webhook && (
            <div className="settings-field-row">
              <label className="settings-label">URL</label>
              <input className="settings-input" placeholder="https://hooks.example.com/..."
                value={form.webhook_url ?? ''}
                onChange={e => setForm(f => ({ ...f, webhook_url: e.target.value || null }))} />
            </div>
          )}

          <div className="settings-field-row" style={{ marginBottom: 0 }}>
            <label className="settings-label">Notification</label>
          </div>
          <div className="settings-field-row">
            <label className="settings-label">Title</label>
            <input className="settings-input" value={form.notif_title}
              onChange={e => setForm(f => ({ ...f, notif_title: e.target.value }))} />
          </div>
          <div className="settings-field-row">
            <label className="settings-label">Body</label>
            <textarea className="settings-input rule-body-textarea" rows={3} value={form.notif_body}
              onChange={e => setForm(f => ({ ...f, notif_body: e.target.value }))} />
          </div>
          <div className="rule-tokens-hint">
            Available tokens: <code>{'{display_text}'}</code> <code>{'{severity}'}</code> <code>{'{source_name}'}</code>
          </div>
        </div>
        {form.delivery_email && !smtpConfigured && (
          <div className="rule-delivery-warning" style={{ margin: '0 20px 0' }}>
            Email delivery requires SMTP to be configured. Go to the Notifications tab to set it up.
          </div>
        )}
        <div className="rule-panel-footer">
          {saveError && <span className="settings-status-msg" style={{ color: 'var(--red)' }}>{saveError}</span>}
          <button className="settings-btn secondary" onClick={onClose}>Cancel</button>
          <button className="settings-btn primary" onClick={() => onSave(form)}
            disabled={saving || (form.delivery_email && !smtpConfigured)}>
            {saving ? 'Saving…' : 'Save Rule'}
          </button>
        </div>
      </div>
    </div>
  )
}

interface NotifyRulesTabProps {
  smtpConfigured: boolean
}

function NotifyRulesTab({ smtpConfigured }: NotifyRulesTabProps) {
  const [ruleList, setRuleList] = useState<Rule[]>([])
  const [sources, setSources] = useState<RuleSource[]>([{ id: null, label: 'Any source', type: null }])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [search, setSearch] = useState('')

  // Panel state
  const [panelOpen, setPanelOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<Rule | null>(null)
  const [prefillInput, setPrefillInput] = useState<CreateRuleInput | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')

  const { isSubscribed } = usePushSubscription()

  const [searchParams, setSearchParams] = useSearchParams()

  const load = () => {
    setLoading(true)
    Promise.all([notifyRules.list(), notifyRules.sources()])
      .then(([listRes, srcRes]) => {
        setRuleList(listRes.data ?? [])
        setSources(srcRes.sources)
      })
      .catch(() => setLoadError('Failed to load rules'))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  // Handle prefill from URL params (triggered by "Save as rule" in EventRow).
  useEffect(() => {
    const prefill = searchParams.get('prefill')
    if (prefill) {
      try {
        const data = JSON.parse(decodeURIComponent(prefill)) as CreateRuleInput
        setPrefillInput(data)
        setEditingRule(null)
        setPanelOpen(true)
        // Clear the prefill param so re-renders don't reopen the panel.
        const next = new URLSearchParams(searchParams)
        next.delete('prefill')
        setSearchParams(next, { replace: true })
      } catch {
        // ignore malformed prefill
      }
    }
  }, [searchParams, setSearchParams])

  const openNew = () => {
    setEditingRule(null)
    setPrefillInput(null)
    setSaveError('')
    setPanelOpen(true)
  }

  const openEdit = (rule: Rule) => {
    setEditingRule(rule)
    setPrefillInput(null)
    setSaveError('')
    setPanelOpen(true)
  }

  const closePanel = () => {
    setPanelOpen(false)
    setEditingRule(null)
    setPrefillInput(null)
    setSaveError('')
  }

  const handleSave = async (input: CreateRuleInput) => {
    if (!input.name.trim()) {
      setSaveError('Name is required.')
      return
    }
    setSaving(true)
    setSaveError('')
    try {
      if (editingRule) {
        await notifyRules.update(editingRule.id, input)
      } else {
        await notifyRules.create(input)
      }
      closePanel()
      load()
    } catch (e: unknown) {
      setSaveError(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  const handleToggle = async (rule: Rule) => {
    try {
      const updated = await notifyRules.toggle(rule.id)
      setRuleList(prev => prev.map(r => r.id === updated.id ? updated : r))
    } catch {
      // leave list unchanged
    }
  }

  const handleDelete = async (rule: Rule) => {
    try {
      await notifyRules.delete(rule.id)
      setRuleList(prev => prev.filter(r => r.id !== rule.id))
    } catch {
      // leave list unchanged
    }
  }

  const sourceName = (rule: Rule) => {
    if (!rule.source_id && !rule.source_type) return 'Any source'
    const src = sources.find(s =>
      rule.source_type === 'app' ? s.id === rule.source_id : s.id === rule.source_type
    )
    return src?.label ?? rule.source_id ?? rule.source_type ?? 'Unknown'
  }

  const conditionSummary = (rule: Rule) => {
    if (!rule.conditions || rule.conditions.length === 0) return '(no conditions — fires on gate match only)'
    return rule.conditions.map(c => `${c.field} ${c.operator.replace('_', ' ')} "${c.value}"`).join(` ${rule.condition_logic} `)
  }

  const deliveryLabels = (rule: Rule) => {
    const labels = []
    if (rule.delivery_email) labels.push('Email')
    if (rule.delivery_push) labels.push('Push')
    if (rule.delivery_webhook) labels.push('Webhook')
    return labels.join(' · ') || 'None'
  }

  // Build the panel form input from the editing rule.
  const panelInput: CreateRuleInput | null = editingRule ? {
    name: editingRule.name,
    enabled: editingRule.enabled,
    source_id: editingRule.source_id,
    source_type: editingRule.source_type,
    severity: editingRule.severity,
    conditions: editingRule.conditions,
    condition_logic: editingRule.condition_logic,
    delivery_email: editingRule.delivery_email,
    delivery_push: editingRule.delivery_push,
    delivery_webhook: editingRule.delivery_webhook,
    webhook_url: editingRule.webhook_url,
    notif_title: editingRule.notif_title,
    notif_body: editingRule.notif_body,
  } : prefillInput

  const filtered = ruleList.filter(r =>
    !search || r.name.toLowerCase().includes(search.toLowerCase())
  )

  return (
    <div className="tab-content">
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Notify Rules</span>
        </div>
        <p className="settings-placeholder" style={{ marginBottom: 16 }}>
          Rules fire outbound notifications when an incoming event matches your conditions.
          Every event is evaluated in real time as it enters NORA.
        </p>
        <div className="rule-list-toolbar">
          <button className="settings-btn primary" onClick={openNew}>+ New Rule</button>
          <input className="settings-input" placeholder="Search rules…" style={{ maxWidth: 240 }}
            value={search} onChange={e => setSearch(e.target.value)} />
        </div>

        {loading ? (
          <div className="settings-placeholder">Loading…</div>
        ) : loadError ? (
          <div className="settings-placeholder" style={{ color: 'var(--red)' }}>{loadError}</div>
        ) : filtered.length === 0 ? (
          <div className="settings-placeholder">
            {search ? 'No rules match your search.' : 'No rules yet. Create your first rule to start getting notified.'}
          </div>
        ) : (
          <div className="rule-list">
            {filtered.map(rule => (
              <div key={rule.id} className={`rule-row${rule.enabled ? '' : ' rule-row--disabled'}`}>
                <div className="rule-row-top">
                  <span className={`rule-status-dot${rule.enabled ? ' rule-status-dot--on' : ''}`}>
                    {rule.enabled ? '●' : '○'}
                  </span>
                  <span className="rule-row-name">
                    {rule.name}
                    {!rule.enabled && <span className="rule-disabled-label"> (disabled)</span>}
                  </span>
                  <span className="rule-row-meta">
                    {rule.severity ? `${rule.severity} · ` : ''}{sourceName(rule)}
                  </span>
                </div>
                <div className="rule-row-bottom">
                  <span className="rule-row-conditions">{conditionSummary(rule)}</span>
                  <span className="rule-row-delivery">{deliveryLabels(rule)}</span>
                </div>
                <div className="rule-row-actions">
                  <button className="settings-btn secondary settings-btn--sm" onClick={() => openEdit(rule)}>Edit</button>
                  <button className="settings-btn secondary settings-btn--sm" onClick={() => handleToggle(rule)}>
                    {rule.enabled ? 'Disable' : 'Enable'}
                  </button>
                  <button className="settings-btn danger settings-btn--sm" onClick={() => handleDelete(rule)}>Delete</button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {panelOpen && (
        <RulePanel
          rule={panelInput}
          editingId={editingRule?.id ?? null}
          sources={sources}
          smtpConfigured={smtpConfigured}
          hasPushSubscription={isSubscribed}
          onSave={handleSave}
          onClose={closePanel}
          saving={saving}
          saveError={saveError}
        />
      )}
    </div>
  )
}

// ── Jobs tab ──────────────────────────────────────────────────────────────────

type RunState = { status: 'idle' | 'running' | 'ok' | 'error'; message?: string; duration?: number }

const CATEGORY_LABELS: Record<string, string> = {
  monitor: 'Monitor',
  scan: 'Scan Engine',
  data: 'Data',
}
const CATEGORY_ORDER = ['monitor', 'scan', 'data']

function JobsTab() {
  const [jobList, setJobList] = useState<Job[]>([])
  const [loading, setLoading] = useState(true)
  const [runState, setRunState] = useState<Record<string, RunState>>({})

  useEffect(() => {
    jobsApi.list()
      .then(r => setJobList(r.data))
      .catch(() => {/* leave empty */})
      .finally(() => setLoading(false))
  }, [])

  const handleRun = async (id: string) => {
    setRunState(prev => ({ ...prev, [id]: { status: 'running' } }))
    try {
      const result: JobRunResult = await jobsApi.run(id)
      setRunState(prev => ({
        ...prev,
        [id]: { status: result.status, message: result.error, duration: result.duration_ms },
      }))
    } catch (e) {
      setRunState(prev => ({ ...prev, [id]: { status: 'error', message: String(e) } }))
    }
    setTimeout(() => {
      setRunState(prev => ({ ...prev, [id]: { status: 'idle' } }))
    }, 3000)
  }

  const grouped = Object.fromEntries(
    CATEGORY_ORDER.map(cat => [cat, jobList.filter(j => j.category === cat)])
  )

  return (
    <div className="tab-content">
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Jobs</span>
        </div>
        <p className="jobs-description">Run built-in background jobs on demand.</p>
        {loading ? (
          <div className="settings-placeholder">Loading…</div>
        ) : (
          CATEGORY_ORDER
            .filter(cat => (grouped[cat]?.length ?? 0) > 0)
            .map(cat => (
              <div key={cat} className="jobs-category">
                <div className="jobs-category-label">{CATEGORY_LABELS[cat] ?? cat.toUpperCase()}</div>
                {grouped[cat].map(job => {
                  const rs = runState[job.id] ?? { status: 'idle' }
                  const isRunning = rs.status === 'running'
                  return (
                    <div key={job.id} className="job-card">
                      <div className="job-card-info">
                        <div className="job-card-name">{job.name}</div>
                        <div className="job-card-desc">{job.description}</div>
                        {job.last_run_at && (
                          <div className="job-card-meta">
                            Last run: {new Date(job.last_run_at).toLocaleString()}
                            {job.last_run_status && (
                              <span className={`job-run-badge job-run-badge--${job.last_run_status}`}>
                                {job.last_run_status}
                              </span>
                            )}
                          </div>
                        )}
                        {rs.status === 'ok' && (
                          <div className="job-card-meta">
                            <span className="job-run-badge job-run-badge--ok">
                              Completed in {rs.duration}ms
                            </span>
                          </div>
                        )}
                        {rs.status === 'error' && (
                          <div className="job-card-meta">
                            <span className="job-run-badge job-run-badge--error">
                              Failed{rs.message ? `: ${rs.message}` : ''}
                            </span>
                          </div>
                        )}
                      </div>
                      <div className="job-card-action">
                        <button
                          className="settings-btn secondary"
                          onClick={() => handleRun(job.id)}
                          disabled={isRunning}
                        >
                          {isRunning ? (
                            <span className="job-btn-running">
                              <span className="job-spinner" />
                              Running…
                            </span>
                          ) : 'Run Now'}
                        </button>
                      </div>
                    </div>
                  )
                })}
              </div>
            ))
        )}
      </section>
    </div>
  )
}

// ── Digest Registry tab ───────────────────────────────────────────────────────

// ── DigestRegistryEntryBody ───────────────────────────────────────────────────

function DigestRegistryEntryBody({
  entry,
  onClose,
  onUpdate,
  onDelete,
}: {
  entry: DigestRegistryEntry
  onClose: () => void
  onUpdate: (updated: DigestRegistryEntry) => void
  onDelete: (id: string) => void
}) {
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    setConfirmDelete(false)
  }, [entry.id])

  const handleActiveToggle = async () => {
    try {
      await digestRegistry.setActive(entry.id, !entry.active)
      onUpdate({ ...entry, active: !entry.active })
    } catch { /* leave unchanged */ }
  }

  const handleDelete = async () => {
    setDeleting(true)
    try {
      await digestRegistry.delete(entry.id)
      onDelete(entry.id)
      onClose()
    } catch {
      setDeleting(false)
      setConfirmDelete(false)
    }
  }

  return (
    <div className="dr-panel-body">

      {/* App icon + profile */}
      <div className="dr-panel-profile">
        <AppTemplateIcon id={entry.profile_id} name={entry.profile_id} />
        <span className="dr-panel-profile-name">{entry.profile_id}</span>
      </div>

      {/* Label / Name / Type / Source — inline */}
      <div className="dr-panel-field dr-panel-field--inline">
        <label className="dr-panel-label">Label</label>
        <span className="dr-panel-value">{entry.label}</span>
      </div>

      <div className="dr-panel-field dr-panel-field--inline">
        <label className="dr-panel-label">Name</label>
        <span className="dr-panel-value">{entry.name}</span>
      </div>

      <div className="dr-panel-field dr-panel-field--inline">
        <label className="dr-panel-label">Type</label>
        <span className="dr-panel-value">{entry.entry_type}</span>
      </div>

      <div className="dr-panel-field dr-panel-field--inline">
        <label className="dr-panel-label">Source</label>
        <span className="dr-panel-value">{entry.source}</span>
      </div>

      {/* Active toggle */}
      <div className="dr-panel-field">
        <label className="dr-panel-label">Status</label>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <button
            className={`settings-btn ${entry.active ? 'primary' : 'secondary'}`}
            onClick={handleActiveToggle}
          >
            {entry.active ? 'Active' : 'Inactive'}
          </button>
          {!entry.active && (
            <span className="settings-hint">Not included in digest generation.</span>
          )}
        </div>
      </div>

      {/* Profile source path */}
      <div className="dr-panel-field">
        <label className="dr-panel-label">App profile</label>
        {entry.profile_source
          ? <code className="dr-panel-code">{entry.profile_source}</code>
          : <span className="settings-hint">Source path not recorded.</span>
        }
      </div>

      {/* Delete */}
      <div className="dr-panel-danger-zone">
        {!confirmDelete ? (
          <button
            className="settings-btn danger"
            disabled={entry.active}
            title={entry.active ? 'Deactivate before deleting' : undefined}
            onClick={() => setConfirmDelete(true)}
          >
            Delete entry
          </button>
        ) : (
          <div className="dr-panel-confirm">
            <p className="modal-delete-warning">
              Permanently remove <strong>{entry.profile_id}/{entry.name}</strong>? This cannot be undone.
            </p>
            <div style={{ display: 'flex', gap: 8 }}>
              <button className="settings-btn secondary" onClick={() => setConfirmDelete(false)}>Cancel</button>
              <button className="settings-btn danger" onClick={handleDelete} disabled={deleting}>
                {deleting ? 'Deleting…' : 'Delete permanently'}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function DigestRegistryTab() {
  const [entries, setEntries] = useState<DigestRegistryEntry[]>([])
  const [loadError, setLoadError] = useState('')
  const [selected, setSelected] = useState<DigestRegistryEntry | null>(null)
  const [filterProfile, setFilterProfile] = useState('')

  useEffect(() => {
    digestRegistry.list()
      .then(r => setEntries(r.data ?? []))
      .catch(() => setLoadError('Failed to load digest registry'))
  }, [])

  const handleUpdate = (updated: DigestRegistryEntry) => {
    setEntries(prev => prev.map(e => e.id === updated.id ? updated : e))
    setSelected(updated)
  }

  const handleDelete = (id: string) => {
    setEntries(prev => prev.filter(e => e.id !== id))
    setSelected(null)
  }

  const profileOptions = Array.from(new Set(entries.map(e => e.profile_id))).sort()
  const filtered = filterProfile ? entries.filter(e => e.profile_id === filterProfile) : entries

  if (loadError) {
    return <div className="tab-content"><p style={{ color: 'var(--red)' }}>{loadError}</p></div>
  }

  return (
    <div className="tab-content">
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Digest Registry</span>
        </div>
        <p className="dr-overview">
          A persistent record of every digest entry declared across all app profiles. Entries are
          automatically reconciled at startup — new categories are inserted, renamed labels are
          updated, and categories removed from a profile are deactivated rather than deleted.
          Click the settings icon on any row to toggle its active state or view its full profile definition.
        </p>

        {entries.length === 0 ? (
          <p className="settings-empty">
            No digest entries registered. Add apps with profiles to populate the registry.
          </p>
        ) : (
          <>
            <div className="dr-filter-row">
              <select
                className="modal-input dr-filter-select"
                value={filterProfile}
                onChange={e => setFilterProfile(e.target.value)}
              >
                <option value="">All app profiles</option>
                {profileOptions.map(p => (
                  <option key={p} value={p}>{p}</option>
                ))}
              </select>
            </div>
          <div className="dr-table-scroll">
            <table className="settings-metrics-table">
              <thead>
                <tr>
                  <th>App Profile</th>
                  <th>Label</th>
                  <th className="dr-col--desktop">Type</th>
                  <th className="dr-col--desktop">Source</th>
                  <th className="dr-col--desktop">Status</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(entry => (
                  <tr
                    key={entry.id}
                    className={entry.active ? '' : 'dr-row--inactive'}
                    style={{ cursor: 'pointer' }}
                    onClick={() => setSelected(entry)}
                  >
                    <td>
                      <div className="dr-profile-cell">
                        <AppTemplateIcon id={entry.profile_id} name={entry.profile_id} />
                        <span>{entry.profile_id}</span>
                      </div>
                    </td>
                    <td>{entry.label}</td>
                    <td className="dr-col--desktop"><span className="app-pill-type">{entry.entry_type}</span></td>
                    <td className="dr-col--desktop"><span className="app-pill-type">{entry.source}</span></td>
                    <td className="dr-col--desktop">
                      <span className={`dr-status-dot ${entry.active ? 'dr-status-dot--on' : 'dr-status-dot--off'}`}>
                        {entry.active ? '● Active' : '● Inactive'}
                      </span>
                    </td>
                    <td style={{ textAlign: 'right' }}>
                      <button
                        className="check-settings-btn"
                        title="Settings"
                        onClick={e => { e.stopPropagation(); setSelected(entry) }}
                      >
                        ⚙
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          </>
        )}
      </section>

      <SlidePanel
        open={selected !== null}
        onClose={() => setSelected(null)}
        title={selected?.label ?? ''}
        subtitle={selected ? `${selected.profile_id} / ${selected.name}` : ''}
        width={520}
        footer={<></>}
      >
        {selected && (
          <DigestRegistryEntryBody
            entry={selected}
            onClose={() => setSelected(null)}
            onUpdate={handleUpdate}
            onDelete={handleDelete}
          />
        )}
      </SlidePanel>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function Settings() {
  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = (searchParams.get('tab') as Tab) || 'apps'
  const [smtpConfigured, setSmtpConfigured] = useState(false)

  useEffect(() => {
    smtpSettings.get()
      .then(s => setSmtpConfigured(!!s.host))
      .catch(() => {/* not configured */})
  }, [])

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
        {activeTab === 'notify_rules' && <NotifyRulesTab smtpConfigured={smtpConfigured} />}
        {activeTab === 'metrics' && <MetricsTab />}
        {activeTab === 'users' && <UsersTab />}
        {activeTab === 'jobs' && <JobsTab />}
        {activeTab === 'digest_registry' && <DigestRegistryTab />}
      </div>
    </>
  )
}
