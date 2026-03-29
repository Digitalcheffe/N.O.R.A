import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { DockerEngineDetail } from '../components/DockerEngineDetail'
import { infrastructure as infraApi, apps as appsApi } from '../api/client'
import type {
  App,
  InfrastructureComponent,
  ResourceSummary,
  ResourceHistory,
  ResourceRollupPoint,
  TraefikComponentDetail,
  TraefikCertWithCheck,
  TraefikRoute,
} from '../api/types'
import './InfraComponentDetail.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

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

function daysUntil(iso: string | null | undefined): number | null {
  if (!iso) return null
  return Math.floor((new Date(iso).getTime() - Date.now()) / 86_400_000)
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

// ── Traefik section ───────────────────────────────────────────────────────────

function TraefikSection({ detail }: { detail: TraefikComponentDetail }) {
  return (
    <>
      <div className="icd-section">
        <div className="icd-section-title">SSL Certificates</div>
        {detail.certs.length === 0 ? (
          <div className="icd-empty">No certificates discovered yet.</div>
        ) : (
          <table className="icd-table">
            <thead>
              <tr><th>Domain</th><th>Issuer</th><th>Expires</th><th>Days</th><th>Check</th></tr>
            </thead>
            <tbody>
              {detail.certs.map((cert: TraefikCertWithCheck) => {
                const days = daysUntil(cert.expires_at)
                const rowCls = days !== null && days <= 7 ? 'icd-row-crit' : days !== null && days <= 30 ? 'icd-row-warn' : ''
                return (
                  <tr key={cert.id} className={rowCls}>
                    <td className="icd-mono">{cert.domain}</td>
                    <td className="icd-muted">{cert.issuer ?? '—'}</td>
                    <td className="icd-muted">{cert.expires_at ? new Date(cert.expires_at).toLocaleDateString() : '—'}</td>
                    <td>{days !== null ? <span className={`icd-badge${rowCls ? ' ' + rowCls : ''}`}>{days}d</span> : '—'}</td>
                    <td>
                      <span className={`icd-check-badge icd-check-${cert.check_status || 'unknown'}`}>
                        {cert.check_status?.toUpperCase() || '—'}
                      </span>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>

      <div className="icd-section">
        <div className="icd-section-title">HTTP Routes</div>
        {detail.routes.length === 0 ? (
          <div className="icd-empty">No HTTP routes discovered yet.</div>
        ) : (
          <table className="icd-table">
            <thead>
              <tr><th>Name</th><th>Rule</th><th>Service</th><th>Status</th></tr>
            </thead>
            <tbody>
              {detail.routes.map((route: TraefikRoute) => (
                <tr key={route.id}>
                  <td className="icd-mono">{route.name}</td>
                  <td className="icd-muted icd-route-rule">{route.rule}</td>
                  <td className="icd-muted">{route.service}</td>
                  <td><span className={`icd-route-status ${route.status}`}>{route.status}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
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
  const [traefikDetail, setTraefikDetail] = useState<TraefikComponentDetail | null>(null)
  const [children,      setChildren]      = useState<InfrastructureComponent[]>([])
  const [linkedApps,    setLinkedApps]    = useState<App[]>([])
  const [allApps,       setAllApps]       = useState<App[]>([])
  const [linkingAppId,  setLinkingAppId]  = useState('')
  const [linkBusy,      setLinkBusy]      = useState(false)
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
        if (comp.type === 'traefik') {
          return infraApi.traefikDetail(id).then(det => setTraefikDetail(det))
        }
      })
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load'))
      .finally(() => setLoading(false))
  }, [id, tick])

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

  const statusClass = (s: string) => {
    if (s === 'online')   return 'online'
    if (s === 'degraded') return 'degraded'
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

  return (
    <>
      <Topbar title={component.name} />
      <div className="content">

        {/* Header */}
        <div className="icd-header">
          <div className="icd-header-left">
            <button className="icd-back-btn" onClick={() => navigate('/topology')}>← Infrastructure</button>
            <h1 className="icd-title">{component.name}</h1>
          </div>
          <div className="icd-header-meta">
            <span className={`icd-status-dot ${statusClass(component.last_status)}`} />
            <span className="icd-status-label">{component.last_status}</span>
            <span className="icd-type-badge">{TYPE_LABEL[component.type] ?? component.type}</span>
            {component.ip && <span className="icd-ip">{component.ip}</span>}
          </div>
        </div>

        {/* Resource metrics (shown for components that have pollers) */}
        {component.type !== 'docker_engine' && component.type !== 'traefik' && (
          <ResourceSection resources={resources} history={history} />
        )}

        {/* Type-specific content */}
        {component.type === 'docker_engine' && (
          <div className="icd-section">
            <div className="icd-section-title">Containers</div>
            <DockerEngineDetail engineId={component.id} onCountsLoaded={() => {}} />
          </div>
        )}

        {component.type === 'traefik' && traefikDetail && (
          <TraefikSection detail={traefikDetail} />
        )}

        {component.type === 'traefik' && !traefikDetail && !loading && (
          <div className="icd-section">
            <div className="icd-empty">Loading Traefik detail…</div>
          </div>
        )}

        {/* Discovered VMs & LXC containers (Proxmox nodes) */}
        {component.type === 'proxmox_node' && (
          <ProxmoxChildrenSection children={children} onNavigate={navigate} />
        )}

        {/* Linked Applications */}
        <LinkedAppsSection
          linkedApps={linkedApps}
          allApps={allApps}
          linkingAppId={linkingAppId}
          linkBusy={linkBusy}
          onSelectApp={setLinkingAppId}
          onLink={() => void handleLinkApp()}
          onUnlink={(appId) => void handleUnlinkApp(appId)}
        />

      </div>
    </>
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
        <div className="icd-empty">No VMs or containers discovered yet. Run Scan Now to discover.</div>
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
