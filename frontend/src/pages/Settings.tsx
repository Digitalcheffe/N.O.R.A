import { Topbar } from '../components/Topbar'
import { InfraIntegrations } from './Integrations'
import './Settings.css'

export function Settings() {
  return (
    <>
      <Topbar title="Settings" />
      <div className="content">
        <div className="settings-sections">
          <section className="settings-section">
            <div className="section-header">
              <span className="section-title">Users</span>
            </div>
            <div className="settings-placeholder">User management — T-08+</div>
          </section>

          <section className="settings-section">
            <div className="section-header">
              <span className="section-title">Notifications</span>
            </div>
            <div className="settings-placeholder">SMTP digest configuration — T-08+</div>
          </section>

          <section className="settings-section">
            <InfraIntegrations />
          </section>

          <section className="settings-section">
            <div className="section-header">
              <span className="section-title">Instance Metrics</span>
            </div>
            <div className="settings-placeholder">Database size, events per day, peak load — T-08+</div>
          </section>
        </div>
      </div>
    </>
  )
}
