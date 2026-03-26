import { useState } from 'react'
import { Topbar } from '../components/Topbar'
import './Dashboard.css'

type TimeFilter = 'day' | 'week' | 'month'

export function Dashboard() {
  const [timeFilter, setTimeFilter] = useState<TimeFilter>('week')

  return (
    <>
      <Topbar
        title="Dashboard"
        timeFilter={timeFilter}
        onTimeFilter={setTimeFilter}
        onAdd={() => {}}
      />
      <div className="content">
        {/* Summary bar, app widgets, infrastructure, events, checks — T-06+ */}
        <div className="dashboard-empty">
          <p className="dashboard-empty-text">No apps configured yet.</p>
          <button className="dashboard-empty-btn" onClick={() => {}}>+ Add your first app</button>
        </div>
      </div>
    </>
  )
}
