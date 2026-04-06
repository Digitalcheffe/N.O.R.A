import { useState, useEffect, useCallback, useRef } from 'react'
import { Link } from 'react-router-dom'
import { discovery, infrastructure as infraApi } from '../api/client'
import './DockerEngineDetail.css'
import './PortainerDetail.css'
import type { DiscoveredContainer, DockerEngineSummary } from '../api/types'
import { formatBytes } from '../utils/format'

// ── Props ─────────────────────────────────────────────────────────────────────

interface Props {
  engineId: string
  onCountsLoaded: (total: number, running: number, unlinked: number) => void
}

// ── StatCard (reuses pt-stat-* classes from PortainerDetail.css) ──────────────

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

// ── Main component ────────────────────────────────────────────────────────────

export function DockerEngineDetail({ engineId, onCountsLoaded }: Props) {
  const [containers,  setContainers]  = useState<DiscoveredContainer[]>([])
  const [summary,     setSummary]     = useState<DockerEngineSummary | null>(null)
  const [loading,     setLoading]     = useState(true)

  const onCountsLoadedRef = useRef(onCountsLoaded)
  onCountsLoadedRef.current = onCountsLoaded

  const load = useCallback(() => {
    setLoading(true)
    Promise.all([
      discovery.containers(engineId),
      infraApi.dockerSummary(engineId).catch(() => null),
    ])
      .then(([ctrs, sum]) => {
        setContainers(ctrs.data)
        setSummary(sum)
        const running  = ctrs.data.filter((c: DiscoveredContainer) => c.status === 'running').length
        const unlinked = ctrs.data.filter((c: DiscoveredContainer) => !c.app_id).length
        onCountsLoadedRef.current(ctrs.data.length, running, unlinked)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [engineId])

  useEffect(() => { load() }, [load])

  if (loading) {
    return <div className="de-loading">Loading…</div>
  }

  const total   = containers.length
  const running = containers.filter(c => c.status === 'running').length
  const stopped = containers.filter(c => c.status !== 'running').length

  return (
    <div className="de-wrapper">
      <div className="de-containers-notice">
        Container details and image update information are available on the{' '}
        <Link to="/infrastructure?view=containers" className="de-containers-link">
          Infrastructure › Containers tab →
        </Link>
      </div>

      <div className="pt-stat-grid">
        <StatCard
          title="Containers"
          lines={[
            { label: 'Running', value: String(summary?.containers_running ?? running) },
            { label: 'Stopped', value: String(summary?.containers_stopped ?? stopped), highlight: (summary?.containers_stopped ?? stopped) > 0 },
            { label: 'Total',   value: String(total) },
          ]}
        />
        <StatCard
          title="Images"
          lines={[
            { label: 'Total',     value: summary ? String(summary.images_total)   : '—' },
            { label: 'Dangling',  value: summary ? String(summary.images_dangling) : '—', highlight: (summary?.images_dangling ?? 0) > 0 },
            { label: 'Disk used', value: summary ? formatBytes(summary.images_disk_bytes) : '—' },
          ]}
        />
        <StatCard
          title="Volumes"
          lines={[
            { label: 'Total',     value: summary ? String(summary.volumes_total)  : '—' },
            { label: 'Unused',    value: summary ? String(summary.volumes_unused) : '—', highlight: (summary?.volumes_unused ?? 0) > 0 },
            { label: 'Disk used', value: summary ? formatBytes(summary.volumes_disk_bytes) : '—' },
          ]}
        />
        <StatCard
          title="Networks"
          lines={[
            { label: 'Total', value: summary ? String(summary.networks_total) : '—' },
          ]}
        />
      </div>

    </div>
  )
}
