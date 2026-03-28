import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { EventRow } from '../components/EventRow'
import { events as eventsApi } from '../api/client'
import type { Event, Severity } from '../api/types'
import './Events.css'

type TimeFilter = 'day' | 'week' | 'month'

const SEVERITIES: Severity[] = ['debug', 'info', 'warn', 'error', 'critical']

function sinceFromTimeFilter(tf: TimeFilter): string {
  const d = new Date()
  if (tf === 'day') d.setDate(d.getDate() - 1)
  else if (tf === 'week') d.setDate(d.getDate() - 7)
  else d.setMonth(d.getMonth() - 1)
  return d.toISOString()
}

export function Events() {
  const navigate = useNavigate()
  const [timeFilter, setTimeFilter] = useState<TimeFilter>('week')
  const [severity, setSeverity] = useState<Severity | ''>('')
  const [eventList, setEventList] = useState<Event[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    eventsApi
      .list({
        from: sinceFromTimeFilter(timeFilter),
        ...(severity ? { severity } : {}),
        limit: 200,
      })
      .then((res) => {
        setEventList(res.data)
        setTotal(res.total)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [timeFilter, severity])

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
            {!loading && !error && (
              <span className="events-count">{total} event{total !== 1 ? 's' : ''}</span>
            )}
          </div>

          {!loading && !error && eventList.length > 0 && (
            <div className="event-row events-col-header">
              <span>Time</span>
              <span />
              <span>App</span>
              <span>Event</span>
              <span>Severity</span>
            </div>
          )}

          {loading && (
            <div className="events-empty"><span>Loading…</span></div>
          )}

          {error && (
            <div className="events-empty"><span>Error: {error}</span></div>
          )}

          {!loading && !error && eventList.length === 0 && (
            <div className="events-empty"><span>No events found</span></div>
          )}

          {!loading && !error && eventList.map((ev) => (
            <EventRow
              key={ev.id}
              event={ev}
              appName={ev.app_name}
              onAppClick={ev.app_id ? (id) => navigate(`/apps/${id}`) : undefined}
            />
          ))}
        </div>
      </div>
    </>
  )
}
