import { Topbar } from '../components/Topbar'
import './Apps.css'

export function Apps() {
  return (
    <>
      <Topbar title="Apps" onAdd={() => {}} />
      <div className="content">
        <div className="section-header">
          <span className="section-title">Configured Apps</span>
          <button className="section-action" onClick={() => {}}>+ Add app</button>
        </div>
        <div className="widget-grid">
          {/* App widgets render here — T-06+ */}
          <div className="apps-empty">No apps configured yet.</div>
        </div>
      </div>
    </>
  )
}
