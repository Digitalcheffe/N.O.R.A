import { useState, useEffect, useCallback } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { infrastructure as infraApi, discovery, apps as appsApi } from '../api/client'
import type {
  InfrastructureComponent,
  ResourceSummary,
  DiscoverResult,
  VolumeResource,
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

function timeAgo(date: Date | null): string {
  if (!date) return '—'
  const diff = Math.floor((Date.now() - date.getTime()) / 1000)
  if (diff < 60) return `${diff}s ago`
  return `${Math.floor(diff / 60)}m ago`
}

function barClass(value: number, isDisk: boolean): string {
  if (!isDisk) return ''
  if (value > 95) return ' crit'
  if (value > 85) return ' warn'
  return ''
}

// ── Resource bar sub-component ────────────────────────────────────────────────

function ResBar({
  label, value, isDisk, noData,
}: { label: string; value: number; isDisk?: boolean; noData?: boolean }) {
  const cls = noData ? '' : barClass(value, !!isDisk)
  return (
    <div className="infra-res-row">
      <span className="infra-res-label">{label}</span>
      <div className="infra-res-track">
        <div
          className={`infra-res-fill${cls}${noData ? ' no-data' : ''}`}
          style={{ width: noData ? '0%' : `${Math.min(value, 100)}%` }}
        />
      </div>
      <span className={`infra-res-pct${noData ? ' no-data' : ''}`}>
        {noData ? 'Collecting…' : `${Math.round(value)}%`}
      </span>
    </div>
  )
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
  const [resourcesMap,    setResourcesMap]    = useState<Record<string, ResourceSummary>>({})
  const [lastPolledAt,    setLastPolledAt]    = useState<Date | null>(null)
  const [loading,         setLoading]         = useState(true)
  const [tick,            setTick]            = useState(0)
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
  const [scanningId,            setScanningId]            = useState<string | null>(null)
  const [scanResults,           setScanResults]           = useState<Record<string, DiscoverResult>>({})

  // ── Polling ─────────────────────────────────────────────────────────────────

  const pollAll = useCallback(async (compList: InfrastructureComponent[]) => {
    if (compList.length === 0) return

    const results = await Promise.allSettled(
      compList
        .filter(c => c.type !== 'traefik')
        .map(c => infraApi.resources(c.id, 'hour').then(r => ({ id: c.id, data: r })))
    )

    const resMap: Record<string, ResourceSummary> = {}
    for (const r of results) {
      if (r.status === 'fulfilled') resMap[r.value.id] = r.value.data
    }

    setResourcesMap(prev => ({ ...prev, ...resMap }))
    setLastPolledAt(new Date())
  }, [])

  // Initial load + auto-refresh
  useEffect(() => {
    infraApi.list()
      .then(res => {
        setComponents(res.data)
        return pollAll(res.data)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [pollAll, refreshTick])

  // 30-second polling interval
  useEffect(() => {
    if (components.length === 0) return
    const id = setInterval(() => { void pollAll(components) }, 30_000)
    return () => clearInterval(id)
  }, [components, pollAll])

  // Tick for time-ago label re-render (every 10s)
  useEffect(() => {
    const id = setInterval(() => setTick(t => t + 1), 10_000)
    return () => clearInterval(id)
  }, [])

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

  // Suppress unused variable warning for tick
  void tick

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

  async function handleScan(id: string) {
    setScanningId(id)
    setScanResults(prev => { const n = { ...prev }; delete n[id]; return n })
    try {
      const result = await infraApi.discover(id)
      setScanResults(prev => ({ ...prev, [id]: result }))
      // Refresh the component list and containers so everything reflects the new state.
      const [res, cRes, aRes] = await Promise.all([
        infraApi.list(),
        discovery.allContainers(),
        appsApi.list(),
      ])
      setComponents(res.data)
      setContainers(cRes.data)
      setAllApps(aRes.data)
      void pollAll(res.data)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Discover failed'
      setScanResults(prev => ({ ...prev, [id]: { status: 'error', discovered: 0, updated: 0, missing: 0, error: msg } }))
    } finally {
      setScanningId(null)
    }
  }

  // ── Render helpers ──────────────────────────────────────────────────────────

  function renderResourceBars(c: InfrastructureComponent) {
    const res = resourcesMap[c.id]
    const noData = !res || res.no_data

    if (c.type === 'synology' && res && !res.no_data && res.volumes && res.volumes.length > 0) {
      return (
        <div className="infra-res-bars">
          <ResBar label="CPU" value={res.cpu_percent} noData={noData} />
          <ResBar label="MEM" value={res.mem_percent} noData={noData} />
          {res.volumes.map((v: VolumeResource) => (
            <ResBar key={v.name} label={v.name.toUpperCase()} value={v.percent} isDisk noData={false} />
          ))}
        </div>
      )
    }

    return (
      <div className="infra-res-bars">
        <ResBar label="CPU" value={noData ? 0 : res!.cpu_percent} noData={noData} />
        <ResBar label="MEM" value={noData ? 0 : res!.mem_percent} noData={noData} />
        {c.type !== 'docker_engine' && (
          <ResBar label="DSK" value={noData ? 0 : res!.disk_percent} isDisk noData={noData} />
        )}
      </div>
    )
  }

  function renderTraefikCard(c: InfrastructureComponent) {
    const isDeleting = deletingId === c.id
    const isScanning = scanningId === c.id

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

        <div className="infra-card-footer">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4, flex: 1 }}>
            {lastPolledAt && (
              <span className="infra-last-updated">Last updated: {timeAgo(lastPolledAt)}</span>
            )}
            {renderScanFeedback(c.id)}
          </div>
          <div className="infra-card-actions">
            <button
              className="infra-card-btn accent"
              onClick={() => void handleScan(c.id)}
              disabled={isDeleting || isScanning || scanningId !== null}
            >
              {isScanning ? 'Discovering…' : 'Discover Now'}
            </button>
          </div>
        </div>
        <button
          className="infra-card-gear-btn"
          title="Settings"
          onClick={e => { e.stopPropagation(); openEdit(c) }}
          disabled={isDeleting || isScanning}
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
    const isScanning = scanningId === c.id

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

        <div className="infra-card-footer">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4, flex: 1 }}>
            {lastPolledAt && (
              <span className="infra-last-updated">Last updated: {timeAgo(lastPolledAt)}</span>
            )}
            {renderScanFeedback(c.id)}
          </div>
          <div className="infra-card-actions">
            <button
              className="infra-card-btn accent"
              onClick={() => void handleScan(c.id)}
              disabled={isDeleting || isScanning || scanningId !== null}
            >
              {isScanning ? 'Discovering…' : 'Discover Now'}
            </button>
          </div>
        </div>
        <button
          className="infra-card-gear-btn"
          title="Settings"
          onClick={e => { e.stopPropagation(); openEdit(c) }}
          disabled={isDeleting || isScanning}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="13" height="13">
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
          </svg>
        </button>
      </div>
    )
  }

  function renderScanFeedback(id: string) {
    const r = scanResults[id]
    if (!r) return null
    if (r.error) return <span className="infra-scan-feedback error">{r.error}</span>
    return <span className="infra-scan-feedback ok">Status: {r.status}</span>
  }

  function renderPortainerCard(c: InfrastructureComponent) {
    const isDeleting = deletingId === c.id
    const isScanning = scanningId === c.id

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

        <div className="infra-card-footer">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4, flex: 1 }}>
            {lastPolledAt && (
              <span className="infra-last-updated">Last updated: {timeAgo(lastPolledAt)}</span>
            )}
            {renderScanFeedback(c.id)}
          </div>
          <div className="infra-card-actions">
            <button
              className="infra-card-btn accent"
              onClick={() => void handleScan(c.id)}
              disabled={isDeleting || isScanning || scanningId !== null}
            >
              {isScanning ? 'Discovering…' : 'Discover Now'}
            </button>
          </div>
        </div>
        <button
          className="infra-card-gear-btn"
          title="Settings"
          onClick={e => { e.stopPropagation(); openEdit(c) }}
          disabled={isDeleting || isScanning}
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

    const res = resourcesMap[c.id]
    const isDeleting = deletingId === c.id
    const isScanning = scanningId === c.id
    const canScan = c.collection_method !== 'none'

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

        {renderResourceBars(c)}

        <div className="infra-card-footer">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4, flex: 1 }}>
            {lastPolledAt && (
              <span className="infra-last-updated">
                Last updated: {timeAgo(lastPolledAt)}
                {res?.recorded_at ? ` · data from ${new Date(res.recorded_at).toLocaleTimeString()}` : ''}
              </span>
            )}
            {renderScanFeedback(c.id)}
          </div>
          {canScan && (
            <div className="infra-card-actions">
              <button
                className="infra-card-btn accent"
                onClick={() => void handleScan(c.id)}
                disabled={isDeleting || isScanning || scanningId !== null}
              >
                {isScanning ? 'Discovering…' : 'Discover Now'}
              </button>
            </div>
          )}
        </div>
        <button
          className="infra-card-gear-btn"
          title="Settings"
          onClick={e => { e.stopPropagation(); openEdit(c) }}
          disabled={isDeleting || isScanning}
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

  function sourceLabel(s: string) {
    if (s === 'docker_engine') return 'Docker'
    if (s === 'portainer')     return 'Portainer'
    return s || '—'
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
            void pollAll([created])
          }
        }}
        onClose={closeModal}
        onDelete={editingComponent ? async () => {
          await infraApi.delete(editingComponent.id)
          setComponents(prev => prev.filter(c => c.id !== editingComponent.id))
          setResourcesMap(prev => { const n = { ...prev }; delete n[editingComponent.id]; return n })
        } : undefined}
      />
    </>
  )
}
