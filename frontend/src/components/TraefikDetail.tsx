import { useState, useEffect, useCallback, useRef } from 'react'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { traefik as traefikApi } from '../api/client'
import type {
  InfrastructureComponent,
  TraefikOverview,
  DiscoveredRoute,
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

function parseServersJSON(raw: string | null): [string, string][] {
  if (!raw) return []
  try {
    const obj = JSON.parse(raw) as Record<string, string>
    return Object.entries(obj)
  } catch {
    return []
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

// ── Routers + Services unified section ───────────────────────────────────────

type RouterStatusFilter = 'all' | 'active' | 'disabled'

function RoutersSection({
  routers,
  loading,
  error,
  onRetry,
  sectionRef,
}: {
  routers: DiscoveredRoute[]
  loading: boolean
  error: string | null
  onRetry: () => void
  sectionRef: React.RefObject<HTMLDivElement | null>
}) {
  const [statusFilter,   setStatusFilter]   = useState<RouterStatusFilter>('all')
  const [providerFilter, setProviderFilter] = useState<string>('all')

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
              <th>Health</th>
              <th>Backends</th>
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
                <td colSpan={10} className="tk-empty-cell">
                  {routers.length === 0
                    ? 'No routers found — verify Traefik API is accessible.'
                    : 'No routers match the current filters.'}
                </td>
              </tr>
            ) : (
              filtered.map(r => {
                const isEnabled   = r.router_status === 'enabled'
                const domain      = r.domain || parseDomain(r.rule)
                const svcName     = r.service_name || '—'
                const eps         = parseEntryPoints(r.entry_points)
                const ep0         = eps[0] ?? '—'
                const servers     = parseServersJSON(r.servers_json)
                const hasHealth   = r.servers_total > 0
                const allDown     = hasHealth && r.servers_down === r.servers_total
                const someDown    = r.servers_down > 0 && !allDown
                const svcDotClass = allDown ? 'offline' : someDown ? 'degraded' : hasHealth ? 'online' : 'unknown'
                const healthLabel = hasHealth ? `${r.servers_up}/${r.servers_total}` : '—'

                return (
                  <tr key={r.id} className={isEnabled ? '' : 'tk-row-disabled'}>
                    <td>
                      <span className={`tk-status-dot ${isEnabled ? 'online' : 'offline'}`} />
                    </td>
                    <td className="tk-domain">{domain}</td>
                    <td className="tk-arrow tk-muted">→</td>
                    <td className="tk-service-name">{svcName}</td>
                    <td>
                      {hasHealth ? (
                        <span className="tk-svc-health">
                          <span className={`tk-status-dot ${svcDotClass}`} />
                          <span className={allDown ? 'tk-health-down' : someDown ? 'tk-health-partial' : 'tk-health-up'}>
                            {healthLabel}
                          </span>
                        </span>
                      ) : (
                        <span className="tk-muted">—</span>
                      )}
                    </td>
                    <td className="tk-backends-cell">
                      {servers.length > 0 ? (
                        <>
                          <span className={servers[0][1].toUpperCase() === 'DOWN' ? 'tk-endpoint-down' : 'tk-endpoint'}>
                            {servers[0][0]}
                          </span>
                          {servers.length > 1 && (
                            <span className="tk-muted"> +{servers.length - 1}</span>
                          )}
                        </>
                      ) : (
                        <span className="tk-muted">—</span>
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

// ── TraefikContent ────────────────────────────────────────────────────────────

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

  useEffect(() => {
    loadOverview()
    loadRouters()
  }, [loadOverview, loadRouters, tick])

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
      />
    </>
  )
}

// Backward compat alias
export { TraefikContent as TraefikDetail }

// Helper used by InfraComponentDetail to build key data points from the overview.
export function traefikKeyDataPoints(overview: TraefikOverview | null): { label: string; value: string }[] {
  return [
    { label: 'Version',  value: overview?.version ?? '—' },
    { label: 'Routers',  value: overview ? String(overview.routers_total) : '—' },
    { label: 'Services', value: overview ? String(overview.services_total) : '—' },
  ]
}
