import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { SummaryCard } from '../components/SummaryCard'
import { AppWidget } from '../components/AppWidget'
import { MonitorWidget } from '../components/MonitorWidget'
import { SSLRow } from '../components/SSLRow'
import { EventRow } from '../components/EventRow'
import { dashboard as dashboardApi, events as eventsApi, infrastructure as infraApi } from '../api/client'
import type { DashboardSummaryResponse, Event, InfrastructureComponent, ResourceSummary } from '../api/types'
import './Dashboard.css'
import './Infrastructure.css'

type TimeFilter = 'day' | 'week' | 'month'

// ── Infra helpers (mirrors Infrastructure.tsx) ────────────────────────────────

const TYPE_LABEL: Record<string, string> = {
  proxmox_node:  'Proxmox Node',
  synology:      'Synology NAS',
  vm:            'VM',
  lxc:           'LXC',
  bare_metal:    'Bare Metal',
  linux_host:    'Linux Host',
  windows_host:  'Windows Host',
  generic_host:  'Generic Host',
  docker_engine: 'Docker Engine',
  traefik:       'Traefik',
  portainer:     'Portainer',
}

const NO_RESOURCE_BARS = new Set(['traefik', 'portainer', 'docker_engine'])

function statusClass(s: string): string {
  if (s === 'online')   return 'online'
  if (s === 'degraded') return 'degraded'
  if (s === 'offline')  return 'offline'
  return 'unknown'
}

function statusLabel(s: string): string {
  if (s === 'online')   return 'Online'
  if (s === 'degraded') return 'Degraded'
  if (s === 'offline')  return 'Offline'
  return 'Unknown'
}

function barClass(value: number, isDisk: boolean): string {
  if (!isDisk) return ''
  if (value > 95) return ' crit'
  if (value > 85) return ' warn'
  return ''
}

function ResBar({
  label, value, isDisk, noData,
}: { label: string; value: number; isDisk?: boolean; noData?: boolean }) {
  const cls = noData ? '' : barClass(value, !!isDisk)
  return (
    <div className="infra-res-row">
      <span className="infra-res-label">{label}</span>
      <div className="infra-res-track">
        <div
          className={`infra-res-fill${cls}${noData ? ' no-data' : ''}`}
          style={{ width: noData ? '0%' : `${Math.min(value, 100)}%` }}
        />
      </div>
      <span className={`infra-res-pct${noData ? ' no-data' : ''}`}>
        {noData ? 'Collecting…' : `${Math.round(value)}%`}
      </span>
    </div>
  )
}

// ── Event severity counts type ────────────────────────────────────────────────

interface EventCounts {
  info: number
  warn: number
  error: number
  critical: number
}

// ── Component ─────────────────────────────────────────────────────────────────

export function Dashboard() {
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()
  const [timeFilter, setTimeFilter] = useState<TimeFilter>('week')
  const [data, setData] = useState<DashboardSummaryResponse | null>(null)
  const [recentEvents, setRecentEvents] = useState<Event[]>([])
  const [eventCounts, setEventCounts] = useState<EventCounts | null>(null)
  const [infraComponents, setInfraComponents] = useState<InfrastructureComponent[]>([])
  const [resourcesMap, setResourcesMap] = useState<Record<string, ResourceSummary>>({})
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    void (async () => {
      setLoading(true)
      try {
        const since24h = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString()

        const [summary, evts, hosts, infoRes, warnRes, errorRes, critRes] = await Promise.all([
          dashboardApi.summary(timeFilter),
          eventsApi.list({ limit: 5 }),
          infraApi.list().catch(() => ({ data: [], total: 0 })),
          eventsApi.list({ level: 'info',     from: since24h, limit: 1 }).catch(() => ({ data: [], total: 0 })),
          eventsApi.list({ level: 'warn',     from: since24h, limit: 1 }).catch(() => ({ data: [], total: 0 })),
          eventsApi.list({ level: 'error',    from: since24h, limit: 1 }).catch(() => ({ data: [], total: 0 })),
          eventsApi.list({ level: 'critical', from: since24h, limit: 1 }).catch(() => ({ data: [], total: 0 })),
        ])

        setData(summary)
        setRecentEvents(evts.data)
        setInfraComponents(hosts.data)
        setEventCounts({
          info:     infoRes.total,
          warn:     warnRes.total,
          error:    errorRes.total,
          critical: critRes.total,
        })

        // Poll resource summaries for components that expose them
        const pollable = hosts.data.filter(c => !NO_RESOURCE_BARS.has(c.type))
        if (pollable.length > 0) {
          const results = await Promise.allSettled(
            pollable.map(c => infraApi.resources(c.id, 'hour').then(r => ({ id: c.id, data: r })))
          )
          const resMap: Record<string, ResourceSummary> = {}
          for (const r of results) {
            if (r.status === 'fulfilled') resMap[r.value.id] = r.value.data
          }
          setResourcesMap(resMap)
        }
      } catch (e) {
        console.error(e)
      } finally {
        setLoading(false)
      }
    })()
  }, [timeFilter, tick])

  const topbarStatus =
    data == null
      ? 'ok'
      : data.status === 'normal'
      ? 'ok'
      : (data.status as 'warn' | 'down')

  // ── Loading skeleton ──────────────────────────────────────────────────────
  if (loading) {
    return (
      <>
        <Topbar title="Dashboard" timeFilter={timeFilter} onTimeFilter={setTimeFilter} />
        <div className="content">
          <div className="summary-bar">
            {[0, 1, 2, 3, 4].map(i => (
              <div key={i} className="skeleton skeleton-bar" />
            ))}
          </div>
          <div className="two-col">
            <div className="col-left">
              <div className="widget-grid">
                {[0, 1, 2, 3].map(i => (
                  <div key={i} className="skeleton skeleton-card" />
                ))}
              </div>
            </div>
            <div className="col-right">
              {[0, 1, 2].map(i => (
                <div key={i} className="skeleton skeleton-bar" style={{ marginBottom: 6 }} />
              ))}
            </div>
          </div>
        </div>
      </>
    )
  }

  // ── Empty state ───────────────────────────────────────────────────────────
  if (!data || (data.apps.length === 0 && data.checks.length === 0 && infraComponents.length === 0)) {
    return (
      <>
        <Topbar title="Dashboard" timeFilter={timeFilter} onTimeFilter={setTimeFilter} />
        <div className="content">
          <div className="dashboard-empty">
            <div className="dashboard-empty-icon">
              <svg
                width="24"
                height="24"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
              >
                <rect x="2" y="3" width="20" height="14" rx="2" />
                <line x1="8" y1="21" x2="16" y2="21" />
                <line x1="12" y1="17" x2="12" y2="21" />
              </svg>
            </div>
            <div className="dashboard-empty-title">No apps configured yet</div>
            <div className="dashboard-empty-text">
              Add your first app to start receiving webhooks and monitoring events.
            </div>
            <div className="dashboard-empty-actions">
              <button
                className="dashboard-empty-btn"
                onClick={() => navigate('/apps')}
              >
                + Add your first app
              </button>
              <button
                className="dashboard-empty-btn secondary"
                onClick={() => navigate('/checks')}
              >
                + Add a monitor check
              </button>
            </div>
          </div>
        </div>
      </>
    )
  }

  // ── App name map for event rows ───────────────────────────────────────────
  const appNameMap: Record<string, string> = {}
  data.apps.forEach(a => {
    appNameMap[a.id] = a.name
  })

  // ── Infra card renderer (read-only, links to detail) ─────────────────────
  function renderInfraCard(host: InfrastructureComponent) {
    const res = resourcesMap[host.id]
    const noData = !res || res.no_data
    const needsResBar = !NO_RESOURCE_BARS.has(host.type)

    return (
      <div
        key={host.id}
        className="infra-card"
        style={{ cursor: 'pointer' }}
        onClick={() => navigate(`/infrastructure/${host.id}`)}
      >
        <div className="infra-card-header">
          <div className="infra-card-title-group">
            <div className="infra-card-name">
              {host.name}
              <span className="infra-card-nav-arrow" aria-hidden="true"> ›</span>
            </div>
            <div className="infra-card-meta">
              {TYPE_LABEL[host.type] ?? host.type} · {host.ip || '—'}
            </div>
          </div>
          <div className="infra-card-status-group">
            <span className={`infra-status-dot ${statusClass(host.last_status)}`} />
            <span className="infra-status-label">{statusLabel(host.last_status)}</span>
          </div>
        </div>

        {needsResBar && (
          <div className="infra-res-bars">
            <ResBar label="CPU" value={noData ? 0 : res!.cpu_percent} noData={noData} />
            <ResBar label="MEM" value={noData ? 0 : res!.mem_percent} noData={noData} />
            <ResBar label="DSK" value={noData ? 0 : res!.disk_percent} isDisk noData={noData} />
          </div>
        )}
      </div>
    )
  }

  // ── Full dashboard ────────────────────────────────────────────────────────
  return (
    <>
      <Topbar
        title="Dashboard"
        status={topbarStatus}
        timeFilter={timeFilter}
        onTimeFilter={setTimeFilter}
        onAdd={() => navigate('/apps')}
      />
      <div className="content">

        {/* Summary Bar */}
        {data.summary_bar.length > 0 && (
          <div className="summary-bar">
            {data.summary_bar.map(item => (
              <SummaryCard key={item.label} item={item} />
            ))}
          </div>
        )}

        {/* Two-column layout */}
        <div className="two-col">

          {/* ── LEFT COLUMN ── */}
          <div className="col-left">

            {/* Apps */}
            <div>
              <div className="section-header">
                <div className="section-title">Apps</div>
                <button className="section-action" onClick={() => navigate('/apps')}>
                  <svg
                    width="10"
                    height="10"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                  >
                    <line x1="12" y1="5" x2="12" y2="19" />
                    <line x1="5" y1="12" x2="19" y2="12" />
                  </svg>
                  Add app
                </button>
              </div>
              <div className="widget-grid">
                {data.apps.map(app => (
                  <AppWidget
                    key={app.id}
                    app={app}
                    onClick={() => navigate(`/apps/${app.id}`)}
                  />
                ))}
              </div>
            </div>

            {/* Events (24h) — severity counts */}
            {eventCounts !== null && (
              <div>
                <div className="section-header">
                  <div className="section-title">Events (24h)</div>
                  <button className="section-action" onClick={() => navigate('/events')}>
                    View all →
                  </button>
                </div>
                <div className="event-counts-row">
                  <div className="event-count-card info">
                    <div className="event-count-value">{eventCounts.info}</div>
                    <div className="event-count-label">Info</div>
                  </div>
                  <div className="event-count-card warn">
                    <div className="event-count-value">{eventCounts.warn}</div>
                    <div className="event-count-label">Warn</div>
                  </div>
                  <div className="event-count-card error">
                    <div className="event-count-value">{eventCounts.error}</div>
                    <div className="event-count-label">Error</div>
                  </div>
                  <div className="event-count-card critical">
                    <div className="event-count-value">{eventCounts.critical}</div>
                    <div className="event-count-label">Critical</div>
                  </div>
                </div>
              </div>
            )}

            {/* Recent Events (below counts) */}
            {recentEvents.length > 0 && (
              <div>
                <div className="section-header">
                  <div className="section-title">Recent Events</div>
                  <button className="section-action" onClick={() => navigate('/events')}>
                    View all →
                  </button>
                </div>
                <div className="events-panel">
                  {recentEvents.map(event => (
                    <EventRow
                      key={event.id}
                      event={event}
                      appName={event.source_name || undefined}
                    />
                  ))}
                </div>
              </div>
            )}

          </div>

          {/* ── RIGHT COLUMN ── */}
          <div className="col-right">

            {/* Monitor Checks */}
            {data.checks.length > 0 && (
              <div>
                <div className="section-header">
                  <div className="section-title">Monitor Checks</div>
                  <button className="section-action" onClick={() => navigate('/checks')}>
                    <svg
                      width="10"
                      height="10"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                    >
                      <line x1="12" y1="5" x2="12" y2="19" />
                      <line x1="5" y1="12" x2="19" y2="12" />
                    </svg>
                    Add check
                  </button>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                  {data.checks.map(check => (
                    <MonitorWidget
                      key={check.id}
                      check={check}
                      onClick={() => navigate(`/checks/${check.id}`)}
                    />
                  ))}
                </div>
              </div>
            )}

            {/* SSL Certificates */}
            {data.ssl_certs.length > 0 && (
              <div>
                <div className="section-header">
                  <div className="section-title">SSL Certificates</div>
                </div>
                <div className="ssl-panel">
                  {data.ssl_certs.map((cert, i) => (
                    <SSLRow key={cert.domain || i} cert={cert} />
                  ))}
                </div>
              </div>
            )}

          </div>
        </div>

        {/* ── INFRASTRUCTURE — full-width below two-col ── */}
        {infraComponents.length > 0 && (
          <div>
            <div className="section-header">
              <div className="section-title">Infrastructure</div>
              <button className="section-action" onClick={() => navigate('/infrastructure')}>
                View all →
              </button>
            </div>
            <div className="dash-infra-grid">
              {infraComponents.map(host => renderInfraCard(host))}
            </div>
          </div>
        )}

      </div>
    </>
  )
}
