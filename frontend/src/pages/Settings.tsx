import { useState, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { InfraIntegrations } from './Integrations'
import { apps as appsApi, appTemplates } from '../api/client'
import type { App, AppTemplate } from '../api/types'
import './Settings.css'

type Tab = 'apps' | 'notifications' | 'metrics'

const TABS: { id: Tab; label: string }[] = [
  { id: 'apps', label: 'Apps' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'metrics', label: 'Instance Metrics' },
]

// ── Add App Modal ─────────────────────────────────────────────────────────────

interface AddAppModalProps {
  onClose: () => void
  onCreated: () => void
}

function AddAppModal({ onClose, onCreated }: AddAppModalProps) {
  const [templates, setTemplates] = useState<AppTemplate[]>([])
  const [loadError, setLoadError] = useState('')
  const [selectedTemplateId, setSelectedTemplateId] = useState('')
  const [name, setName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    appTemplates.list()
      .then(r => {
        setTemplates(r.data)
        if (r.data.length > 0) setSelectedTemplateId(r.data[0].id)
      })
      .catch(() => setLoadError('Failed to load app templates — check backend logs'))
  }, [])

  const handleSubmit = async () => {
    if (!name.trim()) { setError('Name is required'); return }
    setSubmitting(true)
    setError('')
    try {
      await appsApi.create({ name: name.trim(), profile_id: selectedTemplateId || undefined })
      onCreated()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to create app')
      setSubmitting(false)
    }
  }

  // Group templates by category for the select
  const byCategory: Record<string, AppTemplate[]> = {}
  for (const t of templates) {
    ;(byCategory[t.category] ??= []).push(t)
  }

  const selected = templates.find(t => t.id === selectedTemplateId)

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <span className="modal-title">Add App</span>
          <button className="modal-close" onClick={onClose}>✕</button>
        </div>

        <div className="modal-body">
          {loadError ? (
            <div className="modal-error">{loadError}</div>
          ) : templates.length === 0 ? (
            <div className="modal-loading">Loading templates…</div>
          ) : (
            <>
              <div className="modal-field">
                <label className="modal-label">App name</label>
                <input
                  className="settings-input"
                  placeholder="e.g. My Sonarr"
                  value={name}
                  onChange={e => setName(e.target.value)}
                  autoFocus
                />
              </div>

              <div className="modal-field">
                <label className="modal-label">Template</label>
                <select
                  className="settings-input settings-select"
                  value={selectedTemplateId}
                  onChange={e => setSelectedTemplateId(e.target.value)}
                >
                  {Object.entries(byCategory).sort().map(([cat, items]) => (
                    <optgroup key={cat} label={cat}>
                      {items.map(t => (
                        <option key={t.id} value={t.id}>{t.name}</option>
                      ))}
                    </optgroup>
                  ))}
                </select>
              </div>

              {selected && (
                <div className="modal-template-desc">{selected.description}</div>
              )}

              {error && <div className="modal-error">{error}</div>}
            </>
          )}
        </div>

        <div className="modal-footer">
          <button className="settings-btn secondary" onClick={onClose}>Cancel</button>
          <button
            className="settings-btn primary"
            onClick={handleSubmit}
            disabled={submitting || templates.length === 0}
          >
            {submitting ? 'Creating…' : 'Create App'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Apps tab ──────────────────────────────────────────────────────────────────

function AppsTab() {
  const [appList, setAppList] = useState<App[]>([])
  const [showModal, setShowModal] = useState(false)

  const loadApps = () => {
    appsApi.list().then(r => setAppList(r.data)).catch(() => {})
  }

  useEffect(() => { loadApps() }, [])

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

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Apps</span>
          <button className="settings-btn primary" onClick={() => setShowModal(true)}>+ Add app</button>
        </div>
        {appList.length === 0 ? (
          <div className="settings-placeholder">No apps configured. Add an app to start receiving webhook events.</div>
        ) : (
          <div className="apps-list">
            {appList.map(app => (
              <div key={app.id} className="app-row">
                <span className="app-row-name">{app.name}</span>
                <span className="app-row-id">{app.profile_id ?? 'no template'}</span>
              </div>
            ))}
          </div>
        )}
      </section>

      {showModal && (
        <AddAppModal
          onClose={() => setShowModal(false)}
          onCreated={() => { setShowModal(false); loadApps() }}
        />
      )}
    </div>
  )
}

// ── Notifications tab ─────────────────────────────────────────────────────────

function NotificationsTab() {
  return (
    <div className="tab-content">
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">SMTP</span>
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Host</label>
          <input className="settings-input" placeholder="smtp.example.com" />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Port</label>
          <input className="settings-input" placeholder="587" style={{ maxWidth: 120 }} />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Username</label>
          <input className="settings-input" placeholder="user@example.com" />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Password</label>
          <input className="settings-input" type="password" placeholder="••••••••" />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">From</label>
          <input className="settings-input" placeholder="nora@example.com" />
        </div>
        <div className="settings-actions">
          <button className="settings-btn primary">Save</button>
          <button className="settings-btn secondary">Test Connection</button>
        </div>
      </section>

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Digest Schedule</span>
        </div>
        <div className="settings-option-cards">
          <div className="settings-option-card">Disabled</div>
          <div className="settings-option-card">Daily</div>
          <div className="settings-option-card active">Weekly</div>
        </div>
        <div className="settings-field-row" style={{ marginTop: 12 }}>
          <label className="settings-label">Time of day</label>
          <input className="settings-input" type="time" defaultValue="08:00" style={{ maxWidth: 120 }} />
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
