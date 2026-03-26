import { useState } from 'react'
import type { Event } from '../api/types'

function formatEventTime(iso: string): string {
  const d = new Date(iso)
  const now = new Date()
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const startOfYesterday = new Date(startOfToday.getTime() - 86400000)
  if (d >= startOfToday) {
    return d
      .toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', hour12: true })
      .toLowerCase()
  }
  if (d >= startOfYesterday) return 'Yesterday'
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

interface Props {
  event: Event
  appName: string
}

export function EventRow({ event, appName }: Props) {
  const [expanded, setExpanded] = useState(false)
  const sev = event.severity

  return (
    <div className={`event-row-wrapper${expanded ? ' expanded' : ''}`}>
      <div className="event-row" onClick={() => setExpanded(!expanded)}>
        <div className="event-time">{formatEventTime(event.received_at)}</div>
        <div className={`severity-badge ${sev}`} />
        <div className="event-app">{appName || event.app_id}</div>
        <div className="event-text">{event.display_text}</div>
        <div className={`event-sev-label ${sev}`}>{sev}</div>
      </div>
      {expanded && (
        <div className="event-expand">
          {JSON.stringify(event.raw_payload, null, 2)}
        </div>
      )}
    </div>
  )
}
