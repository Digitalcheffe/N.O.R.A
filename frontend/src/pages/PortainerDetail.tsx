import { useState, useEffect, useCallback } from 'react'
import { useParams } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { DetailPageLayout } from '../components/DetailPageLayout'
import {
  infrastructure as infraApi,
  portainer as portainerApi,
} from '../api/client'
import type {
  InfrastructureComponent,
  PortainerEndpoint,
  PortainerEndpointSummary,
  PortainerContainerResource,
} from '../api/types'
import './PortainerDetail.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function timeAgo(iso: string | null | undefined): string {
  if (!iso) return '—'
  const secs = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (secs < 60) return `${secs}s ago`
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`
  return `${Math.floor(secs / 86400)}d ago`
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const tb = bytes / 1_099_511_627_776
  if (tb >= 1) return `${tb.toFixed(1)} TB`
  const gb = bytes / 1_073_741_824
  if (gb >= 1) return `${gb.toFixed(1)} GB`
  const mb = bytes / 1_048_576
  if (mb >= 1) return `${mb.toFixed(0)} MB`
  const kb = bytes / 1_024
  return `${kb.toFixed(0)} KB`
}

function formatCPU(pct: number): string {
  return `${pct.toFixed(1)}%`
}

function formatMem(bytes: number): string {
  return formatBytes(bytes)
}

// ── Sort types ────────────────────────────────────────────────────────────────

type SortKey = 'name' | 'cpu_percent' | 'mem_bytes' | 'mem_percent'
type SortDir = 'asc' | 'desc'

// ── Sub-components ────────────────────────────────────────────────────────────

function StatCard({
  title,
  lines,
}: {
  title: string
  lines: { label: string; value: string; highlight?: boolean }[]
}) {
  return (
    <div className="pt-stat-card">
      <div className="pt-stat-title">{title}</div>
      {lines.map((l, i) => (
        <div key={i} className={`pt-stat-row${l.highlight ? ' pt-stat-highlight' : ''}`}>
          <span className="pt-stat-label">{l.label}</span>
          <span className="pt-stat-value">{l.value}</span>
        </div>
      ))}
    </div>
  )
}

function DismissibleNotice({
  message,
  onDismiss,
}: {
  message: string
  onDismiss: () => void
}) {
  return (
    <div className="pt-notice">
      <span className="pt-notice-icon">⚠</span>
      <span className="pt-notice-text">{message}</span>
      <button className="pt-notice-dismiss" onClick={onDismiss} aria-label="Dismiss">✕</button>
    </div>
  )
}

// ── Endpoint view ─────────────────────────────────────────────────────────────

function EndpointView({
  componentId,
  endpoint,
}: {
  componentId: string
  endpoint: PortainerEndpoint
}) {
  const [summary, setSummary] = useState<PortainerEndpointSummary | null>(null)
  const [containers, setContainers] = useState<PortainerContainerResource[]>([])
  const [loadError, setLoadError] = useState('')
  const [loading, setLoading] = useState(true)
  const [danglingDismissed, setDanglingDismissed] = useState(false)
  const [unusedDismissed, setUnusedDismissed] = useState(false)
  const [sortKey, setSortKey] = useState<SortKey>('name')
  const [sortDir, setSortDir] = useState<SortDir>('asc')
  const { tick } = useAutoRefresh()

  const load = useCallback(() => {
    setLoading(true)
    setLoadError('')
    Promise.all([
      portainerApi.endpointSummary(componentId, endpoint.id),
      portainerApi.endpointContainers(componentId, endpoint.id),
    ])
      .then(([sum, ctrs]) => {
        setSummary(sum)
        setContainers(ctrs.data)
      })
      .catch(err => setLoadError(String(err)))
      .finally(() => setLoading(false))
  }, [componentId, endpoint.id])

  useEffect(() => { load() }, [load, tick])

  function toggleSort(key: SortKey) {
    if (sortKey === key) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    } else {
      setSortKey(key)
      setSortDir('asc')
    }
  }

  function sortIcon(key: SortKey) {
    if (sortKey !== key) return ' ↕'
    return sortDir === 'asc' ? ' ↑' : ' ↓'
  }

  const sorted = [...containers].sort((a, b) => {
    let diff = 0
    if (sortKey === 'name') diff = a.name.localeCompare(b.name)
    else if (sortKey === 'cpu_percent') diff = a.cpu_percent - b.cpu_percent
    else if (sortKey === 'mem_bytes') diff = a.mem_bytes - b.mem_bytes
    else if (sortKey === 'mem_percent') diff = a.mem_percent - b.mem_percent
    return sortDir === 'asc' ? diff : -diff
  })

  if (loading) {
    return <div className="pt-loading">Loading endpoint data…</div>
  }

  if (loadError) {
    return (
      <div className="pt-error">
        <span>{loadError}</span>
        <button className="pt-retry-btn" onClick={load}>Retry</button>
      </div>
    )
  }

  return (
    <div className="pt-endpoint-view">
      {/* ── Summary stat cards ── */}
      {summary && (
        <div className="pt-stat-grid">
          <StatCard
            title="Containers"
            lines={[
              { label: 'Running', value: String(summary.containers_running) },
              { label: 'Stopped', value: String(summary.containers_stopped) },
            ]}
          />
          <StatCard
            title="Images"
            lines={[
              { label: 'Total', value: String(summary.images_total) },
              { label: 'Dangling', value: String(summary.images_dangling), highlight: summary.images_dangling > 0 },
              { label: 'Disk used', value: formatBytes(summary.images_disk_bytes) },
            ]}
          />
          <StatCard
            title="Volumes"
            lines={[
              { label: 'Total', value: String(summary.volumes_total) },
              { label: 'Unused', value: String(summary.volumes_unused), highlight: summary.volumes_unused > 0 },
              { label: 'Disk used', value: formatBytes(summary.volumes_disk_bytes) },
            ]}
          />
          <StatCard
            title="Networks"
            lines={[
              { label: 'Total', value: String(summary.networks_total) },
            ]}
          />
        </div>
      )}

      {/* ── Dismissible notices ── */}
      {summary && summary.images_dangling > 0 && !danglingDismissed && (
        <DismissibleNotice
          message={`${summary.images_dangling} dangling image${summary.images_dangling !== 1 ? 's' : ''} found — ${formatBytes(summary.images_disk_bytes)} of disk space can be reclaimed`}
          onDismiss={() => setDanglingDismissed(true)}
        />
      )}
      {summary && summary.volumes_unused > 0 && !unusedDismissed && (
        <DismissibleNotice
          message={`${summary.volumes_unused} unused volume${summary.volumes_unused !== 1 ? 's' : ''} found`}
          onDismiss={() => setUnusedDismissed(true)}
        />
      )}

      {/* ── Container resource table ── */}
      <div className="pt-section-title">Containers</div>
      {sorted.length === 0 ? (
        <div className="pt-empty">No containers found on this endpoint.</div>
      ) : (
        <table className="pt-table">
          <thead>
            <tr>
              <th
                className="pt-th-sortable"
                onClick={() => toggleSort('name')}
              >
                NAME{sortIcon('name')}
              </th>
              <th
                className="pt-th-sortable"
                onClick={() => toggleSort('cpu_percent')}
              >
                CPU%{sortIcon('cpu_percent')}
              </th>
              <th
                className="pt-th-sortable"
                onClick={() => toggleSort('mem_bytes')}
              >
                MEM{sortIcon('mem_bytes')}
              </th>
              <th
                className="pt-th-sortable"
                onClick={() => toggleSort('mem_percent')}
              >
                MEM%{sortIcon('mem_percent')}
              </th>
              <th>IMAGE</th>
              <th>IMAGE STATUS</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map(c => (
              <tr key={c.id} className={c.state !== 'running' ? 'pt-row-stopped' : ''}>
                <td className="pt-cell-name">
                  {c.name}
                  {c.stack && <span className="pt-stack-badge">{c.stack}</span>}
                </td>
                <td className="pt-cell-mono">
                  {c.state === 'running' ? formatCPU(c.cpu_percent) : '—'}
                </td>
                <td className="pt-cell-mono">
                  {c.state === 'running' ? formatMem(c.mem_bytes) : '—'}
                </td>
                <td className="pt-cell-mono">
                  {c.state === 'running' ? `${c.mem_percent.toFixed(1)}%` : '—'}
                </td>
                <td className="pt-cell-image" title={c.image}>{c.image}</td>
                <td>
                  {c.image_update_available ? (
                    <span className="pt-update-badge pt-update-warn">Update available</span>
                  ) : (
                    <span className="pt-update-badge pt-update-ok">Current</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function PortainerDetail() {
  const { componentId } = useParams<{ componentId: string }>()
  const { tick } = useAutoRefresh()

  const [comp, setComp] = useState<InfrastructureComponent | null>(null)
  const [endpoints, setEndpoints] = useState<PortainerEndpoint[]>([])
  const [loadError, setLoadError] = useState('')
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState(0)

  const loadComponent = useCallback(() => {
    if (!componentId) return
    setLoading(true)
    setLoadError('')
    Promise.all([
      infraApi.get(componentId),
      portainerApi.listEndpoints(componentId),
    ])
      .then(([c, eps]) => {
        setComp(c)
        setEndpoints(eps.data)
        setActiveTab(0)
      })
      .catch(err => setLoadError(String(err)))
      .finally(() => setLoading(false))
  }, [componentId])

  useEffect(() => { loadComponent() }, [loadComponent, tick])

  if (!componentId) return null

  if (loading) {
    return (
      <div className="content">
        <div className="pt-loading">Loading Portainer component…</div>
      </div>
    )
  }

  if (loadError || !comp) {
    return (
      <div className="content">
        <div className="pt-error">
          {loadError || 'Component not found.'}
          <button className="pt-retry-btn" onClick={loadComponent}>Retry</button>
        </div>
      </div>
    )
  }

  // Parse base_url from credential_meta (non-secret fields).
  const baseURL: string = (comp.credential_meta?.base_url as string | undefined) ?? ''

  const statusValue = comp.last_status === 'online' ? 'online'
    : comp.last_status === 'offline' ? 'offline'
    : 'unknown'

  const activeEndpoint = endpoints[activeTab] ?? null

  return (
    <DetailPageLayout
      breadcrumb="Infrastructure"
      breadcrumbPath="/infrastructure"
      name={comp.name}
      status={{ status: statusValue as 'online' | 'offline' | 'unknown' }}
      lastPolled={comp.last_polled_at ? `Last synced: ${timeAgo(comp.last_polled_at)}` : undefined}
      keyDataPoints={[
        { label: 'Type', value: 'Portainer' },
        ...(baseURL ? [{ label: 'URL', value: baseURL }] : []),
        { label: 'Endpoints', value: `${endpoints.length} endpoint${endpoints.length !== 1 ? 's' : ''}` },
      ]}
      sourceId={componentId}
      eventFeedTitle="Image Update Events"
    >
      {/* ── Endpoint tabs (flat if single, tabbed if multiple) ── */}
      {endpoints.length === 0 ? (
        <div className="pt-empty">
          No Portainer endpoints found. Check your Portainer connection and credentials.
        </div>
      ) : endpoints.length === 1 ? (
        // Single endpoint — flat, no tabs.
        <EndpointView componentId={componentId} endpoint={endpoints[0]} />
      ) : (
        // Multiple endpoints — tabbed.
        <div className="pt-tabs-container">
          <div className="pt-tabs">
            {endpoints.map((ep, i) => (
              <button
                key={ep.id}
                className={`pt-tab${activeTab === i ? ' pt-tab-active' : ''}`}
                onClick={() => setActiveTab(i)}
              >
                {ep.name}
              </button>
            ))}
          </div>
          {activeEndpoint && (
            <EndpointView componentId={componentId} endpoint={activeEndpoint} />
          )}
        </div>
      )}
    </DetailPageLayout>
  )
}
