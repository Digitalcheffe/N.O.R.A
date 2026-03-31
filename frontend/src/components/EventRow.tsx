import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { events as eventsApi } from '../api/client'
import type { Event } from '../api/types'
import { formatEventTime } from '../utils/formatTime'
import './EventRow.css'

function getSourceName(event: Event, appName?: string): string {
  if (appName) return appName
  if (event.source_name) return event.source_name
  return 'NORA System'
}

interface Props {
  event: Event
  /** Optional explicit source name override. When absent, derived from event.source_name. */
  appName?: string
  /** When provided, the source name becomes a clickable link */
  onAppClick?: (sourceId: string) => void
}

export function EventRow({ event, appName, onAppClick }: Props) {
  const [expanded, setExpanded] = useState(false)
  const [detail, setDetail] = useState<Record<string, unknown> | null>(null)
  const [fetching, setFetching] = useState(false)
  const navigate = useNavigate()
  const sev = event.level
  const sourceName = getSourceName(event, appName)
  const isAppSource = event.source_type === 'app' && !!event.source_id

  function handleSaveAsRule(e: React.MouseEvent) {
    e.stopPropagation()
    const prefill = {
      source_id: event.source_id || null,
      source_type: event.source_type === 'app' ? 'app' : event.source_type === 'docker_engine' ? 'docker' : 'monitor',
      severity: event.level,
      conditions: [{ field: 'display_text', operator: 'contains', value: event.title.slice(0, 40) }],
    }
    navigate(`/settings?tab=notify_rules&prefill=${encodeURIComponent(JSON.stringify(prefill))}`)
  }

  function handleClick() {
    if (!expanded && detail === null) {
      setFetching(true)
      eventsApi
        .get(event.id)
        .then(e => setDetail(e.payload && Object.keys(e.payload).length > 0 ? e.payload : {}))
        .catch(() => setDetail({}))
        .finally(() => setFetching(false))
    }
    setExpanded(!expanded)
  }

  return (
    <div className={`event-row-wrapper${expanded ? ' expanded' : ''}`}>
      <div className="event-row" onClick={handleClick}>
        <div className="event-time">{formatEventTime(event.created_at)}</div>
        <div className={`severity-badge ${sev}`} />
        <div
          className={`event-app${onAppClick && isAppSource ? ' event-app-link' : ''}`}
          onClick={onAppClick && isAppSource ? (e) => { e.stopPropagation(); onAppClick(event.source_id) } : undefined}
          title={onAppClick && isAppSource ? `Go to ${sourceName}` : undefined}
        >
          {sourceName}
        </div>
        <div className="event-text">{event.title}</div>
        <div className={`event-sev-label ${sev}`}>{sev}</div>
      </div>
      {expanded && (
        <div className="event-expand">
          <div className="event-expand-actions">
            <button className="btn-secondary btn-sm" onClick={handleSaveAsRule}>Save as rule</button>
          </div>
          {fetching ? 'Loading…' : JSON.stringify(detail, null, 2)}
        </div>
      )}
    </div>
  )
}
