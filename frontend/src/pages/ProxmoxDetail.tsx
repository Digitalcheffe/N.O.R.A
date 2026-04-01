import { useState, useEffect, useCallback, useMemo } from 'react'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { proxmox as proxmoxApi } from '../api/client'
import { infrastructure as infraApi } from '../api/client'
import { formatBytes } from '../utils/format'
import type {
  InfrastructureComponent,
  ResourceSummary,
  ProxmoxStoragePool,
  ProxmoxGuestInfo,
  ProxmoxNodeStatusDetail,
  ProxmoxTaskFailure,
} from '../api/types'
import './ProxmoxDetail.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatUptime(secs: number): string {
  if (!secs) return '—'
  const d = Math.floor(secs / 86400)
  const h = Math.floor((secs % 86400) / 3600)
  if (d > 0) return `${d}d ${h}h`
  const m = Math.floor((secs % 3600) / 60)
  return `${h}h ${m}m`
}

function formatTimestamp(unix: number): string {
  if (!unix) return '—'
  return new Date(unix * 1000).toLocaleDateString('en-US', {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

const OS_LABEL: Record<string, string> = {
  l24:   'Linux 2.4',
  l26:   'Linux',
  win7:  'Windows 7',
  win8:  'Windows 8',
  win10: 'Windows 10',
  win11: 'Windows 11',
  w2k:   'Windows 2000',
  w2k3:  'Windows 2003',
  w2k8:  'Windows 2008',
  wvista:'Windows Vista',
  wxp:   'Windows XP',
  other: 'Other',
  solaris: 'Solaris',
}

function osLabel(ostype: string | undefined): string | null {
  if (!ostype) return null
  return OS_LABEL[ostype] ?? ostype
}

function resColor(pct: number): string {
  if (pct >= 90) return 'var(--red)'
  if (pct >= 70) return 'var(--yellow, #eab308)'
  return 'var(--green)'
}

// ── Sub-components ────────────────────────────────────────────────────────────

function ResourceBar({
  label, value, color,
}: { label: string; value: number; color: string }) {
  return (
    <div className="px-res-bar">
      {label && <div className="px-res-label">{label}</div>}
      <div className="px-res-track">
        <div className="px-res-fill" style={{ width: `${Math.min(value, 100)}%`, background: color }} />
      </div>
      <div className="px-res-value">{value.toFixed(1)}%</div>
    </div>
  )
}

function SectionError({ msg, onRetry }: { msg: string; onRetry: () => void }) {
  return (
    <div className="px-section-error">
      <span>{msg}</span>
      <button className="px-retry-btn" onClick={onRetry}>Retry</button>
    </div>
  )
}

function SkeletonRows({ count = 3 }: { count?: number }) {
  return (
    <>
      {Array.from({ length: count }).map((_, i) => (
        <tr key={i} className="px-skeleton-row">
          <td colSpan={99}><div className="px-skeleton-bar" /></td>
        </tr>
      ))}
    </>
  )
}

// ── Node Overview ─────────────────────────────────────────────────────────────

function NodeOverviewSection({
  resources,
  nodeStatuses,
}: {
  resources: ResourceSummary | null
  nodeStatuses: ProxmoxNodeStatusDetail[]
}) {
  const ns = nodeStatuses[0]

  const cpu  = resources?.cpu_percent  ?? 0
  const mem  = resources?.mem_percent  ?? 0
  const disk = resources?.disk_percent ?? 0
  const hasData = resources && !resources.no_data

  return (
    <div className="px-section">
      <div className="px-section-title">Node Overview</div>
      <div className="px-overview-card">
        <div className="px-overview-bars">
          <div className="px-overview-metric">
            <div className="px-overview-metric-label">CPU</div>
            {hasData ? (
              <ResourceBar label="" value={cpu} color={resColor(cpu)} />
            ) : (
              <div className="px-no-data-bar" />
            )}
          </div>
          <div className="px-overview-metric">
            <div className="px-overview-metric-label">MEM</div>
            {hasData ? (
              <ResourceBar label="" value={mem} color={resColor(mem)} />
            ) : (
              <div className="px-no-data-bar" />
            )}
          </div>
          <div className="px-overview-metric">
            <div className="px-overview-metric-label">DISK</div>
            {hasData ? (
              <ResourceBar label="" value={disk} color={resColor(disk)} />
            ) : (
              <div className="px-no-data-bar" />
            )}
          </div>
        </div>
        {ns && (
          <div className="px-overview-meta">
            {ns.uptime > 0 && (
              <span className="px-meta-chip">Uptime {formatUptime(ns.uptime)}</span>
            )}
            {ns.cpu_count > 0 && (
              <span className="px-meta-chip">{ns.cpu_count} vCPU{ns.cpu_count !== 1 ? 's' : ''}</span>
            )}
            {ns.total_mem_bytes > 0 && (
              <span className="px-meta-chip">{formatBytes(ns.total_mem_bytes)} RAM</span>
            )}
            {ns.pve_version && (
              <span className="px-meta-chip">{ns.pve_version}</span>
            )}
          </div>
        )}
        {!hasData && !ns && (
          <div className="px-empty">No resource data collected yet. Run Discover Now to poll.</div>
        )}
      </div>
    </div>
  )
}

// ── Storage Pools ─────────────────────────────────────────────────────────────

function StoragePoolsSection({
  pools,
  loading,
  error,
  onRetry,
}: {
  pools: ProxmoxStoragePool[]
  loading: boolean
  error: string | null
  onRetry: () => void
}) {
  const sorted = [...pools].sort((a, b) => b.used_percent - a.used_percent)

  return (
    <div className="px-section">
      <div className="px-section-title">Storage Pools</div>
      {error ? (
        <SectionError msg={error} onRetry={onRetry} />
      ) : (
        <table className="px-table">
          <thead>
            <tr>
              <th>Pool</th>
              <th>Type</th>
              <th>Utilization</th>
              <th>Used / Total</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <SkeletonRows count={3} />
            ) : sorted.length === 0 ? (
              <tr>
                <td colSpan={5} className="px-empty-cell">
                  No storage pools found — check Proxmox API token permissions (requires Sys.Audit).
                </td>
              </tr>
            ) : (
              sorted.map(pool => (
                <tr key={`${pool.node}/${pool.name}`} className={pool.active ? '' : 'px-row-offline'}>
                  <td className="px-mono">{pool.name}</td>
                  <td className="px-muted">{pool.type}</td>
                  <td className="px-util-cell">
                    {pool.active ? (
                      <>
                        <div className="px-pool-bar-track">
                          <div
                            className="px-pool-bar-fill"
                            style={{
                              width: `${Math.min(pool.used_percent, 100)}%`,
                              background: resColor(pool.used_percent),
                            }}
                          />
                        </div>
                        <span className="px-pool-pct">{pool.used_percent.toFixed(0)}%</span>
                      </>
                    ) : (
                      <span className="px-muted">Offline</span>
                    )}
                  </td>
                  <td className="px-muted">
                    {pool.active
                      ? `${formatBytes(pool.used_bytes)} / ${formatBytes(pool.total_bytes)}`
                      : '—'}
                  </td>
                  <td>
                    <span className={`px-status-dot ${pool.active ? 'online' : 'offline'}`} />
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      )}
    </div>
  )
}

// ── Guests ────────────────────────────────────────────────────────────────────

type GuestTypeFilter   = 'all' | 'vm' | 'lxc'
type GuestStatusFilter = 'all' | 'running' | 'stopped'

function GuestsSection({
  guests,
  loading,
  error,
  onRetry,
}: {
  guests: ProxmoxGuestInfo[]
  loading: boolean
  error: string | null
  onRetry: () => void
}) {
  const [typeFilter,   setTypeFilter]   = useState<GuestTypeFilter>('all')
  const [statusFilter, setStatusFilter] = useState<GuestStatusFilter>('all')

  const filtered = guests.filter(g => {
    if (typeFilter   === 'vm'      && g.guest_type !== 'vm')       return false
    if (typeFilter   === 'lxc'     && g.guest_type !== 'lxc')      return false
    if (statusFilter === 'running' && g.status     !== 'running')  return false
    if (statusFilter === 'stopped' && g.status     === 'running')  return false
    return true
  })

  const runningCount = guests.filter(g => g.status === 'running').length
  const headerCount = statusFilter !== 'all'
    ? `${filtered.length} of ${guests.length} guests`
    : `${guests.length} guest${guests.length !== 1 ? 's' : ''}`

  return (
    <div className="px-section">
      <div className="px-section-header-row">
        <div className="px-section-title" style={{ margin: 0 }}>
          Virtual Machines &amp; Containers
        </div>
        <div className="px-guest-filters">
          <select
            className="px-filter-select"
            value={typeFilter}
            onChange={e => setTypeFilter(e.target.value as GuestTypeFilter)}
          >
            <option value="all">All types</option>
            <option value="vm">VMs only</option>
            <option value="lxc">Containers only</option>
          </select>
          <select
            className="px-filter-select"
            value={statusFilter}
            onChange={e => setStatusFilter(e.target.value as GuestStatusFilter)}
          >
            <option value="all">All status</option>
            <option value="running">Running</option>
            <option value="stopped">Stopped</option>
          </select>
          <span className="px-guest-count">{headerCount}</span>
        </div>
      </div>

      {error ? (
        <SectionError msg={error} onRetry={onRetry} />
      ) : (
        <table className="px-table px-guests-table">
          <thead>
            <tr>
              <th></th>
              <th>Name</th>
              <th>Type</th>
              <th>Status</th>
              <th>vCPU</th>
              <th>Memory</th>
              <th>Disk</th>
              <th>Bridge</th>
              <th>OS / Tags</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <SkeletonRows count={5} />
            ) : filtered.length === 0 ? (
              <tr>
                <td colSpan={9} className="px-empty-cell">
                  {guests.length === 0
                    ? 'No VMs or containers found.'
                    : 'No guests match the current filters.'}
                </td>
              </tr>
            ) : (
              filtered.map(g => {
                const dotClass =
                  g.status === 'running' ? 'online' :
                  g.status === 'paused'  ? 'degraded' : 'offline'

                const bridge0      = g.network_bridges?.[0] ?? ''
                const bridgeExtra  = (g.network_bridges?.length ?? 0) > 1
                  ? ` +${(g.network_bridges?.length ?? 1) - 1}`
                  : ''

                const visibleTags    = g.tags?.slice(0, 3) ?? []
                const hiddenTagCount = (g.tags?.length ?? 0) - visibleTags.length

                return (
                  <tr key={`${g.node}/${g.vmid}`}>
                    <td><span className={`px-status-dot ${dotClass}`} /></td>
                    <td className="px-mono px-guest-name">{g.name}</td>
                    <td>
                      <span className="px-type-badge">{g.guest_type.toUpperCase()}</span>
                    </td>
                    <td className="px-muted px-status-text">{g.status}</td>
                    <td className="px-muted">{g.cpus}</td>
                    <td className="px-muted">{formatBytes(g.max_mem_bytes)}</td>
                    <td className="px-muted">{formatBytes(g.max_disk_bytes)}</td>
                    <td className="px-muted px-mono">
                      {bridge0 ? `${bridge0}${bridgeExtra}` : '—'}
                    </td>
                    <td>
                      <div className="px-guest-tags-cell">
                        {osLabel(g.os_type) && (
                          <span className="px-os-badge">{osLabel(g.os_type)}</span>
                        )}
                        {visibleTags.map(t => (
                          <span key={t} className="px-tag-chip">{t}</span>
                        ))}
                        {hiddenTagCount > 0 && (
                          <span className="px-tag-chip px-muted">+{hiddenTagCount}</span>
                        )}
                        {g.onboot && (
                          <span className="px-onboot-badge" title="Auto-start">⏻</span>
                        )}
                      </div>
                    </td>
                  </tr>
                )
              })
            )}
          </tbody>
        </table>
      )}

      {!loading && !error && guests.length > 0 && (
        <div className="px-guests-footer">
          {runningCount} running of {guests.length} total
        </div>
      )}
    </div>
  )
}

// ── Task Failures ─────────────────────────────────────────────────────────────

const SEVEN_DAYS_MS = 7 * 24 * 60 * 60 * 1000

function TaskFailuresSection({
  failures,
  loading,
  error,
  onRetry,
}: {
  failures: ProxmoxTaskFailure[]
  loading: boolean
  error: string | null
  onRetry: () => void
}) {
  const recent = useMemo(
    () => {
      const now = Date.now()
      return failures.filter(f => now - f.start_time * 1000 < SEVEN_DAYS_MS)
    },
    [failures],
  )

  return (
    <div className="px-section">
      <div className="px-section-title">Recent Task Failures</div>
      {error ? (
        <SectionError msg={error} onRetry={onRetry} />
      ) : loading ? (
        <table className="px-table"><tbody><SkeletonRows count={2} /></tbody></table>
      ) : recent.length === 0 ? (
        <div className="px-task-clean">
          <span className="px-clean-icon">✓</span>
          No task failures in the last 7 days.
        </div>
      ) : (
        <table className="px-table">
          <thead>
            <tr>
              <th></th>
              <th>Description</th>
              <th>Task Type</th>
              <th>When</th>
              <th>User</th>
            </tr>
          </thead>
          <tbody>
            {recent.map(f => (
              <tr key={f.upid}>
                <td><span className="px-task-fail-icon">✕</span></td>
                <td className="px-mono px-task-desc">
                  {f.exit_status}
                  {f.object_id && (
                    <span className="px-muted"> (ID {f.object_id})</span>
                  )}
                </td>
                <td><span className="px-type-badge">{f.type}</span></td>
                <td className="px-muted">{formatTimestamp(f.start_time)}</td>
                <td className="px-muted px-mono">{f.user}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

// ── Updates Banner ────────────────────────────────────────────────────────────

function UpdatesBanner({
  componentId,
  count,
  nodeName,
}: {
  componentId: string
  count: number
  nodeName: string
}) {
  const key = `px-updates-dismissed-${componentId}-${count}`
  const [dismissed, setDismissed] = useState(() => {
    try { return localStorage.getItem(key) === '1' } catch { return false }
  })

  if (dismissed || count <= 0) return null

  function dismiss() {
    try { localStorage.setItem(key, '1') } catch { /* ignore */ }
    setDismissed(true)
  }

  return (
    <div className="px-updates-banner">
      <span className="px-updates-icon">⚠</span>
      <span>{count} package update{count !== 1 ? 's' : ''} available for {nodeName}</span>
      <button className="px-dismiss-btn" onClick={dismiss}>Dismiss</button>
    </div>
  )
}

// ── ProxmoxContent ────────────────────────────────────────────────────────────
// Content-only component — rendered inside InfraComponentDetail's DetailPageLayout
// shell. Manages its own Proxmox-specific data but not the base component or layout.

interface ProxmoxContentProps {
  component: InfrastructureComponent
}

export function ProxmoxContent({ component }: ProxmoxContentProps) {
  const { tick } = useAutoRefresh()
  const componentId = component.id

  const [resources,    setResources]    = useState<ResourceSummary | null>(null)

  const [pools,        setPools]        = useState<ProxmoxStoragePool[]>([])
  const [poolsLoading, setPoolsLoading] = useState(true)
  const [poolsError,   setPoolsError]   = useState<string | null>(null)

  const [guests,        setGuests]       = useState<ProxmoxGuestInfo[]>([])
  const [guestsLoading, setGuestsLoading]= useState(true)
  const [guestsError,   setGuestsError]  = useState<string | null>(null)

  const [nodeStatuses,  setNodeStatuses] = useState<ProxmoxNodeStatusDetail[]>([])
  const [statusLoading, setStatusLoading]= useState(true)
  const [statusError,   setStatusError]  = useState<string | null>(null)

  const [failures,        setFailures]        = useState<ProxmoxTaskFailure[]>([])
  const [failuresLoading, setFailuresLoading] = useState(true)
  const [failuresError,   setFailuresError]   = useState<string | null>(null)

  const loadResources = useCallback(() => {
    infraApi.resources(componentId, 'hour')
      .then(r => setResources(r))
      .catch(() => setResources(null))
  }, [componentId])

  const loadPools = useCallback(() => {
    setPoolsLoading(true)
    proxmoxApi.storage(componentId)
      .then(r => { setPools(r.data); setPoolsError(null) })
      .catch(err => setPoolsError(err instanceof Error ? err.message : 'Failed to load storage pools'))
      .finally(() => setPoolsLoading(false))
  }, [componentId])

  const loadGuests = useCallback(() => {
    setGuestsLoading(true)
    proxmoxApi.guests(componentId)
      .then(r => { setGuests(r.data); setGuestsError(null) })
      .catch(err => setGuestsError(err instanceof Error ? err.message : 'Failed to load guests'))
      .finally(() => setGuestsLoading(false))
  }, [componentId])

  const loadStatus = useCallback(() => {
    setStatusLoading(true)
    proxmoxApi.nodeStatus(componentId)
      .then(r => { setNodeStatuses(r.data); setStatusError(null) })
      .catch(err => setStatusError(err instanceof Error ? err.message : 'Failed to load node status'))
      .finally(() => setStatusLoading(false))
  }, [componentId])

  const loadFailures = useCallback(() => {
    setFailuresLoading(true)
    proxmoxApi.taskFailures(componentId)
      .then(r => { setFailures(r.data); setFailuresError(null) })
      .catch(err => setFailuresError(err instanceof Error ? err.message : 'Failed to load task failures'))
      .finally(() => setFailuresLoading(false))
  }, [componentId])

  useEffect(() => {
    loadResources()
    loadPools()
    loadGuests()
    loadStatus()
    loadFailures()
  }, [loadResources, loadPools, loadGuests, loadStatus, loadFailures, tick])

  const updatesAvailable = nodeStatuses.reduce((sum, ns) => sum + ns.updates_available, 0)

  return (
    <>
      {!statusLoading && !statusError && updatesAvailable > 0 && (
        <UpdatesBanner
          componentId={componentId}
          count={updatesAvailable}
          nodeName={component.name}
        />
      )}

      <NodeOverviewSection
        resources={statusLoading ? null : resources}
        nodeStatuses={statusLoading ? [] : nodeStatuses}
      />

      <StoragePoolsSection
        pools={pools}
        loading={poolsLoading}
        error={poolsError ? `Failed to load storage pools. ${poolsError}` : null}
        onRetry={loadPools}
      />

      <GuestsSection
        guests={guests}
        loading={guestsLoading}
        error={guestsError ? `Failed to load guests. ${guestsError}` : null}
        onRetry={loadGuests}
      />

      <TaskFailuresSection
        failures={failures}
        loading={failuresLoading}
        error={failuresError ? `Failed to load task failures. ${failuresError}` : null}
        onRetry={loadFailures}
      />
    </>
  )
}

// Expose the key data points so InfraComponentDetail can pass them to DetailPageLayout.
export function proxmoxKeyDataPoints(
  nodeStatuses: ProxmoxNodeStatusDetail[],
): { label: string; value: string }[] {
  const ns = nodeStatuses[0]
  return [
    { label: 'Uptime', value: ns?.uptime       ? formatUptime(ns.uptime)           : '—' },
    { label: 'vCPUs',  value: ns?.cpu_count     ? String(ns.cpu_count)              : '—' },
    { label: 'RAM',    value: ns?.total_mem_bytes ? formatBytes(ns.total_mem_bytes) : '—' },
    { label: 'PVE',    value: ns?.pve_version   ?? '—' },
  ]
}

// ── Keep a default export alias for backward compatibility during migration ────
export { ProxmoxContent as ProxmoxDetail }
