import { useState } from 'react'
import { events as eventsApi } from '../api/client'
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
  /** When provided, the app name becomes a clickable link */
  onAppClick?: (appId: string) => void
}

export function EventRow({ event, appName, onAppClick }: Props) {
  const [expanded, setExpanded] = useState(false)
  const [detail, setDetail] = useState<Record<string, unknown> | null>(null)
  const [fetching, setFetching] = useState(false)
  const sev = event.severity

  function handleClick() {
    if (!expanded && detail === null) {
      setFetching(true)
      eventsApi
        .get(event.id)
        .then(e => setDetail((e.raw_payload && Object.keys(e.raw_payload).length > 0)
          ? e.raw_payload as Record<string, unknown>
          : e.fields as Record<string, unknown> ?? {}))
        .catch(() => setDetail({}))
        .finally(() => setFetching(false))
    }
    setExpanded(!expanded)
  }

  return (
    <div className={`event-row-wrapper${expanded ? ' expanded' : ''}`}>
      <div className="event-row" onClick={handleClick}>
        <div className="event-time">{formatEventTime(event.received_at)}</div>
        <div className={`severity-badge ${sev}`} />
        <div
          className={`event-app${onAppClick && event.app_id ? ' event-app-link' : ''}`}
          onClick={onAppClick && event.app_id ? (e) => { e.stopPropagation(); onAppClick(event.app_id) } : undefined}
          title={onAppClick && event.app_id ? `Go to ${appName}` : undefined}
        >
          {appName || '—'}
        </div>
        <div className="event-text">{event.display_text}</div>
        <div className={`event-sev-label ${sev}`}>{sev}</div>
      </div>
      {expanded && (
        <div className="event-expand">
          {fetching ? 'Loading…' : JSON.stringify(detail, null, 2)}
        </div>
      )}
    </div>
  )
}
