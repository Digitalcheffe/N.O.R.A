import { Topbar } from '../components/Topbar'
import './Topology.css'

export function Topology() {
  return (
    <>
      <Topbar title="Infrastructure" onAdd={() => {}} />
      <div className="content">
        <div className="section-header">
          <span className="section-title">Physical Hosts</span>
          <button className="section-action" onClick={() => {}}>+ Add host</button>
        </div>
        <div className="topology-empty">
          No infrastructure configured yet. Add a physical host to get started.
        </div>
      </div>
    </>
  )
}
