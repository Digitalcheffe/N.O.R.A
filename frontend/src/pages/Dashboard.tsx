import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { SummaryCard } from '../components/SummaryCard'
import { AppWidget } from '../components/AppWidget'
import { MonitorWidget } from '../components/MonitorWidget'
import { SSLRow } from '../components/SSLRow'
import { EventRow } from '../components/EventRow'
import { HostWidget } from '../components/HostWidget'
import type { HostData } from '../components/HostWidget'
import { dashboard as dashboardApi, events as eventsApi, topology as topoApi } from '../api/client'
import type { DashboardSummaryResponse, Event, PhysicalHost } from '../api/types'
import './Dashboard.css'

type TimeFilter = 'day' | 'week' | 'month'

export function Dashboard() {
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()
  const [timeFilter, setTimeFilter] = useState<TimeFilter>('week')
  const [data, setData] = useState<DashboardSummaryResponse | null>(null)
  const [recentEvents, setRecentEvents] = useState<Event[]>([])
  const [physicalHosts, setPhysicalHosts] = useState<PhysicalHost[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    Promise.all([
      dashboardApi.summary(timeFilter),
      eventsApi.list({ limit: 5 }),
      topoApi.physicalHosts.list(),
    ])
      .then(([summary, evts, hosts]) => {
        setData(summary)
        setRecentEvents(evts.data)
        setPhysicalHosts(hosts.data)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
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
  if (!data || data.apps.length === 0) {
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

        {/* Summary Bar — only when there are digest categories */}
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

            {/* Recent Events */}
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
                      appName={appNameMap[event.app_id] ?? event.app_id}
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
                    <MonitorWidget key={check.id} check={check} />
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

            {/* Infrastructure — only shown when hosts are configured */}
            {physicalHosts.length > 0 && (
              <div>
                <div className="section-header">
                  <div className="section-title">Infrastructure</div>
                  <button className="section-action" onClick={() => navigate('/topology')}>
                    View all →
                  </button>
                </div>
                <div className="widget-grid">
                  {physicalHosts.map(host => {
                    const hostData: HostData = {
                      id:        host.id,
                      name:      host.name,
                      type:      host.type === 'proxmox_node' ? 'Proxmox Node' : 'Bare Metal',
                      ip:        host.ip,
                      status:    'unknown',
                      resources: [
                        { label: 'CPU', pct: 0 },
                        { label: 'MEM', pct: 0 },
                        { label: 'DSK', pct: 0 },
                      ],
                    }
                    return (
                      <HostWidget
                        key={host.id}
                        host={hostData}
                        onClick={() => navigate('/topology')}
                      />
                    )
                  })}
                </div>
              </div>
            )}

          </div>
        </div>
      </div>
    </>
  )
}
