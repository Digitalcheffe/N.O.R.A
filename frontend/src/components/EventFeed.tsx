import { useState, useEffect } from 'react'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { events as eventsApi } from '../api/client'
import { EventRow } from './EventRow'
import type { Event } from '../api/types'
import './EventFeed.css'
import './EventRow.css'

interface Props {
  /** Optional source_type filter (e.g. "app", "docker_engine", "monitor_check"). */
  sourceType?: string
  /** The entity ID to filter events for. */
  sourceId: string
  /** Section title. Defaults to "RECENT EVENTS". */
  title?: string
}

const DEFAULT_LIMIT = 50

export function EventFeed({ sourceType, sourceId, title = 'RECENT EVENTS' }: Props) {
  const { tick } = useAutoRefresh()
  const [eventList, setEventList] = useState<Event[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [loadingMore, setLoadingMore] = useState(false)

  function fetchEvents(offset: number, append: boolean) {
    if (offset === 0) setLoading(true)
    else setLoadingMore(true)
    eventsApi
      .list({
        ...(sourceType ? { source_type: sourceType } : {}),
        source_id: sourceId,
        limit: DEFAULT_LIMIT,
        offset,
        sort: 'newest',
      })
      .then(res => {
        setEventList(prev => append ? [...prev, ...res.data] : res.data)
        setTotal(res.total)
        setError(null)
      })
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load events'))
      .finally(() => { setLoading(false); setLoadingMore(false) })
  }

  useEffect(() => {
    void (async () => { fetchEvents(0, false) })()
  }, [sourceType, sourceId, tick]) // eslint-disable-line react-hooks/exhaustive-deps

  const hasMore = eventList.length < total

  return (
    <div className="event-feed">
      <div className="event-feed-header">
        <span className="section-title">{title}</span>
        {!loading && !error && total > 0 && (
          <span className="event-feed-count">
            {total} event{total !== 1 ? 's' : ''}
          </span>
        )}
      </div>

      {loading && <div className="event-feed-empty">Loading…</div>}
      {error && <div className="event-feed-empty">Error: {error}</div>}
      {!loading && !error && eventList.length === 0 && (
        <div className="event-feed-empty">No events yet</div>
      )}

      {!loading && !error && eventList.length > 0 && (
        <>
          <div className="event-row events-col-header">
            <span>Time</span>
            <span />
            <span>Source</span>
            <span>Event</span>
            <span>Severity</span>
          </div>
          {eventList.map(ev => (
            <EventRow key={ev.id} event={ev} />
          ))}
        </>
      )}

      {!loading && !error && hasMore && (
        <button
          className="event-feed-load-more"
          onClick={() => fetchEvents(eventList.length, true)}
          disabled={loadingMore}
        >
          {loadingMore ? 'Loading…' : `Load more (${total - eventList.length} remaining)`}
        </button>
      )}
    </div>
  )
}
