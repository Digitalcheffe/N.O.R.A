import { Topbar } from '../components/Topbar'
import './Checks.css'

export function Checks() {
  return (
    <>
      <Topbar title="Monitor Checks" onAdd={() => {}} />
      <div className="content">
        <div className="section-header">
          <span className="section-title">Active Checks</span>
          <button className="section-action" onClick={() => {}}>+ Add check</button>
        </div>
        <div className="checks-empty">
          <span>No monitor checks configured yet.</span>
        </div>
      </div>
    </>
  )
}
