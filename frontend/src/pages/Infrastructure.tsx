import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { infrastructure as infraApi, discovery, apps as appsApi } from '../api/client'
import type {
  InfrastructureComponent,
  DiscoveredContainer,
  App,
} from '../api/types'
import './Infrastructure.css'
import './Relationships.css'
import '../components/CheckForm.css'
import '../components/DockerEngineDetail.css'

import { InfraTypeIcon } from '../components/CheckTypeIcon'
import { InfraEditModal, TYPE_LABEL } from './InfraEditModal'

// ── Helpers ───────────────────────────────────────────────────────────────────

function statusClass(s: string): string {
  if (s === 'online')   return 'online'
  if (s === 'degraded') return 'degraded'
  if (s === 'offline')  return 'offline'
  return 'unknown'
}

function statusLabel(s: string): string {
  if (s === 'online')   return 'Online'
  if (s === 'degraded') return 'Degraded'
  if (s === 'offline')  return 'Offline'
  return 'Unknown'
}

// ── Main page ─────────────────────────────────────────────────────────────────

type InfraView = 'components' | 'containers'

export function Infrastructure() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const { tick: refreshTick } = useAutoRefresh()
  const [view, setView] = useState<InfraView>(
    searchParams.get('view') === 'containers' ? 'containers' : 'components'
  )

  function switchView(v: InfraView) {
    setView(v)
    setSearchParams(v === 'containers' ? { view: 'containers' } : {}, { replace: true })
  }
  const [components,      setComponents]      = useState<InfrastructureComponent[]>([])
  const [loading,         setLoading]         = useState(true)
  const [containers,      setContainers]      = useState<DiscoveredContainer[]>([])
  const [containersLoading, setContainersLoading] = useState(false)
  const [allApps,         setAllApps]         = useState<App[]>([])
  // Panel state
  const [modalOpen,             setModalOpen]             = useState(false)
  const [openKey,               setOpenKey]               = useState(0)
  const [editingComponent,      setEditingComponent]      = useState<InfrastructureComponent | null>(null)
  const [editingHasCreds,       setEditingHasCreds]       = useState(false)
  const [initialParentId,       setInitialParentId]       = useState<string | undefined>(undefined)
  const [deletingId]                                       = useState<string | null>(null)

  // ── Initial load + auto-refresh ─────────────────────────────────────────────
  // The list view no longer shows CPU/MEM/DSK bars per component, so we don't
  // fan out N /resources calls here. Fresh resource numbers are fetched only
  // on the component detail page. Drastically cheaper page load on instances
  // with many components.
  useEffect(() => {
    infraApi.list()
      .then(res => setComponents(res.data))
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [refreshTick])

  // Load containers + apps (always, so container count is available on components tab too)
  useEffect(() => {
    // Only show the loading spinner on the initial load (no data yet).
    if (view === 'containers' && containers.length === 0) setContainersLoading(true)
    Promise.all([
      discovery.allContainers(),
      appsApi.list(),
    ])
      .then(([cRes, aRes]) => { setContainers(cRes.data); setAllApps(aRes.data) })
      .catch(console.error)
      .finally(() => setContainersLoading(false))
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view, refreshTick])

  // ── Modal helpers ────────────────────────────────────────────────────────────

  function openAdd(parentId?: string) {
    setEditingComponent(null)
    setEditingHasCreds(false)
    setInitialParentId(parentId)
    setOpenKey(k => k + 1)
    setModalOpen(true)
  }

  function openEdit(c: InfrastructureComponent) {
    setEditingComponent(c)
    setEditingHasCreds(c.has_credentials ?? false)
    setInitialParentId(undefined)
    setOpenKey(k => k + 1)
    setModalOpen(true)
  }

  function closeModal() {
    setModalOpen(false)
    setEditingComponent(null)
    setEditingHasCreds(false)
    setInitialParentId(undefined)
  }

  // ── Render helpers ──────────────────────────────────────────────────────────

  function renderTraefikCard(c: InfrastructureComponent) {
    const isDeleting = deletingId === c.id

    return (
      <div key={c.id} className="infra-card">
        <div className="infra-card-header" style={{ cursor: 'pointer' }} onClick={() => navigate(`/infrastructure/${c.id}`)}>
          <InfraTypeIcon type={c.type} />
          <div className="infra-card-title-group">
            <div className="infra-card-name">
              {c.name}
              <span className="infra-card-nav-arrow" aria-hidden="true"> ›</span>
            </div>
            <div className="infra-card-meta">Traefik · {c.ip || '—'}</div>
          </div>
          <div className="infra-card-status-group" onClick={e => e.stopPropagation()}>
            <span className={`infra-status-dot ${statusClass(c.last_status)}`} />
            <span className="infra-status-label">{statusLabel(c.last_status)}</span>
          </div>
        </div>

        <button
          className="infra-card-gear-btn"
          title="Settings"
          onClick={e => { e.stopPropagation(); openEdit(c) }}
          disabled={isDeleting}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="13" height="13">
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
          </svg>
        </button>
      </div>
    )
  }

  function renderDockerCard(c: InfrastructureComponent) {
    const isDeleting = deletingId === c.id

    return (
      <div key={c.id} className="infra-card">
        <div
          className="infra-card-header"
          style={{ cursor: 'pointer' }}
          onClick={() => navigate(`/infrastructure/${c.id}`)}
        >
          <InfraTypeIcon type={c.type} />
          <div className="infra-card-title-group">
            <div className="infra-card-name">{c.name}</div>
            <div className="infra-card-meta">Docker Engine · {c.ip || 'local socket'}</div>
          </div>
          <div className="infra-card-status-group" onClick={e => e.stopPropagation()}>
            <span className={`infra-status-dot ${statusClass(c.last_status)}`} />
            <span className="infra-status-label">{statusLabel(c.last_status)}</span>
          </div>
        </div>

        <button
          className="infra-card-gear-btn"
          title="Settings"
          onClick={e => { e.stopPropagation(); openEdit(c) }}
          disabled={isDeleting}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="13" height="13">
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
          </svg>
        </button>
      </div>
    )
  }

  function renderPortainerCard(c: InfrastructureComponent) {
    const isDeleting = deletingId === c.id

    return (
      <div key={c.id} className="infra-card">
        <div className="infra-card-header" style={{ cursor: 'pointer' }} onClick={() => navigate(`/infrastructure/${c.id}`)}>
          <InfraTypeIcon type={c.type} />
          <div className="infra-card-title-group">
            <div className="infra-card-name">
              {c.name}
              <span className="infra-card-nav-arrow" aria-hidden="true"> ›</span>
            </div>
            <div className="infra-card-meta">Portainer · {c.ip || '—'}</div>
          </div>
          <div className="infra-card-status-group" onClick={e => e.stopPropagation()}>
            <span className={`infra-status-dot ${statusClass(c.last_status)}`} />
            <span className="infra-status-label">{statusLabel(c.last_status)}</span>
          </div>
        </div>

        <button
          className="infra-card-gear-btn"
          title="Settings"
          onClick={e => { e.stopPropagation(); openEdit(c) }}
          disabled={isDeleting}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="13" height="13">
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
          </svg>
        </button>
      </div>
    )
  }

  function renderCard(c: InfrastructureComponent) {
    if (c.type === 'traefik')       return renderTraefikCard(c)
    if (c.type === 'docker_engine') return renderDockerCard(c)
    if (c.type === 'portainer')     return renderPortainerCard(c)

    const isDeleting = deletingId === c.id

    const detailPath = `/infrastructure/${c.id}`

    return (
      <div key={c.id} className="infra-card">
        <div className="infra-card-header" style={{ cursor: 'pointer' }} onClick={() => navigate(detailPath)}>
          <InfraTypeIcon type={c.type} />
          <div className="infra-card-title-group">
            <div className="infra-card-name">
              {c.name}
              {c.type === 'proxmox_node' && (
                <span className="infra-card-nav-arrow" aria-hidden="true"> ›</span>
              )}
            </div>
            <div className="infra-card-meta">
              {TYPE_LABEL[c.type]} · {c.ip}
            </div>
          </div>
          <div className="infra-card-status-group" onClick={e => e.stopPropagation()}>
            <span className={`infra-status-dot ${statusClass(c.last_status)}`} />
            <span className="infra-status-label">{statusLabel(c.last_status)}</span>
          </div>
        </div>

        <button
          className="infra-card-gear-btn"
          title="Settings"
          onClick={e => { e.stopPropagation(); openEdit(c) }}
          disabled={isDeleting}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="13" height="13">
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
          </svg>
        </button>
      </div>
    )
  }

  // ── Render ───────────────────────────────────────────────────────────────────

  function containerStatusClass(s: string) {
    if (s === 'running') return 'online'
    if (s === 'stopped' || s === 'exited') return 'offline'
    return 'unknown'
  }

  function appIconUrl(profileId: string | null): string | null {
    return profileId ? `/api/v1/icons/${profileId}` : null
  }

  function renderContainersTable() {
    if (containersLoading) return <div className="infra-empty">Loading…</div>

    const hasDockerOrPortainer = components.some(
      c => c.type === 'docker_engine' || c.type === 'portainer'
    )

    if (containers.length === 0) {
      if (!hasDockerOrPortainer) {
        return (
          <div className="infra-empty">
            No Docker Engine or Portainer components configured.{' '}
            <button className="infra-add-btn" style={{ marginLeft: 8 }} onClick={() => { switchView('components'); openAdd() }}>
              + Add Component
            </button>
          </div>
        )
      }
      return (
        <div className="infra-empty">
          No containers discovered yet. Click <strong>Discover Now</strong> on your Docker Engine or Portainer component.
        </div>
      )
    }

    return (
      <div className="ctr-table">
        {/* Column headers */}
        <div className="ctr-table-header">
          <span>Container</span>
          <span>Status</span>
          <span>App</span>
          <span />
        </div>

        <div className="ctr-cards">
        {containers.map(c => {
          const app = allApps.find(a => a.id === c.app_id)
          const iconUrl = app ? appIconUrl(app.profile_id) : null
          const cardStatusClass = c.status === 'running' ? 'ctr-card-running'
            : (c.status === 'stopped' || c.status === 'exited') ? 'ctr-card-stopped'
            : ''
          return (
            <div key={c.id} className={`ctr-card ${cardStatusClass}`}>

              {/* Name + image */}
              <div className="ctr-card-identity">
                <div className="ctr-card-name">{c.container_name}</div>
                <div className="ctr-card-image">{c.image}</div>
              </div>

              {/* Status + update */}
              <div className="ctr-card-badges">
                <span className={`ctr-card-status-dot ${containerStatusClass(c.status)}`} />
                <span className="ctr-card-status-label">{c.status}</span>
                {c.image_update_available && (
                  <span className="ctr-card-update-pill">Update</span>
                )}
              </div>

              {/* Linked app */}
              <div className="ctr-card-app-col">
                {app ? (
                  <span className="ctr-app-linked">
                    {iconUrl && (
                      <img
                        src={iconUrl}
                        alt=""
                        className="ctr-app-icon"
                        onError={e => { (e.target as HTMLImageElement).style.display = 'none' }}
                      />
                    )}
                    <span className="ctr-card-app-name">{app.name}</span>
                  </span>
                ) : (
                  <span className="ctr-card-no-app">—</span>
                )}
              </div>

              {/* Navigate */}
              <button
                className="ctr-card-open"
                onClick={() => navigate(`/containers/${c.id}`)}
                title={`Open ${c.container_name}`}
              >
                →
              </button>
            </div>
          )
        })}
        </div>
      </div>
    )
  }

  return (
    <>
      <Topbar title="Infrastructure" />
      <div className="content">

        {/* ── Header row ── */}
        <div className="infra-tab-row">
          <div className="infra-tabs">
            <button
              className={`infra-tab${view === 'components' ? ' active' : ''}`}
              onClick={() => switchView('components')}
            >
              Components
            </button>
            <button
              className={`infra-tab${view === 'containers' ? ' active' : ''}`}
              onClick={() => switchView('containers')}
            >
              Containers
            </button>
          </div>
          {view === 'components' && (
            <button className="infra-add-btn" onClick={() => openAdd()}>
              + Add Component
            </button>
          )}
        </div>

        {/* ── Stats strip ── */}
        {!loading && (() => {
          if (view === 'components' && components.length > 0) {
            const online   = components.filter(c => c.last_status === 'online').length
            const offline  = components.filter(c => c.last_status === 'offline').length
            const degraded = components.filter(c => c.last_status === 'degraded').length
            const typeCounts = [
              { label: 'VM',        n: components.filter(c => c.type === 'vm_linux' || c.type === 'vm_windows').length },
              { label: 'Proxmox',   n: components.filter(c => c.type === 'proxmox_node').length },
              { label: 'Docker',    n: components.filter(c => c.type === 'docker_engine').length },
              { label: 'Portainer', n: components.filter(c => c.type === 'portainer').length },
              { label: 'Traefik',   n: components.filter(c => c.type === 'traefik').length },
              { label: 'Synology',  n: components.filter(c => c.type === 'synology').length },
            ].filter(x => x.n > 0)
            const ctrCount = containers.length
            return (
              <div className="infra-stats-strip">
                <span className="infra-stats-pill" style={{ color: 'var(--green)' }}>{online} online</span>
                {degraded > 0 && <span className="infra-stats-pill" style={{ color: 'var(--yellow)' }}>{degraded} degraded</span>}
                {offline  > 0 && <span className="infra-stats-pill" style={{ color: 'var(--red)' }}>{offline} offline</span>}
                {typeCounts.length > 0 && <span className="infra-stats-sep" />}
                {typeCounts.map(({ label, n }) => <span key={label} className="infra-stats-pill">{n} {label}</span>)}
                {ctrCount > 0 && <><span className="infra-stats-sep" /><span className="infra-stats-pill">{ctrCount} container{ctrCount !== 1 ? 's' : ''}</span></>}
              </div>
            )
          }
          if (view === 'containers' && containers.length > 0) {
            const running = containers.filter(c => c.status === 'running').length
            const stopped = containers.filter(c => c.status !== 'running').length
            const linked  = containers.filter(c => c.app_id).length
            return (
              <div className="infra-stats-strip">
                <span className="infra-stats-pill" style={{ color: 'var(--green)' }}>{running} running</span>
                {stopped > 0 && <span className="infra-stats-pill" style={{ color: 'var(--text3)' }}>{stopped} stopped</span>}
                <span className="infra-stats-sep" />
                <span className="infra-stats-pill">{linked} linked to app{linked !== 1 ? 's' : ''}</span>
              </div>
            )
          }
          return null
        })()}

        {view === 'containers' ? renderContainersTable() : loading ? (
          <div className="infra-empty">Loading…</div>
        ) : components.length === 0 ? (
          <div className="infra-empty">
            No infrastructure components configured yet. Click <strong>+ Add Component</strong> to get started.
          </div>
        ) : (
          <div className="infra-card-list">
            {components.map(c => renderCard(c))}
          </div>
        )}

      </div>

      {/* ── Slide panel ── */}
      <InfraEditModal
        key={openKey}
        open={modalOpen}
        component={editingComponent ?? undefined}
        components={components}
        hasCreds={editingHasCreds}
        initialParentId={initialParentId}
        onSave={async (payload) => {
          if (editingComponent) {
            const updated = await infraApi.update(editingComponent.id, payload)
            setComponents(prev => prev.map(c => c.id === editingComponent.id ? updated : c))
          } else {
            const created = await infraApi.create(payload)
            setComponents(prev => [...prev, created])
          }
        }}
        onClose={closeModal}
        onDelete={editingComponent ? async () => {
          await infraApi.delete(editingComponent.id)
          setComponents(prev => prev.filter(c => c.id !== editingComponent.id))
        } : undefined}
      />
    </>
  )
}
