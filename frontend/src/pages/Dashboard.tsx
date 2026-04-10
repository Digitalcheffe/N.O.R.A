import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { SummaryCard } from '../components/SummaryCard'
import { AppWidget } from '../components/AppWidget'
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
  vm_linux:      'VM Linux',
  vm_windows:    'VM Windows',
  vm_other:      'VM Other',
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

        {/* Events + Monitor Checks — side by side, each 2×2 */}
        <div className="dash-signals-row">

          {eventCounts !== null && (
            <div className="dash-signals-col">
              <div className="section-header">
                <div className="section-title">
                  Events ({timeFilter === 'day' ? '24h' : timeFilter === 'week' ? '7d' : '30d'})
                </div>
              </div>
              <div className="event-counts-row">
                <div className="event-count-card info" style={{ cursor: 'pointer' }} onClick={() => navigate('/events?level=info')}>
                  <div className="event-count-type">INFO</div>
                  <div className="event-count-value">{eventCounts.info}</div>
                  <div className="event-count-meta">{eventCounts.info === 1 ? '1 event' : `${eventCounts.info} events`}</div>
                </div>
                <div className="event-count-card warn" style={{ cursor: 'pointer' }} onClick={() => navigate('/events?level=warn')}>
                  <div className="event-count-type">WARN</div>
                  <div className="event-count-value">{eventCounts.warn}</div>
                  <div className="event-count-meta">{eventCounts.warn === 1 ? '1 event' : `${eventCounts.warn} events`}</div>
                </div>
                <div className="event-count-card error" style={{ cursor: 'pointer' }} onClick={() => navigate('/events?level=error')}>
                  <div className="event-count-type">ERROR</div>
                  <div className="event-count-value">{eventCounts.error}</div>
                  <div className="event-count-meta">{eventCounts.error === 1 ? '1 event' : `${eventCounts.error} events`}</div>
                </div>
                <div className="event-count-card critical" style={{ cursor: 'pointer' }} onClick={() => navigate('/events?level=critical')}>
                  <div className="event-count-type">CRITICAL</div>
                  <div className="event-count-value">{eventCounts.critical}</div>
                  <div className="event-count-meta">{eventCounts.critical === 1 ? '1 event' : `${eventCounts.critical} events`}</div>
                </div>
              </div>
            </div>
          )}

          <div className="dash-signals-col">
            <div className="section-header">
              <div className="section-title">Monitor Checks</div>
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

        </div>

        {/* Apps — unified list (all types) */}
        {data.apps.length > 0 && (
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

        {/* ── INFRASTRUCTURE — full-width below two-col ── */}
        {infraComponents.length > 0 && (
          <div>
            <div className="section-header">
              <div className="section-title">Infrastructure</div>
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
