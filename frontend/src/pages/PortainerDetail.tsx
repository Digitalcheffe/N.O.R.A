import { useState, useEffect, useCallback } from 'react'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { portainer as portainerApi } from '../api/client'
import { formatBytes } from '../utils/format'
import type {
  InfrastructureComponent,
  PortainerEndpoint,
  PortainerEndpointSummary,
  PortainerContainerResource,
} from '../api/types'
import './PortainerDetail.css'

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

      <div className="pt-section-title">Containers</div>
      {sorted.length === 0 ? (
        <div className="pt-empty">No containers found on this endpoint.</div>
      ) : (
        <table className="pt-table">
          <thead>
            <tr>
              <th className="pt-th-sortable" onClick={() => toggleSort('name')}>NAME{sortIcon('name')}</th>
              <th className="pt-th-sortable" onClick={() => toggleSort('cpu_percent')}>CPU%{sortIcon('cpu_percent')}</th>
              <th className="pt-th-sortable" onClick={() => toggleSort('mem_bytes')}>MEM{sortIcon('mem_bytes')}</th>
              <th className="pt-th-sortable" onClick={() => toggleSort('mem_percent')}>MEM%{sortIcon('mem_percent')}</th>
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
                <td className="pt-cell-mono">{c.state === 'running' ? `${c.cpu_percent.toFixed(1)}%` : '—'}</td>
                <td className="pt-cell-mono">{c.state === 'running' ? formatBytes(c.mem_bytes) : '—'}</td>
                <td className="pt-cell-mono">{c.state === 'running' ? `${c.mem_percent.toFixed(1)}%` : '—'}</td>
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

// ── PortainerContent ──────────────────────────────────────────────────────────
// Content-only component — rendered as children inside InfraComponentDetail's
// DetailPageLayout shell. Manages its own endpoint data but not the base
// component or page layout.

interface PortainerContentProps {
  component: InfrastructureComponent
}

export function PortainerContent({ component }: PortainerContentProps) {
  const [endpoints, setEndpoints] = useState<PortainerEndpoint[]>([])
  const [loadError, setLoadError] = useState('')
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState(0)
  const { tick } = useAutoRefresh()

  const load = useCallback(() => {
    setLoading(true)
    setLoadError('')
    portainerApi.listEndpoints(component.id)
      .then(eps => { setEndpoints(eps.data); setActiveTab(0) })
      .catch(err => setLoadError(String(err)))
      .finally(() => setLoading(false))
  }, [component.id])

  useEffect(() => { load() }, [load, tick])

  if (loading) return <div className="pt-loading">Loading endpoints…</div>

  if (loadError) {
    return (
      <div className="pt-error">
        {loadError}
        <button className="pt-retry-btn" onClick={load}>Retry</button>
      </div>
    )
  }

  const activeEndpoint = endpoints[activeTab] ?? null

  if (endpoints.length === 0) {
    return (
      <div className="pt-empty">
        No Portainer endpoints found. Check your Portainer connection and credentials.
      </div>
    )
  }

  if (endpoints.length === 1) {
    return <EndpointView componentId={component.id} endpoint={endpoints[0]} />
  }

  return (
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
        <EndpointView componentId={component.id} endpoint={activeEndpoint} />
      )}
    </div>
  )
}

// ── Keep a default export alias so App.tsx import still resolves during migration
export { PortainerContent as PortainerDetail }
