import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { apps as appsApi, dashboard as dashboardApi } from '../api/client'
import type { App, AppSummary, Event, Severity } from '../api/types'
import './AppDetail.css'

type TimeFilter = 'day' | 'week' | 'month'

// ── JSON Viewer ───────────────────────────────────────────────────────────────

function JsonValue({ value, depth }: { value: unknown; depth: number }) {
  const [open, setOpen] = useState(depth < 2)
  const indent = '  '.repeat(depth)

  if (value === null) return <span className="json-null">null</span>
  if (typeof value === 'boolean') return <span className="json-bool">{String(value)}</span>
  if (typeof value === 'number') return <span className="json-num">{value}</span>
  if (typeof value === 'string') return <span className="json-str">"{value}"</span>

  if (Array.isArray(value)) {
    if (value.length === 0) return <span className="json-punct">[]</span>
    return (
      <span>
        <span className="json-punct">[</span>
        {open ? (
          <>
            {value.map((item, i) => (
              <div key={i} style={{ paddingLeft: '16px' }}>
                <JsonValue value={item} depth={depth + 1} />
                {i < value.length - 1 && <span className="json-punct">,</span>}
              </div>
            ))}
            <div>{indent}<span className="json-punct">]</span></div>
          </>
        ) : (
          <span
            className="json-collapse"
            onClick={e => { e.stopPropagation(); setOpen(true) }}
          >
            {value.length} items…
          </span>
        )}
      </span>
    )
  }

  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    if (entries.length === 0) return <span className="json-punct">{'{}'}</span>
    return (
      <span>
        <span className="json-punct">{'{'}</span>
        {open ? (
          <>
            {entries.map(([k, v], i) => (
              <div key={k} style={{ paddingLeft: '16px' }}>
                <span className="json-key">"{k}"</span>
                <span className="json-punct">: </span>
                <JsonValue value={v} depth={depth + 1} />
                {i < entries.length - 1 && <span className="json-punct">,</span>}
              </div>
            ))}
            <div>{indent}<span className="json-punct">{'}'}</span></div>
          </>
        ) : (
          <span
            className="json-collapse"
            onClick={e => { e.stopPropagation(); setOpen(true) }}
          >
            {entries.length} keys…
          </span>
        )}
      </span>
    )
  }

  return <span>{String(value)}</span>
}

function JsonViewer({ data }: { data: Record<string, unknown> }) {
  return (
    <div className="json-viewer">
      <JsonValue value={data} depth={0} />
    </div>
  )
}

// ── Sparkline ─────────────────────────────────────────────────────────────────

function Sparkline({ data, color = 'var(--accent)' }: { data: number[]; color?: string }) {
  if (!data || data.length < 2) return null
  const w = 80
  const h = 20
  const max = Math.max(...data, 1)
  const pts = data
    .map((v, i) => {
      const x = ((i / (data.length - 1)) * w).toFixed(1)
      const y = (h - 2 - (v / max) * (h - 4)).toFixed(1)
      return `${x},${y}`
    })
    .join(' ')
  const closed = `${pts} ${w},${h} 0,${h}`
  return (
    <svg className="detail-sparkline" viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none">
      <polyline points={pts} fill="none" stroke={color} strokeWidth="1.5" opacity="0.8" />
      <polyline points={closed} fill={color} stroke="none" opacity="0.08" />
    </svg>
  )
}

// ── Expanded event detail ─────────────────────────────────────────────────────

function EventDetail({ event, appName }: { event: Event; appName: string }) {
  const received = new Date(event.received_at).toLocaleString('en-US', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: 'numeric', minute: '2-digit', second: '2-digit', hour12: false,
  })

  return (
    <div className="detail-event-expand">
      <div className="detail-expand-meta">
        <div className="detail-meta-row">
          <span className="detail-meta-label">Severity</span>
          <span className={`detail-meta-val sev-${event.severity}`}>{event.severity}</span>
        </div>
        <div className="detail-meta-row">
          <span className="detail-meta-label">App</span>
          <span className="detail-meta-val">{appName}</span>
        </div>
        <div className="detail-meta-row">
          <span className="detail-meta-label">Received</span>
          <span className="detail-meta-val">{received}</span>
        </div>
      </div>

      <div className="detail-expand-payload-label">Raw Payload</div>
      {Object.keys(event.raw_payload ?? {}).length > 0 ? (
        <JsonViewer data={event.raw_payload} />
      ) : Object.keys(event.fields ?? {}).length > 0 ? (
        <JsonViewer data={event.fields} />
      ) : (
        <div className="json-viewer json-empty">No payload</div>
      )}

      <div className="detail-expand-actions">
        <button
          className="detail-rule-btn"
          disabled
          title="Coming in v2"
        >
          Save as notification rule
        </button>
      </div>
    </div>
  )
}

// ── Expanded event row ────────────────────────────────────────────────────────

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

function DetailEventRow({
  event,
  appName,
  expanded,
  onToggle,
}: {
  event: Event
  appName: string
  expanded: boolean
  onToggle: () => void
}) {
  const sev = event.severity
  return (
    <div className={`event-row-wrapper${expanded ? ' expanded' : ''}`}>
      <div className="event-row" onClick={onToggle}>
        <div className="event-time">{formatEventTime(event.received_at)}</div>
        <div className={`severity-badge ${sev}`} />
        <div className="event-text">{event.display_text}</div>
        <div className={`event-sev-label ${sev}`}>{sev}</div>
      </div>
      {expanded && <EventDetail event={event} appName={appName} />}
    </div>
  )
}

// ── AppDetail ─────────────────────────────────────────────────────────────────

const SEVERITIES: Array<Severity | 'all'> = ['all', 'info', 'warn', 'error', 'critical']
const PAGE_SIZE = 50

export function AppDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const [timeFilter, setTimeFilter] = useState<TimeFilter>('week')
  const [severityFilter, setSeverityFilter] = useState<Severity | 'all'>('all')

  const [app, setApp] = useState<App | null>(null)
  const [appSummary, setAppSummary] = useState<AppSummary | null>(null)
  const [events, setEvents] = useState<Event[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [expandedId, setExpandedId] = useState<string | null>(null)

  const appId = id ?? ''

  // Fetch app metadata once
  useEffect(() => {
    if (!appId) return
    appsApi.get(appId).then(setApp).catch(console.error)
  }, [appId])

  // Fetch summary (for stats + sparkline) when time filter changes
  useEffect(() => {
    if (!appId) return
    dashboardApi.summary(timeFilter)
      .then(res => {
        const found = res.apps.find(a => a.id === appId) ?? null
        setAppSummary(found)
      })
      .catch(console.error)
  }, [appId, timeFilter])

  // Fetch events when filter or severity changes (reset list)
  useEffect(() => {
    if (!appId) return
    setLoading(true)
    setOffset(0)
    setEvents([])
    setExpandedId(null)
    const filter = {
      limit: PAGE_SIZE,
      offset: 0,
      ...(severityFilter !== 'all' ? { severity: severityFilter } : {}),
    }
    appsApi.events(appId, filter)
      .then(res => {
        setEvents(res.data)
        setTotal(res.total)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [appId, severityFilter])

  // Redirect if no id — must be in an effect, not during render
  useEffect(() => {
    if (!id) navigate('/apps')
  }, [id, navigate])

  if (!id) return null

  function handleLoadMore() {
    const nextOffset = offset + PAGE_SIZE
    setLoadingMore(true)
    const filter = {
      limit: PAGE_SIZE,
      offset: nextOffset,
      ...(severityFilter !== 'all' ? { severity: severityFilter } : {}),
    }
    appsApi.events(appId, filter)
      .then(res => {
        setEvents(prev => [...prev, ...res.data])
        setOffset(nextOffset)
      })
      .catch(console.error)
      .finally(() => setLoadingMore(false))
  }

  function handleToggle(eventId: string) {
    setExpandedId(prev => prev === eventId ? null : eventId)
  }

  const appName = app?.name ?? appId
  const baseUrl = app?.config?.base_url as string | undefined
  const status = appSummary?.status ?? 'online'
  const topbarStatus = status === 'online' ? 'ok' : (status as 'warn' | 'down')
  const lastEvent = appSummary?.last_event_at
    ? new Date(appSummary.last_event_at).toLocaleString('en-US', {
        month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', hour12: true,
      })
    : null

  const hasMore = events.length < total

  return (
    <>
      <Topbar
        title={appName}
        status={topbarStatus}
        timeFilter={timeFilter}
        onTimeFilter={setTimeFilter}
      />
      <div className="content">

        {/* ── App header ── */}
        <div className="detail-header">
          <div className="detail-header-left">
            <div className="detail-app-icon">
              {appName.slice(0, 2).toUpperCase()}
            </div>
            <div className="detail-app-meta">
              <div className="detail-app-name">{appName}</div>
              {lastEvent && (
                <div className="detail-app-last">Last event: {lastEvent}</div>
              )}
            </div>
            <div className="detail-status-dot-wrap">
              <div className={`status-dot${status !== 'online' ? ` ${status === 'down' ? 'down' : 'warn'}` : ''}`} />
            </div>
          </div>
          {baseUrl && (
            <a
              className="detail-launch-btn"
              href={baseUrl}
              target="_blank"
              rel="noopener noreferrer"
            >
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                <polyline points="15 3 21 3 21 9" />
                <line x1="10" y1="14" x2="21" y2="3" />
              </svg>
              Launch
            </a>
          )}
        </div>

        {/* ── Stats row ── */}
        {appSummary && (
          <div className="detail-stats-row">
            {(appSummary.stats ?? []).map(stat => (
              <div key={stat.label} className="detail-stat-card">
                <div className="detail-stat-label">{stat.label}</div>
                <div className={`detail-stat-value${stat.color ? ` color-${stat.color}` : ''}`}>
                  {stat.value}
                </div>
              </div>
            ))}
            {appSummary.sparkline.some(v => v > 0) && (
              <div className="detail-stat-card detail-stat-sparkline-card">
                <div className="detail-stat-label">Activity</div>
                <Sparkline data={Array.from(appSummary.sparkline)} />
              </div>
            )}
          </div>
        )}

        {/* ── Events section ── */}
        <div className="detail-events-section">
          <div className="section-header">
            <div className="section-title">Events</div>
            <div className="detail-sev-pills">
              {SEVERITIES.map(s => (
                <button
                  key={s}
                  className={`detail-sev-pill sev-pill-${s}${severityFilter === s ? ' active' : ''}`}
                  onClick={() => setSeverityFilter(s)}
                >
                  {s === 'all' ? 'All' : s.charAt(0).toUpperCase() + s.slice(1)}
                </button>
              ))}
            </div>
          </div>

          {loading ? (
            <div className="detail-events-loading">
              {[0, 1, 2, 3].map(i => (
                <div key={i} className="skeleton" style={{ height: 40, marginBottom: 4 }} />
              ))}
            </div>
          ) : events.length === 0 ? (
            <div className="detail-events-empty">No events found</div>
          ) : (
            <div className="events-panel">
              {events.map(event => (
                <DetailEventRow
                  key={event.id}
                  event={event}
                  appName={appName}
                  expanded={expandedId === event.id}
                  onToggle={() => handleToggle(event.id)}
                />
              ))}
            </div>
          )}

          {!loading && hasMore && (
            <button
              className="detail-load-more"
              onClick={handleLoadMore}
              disabled={loadingMore}
            >
              {loadingMore ? 'Loading…' : `Load more (${total - events.length} remaining)`}
            </button>
          )}
        </div>

      </div>
    </>
  )
}
