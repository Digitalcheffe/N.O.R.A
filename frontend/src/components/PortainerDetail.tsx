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

// ── Sub-components ────────────────────────────────────────────────────────────

function containerDotClass(state: string): string {
  if (state === 'running') return 'green'
  if (state === 'exited')  return 'red'
  return 'grey'
}

function MiniBar({ value, label }: { value: number | null; label: string }) {
  const pct    = value ?? 0
  const noData = value === null
  const cls    = noData ? 'no-data' : pct >= 90 ? 'crit' : pct >= 70 ? 'warn' : ''
  return (
    <div className="pt-res-row">
      <span className="pt-res-label">{label}</span>
      <div className="pt-res-track">
        <div className={`pt-res-fill${cls ? ` ${cls}` : ''}`} style={{ width: noData ? '0%' : `${Math.min(pct, 100)}%` }} />
      </div>
      <span className={`pt-res-pct${noData ? ' no-data' : ''}`}>{noData ? '—' : `${Math.round(pct)}%`}</span>
    </div>
  )
}

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

  const sorted = [...containers].sort((a, b) => a.name.localeCompare(b.name))

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

      <div className="pt-section-title">
        Containers
        {sorted.length > 0 && (
          <span className="pt-container-count">
            {sorted.filter(c => c.state === 'running').length} running / {sorted.length} total
          </span>
        )}
      </div>
      {sorted.length === 0 ? (
        <div className="pt-empty">No containers found on this endpoint.</div>
      ) : (
        <div className="pt-card-grid">
          {sorted.map(c => {
            const dotCls = containerDotClass(c.state)
            return (
              <div key={c.id} className={`pt-container-card${c.state !== 'running' ? ' stopped' : ''}`}>
                {/* Header: status dot + name + state */}
                <div className="pt-card-header">
                  <span className={`pt-dot ${dotCls}`} />
                  <span className="pt-card-name">{c.name}</span>
                  <span className={`pt-card-state ${dotCls}`}>{c.state}</span>
                </div>

                {/* Meta: stack badge and/or update badge — fixed min-height keeps cards aligned */}
                <div className="pt-card-meta">
                  {c.stack && <span className="pt-stack-badge">{c.stack}</span>}
                  {c.image_update_available && (
                    <span className="pt-card-update-badge">Update available</span>
                  )}
                </div>

                {/* Resource bars */}
                <div className="pt-card-res">
                  <MiniBar label="CPU" value={c.state === 'running' ? c.cpu_percent : null} />
                  <MiniBar label="MEM" value={c.state === 'running' ? c.mem_percent : null} />
                </div>

                {/* Footer: image name */}
                <div className="pt-card-footer">
                  <span className="pt-card-image" title={c.image}>{c.image}</span>
                  <span className="pt-card-mem-detail">
                    {c.state === 'running' ? `${formatBytes(c.mem_bytes)} / ${formatBytes(c.mem_limit_bytes)}` : '—'}
                  </span>
                </div>
              </div>
            )
          })}
        </div>
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
