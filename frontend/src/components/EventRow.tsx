import { useState } from 'react'
import { events as eventsApi } from '../api/client'
import type { Event } from '../api/types'
import { formatEventTime } from '../utils/formatTime'
import './EventRow.css'

/**
 * Derive the display name for the event source.
 * Priority: explicit appName override → app_name → fields.component_name →
 *           fields.check_name → fields.source → '—'
 */
export function getSourceName(event: Event, appName?: string): string {
  if (appName) return appName
  if (event.app_name) return event.app_name
  const f = event.fields as Record<string, unknown>
  if (f?.component_name && typeof f.component_name === 'string') return f.component_name
  if (f?.check_name && typeof f.check_name === 'string') return f.check_name
  if (f?.source && typeof f.source === 'string') return f.source
  return 'NORA System'
}

interface Props {
  event: Event
  /** Optional explicit source name override. When absent, derived from event fields. */
  appName?: string
  /** When provided, the source name becomes a clickable link */
  onAppClick?: (appId: string) => void
}

export function EventRow({ event, appName, onAppClick }: Props) {
  const [expanded, setExpanded] = useState(false)
  const [detail, setDetail] = useState<Record<string, unknown> | null>(null)
  const [fetching, setFetching] = useState(false)
  const sev = event.severity
  const sourceName = getSourceName(event, appName)

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
          title={onAppClick && event.app_id ? `Go to ${sourceName}` : undefined}
        >
          {sourceName}
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
