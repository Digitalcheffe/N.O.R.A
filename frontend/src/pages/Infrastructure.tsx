import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { infrastructure as infraApi } from '../api/client'
import type {
  InfrastructureComponent,
  ResourceSummary,
  DiscoverResult,
  VolumeResource,
} from '../api/types'
import './Infrastructure.css'
import '../components/CheckForm.css'

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

export function Infrastructure() {
  const navigate = useNavigate()
  const { tick: refreshTick } = useAutoRefresh()
  const [components,      setComponents]      = useState<InfrastructureComponent[]>([])
  const [resourcesMap,    setResourcesMap]    = useState<Record<string, ResourceSummary>>({})
  const [lastPolledAt,    setLastPolledAt]    = useState<Date | null>(null)
  const [loading,         setLoading]         = useState(true)
  const [tick,            setTick]            = useState(0)

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
      // Refresh the component list so last_status updates immediately.
      const res = await infraApi.list()
      setComponents(res.data)
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

  return (
    <>
      <Topbar title="Infrastructure" />
      <div className="content">

        {/* ── Header row ── */}
        <div className="infra-tab-row">
          <button className="infra-add-btn" onClick={() => openAdd()}>
            + Add Component
          </button>
        </div>

        {loading ? (
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
