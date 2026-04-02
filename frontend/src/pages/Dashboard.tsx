import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { SummaryCard } from '../components/SummaryCard'
import { AppWidget } from '../components/AppWidget'
import { BookmarkWidget } from '../components/BookmarkWidget'
import { dashboard as dashboardApi, events as eventsApi, infrastructure as infraApi } from '../api/client'
import type { DashboardSummaryResponse, InfrastructureComponent, ResourceSummary } from '../api/types'
import { InfraTypeIcon, CheckTypeIcon } from '../components/CheckTypeIcon'
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
  const [eventCounts, setEventCounts] = useState<EventCounts | null>(null)
  const [infraComponents, setInfraComponents] = useState<InfrastructureComponent[]>([])
  const [resourcesMap, setResourcesMap] = useState<Record<string, ResourceSummary>>({})
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    void (async () => {
      setLoading(true)
      try {
        const periodMs = timeFilter === 'day' ? 86_400_000 : timeFilter === 'week' ? 7 * 86_400_000 : 30 * 86_400_000
        const sinceFilter = new Date(Date.now() - periodMs).toISOString()

        const [summary, hosts, infoRes, warnRes, errorRes, critRes] = await Promise.all([
          dashboardApi.summary(timeFilter),
          infraApi.list().catch(() => ({ data: [], total: 0 })),
          eventsApi.list({ level: 'info',     from: sinceFilter, limit: 1 }).catch(() => ({ data: [], total: 0 })),
          eventsApi.list({ level: 'warn',     from: sinceFilter, limit: 1 }).catch(() => ({ data: [], total: 0 })),
          eventsApi.list({ level: 'error',    from: sinceFilter, limit: 1 }).catch(() => ({ data: [], total: 0 })),
          eventsApi.list({ level: 'critical', from: sinceFilter, limit: 1 }).catch(() => ({ data: [], total: 0 })),
        ])

        setData(summary)
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
          <div className="widget-grid">
            {[0, 1, 2, 3].map(i => (
              <div key={i} className="skeleton skeleton-card" />
            ))}
          </div>
          <div className="check-rollup-grid">
            {[0, 1, 2, 3].map(i => (
              <div key={i} className="skeleton skeleton-bar" />
            ))}
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

  // ── Check type rollup — always show all 4 types ──────────────────────────
  // avgUptime is derived from last_status on the backend (up=100, warn=75, down/critical/unknown=0).
  // Colour thresholds: green ≥95%, yellow ≥75%, red <75%, blue = no checks configured.
  const ALL_CHECK_TYPES = ['url', 'ssl', 'dns', 'ping'] as const
  type RollupStatus = 'up' | 'warn' | 'down' | 'empty'
  type RollupEntry = { type: string; total: number; avgUptime: number; notUp: number; status: RollupStatus }

  function rollupColour(avgUptime: number): 'up' | 'warn' | 'down' {
    if (avgUptime >= 95) return 'up'
    if (avgUptime >= 75) return 'warn'
    return 'down'
  }

  const checkRollup: RollupEntry[] = ALL_CHECK_TYPES.map(type => {
    const ofType = data.checks.filter(c => c.type === type)
    if (ofType.length === 0) return { type, total: 0, avgUptime: 0, notUp: 0, status: 'empty' as RollupStatus }
    const upPctSum = ofType.reduce((acc, c) => acc + c.uptime_pct, 0)
    const notUp = ofType.filter(c => c.status !== 'up').length
    const avgUptime = upPctSum / ofType.length
    return { type, total: ofType.length, avgUptime, notUp, status: rollupColour(avgUptime) }
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
          <InfraTypeIcon type={host.type} />
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

        {/* Apps — split into full widgets (webhook/digest capable) and service cards */}
        {(() => {
          const SERVICE_CAPS = new Set(['monitor_only', 'docker_only'])
          const fullApps = data.apps.filter(a => !SERVICE_CAPS.has(a.capability ?? ''))
          const serviceApps = data.apps.filter(a => SERVICE_CAPS.has(a.capability ?? ''))

          return (
            <>
              {fullApps.length > 0 && (
                <div>
                  <div className="section-header">
                    <div className="section-title">Apps</div>
                    <button className="section-action" onClick={() => navigate('/apps')}>
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <line x1="12" y1="5" x2="12" y2="19" />
                        <line x1="5" y1="12" x2="19" y2="12" />
                      </svg>
                      Add app
                    </button>
                  </div>
                  <div className="widget-grid">
                    {fullApps.map(app => (
                      <AppWidget
                        key={app.id}
                        app={app}
                        onClick={() => navigate(`/apps/${app.id}`)}
                      />
                    ))}
                  </div>
                </div>
              )}

              {serviceApps.length > 0 && (
                <div>
                  <div className="section-header">
                    <div className="section-title">Services</div>
                    {fullApps.length === 0 && (
                      <button className="section-action" onClick={() => navigate('/apps')}>
                        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                          <line x1="12" y1="5" x2="12" y2="19" />
                          <line x1="5" y1="12" x2="19" y2="12" />
                        </svg>
                        Add app
                      </button>
                    )}
                  </div>
                  <div className="services-grid">
                    {serviceApps.map(app => (
                      <BookmarkWidget
                        key={app.id}
                        app={app}
                        onClick={() => navigate(`/apps/${app.id}`)}
                      />
                    ))}
                  </div>
                </div>
              )}

              {data.apps.length > 0 && fullApps.length === 0 && serviceApps.length === 0 && (
                <div>
                  <div className="section-header">
                    <div className="section-title">Apps</div>
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
              )}
            </>
          )
        })()}

        {/* Events — severity counts */}
        {eventCounts !== null && (
          <div>
            <div className="section-header">
              <div className="section-title">
                Events ({timeFilter === 'day' ? '24h' : timeFilter === 'week' ? '7d' : '30d'})
              </div>
              <button className="section-action" onClick={() => navigate('/events')}>
                View all →
              </button>
            </div>
            <div className="event-counts-row">
              <div className="event-count-card info" style={{ cursor: 'pointer' }} onClick={() => navigate('/events?level=info')}>
                <svg className="event-count-icon" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                  <circle cx="10" cy="10" r="8" />
                  <path d="M10 9v5M10 7h.01" />
                </svg>
                <div className="event-count-value">{eventCounts.info}</div>
                <div className="event-count-label">Info</div>
              </div>
              <div className="event-count-card warn" style={{ cursor: 'pointer' }} onClick={() => navigate('/events?level=warn')}>
                <svg className="event-count-icon" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M10 3L18.5 17H1.5L10 3z" />
                  <path d="M10 9v4M10 15h.01" />
                </svg>
                <div className="event-count-value">{eventCounts.warn}</div>
                <div className="event-count-label">Warn</div>
              </div>
              <div className="event-count-card error" style={{ cursor: 'pointer' }} onClick={() => navigate('/events?level=error')}>
                <svg className="event-count-icon" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                  <circle cx="10" cy="10" r="8" />
                  <path d="M7 7l6 6M13 7l-6 6" />
                </svg>
                <div className="event-count-value">{eventCounts.error}</div>
                <div className="event-count-label">Error</div>
              </div>
              <div className="event-count-card critical" style={{ cursor: 'pointer' }} onClick={() => navigate('/events?level=critical')}>
                <svg className="event-count-icon" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M10 2c0 4-4 5-4 9a4 4 0 0 0 8 0c0-4-4-5-4-9z" />
                  <path d="M8 17.5a2 2 0 0 0 4 0" />
                </svg>
                <div className="event-count-value">{eventCounts.critical}</div>
                <div className="event-count-label">Critical</div>
              </div>
            </div>
          </div>
        )}

        {/* Monitor Checks — always show all 4 types */}
        <div>
          <div className="section-header">
            <div className="section-title">Monitor Checks</div>
            <button className="section-action" onClick={() => navigate('/checks')}>
              <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <line x1="12" y1="5" x2="12" y2="19" />
                <line x1="5" y1="12" x2="19" y2="12" />
              </svg>
              Add check
            </button>
          </div>
          <div className="check-rollup-grid">
            {checkRollup.map(r => (
              <div
                key={r.type}
                className={`check-rollup-card ${r.status}`}
                onClick={() => navigate('/checks')}
              >
                <div className="check-rollup-type">
                  <CheckTypeIcon type={r.type} size={14} />
                  {r.type.toUpperCase()}
                </div>
                <div className="check-rollup-uptime">
                  {r.status === 'empty' ? '—' : `${r.avgUptime.toFixed(1)}%`}
                </div>
                <div className="check-rollup-meta">
                  {r.total} check{r.total !== 1 ? 's' : ''}
                  {r.notUp > 0 && <span className="check-rollup-not-up"> · {r.notUp} not up</span>}
                </div>
              </div>
            ))}
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
