import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { EventRow } from '../components/EventRow'
import { events as eventsApi } from '../api/client'
import type { Event, EventFilter, EventSort, Severity, TimeseriesBucket } from '../api/types'
import './Events.css'

type TimeFilter = 'day' | 'week' | 'month'
type ChartRange = 'day' | 'week' | 'month' | '3m'
type SourceType = '' | 'app' | 'infra' | 'check'

const SEVERITIES: Severity[] = ['debug', 'info', 'warn', 'error', 'critical']
const PAGE_SIZES = [25, 50, 100, 500]

function sinceFromTimeFilter(tf: TimeFilter): string {
  const d = new Date()
  if (tf === 'day') d.setDate(d.getDate() - 1)
  else if (tf === 'week') d.setDate(d.getDate() - 7)
  else d.setMonth(d.getMonth() - 1)
  return d.toISOString()
}

function chartRangeParams(range: ChartRange): { since: string; until: string; granularity: 'hour' | 'day' } {
  const now = new Date()
  const until = now.toISOString()
  switch (range) {
    case 'day': {
      const since = new Date(now.getTime() - 24 * 60 * 60 * 1000).toISOString()
      return { since, until, granularity: 'hour' }
    }
    case 'week': {
      const since = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000).toISOString()
      return { since, until, granularity: 'day' }
    }
    case 'month': {
      const since = new Date(now.getFullYear(), now.getMonth(), 1).toISOString()
      return { since, until, granularity: 'day' }
    }
    case '3m': {
      const since = new Date(now.getTime() - 90 * 24 * 60 * 60 * 1000).toISOString()
      return { since, until, granularity: 'day' }
    }
  }
}

function pad(n: number) {
  return String(n).padStart(2, '0')
}

function fillBuckets(
  buckets: TimeseriesBucket[],
  since: string,
  until: string,
  granularity: 'hour' | 'day',
): TimeseriesBucket[] {
  const lookup = new Map(buckets.map(b => [b.time, b.count]))
  const filled: TimeseriesBucket[] = []
  const start = new Date(since)
  const end = new Date(until)

  if (granularity === 'hour') {
    const cur = new Date(start)
    cur.setUTCMinutes(0, 0, 0)
    while (cur <= end) {
      const key = `${cur.getUTCFullYear()}-${pad(cur.getUTCMonth() + 1)}-${pad(cur.getUTCDate())}T${pad(cur.getUTCHours())}:00:00Z`
      filled.push({ time: key, count: lookup.get(key) ?? 0 })
      cur.setUTCHours(cur.getUTCHours() + 1)
    }
  } else {
    const cur = new Date(start)
    cur.setUTCHours(0, 0, 0, 0)
    while (cur <= end) {
      const key = `${cur.getUTCFullYear()}-${pad(cur.getUTCMonth() + 1)}-${pad(cur.getUTCDate())}`
      filled.push({ time: key, count: lookup.get(key) ?? 0 })
      cur.setUTCDate(cur.getUTCDate() + 1)
    }
  }
  return filled
}

function formatBucketLabel(time: string, granularity: 'hour' | 'day'): string {
  if (granularity === 'hour') {
    const m = time.match(/T(\d\d):00:00Z$/)
    return m ? `${m[1]}:00` : time
  }
  const d = new Date(time + 'T00:00:00Z')
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' })
}

function EventsLineChart({
  buckets,
  granularity,
}: {
  buckets: TimeseriesBucket[]
  granularity: 'hour' | 'day'
}) {
  if (buckets.length === 0) {
    return <div className="chart-empty">No event data for this range</div>
  }

  const W = 800
  const H = 180
  const padL = 38
  const padR = 16
  const padT = 12
  const padB = 28
  const chartW = W - padL - padR
  const chartH = H - padT - padB

  const maxCount = Math.max(...buckets.map(b => b.count), 1)

  const toX = (i: number) =>
    padL + (buckets.length > 1 ? (i / (buckets.length - 1)) * chartW : chartW / 2)
  const toY = (count: number) => padT + (1 - count / maxCount) * chartH

  const pathParts = buckets.map((b, i) => `${i === 0 ? 'M' : 'L'} ${toX(i).toFixed(1)} ${toY(b.count).toFixed(1)}`)
  const pathD = pathParts.join(' ')
  const fillD =
    buckets.length > 1
      ? `${pathD} L ${toX(buckets.length - 1).toFixed(1)} ${(padT + chartH).toFixed(1)} L ${toX(0).toFixed(1)} ${(padT + chartH).toFixed(1)} Z`
      : ''

  // Y labels: 0, midpoint, max
  const yLabels = [
    { y: padT, label: String(maxCount) },
    { y: padT + chartH / 2, label: String(Math.round(maxCount / 2)) },
    { y: padT + chartH, label: '0' },
  ]

  // X labels: select up to 8 evenly spaced
  const maxLabels = 8
  const step = Math.max(1, Math.floor(buckets.length / maxLabels))
  const xLabelIndices = new Set<number>()
  for (let i = 0; i < buckets.length; i += step) xLabelIndices.add(i)
  xLabelIndices.add(buckets.length - 1)

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      className="events-chart-svg"
      preserveAspectRatio="xMidYMid meet"
    >
      {/* horizontal grid lines */}
      {[0, 0.25, 0.5, 0.75, 1].map(t => (
        <line
          key={t}
          x1={padL}
          y1={padT + t * chartH}
          x2={W - padR}
          y2={padT + t * chartH}
          stroke="var(--border)"
          strokeWidth="1"
        />
      ))}

      {/* area fill */}
      {fillD && <path d={fillD} fill="var(--accent)" fillOpacity="0.07" />}

      {/* line */}
      <path
        d={pathD}
        fill="none"
        stroke="var(--accent)"
        strokeWidth="1.5"
        strokeLinejoin="round"
        strokeLinecap="round"
      />

      {/* y-axis labels */}
      {yLabels.map(({ y, label }) => (
        <text
          key={label + y}
          x={padL - 5}
          y={y + 4}
          textAnchor="end"
          fontSize="10"
          fill="var(--text3)"
          fontFamily="var(--mono)"
        >
          {label}
        </text>
      ))}

      {/* x-axis labels */}
      {Array.from(xLabelIndices).map(i => (
        <text
          key={i}
          x={toX(i)}
          y={H - 4}
          textAnchor="middle"
          fontSize="10"
          fill="var(--text3)"
          fontFamily="var(--mono)"
        >
          {formatBucketLabel(buckets[i].time, granularity)}
        </text>
      ))}
    </svg>
  )
}

export function Events() {
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()
  const [timeFilter, setTimeFilter] = useState<TimeFilter>('week')
  const [severity, setSeverity] = useState<Severity | ''>('')
  const [sourceType, setSourceType] = useState<SourceType>('')
  const [search, setSearch] = useState('')
  const [searchDraft, setSearchDraft] = useState('')
  const [searchTrigger, setSearchTrigger] = useState(0)
  const [fromDate, setFromDate] = useState('')
  const [toDate, setToDate] = useState('')
  const [sort, setSort] = useState<EventSort>('newest')
  const [pageSize, setPageSize] = useState(50)
  const [page, setPage] = useState(0)
  const [chartRange, setChartRange] = useState<ChartRange>('week')

  const [eventList, setEventList] = useState<Event[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [chartFilled, setChartFilled] = useState<TimeseriesBucket[]>([])
  const [chartGranularity, setChartGranularity] = useState<'hour' | 'day'>('day')
  const [chartLoading, setChartLoading] = useState(true)

  const listRef = useRef<HTMLDivElement>(null)

  // Fetch events list
  useEffect(() => {
    setLoading(true)
    setError(null)
    const filter: EventFilter = {
      sort,
      limit: pageSize,
      offset: page * pageSize,
    }
    // Date range: custom dates take priority over timeFilter tabs
    if (fromDate) {
      filter.from = new Date(fromDate).toISOString()
    } else {
      filter.from = sinceFromTimeFilter(timeFilter)
    }
    if (toDate) {
      filter.to = new Date(toDate + 'T23:59:59').toISOString()
    }
    if (severity) filter.severity = severity
    if (sourceType) filter.source_type = sourceType as 'app' | 'infra' | 'check'
    if (search) filter.search = search
    eventsApi
      .list(filter)
      .then(res => {
        setEventList(res.data)
        setTotal(res.total)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [timeFilter, severity, sourceType, search, searchTrigger, fromDate, toDate, sort, pageSize, page, tick])

  // Fetch chart data
  useEffect(() => {
    setChartLoading(true)
    const { since, until, granularity } = chartRangeParams(chartRange)
    setChartGranularity(granularity)
    eventsApi
      .timeseries({ since, until, granularity, ...(severity ? { severity } : {}) })
      .then(res => {
        setChartFilled(fillBuckets(res.data, since, until, granularity))
      })
      .catch(() => {
        setChartFilled([])
      })
      .finally(() => setChartLoading(false))
  }, [chartRange, severity])

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  function changePage(next: number) {
    setPage(next)
    listRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <>
      <Topbar
        title="Events"
        timeFilter={timeFilter}
        onTimeFilter={t => { setTimeFilter(t as TimeFilter); setPage(0) }}
      />
      <div className="content">

        {/* ── Chart ── */}
        <div className="events-chart-container">
          <div className="events-chart-header">
            <span className="section-title">Event Volume</span>
            <div className="chart-range-tabs">
              {(['day', 'week', 'month', '3m'] as ChartRange[]).map(r => (
                <button
                  key={r}
                  className={`chart-range-tab${chartRange === r ? ' active' : ''}`}
                  onClick={() => setChartRange(r)}
                >
                  {r === 'day' ? '24h' : r === 'week' ? '7d' : r === 'month' ? '30d' : '3M'}
                </button>
              ))}
            </div>
          </div>
          <div className="events-chart-body">
            {chartLoading ? (
              <div className="chart-empty">Loading…</div>
            ) : (
              <EventsLineChart buckets={chartFilled} granularity={chartGranularity} />
            )}
          </div>
        </div>

        {/* ── Filters + controls ── */}
        <div className="events-filter-row">
          <div className="filter-group">
            {SEVERITIES.map(s => (
              <button
                key={s}
                className={`filter-chip sev-${s}${severity === s ? ' active' : ''}`}
                onClick={() => { setSeverity(severity === s ? '' : s); setPage(0) }}
              >
                {s}
              </button>
            ))}
          </div>
          <div className="events-controls">
            <select
              className="events-select"
              value={sort}
              onChange={e => { setSort(e.target.value as EventSort); setPage(0) }}
            >
              <option value="newest">Newest first</option>
              <option value="oldest">Oldest first</option>
              <option value="severity_desc">Severity ↓</option>
              <option value="severity_asc">Severity ↑</option>
            </select>
            <select
              className="events-select"
              value={pageSize}
              onChange={e => { setPageSize(Number(e.target.value)); setPage(0) }}
            >
              {PAGE_SIZES.map(n => (
                <option key={n} value={n}>{n} / page</option>
              ))}
            </select>
          </div>
        </div>

        {/* ── Advanced filters row ── */}
        <div className="events-adv-filter-row">
          <select
            className="events-select"
            value={sourceType}
            onChange={e => { setSourceType(e.target.value as SourceType); setPage(0) }}
          >
            <option value="">All sources</option>
            <option value="app">Apps</option>
            <option value="infra">Infrastructure</option>
            <option value="check">Checks</option>
          </select>
          <div className="events-date-range">
            <input
              type="date"
              className="events-date-input"
              value={fromDate}
              onChange={e => { setFromDate(e.target.value); setPage(0) }}
              title="From date"
            />
            <span className="events-date-sep">–</span>
            <input
              type="date"
              className="events-date-input"
              value={toDate}
              onChange={e => { setToDate(e.target.value); setPage(0) }}
              title="To date"
            />
            {(fromDate || toDate) && (
              <button
                className="events-date-clear"
                onClick={() => { setFromDate(''); setToDate(''); setPage(0) }}
                title="Clear date range"
              >
                ✕
              </button>
            )}
          </div>
          <div className="events-search-wrap">
            <input
              type="text"
              className="events-search-input"
              placeholder="Search events…"
              value={searchDraft}
              onChange={e => setSearchDraft(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter') { setSearch(searchDraft); setSearchTrigger(t => t + 1); setPage(0) }
                if (e.key === 'Escape') { setSearchDraft(''); setSearch(''); setSearchTrigger(t => t + 1); setPage(0) }
              }}
            />
            <button
              className="events-search-btn"
              onClick={() => { setSearch(searchDraft); setSearchTrigger(t => t + 1); setPage(0) }}
            >
              Search
            </button>
            {search && (
              <button
                className="events-date-clear"
                onClick={() => { setSearchDraft(''); setSearch(''); setSearchTrigger(t => t + 1); setPage(0) }}
                title="Clear search"
              >
                ✕
              </button>
            )}
          </div>
        </div>

        {/* ── Event list panel ── */}
        <div className="events-panel" ref={listRef}>
          <div className="events-header">
            <span className="section-title">Recent Events</span>
            {!loading && !error && (
              <span className="events-count">
                {total} event{total !== 1 ? 's' : ''}
                {totalPages > 1 && ` · page ${page + 1} of ${totalPages}`}
              </span>
            )}
          </div>

          {!loading && !error && eventList.length > 0 && (
            <div className="event-row events-col-header">
              <span>Time</span>
              <span />
              <span>Source</span>
              <span>Event</span>
              <span>Severity</span>
            </div>
          )}

          {loading && <div className="events-empty"><span>Loading…</span></div>}
          {error && <div className="events-empty"><span>Error: {error}</span></div>}
          {!loading && !error && eventList.length === 0 && (
            <div className="events-empty"><span>No events found</span></div>
          )}

          {!loading && !error && eventList.map(ev => (
            <EventRow
              key={ev.id}
              event={ev}
              onAppClick={ev.app_id ? id => navigate(`/apps/${id}`) : undefined}
            />
          ))}

          {!loading && !error && totalPages > 1 && (
            <div className="events-pagination">
              <button
                className="events-page-btn"
                disabled={page === 0}
                onClick={() => changePage(page - 1)}
              >
                ← Prev
              </button>
              <span className="events-page-info">
                {page + 1} / {totalPages}
              </span>
              <button
                className="events-page-btn"
                disabled={page >= totalPages - 1}
                onClick={() => changePage(page + 1)}
              >
                Next →
              </button>
            </div>
          )}
        </div>

      </div>
    </>
  )
}
