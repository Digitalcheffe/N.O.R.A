import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { DetailPageLayout } from '../components/DetailPageLayout'
import { DiscoverNowButton } from '../components/DiscoverNowButton'
import { DockerEngineDetail } from '../components/DockerEngineDetail'
import { PortainerContent } from '../components/PortainerDetail'
import { ProxmoxContent } from '../components/ProxmoxDetail'
import { SynologyContent } from '../components/SynologyDetail'
import { TraefikContent } from '../components/TraefikDetail'
import { infrastructure as infraApi, apps as appsApi } from '../api/client'
import type {
  App,
  InfrastructureComponent,
  InfrastructureComponentInput,
  ResourceSummary,
  ResourceHistory,
  ResourceRollupPoint,
  SNMPDetail,
  SNMPDisk,
  SynologyDetail,
  TraefikOverview,
} from '../api/types'
import { timeAgo, formatBytes } from '../utils/format'
import { InfraTypeIcon } from '../components/CheckTypeIcon'
import { InfraEditModal, TYPE_LABEL } from './InfraEditModal'
import './InfraComponentDetail.css'


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

// ── Shell edit props ──────────────────────────────────────────────────────────
// Passed into TraefikShell / SynologyShell so they can render the Edit button
// and InfraEditModal despite being module-level components.

interface ShellEditProps {
  allComponents: InfrastructureComponent[]
  editOpen: boolean
  hasCreds: boolean
  onEditOpen: () => void
  onEditSave: (payload: InfrastructureComponentInput) => Promise<void>
  onEditClose: () => void
}

// ── Traefik shell ─────────────────────────────────────────────────────────────

function TraefikShell({ component, ep }: { component: InfrastructureComponent; ep: ShellEditProps }) {
  const [overview, setOverview] = useState<TraefikOverview | null>(null)

  const keyDataPoints = [
    { label: 'Version',  value: overview?.version ?? '—' },
    { label: 'Routers',  value: overview ? String(overview.routers_total)  : '—' },
    { label: 'Services', value: overview ? String(overview.services_total) : '—' },
  ]

  return (
    <DetailPageLayout
      breadcrumb="Infrastructure"
      breadcrumbPath="/infrastructure"
      name={component.name}
      icon={<InfraTypeIcon type={component.type} size={24} />}
      status={{ status: dplStatus(component.last_status) }}
      lastPolled={overview?.updated_at ? `Polled ${timeAgo(overview.updated_at)}` : undefined}
      keyDataPoints={keyDataPoints}
      actions={
        <>
          <button className="icd-edit-btn" onClick={ep.onEditOpen}>Edit</button>
          <DiscoverNowButton
            entityType="traefik"
            entityId={component.id}
            onSuccess={() => { /* TraefikContent auto-refreshes via tick */ }}
          />
        </>
      }
      sourceId={component.id}
    >
      <InfraEditModal
        open={ep.editOpen}
        component={component}
        components={ep.allComponents}
        hasCreds={ep.hasCreds}
        onSave={ep.onEditSave}
        onClose={ep.onEditClose}
      />
      <TraefikContent component={component} onOverviewLoaded={setOverview} />
    </DetailPageLayout>
  )
}

// ── Synology shell ────────────────────────────────────────────────────────────
// Wraps SynologyContent in the shared DetailPageLayout, lifting the key data
// points out of the content component via the onDetailLoaded callback.

function SynologyShell({ component, ep }: { component: InfrastructureComponent; ep: ShellEditProps }) {
  const [synDetail, setSynDetail] = useState<SynologyDetail | null>(null)
  const [synNoData, setSynNoData] = useState(false)

  const handleDetailLoaded = useCallback(
    (detail: SynologyDetail | null, noData: boolean) => {
      setSynDetail(detail)
      setSynNoData(noData)
    },
    [],
  )

  const totalStorageBytes = synDetail?.volumes.reduce((sum, v) => sum + v.total_bytes, 0) ?? 0
  const keyDataPoints = [
    { label: 'Type', value: 'Synology NAS' },
    ...(component.ip ? [{ label: 'IP', value: component.ip }] : []),
    ...(!synNoData ? [
      { label: 'Model',   value: synDetail?.model       || '—' },
      { label: 'DSM',     value: synDetail?.dsm_version || '—' },
      { label: 'Storage', value: totalStorageBytes > 0 ? formatBytes(totalStorageBytes) : '—' },
      { label: 'Uptime',  value: synDetail?.uptime      || '—' },
    ] : []),
  ]

  return (
    <DetailPageLayout
      breadcrumb="Infrastructure"
      breadcrumbPath="/infrastructure"
      name={component.name}
      icon={<InfraTypeIcon type={component.type} size={24} />}
      status={{ status: dplStatus(component.last_status) }}
      lastPolled={synDetail?.polled_at ? `Polled ${timeAgo(synDetail.polled_at)}` : undefined}
      keyDataPoints={keyDataPoints}
      actions={
        <>
          <button className="icd-edit-btn" onClick={ep.onEditOpen}>Edit</button>
          <DiscoverNowButton
            entityType="synology"
            entityId={component.id}
            onSuccess={() => { /* SynologyContent auto-refreshes via tick */ }}
          />
        </>
      }
      sourceId={component.id}
    >
      <InfraEditModal
        open={ep.editOpen}
        component={component}
        components={ep.allComponents}
        hasCreds={ep.hasCreds}
        onSave={ep.onEditSave}
        onClose={ep.onEditClose}
      />
      <SynologyContent component={component} onDetailLoaded={handleDetailLoaded} />
    </DetailPageLayout>
  )
}

function dplStatus(s: string): 'online' | 'offline' | 'unknown' | 'warning' {
  if (s === 'online')                   return 'online'
  if (s === 'degraded')                 return 'warning'
  if (s === 'offline' || s === 'down')  return 'offline'
  return 'unknown'
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function InfraComponentDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()

  const [component,        setComponent]        = useState<InfrastructureComponent | null>(null)
  const [resources,        setResources]        = useState<ResourceSummary | null>(null)
  const [history,          setHistory]          = useState<ResourceHistory | null>(null)
  const [snmpDetail,       setSnmpDetail]       = useState<SNMPDetail | null>(null)
  const [linkedApps,       setLinkedApps]       = useState<App[]>([])
  const [allApps,          setAllApps]          = useState<App[]>([])
  const [allComponents,    setAllComponents]    = useState<InfrastructureComponent[]>([])
  const [linkingAppId,     setLinkingAppId]     = useState('')
  const [linkBusy,         setLinkBusy]         = useState(false)
  const [dockerCounts,     setDockerCounts]     = useState({ total: 0, running: 0 })
  const [portainerCounts,  setPortainerCounts]  = useState({ total: 0, running: 0 })
  const [editOpen,         setEditOpen]         = useState(false)
  const [loading,          setLoading]          = useState(true)
  const [error,            setError]            = useState<string | null>(null)

  useEffect(() => {
    if (!id) return

    Promise.all([
      infraApi.get(id),
      infraApi.resources(id, 'hour'),
      infraApi.resourceHistory(id, 'hour', 24),
      infraApi.linkedApps(id),
      appsApi.list(),
      infraApi.list(),
    ])
      .then(([comp, res, hist, linked, allA, allComps]) => {
        setComponent(comp)
        setResources(res)
        setHistory(hist)
        setLinkedApps(linked.data)
        setAllApps(allA.data)
        setAllComponents(allComps.data)
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

  async function handleEditSave(payload: InfrastructureComponentInput) {
    if (!id) return
    const updated = await infraApi.update(id, payload)
    setComponent(updated)
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
          <button className="icd-back-btn" onClick={() => navigate('/infrastructure')}>← Back</button>
        </div>
      </>
    )
  }

  // Portainer: render content inline — no redirect needed.
  if (component.type === 'portainer') {
    const baseURL: string = (component.credential_meta?.base_url as string | undefined) ?? ''

    return (
      <DetailPageLayout
        breadcrumb="Infrastructure"
        breadcrumbPath="/infrastructure"
        name={component.name}
        icon={<InfraTypeIcon type={component.type} size={24} />}
        status={{ status: dplStatus(component.last_status) }}
        lastPolled={component.last_polled_at ? `Last synced: ${timeAgo(component.last_polled_at)}` : undefined}
        keyDataPoints={[
          { label: 'Type', value: 'Portainer' },
          ...(baseURL ? [{ label: 'URL', value: baseURL }] : []),
          ...(portainerCounts.total > 0 ? [
            { label: 'Containers', value: `${portainerCounts.running} running / ${portainerCounts.total} total` },
          ] : []),
        ]}
        actions={
          <>
            <button className="icd-edit-btn" onClick={() => setEditOpen(true)}>Edit</button>
            <DiscoverNowButton
              entityType="portainer"
              entityId={component.id}
              onSuccess={() => void infraApi.get(component.id)}
            />
          </>
        }
        sourceId={component.id}
      >
        <InfraEditModal
          open={editOpen}
          component={component}
          components={allComponents}
          hasCreds={component.has_credentials ?? false}
          onSave={handleEditSave}
          onClose={() => setEditOpen(false)}
        />
        <PortainerContent
          component={component}
          onCountsLoaded={(total, running) => setPortainerCounts({ total, running })}
        />
      </DetailPageLayout>
    )
  }

  // Proxmox: render content inline using the shared shell.
  if (component.type === 'proxmox_node') {
    return (
      <DetailPageLayout
        breadcrumb="Infrastructure"
        breadcrumbPath="/infrastructure"
        name={component.name}
        icon={<InfraTypeIcon type={component.type} size={24} />}
        status={{ status: dplStatus(component.last_status) }}
        lastPolled={component.last_polled_at ? `Polled ${timeAgo(component.last_polled_at)}` : undefined}
        keyDataPoints={[
          { label: 'Type', value: 'Proxmox VE' },
          ...(component.ip ? [{ label: 'IP', value: component.ip }] : []),
        ]}
        actions={
          <>
            <button className="icd-edit-btn" onClick={() => setEditOpen(true)}>Edit</button>
            <DiscoverNowButton
              entityType="proxmox_node"
              entityId={component.id}
              onSuccess={() => void infraApi.get(component.id)}
            />
          </>
        }
        sourceId={component.id}
      >
        <InfraEditModal
          open={editOpen}
          component={component}
          components={allComponents}
          hasCreds={component.has_credentials ?? false}
          onSave={handleEditSave}
          onClose={() => setEditOpen(false)}
        />
        <ProxmoxContent component={component} />
      </DetailPageLayout>
    )
  }

  // Traefik: render content inline using the shared shell.
  if (component.type === 'traefik') {
    const ep: ShellEditProps = {
      allComponents,
      editOpen,
      hasCreds: component.has_credentials ?? false,
      onEditOpen:  () => setEditOpen(true),
      onEditSave:  handleEditSave,
      onEditClose: () => setEditOpen(false),
    }
    return <TraefikShell component={component} ep={ep} />
  }

  // Synology: render content inline using the shared shell.
  if (component.type === 'synology') {
    const ep: ShellEditProps = {
      allComponents,
      editOpen,
      hasCreds: component.has_credentials ?? false,
      onEditOpen:  () => setEditOpen(true),
      onEditSave:  handleEditSave,
      onEditClose: () => setEditOpen(false),
    }
    return <SynologyShell component={component} ep={ep} />
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

  return (
    <DetailPageLayout
      breadcrumb="Infrastructure"
      breadcrumbPath="/infrastructure"
      name={component.name}
      icon={<InfraTypeIcon type={component.type} size={24} />}
      status={{ status: dplStatus(component.last_status) }}
      lastPolled={component.last_polled_at ? `Polled ${timeAgo(component.last_polled_at)}` : undefined}
      keyDataPoints={keyDataPoints}
      actions={
        <>
          <button className="icd-edit-btn" onClick={() => setEditOpen(true)}>Edit</button>
          {component.collection_method !== 'none' && (
            <DiscoverNowButton
              entityType={component.type}
              entityId={component.id}
              onSuccess={() => void handleDiscoverSuccess()}
            />
          )}
        </>
      }
      sourceId={component.id}
    >
      <InfraEditModal
        open={editOpen}
        component={component}
        components={allComponents}
        hasCreds={component.has_credentials ?? false}
        onSave={handleEditSave}
        onClose={() => setEditOpen(false)}
      />

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
        <DockerEngineDetail
          engineId={component.id}
          onCountsLoaded={(total, running) => setDockerCounts({ total, running })}
        />
      )}

      {/* Linked Applications */}
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
