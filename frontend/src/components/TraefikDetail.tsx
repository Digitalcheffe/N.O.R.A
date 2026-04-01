import { useState, useEffect, useCallback, useRef } from 'react'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { traefik as traefikApi } from '../api/client'
import type {
  InfrastructureComponent,
  TraefikOverview,
  DiscoveredRoute,
  TraefikServiceDetail,
} from '../api/types'
import './TraefikDetail.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function parseDomain(rule: string): string {
  const m = rule.match(/Host\(`([^`]+)`\)/)
  if (m) return m[1]
  return rule.length > 60 ? rule.slice(0, 57) + '…' : rule
}

function parseEntryPoints(ep: string | null): string[] {
  if (!ep) return []
  try {
    const parsed = JSON.parse(ep)
    if (Array.isArray(parsed)) return parsed
  } catch {
    return [ep]
  }
  return []
}

function parseServerStatus(json: string): Record<string, string> {
  try {
    return JSON.parse(json) as Record<string, string>
  } catch {
    return {}
  }
}

// ── Shared sub-components ─────────────────────────────────────────────────────

function SectionError({ msg, onRetry }: { msg: string; onRetry: () => void }) {
  return (
    <div className="tk-section-error">
      <span>{msg}</span>
      <button className="tk-retry-btn" onClick={onRetry}>Retry</button>
    </div>
  )
}

function SkeletonRows({ count = 4 }: { count?: number }) {
  return (
    <>
      {Array.from({ length: count }).map((_, i) => (
        <tr key={i} className="tk-skeleton-row">
          <td colSpan={99}><div className="tk-skeleton-bar" /></td>
        </tr>
      ))}
    </>
  )
}

// ── Overview section ──────────────────────────────────────────────────────────

function OverviewSection({
  overview,
  loading,
  error,
  onRetry,
  routersRef,
}: {
  overview: TraefikOverview | null
  loading: boolean
  error: string | null
  onRetry: () => void
  routersRef: React.RefObject<HTMLDivElement | null>
}) {
  function scrollToRouters() {
    routersRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <div className="tk-section">
      <div className="tk-section-title">Overview</div>
      {error ? (
        <SectionError msg={error} onRetry={onRetry} />
      ) : (
        <div className="tk-overview-card">
          {loading ? (
            <div className="tk-overview-loading">
              <div className="tk-skeleton-bar" style={{ width: '60%' }} />
              <div className="tk-skeleton-bar" style={{ width: '50%', marginTop: 8 }} />
              <div className="tk-skeleton-bar" style={{ width: '40%', marginTop: 8 }} />
            </div>
          ) : overview ? (
            <>
              <div className="tk-overview-row">
                <span className="tk-overview-label">Routers</span>
                <span className="tk-overview-value">{overview.routers_total} total</span>
                {overview.routers_errors > 0 && (
                  <span className="tk-badge error">✕ {overview.routers_errors} error{overview.routers_errors !== 1 ? 's' : ''}</span>
                )}
                {overview.routers_warnings > 0 && (
                  <span className="tk-badge warn">─ {overview.routers_warnings} warning{overview.routers_warnings !== 1 ? 's' : ''}</span>
                )}
              </div>
              <div className="tk-overview-row">
                <span className="tk-overview-label">Services</span>
                <span className="tk-overview-value">{overview.services_total} total</span>
                {overview.services_errors > 0 && (
                  <span className="tk-badge error">✕ {overview.services_errors} error{overview.services_errors !== 1 ? 's' : ''}</span>
                )}
              </div>
              <div className="tk-overview-row">
                <span className="tk-overview-label">Middlewares</span>
                <span className="tk-overview-value">{overview.middlewares_total} total</span>
              </div>
              {overview.routers_errors > 0 && (
                <div
                  className="tk-error-banner"
                  onClick={scrollToRouters}
                  role="button"
                  tabIndex={0}
                  onKeyDown={e => e.key === 'Enter' && scrollToRouters()}
                >
                  ⚠ {overview.routers_errors} router{overview.routers_errors !== 1 ? 's' : ''} have configuration errors — check the Routers section below
                </div>
              )}
            </>
          ) : (
            <div className="tk-empty">No overview data — run Discover Now to poll Traefik.</div>
          )}
        </div>
      )}
    </div>
  )
}

// ── Routers section ───────────────────────────────────────────────────────────

type RouterStatusFilter = 'all' | 'active' | 'disabled'

function RoutersSection({
  routers,
  loading,
  error,
  onRetry,
  sectionRef,
  serviceHealthMap,
}: {
  routers: DiscoveredRoute[]
  loading: boolean
  error: string | null
  onRetry: () => void
  sectionRef: React.RefObject<HTMLDivElement | null>
  serviceHealthMap: Record<string, string>
}) {
  const [statusFilter,   setStatusFilter]   = useState<RouterStatusFilter>('all')
  const [providerFilter, setProviderFilter] = useState<string>('all')
  const [tooltipFor,     setTooltipFor]     = useState<string | null>(null)

  const providers = Array.from(
    new Set(routers.map(r => r.provider).filter((p): p is string => !!p))
  ).sort()

  const sorted = [...routers].sort((a, b) => {
    const aDisabled = a.router_status !== 'enabled'
    const bDisabled = b.router_status !== 'enabled'
    if (aDisabled && !bDisabled) return -1
    if (!aDisabled && bDisabled) return 1
    return a.router_name.localeCompare(b.router_name)
  })

  const filtered = sorted.filter(r => {
    if (statusFilter === 'active'   && r.router_status !== 'enabled') return false
    if (statusFilter === 'disabled' && r.router_status === 'enabled') return false
    if (providerFilter !== 'all' && r.provider !== providerFilter)    return false
    return true
  })

  const disabledCount = routers.filter(r => r.router_status !== 'enabled').length
  const countLabel = statusFilter !== 'all'
    ? `${filtered.length} of ${routers.length} routers`
    : disabledCount > 0
      ? `${disabledCount} disabled of ${routers.length}`
      : `${routers.length} router${routers.length !== 1 ? 's' : ''}`

  return (
    <div className="tk-section" ref={sectionRef}>
      <div className="tk-section-header-row">
        <div className="tk-section-title" style={{ margin: 0 }}>Routers</div>
        <div className="tk-filters">
          <select
            className="tk-filter-select"
            value={statusFilter}
            onChange={e => setStatusFilter(e.target.value as RouterStatusFilter)}
          >
            <option value="all">All</option>
            <option value="active">Active only</option>
            <option value="disabled">Disabled only</option>
          </select>
          <select
            className="tk-filter-select"
            value={providerFilter}
            onChange={e => setProviderFilter(e.target.value)}
          >
            <option value="all">All providers</option>
            {providers.map(p => (
              <option key={p} value={p}>{p}</option>
            ))}
          </select>
          <span className="tk-count-label">{countLabel}</span>
        </div>
      </div>

      {error ? (
        <SectionError msg={error} onRetry={onRetry} />
      ) : (
        <table className="tk-table">
          <thead>
            <tr>
              <th></th>
              <th>Domain / Rule</th>
              <th></th>
              <th>Service</th>
              <th>Provider</th>
              <th>Entrypoint</th>
              <th></th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <SkeletonRows count={5} />
            ) : filtered.length === 0 ? (
              <tr>
                <td colSpan={8} className="tk-empty-cell">
                  {routers.length === 0
                    ? 'No routers found — verify Traefik API is accessible.'
                    : 'No routers match the current filters.'}
                </td>
              </tr>
            ) : (
              filtered.map(r => {
                const isEnabled = r.router_status === 'enabled'
                const domain    = r.domain || parseDomain(r.rule)
                const svcName   = r.service_name || r.backend_service || '—'
                const eps       = parseEntryPoints(r.entry_points)
                const ep0       = eps[0] ?? '—'
                const health    = serviceHealthMap[svcName]

                return (
                  <tr key={r.id} className={isEnabled ? '' : 'tk-row-disabled'}>
                    <td>
                      <span className={`tk-status-dot ${isEnabled ? 'online' : 'offline'}`} />
                    </td>
                    <td className="tk-domain">{domain}</td>
                    <td className="tk-arrow tk-muted">→</td>
                    <td
                      className="tk-service-name"
                      onMouseEnter={() => health ? setTooltipFor(r.id) : undefined}
                      onMouseLeave={() => setTooltipFor(null)}
                      style={{ position: 'relative' }}
                    >
                      {svcName}
                      {tooltipFor === r.id && health && (
                        <div className="tk-tooltip">{health}</div>
                      )}
                    </td>
                    <td>
                      {r.provider && (
                        <span className="tk-badge-muted">{r.provider}</span>
                      )}
                    </td>
                    <td className="tk-muted">{ep0}</td>
                    <td>
                      {r.has_tls_resolver === 1 && (
                        <span className="tk-tls-icon" title={r.cert_resolver_name ?? 'TLS'}>🔒</span>
                      )}
                    </td>
                    <td>
                      {!isEnabled && (
                        <span className="tk-badge error tk-badge-sm">DISABLED</span>
                      )}
                    </td>
                  </tr>
                )
              })
            )}
          </tbody>
        </table>
      )}
    </div>
  )
}

// ── Services section ──────────────────────────────────────────────────────────

type ServiceStatusFilter = 'all' | 'healthy' | 'degraded'

function ServicesSection({
  services,
  loading,
  error,
  onRetry,
}: {
  services: TraefikServiceDetail[]
  loading: boolean
  error: string | null
  onRetry: () => void
}) {
  const [statusFilter, setStatusFilter] = useState<ServiceStatusFilter>('all')

  const sorted = [...services].sort((a, b) => {
    const aDown = a.servers_down
    const bDown = b.servers_down
    if (aDown > 0 && bDown === 0) return -1
    if (aDown === 0 && bDown > 0) return 1
    return a.service_name.localeCompare(b.service_name)
  })

  const filtered = sorted.filter(s => {
    if (statusFilter === 'healthy'  && s.servers_down > 0)  return false
    if (statusFilter === 'degraded' && s.servers_down === 0) return false
    return true
  })

  const degradedCount = services.filter(s => s.servers_down > 0).length
  const countLabel = statusFilter !== 'all'
    ? `${filtered.length} of ${services.length} services`
    : degradedCount > 0
      ? `${degradedCount} degraded of ${services.length}`
      : `${services.length} service${services.length !== 1 ? 's' : ''}`

  return (
    <div className="tk-section">
      <div className="tk-section-header-row">
        <div className="tk-section-title" style={{ margin: 0 }}>Services</div>
        <div className="tk-filters">
          <select
            className="tk-filter-select"
            value={statusFilter}
            onChange={e => setStatusFilter(e.target.value as ServiceStatusFilter)}
          >
            <option value="all">All</option>
            <option value="healthy">Healthy</option>
            <option value="degraded">Degraded</option>
          </select>
          <span className="tk-count-label">{countLabel}</span>
        </div>
      </div>

      {error ? (
        <SectionError msg={error} onRetry={onRetry} />
      ) : (
        <table className="tk-table">
          <thead>
            <tr>
              <th></th>
              <th>Service</th>
              <th>Type</th>
              <th>Health</th>
              <th>Endpoints</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <SkeletonRows count={5} />
            ) : filtered.length === 0 ? (
              <tr>
                <td colSpan={5} className="tk-empty-cell">
                  {services.length === 0
                    ? 'No services found.'
                    : 'No services match the current filters.'}
                </td>
              </tr>
            ) : (
              filtered.map(s => {
                const serverMap    = parseServerStatus(s.server_status_json)
                const serverEntries = Object.entries(serverMap)
                const first        = serverEntries[0]
                const extra        = serverEntries.length > 1 ? serverEntries.length - 1 : 0

                const allDown  = s.server_count > 0 && s.servers_down === s.server_count
                const someDown = s.servers_down > 0 && !allDown
                const dotClass = allDown ? 'offline' : someDown ? 'degraded' : 'online'
                const healthFraction = s.server_count > 0 ? `${s.servers_up}/${s.server_count} UP` : '—'

                return (
                  <tr key={s.id} className={s.servers_down > 0 ? 'tk-row-error' : ''}>
                    <td><span className={`tk-status-dot ${dotClass}`} /></td>
                    <td className="tk-svc-name">{s.service_name}</td>
                    <td><span className="tk-badge-muted">{s.service_type}</span></td>
                    <td className={allDown ? 'tk-health-down' : someDown ? 'tk-health-partial' : 'tk-health-up'}>
                      {healthFraction}
                    </td>
                    <td className="tk-endpoints-cell">
                      {first ? (
                        <>
                          <span className={first[1].toUpperCase() === 'DOWN' ? 'tk-endpoint-down' : 'tk-endpoint'}>
                            {first[0]}
                          </span>
                          {extra > 0 && <span className="tk-muted"> +{extra}</span>}
                        </>
                      ) : (
                        <span className="tk-muted">—</span>
                      )}
                    </td>
                  </tr>
                )
              })
            )}
          </tbody>
        </table>
      )}
    </div>
  )
}

// ── TraefikContent ────────────────────────────────────────────────────────────
// Content-only component rendered inside InfraComponentDetail's DetailPageLayout
// shell. Exposes key data points via onOverviewLoaded so the parent can show
// them in the header.

interface TraefikContentProps {
  component: InfrastructureComponent
  onOverviewLoaded?: (overview: TraefikOverview | null) => void
}

export function TraefikContent({ component, onOverviewLoaded }: TraefikContentProps) {
  const { tick } = useAutoRefresh()
  const componentId = component.id
  const routersSectionRef = useRef<HTMLDivElement>(null)

  const [overview,        setOverview]        = useState<TraefikOverview | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(true)
  const [overviewError,   setOverviewError]   = useState<string | null>(null)

  const [routers,        setRouters]        = useState<DiscoveredRoute[]>([])
  const [routersLoading, setRoutersLoading] = useState(true)
  const [routersError,   setRoutersError]   = useState<string | null>(null)

  const [services,        setServices]        = useState<TraefikServiceDetail[]>([])
  const [servicesLoading, setServicesLoading] = useState(true)
  const [servicesError,   setServicesError]   = useState<string | null>(null)

  const loadOverview = useCallback(() => {
    setOverviewLoading(true)
    traefikApi.getOverview(componentId)
      .then(ov => { setOverview(ov); setOverviewError(null); onOverviewLoaded?.(ov) })
      .catch(err => { setOverviewError(err instanceof Error ? err.message : 'Failed to load overview'); onOverviewLoaded?.(null) })
      .finally(() => setOverviewLoading(false))
  }, [componentId, onOverviewLoaded])

  const loadRouters = useCallback(() => {
    setRoutersLoading(true)
    traefikApi.getRouters(componentId)
      .then(r => { setRouters(r.data); setRoutersError(null) })
      .catch(err => setRoutersError(err instanceof Error ? err.message : 'Failed to load routers'))
      .finally(() => setRoutersLoading(false))
  }, [componentId])

  const loadServices = useCallback(() => {
    setServicesLoading(true)
    traefikApi.getServices(componentId)
      .then(r => { setServices(r.data); setServicesError(null) })
      .catch(err => setServicesError(err instanceof Error ? err.message : 'Failed to load services'))
      .finally(() => setServicesLoading(false))
  }, [componentId])

  useEffect(() => {
    loadOverview()
    loadRouters()
    loadServices()
  }, [loadOverview, loadRouters, loadServices, tick])

  const serviceHealthMap: Record<string, string> = {}
  for (const s of services) {
    serviceHealthMap[s.service_name] = `${s.servers_up}/${s.server_count} UP`
  }

  return (
    <>
      <OverviewSection
        overview={overviewLoading ? null : overview}
        loading={overviewLoading}
        error={overviewError}
        onRetry={loadOverview}
        routersRef={routersSectionRef}
      />
      <RoutersSection
        routers={routers}
        loading={routersLoading}
        error={routersError}
        onRetry={loadRouters}
        sectionRef={routersSectionRef}
        serviceHealthMap={serviceHealthMap}
      />
      <ServicesSection
        services={services}
        loading={servicesLoading}
        error={servicesError}
        onRetry={loadServices}
      />
    </>
  )
}

// Backward compat alias — App.tsx still imports TraefikDetail for the old route
// which will be removed in this same cleanup pass.
export { TraefikContent as TraefikDetail }

// Helper used by InfraComponentDetail to build key data points from the overview.
export function traefikKeyDataPoints(overview: TraefikOverview | null): { label: string; value: string }[] {
  return [
    { label: 'Version',  value: overview?.version ?? '—' },
    { label: 'Routers',  value: overview ? String(overview.routers_total) : '—' },
    { label: 'Services', value: overview ? String(overview.services_total) : '—' },
  ]
}

