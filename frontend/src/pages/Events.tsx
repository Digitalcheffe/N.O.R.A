import { useState } from 'react'
import { Topbar } from '../components/Topbar'
import type { Severity } from '../api/types'
import './Events.css'

type TimeFilter = 'day' | 'week' | 'month'

const SEVERITIES: Severity[] = ['debug', 'info', 'warn', 'error', 'critical']

export function Events() {
  const [timeFilter, setTimeFilter] = useState<TimeFilter>('week')
  const [severity, setSeverity] = useState<Severity | ''>('')

  return (
    <>
      <Topbar title="Events" timeFilter={timeFilter} onTimeFilter={setTimeFilter} />
      <div className="content">
        <div className="events-filters">
          <div className="filter-group">
            {SEVERITIES.map((s) => (
              <button
                key={s}
                className={`filter-chip sev-${s}${severity === s ? ' active' : ''}`}
                onClick={() => setSeverity(severity === s ? '' : s)}
              >
                {s}
              </button>
            ))}
          </div>
        </div>

        <div className="events-panel">
          <div className="events-header">
            <span className="section-title">Recent Events</span>
          </div>
          <div className="events-empty">
            <span>No events found</span>
          </div>
        </div>
      </div>
    </>
  )
}
