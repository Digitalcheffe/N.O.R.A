import { Topbar } from '../components/Topbar'
import './Settings.css'

export function Profile() {
  return (
    <>
      <Topbar title="Profile" />
      <div className="content">
        <div className="tab-content">
          <section className="settings-section">
            <div className="section-header">
              <span className="section-title">Account</span>
            </div>
            <div className="settings-field-row">
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
      </div>
    </>
  )
}
