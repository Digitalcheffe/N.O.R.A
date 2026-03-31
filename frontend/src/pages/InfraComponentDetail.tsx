import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { DetailPageLayout } from '../components/DetailPageLayout'
import { DiscoverNowButton } from '../components/DiscoverNowButton'
import { DockerEngineDetail } from '../components/DockerEngineDetail'
import { infrastructure as infraApi, apps as appsApi } from '../api/client'
import type {
  App,
  InfrastructureComponent,
  ResourceSummary,
  ResourceHistory,
  ResourceRollupPoint,
  SNMPDetail,
  SNMPDisk,
} from '../api/types'
import './InfraComponentDetail.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function timeAgo(iso: string | null | undefined): string {
  if (!iso) return '—'
  const secs = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (secs < 60) return `${secs}s ago`
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`
  return `${Math.floor(secs / 86400)}d ago`
}

const TYPE_LABEL: Record<string, string> = {
  proxmox_node:  'Proxmox Node',
  synology:      'Synology NAS',
  vm:            'VM',
  lxc:           'LXC',
  bare_metal:    'Bare Metal',
  linux_host:    'Linux Host',
  windows_host:  'Windows Host',
  generic_host:  'Generic Host',
  docker_engine: 'Docker Engine',
  traefik:       'Traefik',
}


// ── Sparkline ─────────────────────────────────────────────────────────────────

function Sparkline({ points, color = 'var(--accent)' }: { points: ResourceRollupPoint[]; color?: string }) {
  if (points.length < 2) {
    return <svg width={120} height={32} style={{ display: 'block' }} />
  }
  const w = 120, h = 32
  const vals = points.map(p => p.avg)
  const coords = points.map((_, i) => {
    const x = (i / (points.length - 1)) * w
    const y = h - (vals[i] / 100) * (h - 4) - 2
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })
  return (
    <svg width={w} height={h} style={{ display: 'block' }}>
      <polyline points={coords.join(' ')} fill="none" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  )
}

// ── Resource section ──────────────────────────────────────────────────────────

function ResourceBar({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div className="icd-res-bar">
      <div className="icd-res-label">{label}</div>
      <div className="icd-res-track">
        <div className="icd-res-fill" style={{ width: `${Math.min(value, 100)}%`, background: color }} />
      </div>
      <div className="icd-res-value">{value.toFixed(1)}%</div>
    </div>
  )
}

function ResourceSection({ resources, history }: { resources: ResourceSummary | null; history: ResourceHistory | null }) {
  const byMetric: Record<string, ResourceRollupPoint[]> = {}
  if (history) {
    for (const pt of history.data) {
      if (!byMetric[pt.metric]) byMetric[pt.metric] = []
      byMetric[pt.metric].push(pt)
    }
  }

  const metrics = [
    { key: 'cpu_percent',  label: 'CPU',  value: resources?.cpu_percent  ?? 0, color: 'var(--accent)' },
    { key: 'mem_percent',  label: 'Mem',  value: resources?.mem_percent  ?? 0, color: 'var(--green)' },
    { key: 'disk_percent', label: 'Disk', value: resources?.disk_percent ?? 0, color: 'var(--yellow, #eab308)' },
  ]

  const hasData = resources && !resources.no_data
  const hasHistory = Object.keys(byMetric).length > 0

  if (!hasData && !hasHistory) {
    return (
      <div className="icd-section">
        <div className="icd-section-title">Resources</div>
        <div className="icd-empty">No resource data collected yet.</div>
      </div>
    )
  }

  return (
    <div className="icd-section">
      <div className="icd-section-title">Resources</div>
      <div className="icd-resource-grid">
        {metrics.map(m => (
          <div key={m.key} className="icd-resource-card">
            <div className="icd-resource-card-header">
              <span className="icd-resource-card-label">{m.label}</span>
              {hasData && <span className="icd-resource-card-value">{m.value.toFixed(1)}%</span>}
            </div>
            {hasData && <ResourceBar label="" value={m.value} color={m.color} />}
            {hasHistory && byMetric[m.key] && byMetric[m.key].length >= 2 && (
              <div className="icd-resource-spark">
                <Sparkline points={byMetric[m.key]} color={m.color} />
                <div className="icd-spark-label">Last {byMetric[m.key].length}h</div>
              </div>
            )}
          </div>
        ))}
      </div>
      {/* Synology volumes */}
      {hasData && resources.volumes && resources.volumes.length > 0 && (
        <div className="icd-volumes">
          <div className="icd-volumes-label">Volumes</div>
          {resources.volumes.map(v => (
            <ResourceBar key={v.name} label={v.name} value={v.percent} color="var(--purple, #a855f7)" />
          ))}
        </div>
      )}
    </div>
  )
}

// ── SNMP section ──────────────────────────────────────────────────────────────

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  if (bytes >= 1e12) return `${(bytes / 1e12).toFixed(1)} TB`
  if (bytes >= 1e9)  return `${(bytes / 1e9).toFixed(1)} GB`
  if (bytes >= 1e6)  return `${(bytes / 1e6).toFixed(1)} MB`
  return `${(bytes / 1e3).toFixed(0)} KB`
}

function snmpBarColor(pct: number): string {
  if (pct >= 90) return 'var(--red)'
  if (pct >= 70) return 'var(--yellow, #eab308)'
  return 'var(--accent)'
}

function SNMPResourceRow({
  label,
  pct,
  sub,
  noData,
}: {
  label: string
  pct: number
  sub?: string
  noData: boolean
}) {
  const color = noData ? 'var(--border)' : snmpBarColor(pct)
  return (
    <div className="snmp-res-row">
      <span className="snmp-res-label">{label}</span>
      <div className="snmp-res-track">
        <div
          className="snmp-res-fill"
          style={{ width: noData ? '0%' : `${Math.min(pct, 100)}%`, background: color }}
        />
      </div>
      {noData ? (
        <span className="snmp-res-pct muted">—</span>
      ) : (
        <span className="snmp-res-pct" style={{ color }}>{Math.round(pct)}%</span>
      )}
      {sub && !noData && <span className="snmp-res-sub">{sub}</span>}
    </div>
  )
}

function SNMPSection({ detail }: { detail: SNMPDetail | null }) {
  const [diskExpanded, setDiskExpanded] = useState(false)
  const noData = !detail || !!detail.no_data

  const disks: SNMPDisk[] = detail?.disks ?? []
  const DISK_LIMIT = 6
  const visibleDisks = diskExpanded ? disks : disks.slice(0, DISK_LIMIT)
  const hiddenCount = disks.length - DISK_LIMIT

  return (
    <>
      {/* ── Section 1: System Info ── */}
      <div className="icd-section">
        <div className="icd-section-title">System Info</div>
        <div className="snmp-info-grid">
          <div className="snmp-info-row">
            <span className="snmp-info-label">Hostname</span>
            <span className="snmp-info-value">{noData || !detail?.hostname ? '—' : detail.hostname}</span>
          </div>
          <div className="snmp-info-row">
            <span className="snmp-info-label">OS</span>
            {noData || !detail?.os_description ? (
              <span className="snmp-info-value muted">—</span>
            ) : (
              <span className="snmp-info-value snmp-os-descr" title={detail.os_description}>
                {detail.os_description.length > 60
                  ? detail.os_description.slice(0, 60) + '…'
                  : detail.os_description}
              </span>
            )}
          </div>
          <div className="snmp-info-row">
            <span className="snmp-info-label">Uptime</span>
            <span className="snmp-info-value">{noData || !detail?.uptime ? '—' : detail.uptime}</span>
          </div>
        </div>
        {noData && <div className="snmp-pending">Awaiting first scan</div>}
      </div>

      {/* ── Section 2: CPU & Memory ── */}
      <div className="icd-section">
        <div className="icd-section-title">CPU &amp; Memory</div>
        <div className="snmp-res-rows">
          <SNMPResourceRow
            label="CPU"
            pct={detail?.cpu_percent ?? 0}
            noData={noData}
          />
          <SNMPResourceRow
            label="MEM"
            pct={detail?.memory?.percent ?? 0}
            sub={
              detail?.memory
                ? `${formatBytes(detail.memory.used_bytes)} / ${formatBytes(detail.memory.total_bytes)}`
                : undefined
            }
            noData={noData}
          />
        </div>
      </div>

      {/* ── Section 3: Disk ── */}
      <div className="icd-section">
        <div className="icd-section-title">Disk</div>
        {noData || disks.length === 0 ? (
          <div className="snmp-pending">Pending first scan</div>
        ) : (
          <div className="snmp-disk-rows">
            {visibleDisks.map(d => (
              <SNMPResourceRow
                key={d.label}
                label={d.label}
                pct={d.percent}
                sub={`${formatBytes(d.used_bytes)} / ${formatBytes(d.total_bytes)}`}
                noData={false}
              />
            ))}
            {!diskExpanded && hiddenCount > 0 && (
              <button className="snmp-expand-btn" onClick={() => setDiskExpanded(true)}>
                and {hiddenCount} more…
              </button>
            )}
          </div>
        )}
      </div>
    </>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function InfraComponentDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()

  const [component,     setComponent]     = useState<InfrastructureComponent | null>(null)
  const [resources,     setResources]     = useState<ResourceSummary | null>(null)
  const [history,       setHistory]       = useState<ResourceHistory | null>(null)
  const [snmpDetail,    setSnmpDetail]    = useState<SNMPDetail | null>(null)
  const [children,      setChildren]      = useState<InfrastructureComponent[]>([])
  const [linkedApps,    setLinkedApps]    = useState<App[]>([])
  const [allApps,       setAllApps]       = useState<App[]>([])
  const [linkingAppId,  setLinkingAppId]  = useState('')
  const [linkBusy,      setLinkBusy]      = useState(false)
  const [dockerCounts,  setDockerCounts]  = useState({ total: 0, running: 0 })
  const [loading,       setLoading]       = useState(true)
  const [error,         setError]         = useState<string | null>(null)

  useEffect(() => {
    if (!id) return

    Promise.all([
      infraApi.get(id),
      infraApi.resources(id, 'hour'),
      infraApi.resourceHistory(id, 'hour', 24),
      infraApi.children(id),
      infraApi.linkedApps(id),
      appsApi.list(),
    ])
      .then(([comp, res, hist, ch, linked, allA]) => {
        setComponent(comp)
        setResources(res)
        setHistory(hist)
        setChildren(ch.data)
        setLinkedApps(linked.data)
        setAllApps(allA.data)
        const extras: Promise<unknown>[] = []
        if (comp.collection_method === 'snmp') {
          extras.push(infraApi.snmpDetail(id).then(det => setSnmpDetail(det)))
        }
        return Promise.all(extras)
      })
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load'))
      .finally(() => setLoading(false))
  }, [id, tick])

  async function handleDiscoverSuccess() {
    if (!id || !component) return
    const [comp, res] = await Promise.all([
      infraApi.get(id),
      infraApi.resources(id, 'hour'),
    ])
    setComponent(comp)
    setResources(res)
    if (component.collection_method === 'snmp') {
      const det = await infraApi.snmpDetail(id)
      setSnmpDetail(det)
    }
  }

  async function handleLinkApp() {
    if (!id || !linkingAppId) return
    setLinkBusy(true)
    try {
      await infraApi.linkApp(id, linkingAppId)
      const linked = await infraApi.linkedApps(id)
      setLinkedApps(linked.data)
      setLinkingAppId('')
    } finally {
      setLinkBusy(false)
    }
  }

  async function handleUnlinkApp(appId: string) {
    if (!id) return
    await infraApi.unlinkApp(id, appId)
    setLinkedApps(prev => prev.filter(a => a.id !== appId))
  }

  function dplStatus(s: string): 'online' | 'offline' | 'unknown' | 'warning' {
    if (s === 'online')   return 'online'
    if (s === 'degraded') return 'warning'
    if (s === 'offline')  return 'offline'
    return 'unknown'
  }

  if (loading) {
    return (
      <>
        <Topbar title="Component" />
        <div className="content"><div className="icd-loading">Loading…</div></div>
      </>
    )
  }

  if (error || !component) {
    return (
      <>
        <Topbar title="Component" />
        <div className="content">
          <div className="icd-error">{error ?? 'Component not found'}</div>
          <button className="icd-back-btn" onClick={() => navigate('/topology')}>← Back</button>
        </div>
      </>
    )
  }

  // Traefik components have their own detail page.
  if (component.type === 'traefik') {
    navigate(`/topology/traefik/${component.id}`, { replace: true })
    return null
  }

  // Synology components have their own detail page.
  if (component.type === 'synology') {
    navigate(`/topology/synology/${component.id}`, { replace: true })
    return null
  }

  const keyDataPoints = [
    { label: 'Type', value: TYPE_LABEL[component.type] ?? component.type },
    ...(component.ip ? [{ label: 'IP', value: component.ip }] : []),
    ...(component.type === 'docker_engine' ? [
      { label: 'Containers', value: `${dockerCounts.running} running / ${dockerCounts.total} total` },
    ] : []),
    ...(component.collection_method === 'snmp' && snmpDetail && !snmpDetail.no_data ? [
      { label: 'Hostname', value: snmpDetail.hostname || '—' },
      { label: 'Uptime', value: snmpDetail.uptime || '—' },
      { label: 'OS', value: snmpDetail.os_description
          ? (snmpDetail.os_description.length > 30 ? snmpDetail.os_description.slice(0, 30) + '…' : snmpDetail.os_description)
          : '—' },
    ] : []),
  ]

  const linkedIds = new Set(linkedApps.map(a => a.id))
  const availableApps = allApps.filter(a => !linkedIds.has(a.id))

  const dockerLinkedAppsHeader = component.type === 'docker_engine' ? (
    <div className="dpl-linked-apps-compact">
      <span className="dpl-linked-apps-label">Linked Apps</span>
      {linkedApps.map(app => (
        <span key={app.id} className="dpl-linked-app-chip">
          {app.name}
          <button onClick={() => void handleUnlinkApp(app.id)} title="Unlink">×</button>
        </span>
      ))}
      {availableApps.length > 0 && (
        <>
          <select
            className="dpl-linked-apps-select"
            value={linkingAppId}
            onChange={e => setLinkingAppId(e.target.value)}
          >
            <option value="">— link app —</option>
            {availableApps.map(a => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </select>
          <button
            className="dpl-linked-apps-link-btn"
            disabled={!linkingAppId || linkBusy}
            onClick={() => void handleLinkApp()}
          >
            {linkBusy ? 'Linking…' : 'Link'}
          </button>
        </>
      )}
      {linkedApps.length === 0 && availableApps.length === 0 && (
        <span style={{ fontSize: 12, color: 'var(--text3)' }}>None</span>
      )}
    </div>
  ) : undefined

  return (
    <DetailPageLayout
      breadcrumb="Infrastructure"
      breadcrumbPath="/topology"
      name={component.name}
      status={{ status: dplStatus(component.last_status) }}
      lastPolled={component.last_polled_at ? `Polled ${timeAgo(component.last_polled_at)}` : undefined}
      keyDataPoints={keyDataPoints}
      headerExtra={dockerLinkedAppsHeader}
      actions={
        component.collection_method !== 'none' ? (
          <DiscoverNowButton
            entityType={component.type}
            entityId={component.id}
            onSuccess={() => void handleDiscoverSuccess()}
          />
        ) : undefined
      }
      sourceId={component.id}
    >
      {/* SNMP hosts: three-section detail view */}
      {component.collection_method === 'snmp' && (
        <SNMPSection detail={snmpDetail} />
      )}

      {/* Non-SNMP resource metrics */}
      {component.type !== 'docker_engine' && component.collection_method !== 'snmp' && (
        <ResourceSection resources={resources} history={history} />
      )}

      {/* Type-specific content */}
      {component.type === 'docker_engine' && (
        <div className="icd-section">
          <div className="icd-section-title">Containers</div>
          <DockerEngineDetail
            engineId={component.id}
            onCountsLoaded={(total, running) => setDockerCounts({ total, running })}
          />
        </div>
      )}

      {/* Discovered VMs & LXC containers (Proxmox nodes) */}
      {component.type === 'proxmox_node' && (
        <ProxmoxChildrenSection children={children} onNavigate={navigate} />
      )}

      {/* Linked Applications — only for non-docker types (docker uses headerExtra) */}
      {component.type !== 'docker_engine' && (
        <LinkedAppsSection
          linkedApps={linkedApps}
          allApps={allApps}
          linkingAppId={linkingAppId}
          linkBusy={linkBusy}
          onSelectApp={setLinkingAppId}
          onLink={() => void handleLinkApp()}
          onUnlink={(appId) => void handleUnlinkApp(appId)}
        />
      )}
    </DetailPageLayout>
  )
}

// ── Proxmox children section ──────────────────────────────────────────────────

function statusDotClass(s: string) {
  if (s === 'online')   return 'online'
  if (s === 'degraded') return 'degraded'
  if (s === 'offline')  return 'offline'
  return 'unknown'
}

const CHILD_TYPE_LABEL: Record<string, string> = {
  vm:  'VM',
  lxc: 'LXC',
}

interface ProxmoxChildrenSectionProps {
  children: InfrastructureComponent[]
  onNavigate: (path: string) => void
}

function ProxmoxChildrenSection({ children, onNavigate }: ProxmoxChildrenSectionProps) {
  const vms  = children.filter(c => c.type === 'vm')
  const lxcs = children.filter(c => c.type === 'lxc')

  if (children.length === 0) {
    return (
      <div className="icd-section">
        <div className="icd-section-title">Virtual Machines</div>
        <div className="icd-empty">No VMs or containers discovered yet. Run Discover Now to discover.</div>
      </div>
    )
  }

  return (
    <>
      {vms.length > 0 && (
        <div className="icd-section">
          <div className="icd-section-title">Virtual Machines</div>
          <div className="icd-child-grid">
            {vms.map(c => (
              <div
                key={c.id}
                className="icd-child-card"
                onClick={() => onNavigate(`/topology/${c.id}`)}
              >
                <div className="icd-child-header">
                  <span className={`icd-status-dot ${statusDotClass(c.last_status)}`} />
                  <span className="icd-child-name">{c.name}</span>
                  <span className="icd-child-type">{CHILD_TYPE_LABEL[c.type] ?? c.type}</span>
                </div>
                <div className="icd-child-meta">
                  {c.ip && <span className="icd-child-ip">{c.ip}</span>}
                  <span className="icd-child-status">{c.last_status}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {lxcs.length > 0 && (
        <div className="icd-section">
          <div className="icd-section-title">LXC Containers</div>
          <div className="icd-child-grid">
            {lxcs.map(c => (
              <div
                key={c.id}
                className="icd-child-card"
                onClick={() => onNavigate(`/topology/${c.id}`)}
              >
                <div className="icd-child-header">
                  <span className={`icd-status-dot ${statusDotClass(c.last_status)}`} />
                  <span className="icd-child-name">{c.name}</span>
                  <span className="icd-child-type">{CHILD_TYPE_LABEL[c.type] ?? c.type}</span>
                </div>
                <div className="icd-child-meta">
                  {c.ip && <span className="icd-child-ip">{c.ip}</span>}
                  <span className="icd-child-status">{c.last_status}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </>
  )
}

// ── Linked applications section ───────────────────────────────────────────────

interface LinkedAppsSectionProps {
  linkedApps: App[]
  allApps: App[]
  linkingAppId: string
  linkBusy: boolean
  onSelectApp: (id: string) => void
  onLink: () => void
  onUnlink: (appId: string) => void
}

function LinkedAppsSection({
  linkedApps, allApps, linkingAppId, linkBusy, onSelectApp, onLink, onUnlink,
}: LinkedAppsSectionProps) {
  const linkedIds = new Set(linkedApps.map(a => a.id))
  const available = allApps.filter(a => !linkedIds.has(a.id))

  return (
    <div className="icd-section">
      <div className="icd-section-title">Linked Applications</div>

      {linkedApps.length === 0 ? (
        <div className="icd-empty">No applications linked to this host yet.</div>
      ) : (
        <div className="icd-linked-apps">
          {linkedApps.map(app => (
            <div key={app.id} className="icd-linked-app-row">
              <span className="icd-linked-app-name">{app.name}</span>
              <button
                className="icd-unlink-btn"
                onClick={() => onUnlink(app.id)}
              >
                Unlink
              </button>
            </div>
          ))}
        </div>
      )}

      {available.length > 0 && (
        <div className="icd-link-app-row">
          <select
            className="icd-link-select"
            value={linkingAppId}
            onChange={e => onSelectApp(e.target.value)}
          >
            <option value="">— select app to link —</option>
            {available.map(a => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </select>
          <button
            className="icd-link-btn"
            disabled={!linkingAppId || linkBusy}
            onClick={onLink}
          >
            {linkBusy ? 'Linking…' : 'Link App'}
          </button>
        </div>
      )}
    </div>
  )
}
