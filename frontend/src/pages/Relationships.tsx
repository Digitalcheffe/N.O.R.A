import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { SlidePanel } from '../components/SlidePanel'
import {
  apps as appsApi,
  infrastructure as infraApi,
  links as linksApi,
  discovery,
  checks as checksApi,
} from '../api/client'
import type {
  App,
  ComponentLink,
  InfrastructureComponent,
  DiscoveredContainer,
  MonitorCheck,
} from '../api/types'
import './Relationships.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

const TYPE_LABEL: Record<string, string> = {
  proxmox_node:   'Proxmox Node',
  vm_linux:       'VM Linux',
  vm_windows:     'VM Windows',
  vm_other:       'VM Other',
  linux_host:     'Linux Host',
  windows_host:   'Windows Host',
  generic_host:   'Generic Host',
  synology:       'Synology NAS',
  docker_engine:  'Docker Engine',
  traefik:        'Traefik',
  portainer:      'Portainer',
}

function appIconUrl(profileId: string | null): string | null {
  return profileId ? `/api/v1/icons/${profileId}` : null
}

function formatImage(image: string): string {
  if (image.startsWith('sha256:')) return 'sha256:' + image.slice(7, 19) + '…'
  const colonIdx = image.lastIndexOf(':')
  if (colonIdx === -1) return image.length > 40 ? image.slice(0, 39) + '…' : image
  const name = image.slice(0, colonIdx)
  const tag  = image.slice(colonIdx + 1)
  const short = name.length > 35 ? name.slice(0, 34) + '…' : name
  return `${short}:${tag}`
}

function StatusDot({ status }: { status?: string }) {
  const s = status ?? 'unknown'
  const cls =
    s === 'online'  ? 'rel-dot rel-dot--online'  :
    s === 'offline' ? 'rel-dot rel-dot--offline' :
                      'rel-dot rel-dot--unknown'
  return <span className={cls} title={s} />
}

function AppCell({ app }: { app: App }) {
  const [imgFailed, setImgFailed] = useState(false)
  useEffect(() => { setImgFailed(false) }, [app.profile_id])

  return (
    <div className="rel-app-cell">
      <span className="rel-app-icon">
        {app.profile_id && !imgFailed ? (
          <img
            src={`/api/v1/icons/${app.profile_id}`}
            alt={app.name}
            onError={() => setImgFailed(true)}
          />
        ) : (
          <span className="rel-app-monogram">
            {app.name.trim().slice(0, 2).toUpperCase()}
          </span>
        )}
      </span>
      <span className="rel-app-name">{app.name}</span>
    </div>
  )
}

function GearIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3"/>
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06-.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
    </svg>
  )
}

// ── Topology rules: valid parent types per child type ────────────────────────
const VALID_PARENTS: Record<string, string[]> = {
  vm_linux:       ['proxmox'],
  vm_windows:     ['proxmox'],
  vm_other:       ['proxmox'],
  docker_engine:  ['vm_linux', 'vm_windows', 'vm_other', 'linux_host', 'windows_host', 'synology'],
  portainer:      ['vm_linux', 'vm_windows', 'vm_other', 'linux_host', 'windows_host', 'synology'],
  traefik:        ['vm_linux', 'vm_windows', 'vm_other', 'linux_host', 'windows_host'],
  traefik_router: ['traefik'],
  traefik_service:['traefik'],
  container:      ['portainer', 'docker_engine'],
  // proxmox, linux_host, windows_host, synology cannot be children
}

// ── Page ──────────────────────────────────────────────────────────────────────

type Tab = 'apps' | 'containers' | 'monitors' | 'infrastructure'

export function Relationships() {
  const navigate = useNavigate()

  const [tab, setTab] = useState<Tab>('apps')

  // ── Shared data ─────────────────────────────────────────────────────────────
  const [allApps,       setAllApps]       = useState<App[]>([])
  const [allContainers, setAllContainers] = useState<DiscoveredContainer[]>([])
  const [allComponents, setAllComponents] = useState<InfrastructureComponent[]>([])
  const [allLinks,      setAllLinks]      = useState<ComponentLink[]>([])
  const [monitors,      setMonitors]      = useState<MonitorCheck[]>([])
  const [loading,       setLoading]       = useState(true)
  const [error,         setError]         = useState('')

  // ── App panel state ─────────────────────────────────────────────────────────
  const [appPanelApp,    setAppPanelApp]    = useState<App | null>(null)
  const [appPanelCtrId,  setAppPanelCtrId]  = useState('')
  const [appPanelBusy,   setAppPanelBusy]   = useState(false)
  const [appPanelErr,    setAppPanelErr]    = useState('')

  // ── Infra panel state ───────────────────────────────────────────────────────
  const [infraPanelComp,     setInfraPanelComp]     = useState<InfrastructureComponent | null>(null)
  const [infraPanelParentId, setInfraPanelParentId] = useState('')
  const [infraPanelBusy,     setInfraPanelBusy]     = useState(false)
  const [infraPanelErr,      setInfraPanelErr]       = useState('')

  // ── Monitor panel state ─────────────────────────────────────────────────────
  const [moniPanelMon,   setMoniPanelMon]   = useState<MonitorCheck | null>(null)
  const [moniPanelAppId, setMoniPanelAppId] = useState('')
  const [moniPanelBusy,  setMoniPanelBusy]  = useState(false)
  const [moniPanelErr,   setMoniPanelErr]   = useState('')

  // ── Load all shared data once ───────────────────────────────────────────────
  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      setError('')
      try {
        const [appRes, ctrRes, infraRes, linkRes, moniRes] = await Promise.all([
          appsApi.list(),
          discovery.allContainers(),
          infraApi.list(),
          linksApi.list(),
          checksApi.list(),
        ])
        if (cancelled) return
        setAllApps(appRes.data)
        setAllContainers(ctrRes.data)
        setAllComponents(infraRes.data)
        setAllLinks(linkRes.data)
        setMonitors(moniRes.data)
      } catch (e) {
        if (!cancelled) setError(String(e))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    load()
    return () => { cancelled = true }
  }, [])

  // ── Derived maps ────────────────────────────────────────────────────────────
  // app_id → container (the container that has this app linked)
  const containerByAppId = new Map<string, DiscoveredContainer>()
  for (const c of allContainers) {
    if (c.app_id) containerByAppId.set(c.app_id, c)
  }

  // infra_component_id → component
  const componentById = new Map<string, InfrastructureComponent>()
  for (const c of allComponents) componentById.set(c.id, c)

  // app_id → component_link (for direct infra links, fallback)
  const directLinkByAppId = new Map<string, ComponentLink>()
  for (const l of allLinks) {
    if (l.child_type === 'app') directLinkByAppId.set(l.child_id, l)
  }

  // ── Infra panel helpers ─────────────────────────────────────────────────────
  // Build parent lookup: child_id → parent component
  const infraParentByChildId = new Map<string, InfrastructureComponent>()
  for (const l of allLinks) {
    if (l.child_type in TYPE_LABEL || Object.keys(TYPE_LABEL).includes(l.child_type)) {
      const parent = componentById.get(l.parent_id)
      if (parent) infraParentByChildId.set(l.child_id, parent)
    }
  }

  function openInfraPanel(comp: InfrastructureComponent) {
    const parent = infraParentByChildId.get(comp.id)
    setInfraPanelComp(comp)
    setInfraPanelParentId(parent?.id ?? '')
    setInfraPanelErr('')
  }

  async function handleInfraLink() {
    if (!infraPanelComp || !infraPanelParentId) return
    setInfraPanelBusy(true)
    setInfraPanelErr('')
    try {
      const parent = componentById.get(infraPanelParentId)
      await linksApi.setParent(parent?.type ?? 'unknown', infraPanelParentId, infraPanelComp.type, infraPanelComp.id)
      setAllLinks(prev => {
        const filtered = prev.filter(l => !(l.child_type === infraPanelComp.type && l.child_id === infraPanelComp.id))
        return [...filtered, {
          parent_type: parent?.type ?? 'unknown',
          parent_id:   infraPanelParentId,
          child_type:  infraPanelComp.type,
          child_id:    infraPanelComp.id,
          created_at:  new Date().toISOString(),
        }]
      })
      setInfraPanelComp(null)
    } catch (e) {
      setInfraPanelErr(String(e))
    } finally {
      setInfraPanelBusy(false)
    }
  }

  async function handleInfraUnlink() {
    if (!infraPanelComp) return
    setInfraPanelBusy(true)
    setInfraPanelErr('')
    try {
      await linksApi.removeParent(infraPanelComp.type, infraPanelComp.id)
      setAllLinks(prev => prev.filter(l => !(l.child_type === infraPanelComp.type && l.child_id === infraPanelComp.id)))
      setInfraPanelComp(null)
    } catch (e) {
      setInfraPanelErr(String(e))
    } finally {
      setInfraPanelBusy(false)
    }
  }

  // ── App panel handlers ──────────────────────────────────────────────────────
  function openAppPanel(app: App) {
    const linkedCtr = containerByAppId.get(app.id)
    setAppPanelApp(app)
    setAppPanelCtrId(linkedCtr?.id ?? '')
    setAppPanelErr('')
  }

  async function handleAppLink() {
    if (!appPanelApp || !appPanelCtrId) return
    setAppPanelBusy(true)
    setAppPanelErr('')
    try {
      await discovery.linkContainerApp(appPanelCtrId, { mode: 'existing', app_id: appPanelApp.id })
      setAllContainers(prev => prev.map(c =>
        c.id === appPanelCtrId ? { ...c, app_id: appPanelApp.id } : c
      ))
      setAppPanelApp(null)
    } catch (e) {
      setAppPanelErr(String(e))
    } finally {
      setAppPanelBusy(false)
    }
  }

  async function handleAppUnlink() {
    if (!appPanelApp) return
    const linkedCtr = containerByAppId.get(appPanelApp.id)
    if (!linkedCtr) return
    setAppPanelBusy(true)
    setAppPanelErr('')
    try {
      await discovery.unlinkContainerApp(linkedCtr.id)
      setAllContainers(prev => prev.map(c =>
        c.id === linkedCtr.id ? { ...c, app_id: null } : c
      ))
      // Also remove direct infra link if any
      await linksApi.removeParent('app', appPanelApp.id).catch(() => {})
      setAllLinks(prev => prev.filter(l => !(l.child_type === 'app' && l.child_id === appPanelApp.id)))
      setAppPanelApp(null)
    } catch (e) {
      setAppPanelErr(String(e))
    } finally {
      setAppPanelBusy(false)
    }
  }

  // ── Monitor panel handlers ──────────────────────────────────────────────────
  function openMoniPanel(m: MonitorCheck) {
    setMoniPanelMon(m)
    setMoniPanelAppId(m.app_id ?? '')
    setMoniPanelErr('')
  }

  async function handleMoniSave() {
    if (!moniPanelMon) return
    setMoniPanelBusy(true)
    setMoniPanelErr('')
    try {
      const updated = await checksApi.update(moniPanelMon.id, { app_id: moniPanelAppId || undefined })
      setMonitors(prev => prev.map(m => m.id === moniPanelMon.id ? updated : m))
      setMoniPanelMon(null)
    } catch (e) {
      setMoniPanelErr(String(e))
    } finally {
      setMoniPanelBusy(false)
    }
  }

  async function handleMoniUnlink() {
    if (!moniPanelMon) return
    setMoniPanelBusy(true)
    setMoniPanelErr('')
    try {
      const updated = await checksApi.update(moniPanelMon.id, { app_id: undefined })
      setMonitors(prev => prev.map(m => m.id === moniPanelMon.id ? { ...updated, app_id: null } : m))
      setMoniPanelMon(null)
    } catch (e) {
      setMoniPanelErr(String(e))
    } finally {
      setMoniPanelBusy(false)
    }
  }

  // ── Counts ──────────────────────────────────────────────────────────────────
  const appLinkedCount   = allApps.filter(a => containerByAppId.has(a.id)).length
  const ctrLinkedCount   = allContainers.filter(c => !!c.app_id).length
  const moniLinkedCount  = monitors.filter(m => !!m.app_id).length
  const infraLinkedCount = allComponents.filter(c => infraParentByChildId.has(c.id)).length

  const totalLinked   = appLinkedCount + ctrLinkedCount + moniLinkedCount + infraLinkedCount
  const totalUnlinked =
    (allApps.length - appLinkedCount) +
    (allContainers.length - ctrLinkedCount) +
    (monitors.length - moniLinkedCount) +
    (allComponents.length - infraLinkedCount)

  // ── Unlinked containers: not yet linked to any app ──────────────────────────
  const unlinkedContainers = allContainers.filter(c => !c.app_id)

  // ── Containers linked to this app ───────────────────────────────────────────
  const appPanelLinkedCtr = appPanelApp ? containerByAppId.get(appPanelApp.id) : undefined

  return (
    <>
      <Topbar title="Relationships" />
      <div className="content">

        {/* ── Tab bar ── */}
        <div className="rel-controls">
          <div className="rel-filter-group">
            {(['apps', 'containers', 'monitors', 'infrastructure'] as Tab[]).map(t => {
              const label =
                t === 'apps'       ? `Apps (${allApps.length})`            :
                t === 'containers' ? `Containers (${allContainers.length})` :
                t === 'monitors'   ? `Monitors (${monitors.length})`        :
                                     `Infrastructure (${allComponents.length})`
              return (
                <button
                  key={t}
                  className={`rel-filter-btn${tab === t ? ' active' : ''}`}
                  onClick={() => setTab(t)}
                >
                  {label}
                </button>
              )
            })}
          </div>
          {!loading && (
            <span className="rel-tab-summary rel-dim">
              {`${totalLinked} linked · ${totalUnlinked} unlinked`}
            </span>
          )}
        </div>

        {loading && <div className="rel-state">Loading…</div>}
        {error   && <div className="rel-state rel-state--error">{error}</div>}

        {!loading && !error && (
          <>
            {/* ══════════════════ APPS TAB ══════════════════ */}
            {tab === 'apps' && (
              allApps.length === 0 ? (
                <div className="rel-state">No apps found.</div>
              ) : (
                <div className="rel-table-wrap">
                  <table className="rel-table">
                    <thead>
                      <tr>
                        <th>App</th>
                        <th>App Profile</th>
                        <th>Container</th>
                        <th>Status</th>
                        <th></th>
                      </tr>
                    </thead>
                    <tbody>
                      {[...allApps].sort((a, b) => a.name.localeCompare(b.name)).map(app => {
                        const ctr  = containerByAppId.get(app.id)
                        const comp = ctr ? componentById.get(ctr.infra_component_id) : undefined
                        return (
                          <tr
                            key={app.id}
                            className="rel-row"
                            onClick={() => navigate(`/apps/${app.id}`)}
                            title={`Open ${app.name}`}
                          >
                            <td><AppCell app={app} /></td>

                            <td className="rel-mono">
                              {app.profile_id
                                ? <span className="rel-profile-badge">{app.profile_id}</span>
                                : <span className="rel-dim">—</span>
                              }
                            </td>

                            <td className="rel-mono" style={{ fontSize: 12 }}>
                              {ctr
                                ? <span style={{ color: 'var(--text2)' }}>{ctr.container_name}</span>
                                : <span className="rel-dim rel-unlinked-hint">Not linked</span>
                              }
                            </td>

                            <td>
                              <div className="rel-status-cell">
                                <StatusDot status={comp?.last_status} />
                                <span className="rel-status-label rel-dim">
                                  {comp?.last_status ?? '—'}
                                </span>
                              </div>
                            </td>

                            <td className="rel-action-cell">
                              <button
                                className="rel-gear-btn"
                                title="Manage link"
                                onClick={e => { e.stopPropagation(); openAppPanel(app) }}
                              >
                                <GearIcon />
                              </button>
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )
            )}

            {/* ══════════════════ CONTAINERS TAB ══════════════════ */}
            {tab === 'containers' && (
              allContainers.length === 0 ? (
                <div className="rel-state">No containers discovered yet.</div>
              ) : (
                <div className="rel-table-wrap">
                  <table className="rel-table">
                    <thead>
                      <tr>
                        <th>Container</th>
                        <th>Image</th>
                        <th>Source</th>
                        <th>Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {allContainers.map(c => (
                        <tr key={c.id} className="rel-row">
                          <td className="rel-mono" style={{ color: 'var(--text)', fontWeight: 500 }}>
                            {c.container_name}
                          </td>

                          <td className="rel-mono rel-dim" style={{ fontSize: 11 }}>
                            {formatImage(c.image)}
                          </td>

                          <td>
                            <span className={`rel-source-badge rel-source-${c.source_type}`}>
                              {c.source_type === 'docker_engine' ? 'Docker' :
                               c.source_type === 'portainer'    ? 'Portainer' : c.source_type}
                            </span>
                          </td>

                          <td>
                            <span className={`rel-ctr-status rel-ctr-status--${c.status}`}>
                              {c.status}
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )
            )}

            {/* ══════════════════ MONITORS TAB ══════════════════ */}
            {tab === 'monitors' && (
              monitors.length === 0 ? (
                <div className="rel-state">No monitors found.</div>
              ) : (
                <div className="rel-table-wrap">
                  <table className="rel-table">
                    <thead>
                      <tr>
                        <th>Monitor</th>
                        <th>Type</th>
                        <th>Target</th>
                        <th>Status</th>
                        <th>App</th>
                        <th></th>
                      </tr>
                    </thead>
                    <tbody>
                      {monitors.map(m => {
                        const linkedApp = allApps.find(a => a.id === m.app_id)
                        const iconUrl   = linkedApp ? appIconUrl(linkedApp.profile_id ?? null) : null
                        return (
                          <tr key={m.id} className="rel-row" onClick={() => openMoniPanel(m)}>
                            <td style={{ color: 'var(--text)', fontWeight: 500 }}>{m.name}</td>

                            <td>
                              <span className={`rel-source-badge rel-moni-type--${m.type}`}>
                                {m.type.toUpperCase()}
                              </span>
                            </td>

                            <td className="rel-mono rel-dim" style={{ fontSize: 11 }}>
                              {m.target.length > 50 ? m.target.slice(0, 49) + '…' : m.target}
                            </td>

                            <td>
                              <span className={`rel-ctr-status rel-ctr-status--${m.last_status ?? 'unknown'}`}>
                                {m.last_status ?? '—'}
                              </span>
                            </td>

                            <td>
                              {linkedApp ? (
                                <div className="rel-linked-app">
                                  {iconUrl && (
                                    <img
                                      src={iconUrl}
                                      alt=""
                                      className="rel-linked-app-icon"
                                      onError={e => { (e.target as HTMLImageElement).style.display = 'none' }}
                                    />
                                  )}
                                  <span style={{ color: 'var(--accent)' }}>{linkedApp.name}</span>
                                </div>
                              ) : (
                                <span className="rel-dim">—</span>
                              )}
                            </td>

                            <td className="rel-action-cell">
                              <button
                                className="rel-gear-btn"
                                title="Manage link"
                                onClick={e => { e.stopPropagation(); openMoniPanel(m) }}
                              >
                                <GearIcon />
                              </button>
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )
            )}

            {/* ══════════════════ INFRA TAB ══════════════════ */}
            {tab === 'infrastructure' && (
              allComponents.length === 0 ? (
                <div className="rel-state">No infrastructure components found.</div>
              ) : (() => {
                return (
                  <div className="rel-table-wrap">
                    <table className="rel-table">
                      <thead>
                        <tr>
                          <th>Component</th>
                          <th>Type</th>
                          <th>Status</th>
                          <th>Parent</th>
                          <th>Containers</th>
                          <th></th>
                        </tr>
                      </thead>
                      <tbody>
                        {[...allComponents].sort((a, b) => a.name.localeCompare(b.name)).map(comp => {
                          const parent = infraParentByChildId.get(comp.id)
                          const ctrs   = allContainers.filter(c => c.infra_component_id === comp.id)
                          return (
                            <tr key={comp.id} className="rel-row" onClick={() => navigate(`/infrastructure/${comp.id}`)}>
                              <td style={{ color: 'var(--text)', fontWeight: 500 }}>{comp.name}</td>
                              <td className="rel-mono rel-type">{TYPE_LABEL[comp.type] ?? comp.type}</td>
                              <td>
                                <div className="rel-status-cell">
                                  <StatusDot status={comp.last_status} />
                                  <span className="rel-status-label rel-dim">{comp.last_status ?? '—'}</span>
                                </div>
                              </td>
                              <td>
                                {parent
                                  ? <span style={{ fontSize: 12, color: 'var(--text2)', fontFamily: 'var(--mono)' }}>{parent.name}</span>
                                  : <span className="rel-dim">—</span>
                                }
                              </td>
                              <td className="rel-mono rel-dim" style={{ fontSize: 11 }}>
                                {ctrs.length > 0 ? `${ctrs.length} container${ctrs.length !== 1 ? 's' : ''}` : <span className="rel-dim">—</span>}
                              </td>
                              <td className="rel-action-cell">
                                {VALID_PARENTS[comp.type] && (
                                  <button
                                    className="rel-gear-btn"
                                    title="Manage relationship"
                                    onClick={e => { e.stopPropagation(); openInfraPanel(comp) }}
                                  >
                                    <GearIcon />
                                  </button>
                                )}
                              </td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                )
              })()
            )}
          </>
        )}
      </div>

      {/* ══════════════ APP LINK PANEL ══════════════ */}
      <SlidePanel
        open={!!appPanelApp}
        onClose={() => setAppPanelApp(null)}
        title="Link App to Container"
        subtitle={appPanelApp?.name}
        width={420}
        footer={
          appPanelLinkedCtr ? (
            <button
              className="rel-panel-unlink-btn"
              disabled={appPanelBusy}
              onClick={() => void handleAppUnlink()}
            >
              {appPanelBusy ? 'Unlinking…' : 'Unlink'}
            </button>
          ) : (
            <button
              className="rel-panel-link-btn"
              disabled={appPanelBusy || !appPanelCtrId}
              onClick={() => void handleAppLink()}
            >
              {appPanelBusy ? 'Linking…' : 'Link'}
            </button>
          )
        }
      >
        {appPanelApp && (
          <div className="rel-panel-body">
            {appPanelLinkedCtr ? (
              <>
                <p className="rel-panel-label">Currently linked to container</p>
                <div className="rel-panel-current">
                  <div className="rel-panel-current-name">{appPanelLinkedCtr.container_name}</div>
                  <div className="rel-panel-current-type rel-dim rel-mono">
                    {formatImage(appPanelLinkedCtr.image)}
                  </div>
                </div>
                <p className="rel-panel-label" style={{ marginTop: 16 }}>
                  Use Unlink to remove, or pick a different container below.
                </p>
              </>
            ) : (
              <p className="rel-panel-label">Link this app to a container</p>
            )}

            <select
              className="rel-panel-select"
              value={appPanelCtrId}
              onChange={e => setAppPanelCtrId(e.target.value)}
            >
              <option value="">— Select container —</option>
              {/* Show unlinked containers first, then all others */}
              {unlinkedContainers.length > 0 && (
                <optgroup label="Available (unlinked)">
                  {unlinkedContainers.map(c => (
                    <option key={c.id} value={c.id}>{c.container_name}</option>
                  ))}
                </optgroup>
              )}
              {allContainers.filter(c => c.app_id && c.app_id !== appPanelApp?.id).length > 0 && (
                <optgroup label="Already linked to another app">
                  {allContainers.filter(c => c.app_id && c.app_id !== appPanelApp?.id).map(c => (
                    <option key={c.id} value={c.id}>{c.container_name}</option>
                  ))}
                </optgroup>
              )}
            </select>

            {appPanelErr && <div className="rel-panel-error">{appPanelErr}</div>}
          </div>
        )}
      </SlidePanel>

      {/* ══════════════ MONITOR LINK PANEL ══════════════ */}
      <SlidePanel
        open={!!moniPanelMon}
        onClose={() => setMoniPanelMon(null)}
        title="Link Monitor to App"
        subtitle={moniPanelMon?.name}
        width={420}
        footer={
          moniPanelMon?.app_id ? (
            <>
              <button
                className="rel-panel-link-btn"
                disabled={moniPanelBusy || !moniPanelAppId || moniPanelAppId === moniPanelMon.app_id}
                onClick={() => void handleMoniSave()}
              >
                {moniPanelBusy ? 'Saving…' : 'Change'}
              </button>
              <button
                className="rel-panel-unlink-btn"
                disabled={moniPanelBusy}
                onClick={() => void handleMoniUnlink()}
              >
                {moniPanelBusy ? '…' : 'Unlink'}
              </button>
            </>
          ) : (
            <button
              className="rel-panel-link-btn"
              disabled={moniPanelBusy || !moniPanelAppId}
              onClick={() => void handleMoniSave()}
            >
              {moniPanelBusy ? 'Linking…' : 'Link'}
            </button>
          )
        }
      >
        {moniPanelMon && (
          <div className="rel-panel-body">
            <div className="rel-panel-info-grid">
              <span className="rel-panel-info-label">Type</span>
              <span className="rel-panel-info-value rel-mono">{moniPanelMon.type.toUpperCase()}</span>
              <span className="rel-panel-info-label">Target</span>
              <span className="rel-panel-info-value rel-mono" style={{ wordBreak: 'break-all' }}>
                {moniPanelMon.target}
              </span>
              <span className="rel-panel-info-label">Status</span>
              <span className={`rel-panel-info-value rel-mono rel-ctr-status--${moniPanelMon.last_status ?? 'unknown'}`}>
                {moniPanelMon.last_status ?? '—'}
              </span>
            </div>

            {moniPanelMon.app_id && (() => {
              const a = allApps.find(x => x.id === moniPanelMon.app_id)
              const icon = a ? appIconUrl(a.profile_id ?? null) : null
              return a ? (
                <div className="rel-panel-linked-app">
                  {icon && <img src={icon} alt="" className="rel-linked-app-icon" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />}
                  <span>{a.name}</span>
                </div>
              ) : null
            })()}

            <p className="rel-panel-label">
              {moniPanelMon.app_id ? 'Change or unlink the app for this monitor.' : 'Link this monitor to an app'}
            </p>

            <select
              className="rel-panel-select"
              value={moniPanelAppId}
              onChange={e => setMoniPanelAppId(e.target.value)}
            >
              <option value="">— Select app —</option>
              {allApps.map(a => (
                <option key={a.id} value={a.id}>{a.name}</option>
              ))}
            </select>

            {moniPanelErr && <div className="rel-panel-error">{moniPanelErr}</div>}
          </div>
        )}
      </SlidePanel>

      {/* ══════════════ INFRA RELATIONSHIP PANEL ══════════════ */}
      <SlidePanel
        open={!!infraPanelComp}
        onClose={() => setInfraPanelComp(null)}
        title="Manage Infra Relationship"
        subtitle={infraPanelComp?.name}
        width={420}
        footer={
          infraParentByChildId.has(infraPanelComp?.id ?? '') ? (
            <>
              <button
                className="rel-panel-link-btn"
                disabled={infraPanelBusy || !infraPanelParentId || infraPanelParentId === infraParentByChildId.get(infraPanelComp!.id)?.id}
                onClick={() => void handleInfraLink()}
              >
                {infraPanelBusy ? 'Saving…' : 'Change'}
              </button>
              <button
                className="rel-panel-unlink-btn"
                disabled={infraPanelBusy}
                onClick={() => void handleInfraUnlink()}
              >
                {infraPanelBusy ? '…' : 'Remove'}
              </button>
            </>
          ) : (
            <button
              className="rel-panel-link-btn"
              disabled={infraPanelBusy || !infraPanelParentId}
              onClick={() => void handleInfraLink()}
            >
              {infraPanelBusy ? 'Saving…' : 'Set Parent'}
            </button>
          )
        }
      >
        {infraPanelComp && (
          <div className="rel-panel-body">
            <div className="rel-panel-info-grid">
              <span className="rel-panel-info-label">Type</span>
              <span className="rel-panel-info-value rel-mono">{TYPE_LABEL[infraPanelComp.type] ?? infraPanelComp.type}</span>
              <span className="rel-panel-info-label">Status</span>
              <span className="rel-panel-info-value rel-mono">{infraPanelComp.last_status ?? '—'}</span>
            </div>

            {(() => {
              const parent = infraParentByChildId.get(infraPanelComp.id)
              return parent ? (
                <>
                  <p className="rel-panel-label">Current parent</p>
                  <div className="rel-panel-current">
                    <div className="rel-panel-current-name">{parent.name}</div>
                    <div className="rel-panel-current-type rel-dim">{TYPE_LABEL[parent.type] ?? parent.type}</div>
                  </div>
                  <p className="rel-panel-label" style={{ marginTop: 16 }}>Change to a different parent, or remove the relationship.</p>
                </>
              ) : (
                <p className="rel-panel-label">Set a parent infrastructure component</p>
              )
            })()}

            {(() => {
              const validParentTypes = new Set(VALID_PARENTS[infraPanelComp.type] ?? [])
              const validParents = allComponents
                .filter(c => c.id !== infraPanelComp.id && validParentTypes.has(c.type))
                .sort((a, b) => a.name.localeCompare(b.name))
              return validParents.length === 0 ? (
                <p className="rel-panel-label" style={{ color: 'var(--red)' }}>
                  No valid parent types exist for {TYPE_LABEL[infraPanelComp.type] ?? infraPanelComp.type} components.
                </p>
              ) : (
                <select
                  className="rel-panel-select"
                  value={infraPanelParentId}
                  onChange={e => setInfraPanelParentId(e.target.value)}
                >
                  <option value="">— Select parent —</option>
                  {validParents.map(c => (
                    <option key={c.id} value={c.id}>
                      {c.name} ({TYPE_LABEL[c.type] ?? c.type})
                    </option>
                  ))}
                </select>
              )
            })()}

            {infraPanelErr && <div className="rel-panel-error">{infraPanelErr}</div>}
          </div>
        )}
      </SlidePanel>
    </>
  )
}
