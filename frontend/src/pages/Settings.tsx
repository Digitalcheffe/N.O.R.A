import { useSearchParams } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { InfraIntegrations } from './Integrations'
import './Settings.css'

type Tab = 'apps' | 'notifications' | 'metrics' | 'profile'

const TABS: { id: Tab; label: string }[] = [
  { id: 'apps', label: 'Apps' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'metrics', label: 'Instance Metrics' },
  { id: 'profile', label: 'Profile' },
]

// ── Apps tab ──────────────────────────────────────────────────────────────────

function AppsTab() {
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
          <button className="settings-btn primary">+ Add app</button>
        </div>
        <div className="settings-placeholder">No apps configured. Add an app to start receiving webhook events.</div>
      </section>
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

// ── Profile tab ───────────────────────────────────────────────────────────────

function ProfileTab() {
  return (
    <div className="tab-content">
      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Profile</span>
        </div>
        <div className="settings-avatar-row">
          <div className="settings-avatar">A</div>
          <div>
            <div className="settings-avatar-name">Admin</div>
            <div className="settings-avatar-email">admin@nora.local</div>
          </div>
        </div>
        <div className="settings-field-row" style={{ marginTop: 16 }}>
          <label className="settings-label">Display name</label>
          <input className="settings-input" defaultValue="Admin" />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Email</label>
          <input className="settings-input" defaultValue="admin@nora.local" />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Timezone</label>
          <input className="settings-input" defaultValue="UTC" />
        </div>
        <div className="settings-actions">
          <button className="settings-btn primary">Save</button>
        </div>
      </section>

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title">Change Password</span>
        </div>
        <div className="settings-field-row">
          <label className="settings-label">Current password</label>
          <input className="settings-input" type="password" placeholder="••••••••" />
        </div>
        <div className="settings-field-row">
          <label className="settings-label">New password</label>
          <input className="settings-input" type="password" placeholder="••••••••" />
        </div>
        <div className="settings-actions">
          <button className="settings-btn primary">Update Password</button>
        </div>
      </section>

      <section className="settings-section">
        <div className="section-header">
          <span className="section-title" style={{ color: 'var(--red)' }}>Danger Zone</span>
        </div>
        <div className="settings-placeholder">Delete account — removes all data associated with this user.</div>
        <button className="settings-btn danger" style={{ marginTop: 12 }}>Delete Account</button>
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
        {activeTab === 'profile' && <ProfileTab />}
      </div>
    </>
  )
}
