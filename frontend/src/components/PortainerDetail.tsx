import { useState, useEffect, useCallback } from 'react'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { portainer as portainerApi } from '../api/client'
import { DockerEngineDetail } from './DockerEngineDetail'
import { formatBytes } from '../utils/format'
import type {
  InfrastructureComponent,
  PortainerEndpoint,
  PortainerEndpointSummary,
} from '../api/types'
import './PortainerDetail.css'
import '../pages/InfraComponentDetail.css'

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
// Shows per-endpoint summary stats (containers, images, volumes, networks) and
// dismissible housekeeping notices. Container cards with app-linking are rendered
// once at the PortainerContent level via DockerEngineDetail.

function EndpointView({
  componentId,
  endpoint,
}: {
  componentId: string
  endpoint: PortainerEndpoint
}) {
  const [summary, setSummary] = useState<PortainerEndpointSummary | null>(null)
  const [loadError, setLoadError] = useState('')
  const [loading, setLoading] = useState(true)
  const [danglingDismissed, setDanglingDismissed] = useState(false)
  const [unusedDismissed, setUnusedDismissed] = useState(false)
  const { tick } = useAutoRefresh()

  const load = useCallback(() => {
    setLoading(true)
    setLoadError('')
    portainerApi.endpointSummary(componentId, endpoint.id)
      .then(sum => setSummary(sum))
      .catch(err => setLoadError(String(err)))
      .finally(() => setLoading(false))
  }, [componentId, endpoint.id])

  useEffect(() => { load() }, [load, tick])

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
    </div>
  )
}

// ── PortainerContent ──────────────────────────────────────────────────────────
// Content-only component — rendered as children inside InfraComponentDetail's
// DetailPageLayout shell. Manages its own endpoint data but not the base
// component or page layout.

interface PortainerContentProps {
  component: InfrastructureComponent
  onCountsLoaded?: (total: number, running: number, unlinked: number) => void
}

export function PortainerContent({ component, onCountsLoaded }: PortainerContentProps) {
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

  const endpointSection = endpoints.length === 0 ? (
    <div className="pt-empty">
      No Portainer endpoints found. Check your Portainer connection and credentials.
    </div>
  ) : endpoints.length === 1 ? (
    <EndpointView componentId={component.id} endpoint={endpoints[0]} />
  ) : (
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

  return (
    <>
      {endpointSection}

      {/* Container cards with Add App — uses discovered_containers populated by the enrichment worker */}
      <div className="icd-section">
        <div className="icd-section-title">Containers</div>
        <DockerEngineDetail
          engineId={component.id}
          onCountsLoaded={onCountsLoaded ?? (() => {})}
        />
      </div>
    </>
  )
}

// ── Keep a default export alias so App.tsx import still resolves during migration
export { PortainerContent as PortainerDetail }
