import { useState, useEffect, useCallback } from 'react'
import type { ReactNode } from 'react'
import { Topbar } from '../components/Topbar'
import { topology as topoApi } from '../api/client'
import { DockerEngineDetail } from '../components/DockerEngineDetail'
import type {
  PhysicalHost,
  PhysicalHostType,
  VirtualHost,
  VirtualHostType,
  DockerEngine,
  SocketType,
} from '../api/types'
import './Checks.css'
import './Topology.css'

// ── Form field types ──────────────────────────────────────────────────────────

type PhysicalForm = { name: string; ip: string; type: PhysicalHostType; notes: string }
type VirtualForm  = { name: string; ip: string; type: VirtualHostType }
type DockerForm   = { name: string; socket_type: SocketType; socket_path: string }

const defaultPhysical: PhysicalForm = { name: '', ip: '', type: 'bare_metal', notes: '' }
const defaultVirtual: VirtualForm   = { name: '', ip: '', type: 'vm' }
const defaultDocker: DockerForm     = { name: '', socket_type: 'local', socket_path: '/var/run/docker.sock' }

// ── Interaction state types ───────────────────────────────────────────────────

type AddTarget =
  | { kind: 'physical' }
  | { kind: 'virtual'; physicalId: string }
  | { kind: 'docker'; virtualId: string }

type EditTarget =
  | { kind: 'physical'; id: string }
  | { kind: 'virtual'; id: string }
  | { kind: 'docker'; id: string }

// ── Label helpers ─────────────────────────────────────────────────────────────

function labelPhysicalType(t: PhysicalHostType): string {
  return t === 'proxmox_node' ? 'Proxmox Node' : 'Bare Metal'
}

function labelVirtualType(t: VirtualHostType): string {
  return t.toUpperCase()
}

// ── Inline form wrapper ───────────────────────────────────────────────────────

interface FormWrapProps {
  title: string
  error: string | null
  submitting: boolean
  onSubmit: () => void
  onCancel: () => void
  children: ReactNode
  isEdit?: boolean
  onDelete?: () => void
  deleting?: boolean
}

function FormWrap({
  title, error, submitting, onSubmit, onCancel, children, isEdit, onDelete, deleting,
}: FormWrapProps) {
  return (
    <div className="add-form">
      <div className="form-title">{title}</div>
      <div className="form-fields">{children}</div>
      {error && <div className="form-error">{error}</div>}
      <div className="form-actions">
        <button className="form-btn primary" onClick={onSubmit} disabled={submitting}>
          {submitting ? 'Saving…' : isEdit ? 'Save' : 'Add'}
        </button>
        <button className="form-btn secondary" onClick={onCancel}>
          Cancel
        </button>
        {isEdit && onDelete && (
          <button
            className="form-btn danger"
            onClick={onDelete}
            disabled={deleting}
            style={{ marginLeft: 'auto' }}
          >
            {deleting ? 'Deleting…' : 'Delete'}
          </button>
        )}
      </div>
    </div>
  )
}

// ── Sub-form field groups ─────────────────────────────────────────────────────

function PhysicalFormFields({ form, onChange }: { form: PhysicalForm; onChange: (f: PhysicalForm) => void }) {
  return (
    <>
      <div className="form-field">
        <div className="form-label">Name</div>
        <input
          className="form-input"
          value={form.name}
          onChange={e => onChange({ ...form, name: e.target.value })}
          placeholder="e.g. proxmox-node1"
        />
      </div>
      <div className="form-field">
        <div className="form-label">IP Address</div>
        <input
          className="form-input"
          value={form.ip}
          onChange={e => onChange({ ...form, ip: e.target.value })}
          placeholder="e.g. 192.168.1.10"
        />
      </div>
      <div className="form-field">
        <div className="form-label">Type</div>
        <select
          className="form-input"
          value={form.type}
          onChange={e => onChange({ ...form, type: e.target.value as PhysicalHostType })}
        >
          <option value="bare_metal">Bare Metal</option>
          <option value="proxmox_node">Proxmox Node</option>
        </select>
      </div>
      <div className="form-field">
        <div className="form-label">Notes</div>
        <input
          className="form-input"
          value={form.notes}
          onChange={e => onChange({ ...form, notes: e.target.value })}
          placeholder="Optional"
        />
      </div>
    </>
  )
}

function VirtualFormFields({ form, onChange }: { form: VirtualForm; onChange: (f: VirtualForm) => void }) {
  return (
    <>
      <div className="form-field">
        <div className="form-label">Name</div>
        <input
          className="form-input"
          value={form.name}
          onChange={e => onChange({ ...form, name: e.target.value })}
          placeholder="e.g. rocky-vm01"
        />
      </div>
      <div className="form-field">
        <div className="form-label">IP Address</div>
        <input
          className="form-input"
          value={form.ip}
          onChange={e => onChange({ ...form, ip: e.target.value })}
          placeholder="e.g. 192.168.1.20"
        />
      </div>
      <div className="form-field">
        <div className="form-label">Type</div>
        <select
          className="form-input"
          value={form.type}
          onChange={e => onChange({ ...form, type: e.target.value as VirtualHostType })}
        >
          <option value="vm">VM</option>
          <option value="lxc">LXC</option>
          <option value="wsl">WSL</option>
        </select>
      </div>
    </>
  )
}

function DockerFormFields({ form, onChange }: { form: DockerForm; onChange: (f: DockerForm) => void }) {
  return (
    <>
      <div className="form-field">
        <div className="form-label">Name</div>
        <input
          className="form-input"
          value={form.name}
          onChange={e => onChange({ ...form, name: e.target.value })}
          placeholder="e.g. docker-engine-01"
        />
      </div>
      <div className="form-field">
        <div className="form-label">Socket Type</div>
        <select
          className="form-input"
          value={form.socket_type}
          onChange={e => onChange({ ...form, socket_type: e.target.value as SocketType })}
        >
          <option value="local">Local</option>
          <option value="remote_proxy">Remote Proxy</option>
        </select>
      </div>
      <div className="form-field" style={{ gridColumn: '1 / -1' }}>
        <div className="form-label">Socket Path</div>
        <input
          className="form-input"
          value={form.socket_path}
          onChange={e => onChange({ ...form, socket_path: e.target.value })}
          placeholder="/var/run/docker.sock"
        />
      </div>
    </>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function Topology() {
  const [physicalHosts, setPhysicalHosts] = useState<PhysicalHost[]>([])
  const [virtualHosts,  setVirtualHosts]  = useState<VirtualHost[]>([])
  const [dockerEngines, setDockerEngines] = useState<DockerEngine[]>([])
  const [loading, setLoading] = useState(true)

  const [expandedPhysical, setExpandedPhysical] = useState<Set<string>>(new Set())
  const [expandedVirtual,  setExpandedVirtual]  = useState<Set<string>>(new Set())
  const [expandedDocker,   setExpandedDocker]   = useState<Set<string>>(new Set())

  // container count badge: engineId → { total, unlinked }
  const [containerCounts, setContainerCounts] = useState<Record<string, { total: number; unlinked: number }>>({})

  const handleCountsLoaded = useCallback((engineId: string, total: number, unlinked: number) => {
    setContainerCounts(prev => ({ ...prev, [engineId]: { total, unlinked } }))
  }, [])

  const [addTarget,  setAddTarget]  = useState<AddTarget | null>(null)
  const [editTarget, setEditTarget] = useState<EditTarget | null>(null)

  const [physicalForm, setPhysicalForm] = useState<PhysicalForm>(defaultPhysical)
  const [virtualForm,  setVirtualForm]  = useState<VirtualForm>(defaultVirtual)
  const [dockerForm,   setDockerForm]   = useState<DockerForm>(defaultDocker)

  const [formError,   setFormError]   = useState<string | null>(null)
  const [submitting,  setSubmitting]  = useState(false)
  const [deletingIds, setDeletingIds] = useState<Set<string>>(new Set())

  // ── Load ──────────────────────────────────────────────────────────────────

  useEffect(() => {
    Promise.all([
      topoApi.physicalHosts.list(),
      topoApi.virtualHosts.list(),
      topoApi.dockerEngines.list(),
    ])
      .then(([ph, vh, de]) => {
        setPhysicalHosts(ph.data)
        setVirtualHosts(vh.data)
        setDockerEngines(de.data)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  // ── Tree helpers ──────────────────────────────────────────────────────────

  function virtualsFor(physicalId: string): VirtualHost[] {
    return virtualHosts.filter(v => v.physical_host_id === physicalId)
  }

  function enginesFor(virtualId: string): DockerEngine[] {
    return dockerEngines.filter(e => e.virtual_host_id === virtualId)
  }

  // ── Expand / collapse ─────────────────────────────────────────────────────

  function togglePhysical(id: string) {
    setExpandedPhysical(prev => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
    setAddTarget(null)
    setEditTarget(null)
    setFormError(null)
  }

  function toggleVirtual(id: string) {
    setExpandedVirtual(prev => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
    setAddTarget(null)
    setEditTarget(null)
    setFormError(null)
  }

  function toggleDocker(id: string) {
    setExpandedDocker(prev => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  // ── Start edit ────────────────────────────────────────────────────────────

  function startEditPhysical(ev: React.MouseEvent, host: PhysicalHost) {
    ev.stopPropagation()
    setEditTarget({ kind: 'physical', id: host.id })
    setPhysicalForm({ name: host.name, ip: host.ip, type: host.type, notes: host.notes ?? '' })
    setFormError(null)
    setAddTarget(null)
  }

  function startEditVirtual(ev: React.MouseEvent, host: VirtualHost) {
    ev.stopPropagation()
    setEditTarget({ kind: 'virtual', id: host.id })
    setVirtualForm({ name: host.name, ip: host.ip, type: host.type })
    setFormError(null)
    setAddTarget(null)
  }

  function startEditDocker(ev: React.MouseEvent, engine: DockerEngine) {
    ev.stopPropagation()
    setEditTarget({ kind: 'docker', id: engine.id })
    setDockerForm({ name: engine.name, socket_type: engine.socket_type, socket_path: engine.socket_path })
    setFormError(null)
    setAddTarget(null)
  }

  function cancelForm() {
    setAddTarget(null)
    setEditTarget(null)
    setFormError(null)
  }

  // ── Start add ─────────────────────────────────────────────────────────────

  function startAddPhysical() {
    setAddTarget({ kind: 'physical' })
    setPhysicalForm(defaultPhysical)
    setFormError(null)
    setEditTarget(null)
  }

  function startAddVirtual(ev: React.MouseEvent, physicalId: string) {
    ev.stopPropagation()
    setAddTarget({ kind: 'virtual', physicalId })
    setVirtualForm(defaultVirtual)
    setFormError(null)
    setEditTarget(null)
  }

  function startAddDocker(ev: React.MouseEvent, virtualId: string) {
    ev.stopPropagation()
    setAddTarget({ kind: 'docker', virtualId })
    setDockerForm(defaultDocker)
    setFormError(null)
    setEditTarget(null)
  }

  // ── Submit add ────────────────────────────────────────────────────────────

  async function handleAddPhysical() {
    if (!physicalForm.name.trim()) { setFormError('Name is required'); return }
    if (!physicalForm.ip.trim())   { setFormError('IP is required');   return }
    setSubmitting(true)
    try {
      const created = await topoApi.physicalHosts.create({
        name:  physicalForm.name.trim(),
        ip:    physicalForm.ip.trim(),
        type:  physicalForm.type,
        notes: physicalForm.notes.trim() || null,
      })
      setPhysicalHosts(prev => [...prev, created])
      cancelForm()
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : 'Failed to add host')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleAddVirtual(physicalId: string) {
    if (!virtualForm.name.trim()) { setFormError('Name is required'); return }
    if (!virtualForm.ip.trim())   { setFormError('IP is required');   return }
    setSubmitting(true)
    try {
      const created = await topoApi.virtualHosts.create({
        name:             virtualForm.name.trim(),
        ip:               virtualForm.ip.trim(),
        type:             virtualForm.type,
        physical_host_id: physicalId,
      })
      setVirtualHosts(prev => [...prev, created])
      cancelForm()
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : 'Failed to add VM')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleAddDocker(virtualId: string) {
    if (!dockerForm.name.trim())        { setFormError('Name is required');        return }
    if (!dockerForm.socket_path.trim()) { setFormError('Socket path is required'); return }
    setSubmitting(true)
    try {
      const created = await topoApi.dockerEngines.create({
        name:            dockerForm.name.trim(),
        socket_type:     dockerForm.socket_type,
        socket_path:     dockerForm.socket_path.trim(),
        virtual_host_id: virtualId,
      })
      setDockerEngines(prev => [...prev, created])
      cancelForm()
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : 'Failed to add Docker engine')
    } finally {
      setSubmitting(false)
    }
  }

  // ── Submit edit ───────────────────────────────────────────────────────────

  async function handleEditPhysical(id: string) {
    if (!physicalForm.name.trim()) { setFormError('Name is required'); return }
    if (!physicalForm.ip.trim())   { setFormError('IP is required');   return }
    setSubmitting(true)
    try {
      const updated = await topoApi.physicalHosts.update(id, {
        name:  physicalForm.name.trim(),
        ip:    physicalForm.ip.trim(),
        type:  physicalForm.type,
        notes: physicalForm.notes.trim() || null,
      })
      setPhysicalHosts(prev => prev.map(h => h.id === id ? updated : h))
      setEditTarget(null)
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleEditVirtual(id: string) {
    if (!virtualForm.name.trim()) { setFormError('Name is required'); return }
    if (!virtualForm.ip.trim())   { setFormError('IP is required');   return }
    setSubmitting(true)
    try {
      const updated = await topoApi.virtualHosts.update(id, {
        name: virtualForm.name.trim(),
        ip:   virtualForm.ip.trim(),
        type: virtualForm.type,
      })
      setVirtualHosts(prev => prev.map(h => h.id === id ? updated : h))
      setEditTarget(null)
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleEditDocker(id: string) {
    if (!dockerForm.name.trim())        { setFormError('Name is required');        return }
    if (!dockerForm.socket_path.trim()) { setFormError('Socket path is required'); return }
    setSubmitting(true)
    try {
      const updated = await topoApi.dockerEngines.update(id, {
        name:        dockerForm.name.trim(),
        socket_type: dockerForm.socket_type,
        socket_path: dockerForm.socket_path.trim(),
      })
      setDockerEngines(prev => prev.map(e => e.id === id ? updated : e))
      setEditTarget(null)
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSubmitting(false)
    }
  }

  // ── Delete ────────────────────────────────────────────────────────────────

  async function deletePhysical(ev: React.MouseEvent | null, id: string) {
    ev?.stopPropagation()
    setDeletingIds(prev => new Set(prev).add(id))
    try {
      await topoApi.physicalHosts.delete(id)
      setPhysicalHosts(prev => prev.filter(h => h.id !== id))
      setVirtualHosts(prev => prev.filter(v => v.physical_host_id !== id))
      setEditTarget(null)
      setExpandedPhysical(prev => { const n = new Set(prev); n.delete(id); return n })
    } catch { /* keep in list */ } finally {
      setDeletingIds(prev => { const n = new Set(prev); n.delete(id); return n })
    }
  }

  async function deleteVirtual(ev: React.MouseEvent | null, id: string) {
    ev?.stopPropagation()
    setDeletingIds(prev => new Set(prev).add(id))
    try {
      await topoApi.virtualHosts.delete(id)
      setVirtualHosts(prev => prev.filter(h => h.id !== id))
      setDockerEngines(prev => prev.filter(de => de.virtual_host_id !== id))
      setEditTarget(null)
      setExpandedVirtual(prev => { const n = new Set(prev); n.delete(id); return n })
    } catch { /* keep */ } finally {
      setDeletingIds(prev => { const n = new Set(prev); n.delete(id); return n })
    }
  }

  async function deleteDocker(ev: React.MouseEvent | null, id: string) {
    ev?.stopPropagation()
    setDeletingIds(prev => new Set(prev).add(id))
    try {
      await topoApi.dockerEngines.delete(id)
      setDockerEngines(prev => prev.filter(de => de.id !== id))
      setEditTarget(null)
    } catch { /* keep */ } finally {
      setDeletingIds(prev => { const n = new Set(prev); n.delete(id); return n })
    }
  }

  // ── Render ────────────────────────────────────────────────────────────────

  return (
    <>
      <Topbar title="Infrastructure" onAdd={startAddPhysical} />
      <div className="content">

        {addTarget?.kind === 'physical' && (
          <FormWrap
            title="Add Physical Host"
            error={formError}
            submitting={submitting}
            onSubmit={() => void handleAddPhysical()}
            onCancel={cancelForm}
          >
            <PhysicalFormFields form={physicalForm} onChange={setPhysicalForm} />
          </FormWrap>
        )}

        <div className="section-header">
          <span className="section-title">Physical Hosts</span>
          <button className="section-action" onClick={startAddPhysical}>+ Add host</button>
        </div>

        {loading ? (
          <div className="topology-empty">Loading…</div>
        ) : physicalHosts.length === 0 ? (
          <div className="topology-empty">
            No infrastructure configured yet. Add a physical host to get started.
          </div>
        ) : (
          <div className="topo-list">
            {physicalHosts.map(ph => {
              const expanded  = expandedPhysical.has(ph.id)
              const isEditing = editTarget?.kind === 'physical' && editTarget.id === ph.id
              const vhosts    = virtualsFor(ph.id)

              return (
                <div key={ph.id} className="topo-node">

                  {/* ── Physical host row ── */}
                  <div
                    className={`topo-row${isEditing ? ' editing' : ''}`}
                    onClick={() => !isEditing && togglePhysical(ph.id)}
                  >
                    <span className={`topo-chevron${expanded ? ' open' : ''}`}>
                      {expanded ? '▼' : '▶'}
                    </span>
                    <div className="topo-icon">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <rect x="2" y="2" width="20" height="8" rx="2" />
                        <rect x="2" y="14" width="20" height="8" rx="2" />
                        <line x1="6" y1="6" x2="6.01" y2="6" />
                        <line x1="6" y1="18" x2="6.01" y2="18" />
                      </svg>
                    </div>
                    <div className="topo-info">
                      <div className="topo-name">{ph.name}</div>
                      <div className="topo-meta">{labelPhysicalType(ph.type)} · {ph.ip}</div>
                    </div>
                    <div className="topo-actions">
                      <button
                        className="topo-action-btn"
                        title="Edit"
                        onClick={ev => startEditPhysical(ev, ph)}
                      >✎</button>
                      <button
                        className="topo-action-btn danger"
                        title="Delete"
                        disabled={deletingIds.has(ph.id)}
                        onClick={ev => void deletePhysical(ev, ph.id)}
                      >✕</button>
                    </div>
                  </div>

                  {/* ── Edit form ── */}
                  {isEditing && (
                    <div className="topo-edit-panel">
                      <FormWrap
                        title="Edit Physical Host"
                        error={formError}
                        submitting={submitting}
                        onSubmit={() => void handleEditPhysical(ph.id)}
                        onCancel={cancelForm}
                        isEdit
                        onDelete={() => void deletePhysical(null, ph.id)}
                        deleting={deletingIds.has(ph.id)}
                      >
                        <PhysicalFormFields form={physicalForm} onChange={setPhysicalForm} />
                      </FormWrap>
                    </div>
                  )}

                  {/* ── Expanded: virtual hosts ── */}
                  {expanded && (
                    <div className="topo-children">
                      {vhosts.length === 0 && addTarget?.kind !== 'virtual' && (
                        <div className="topo-empty-children">No VMs configured under this host.</div>
                      )}

                      {vhosts.map(vh => {
                        const vExpanded = expandedVirtual.has(vh.id)
                        const vEditing  = editTarget?.kind === 'virtual' && editTarget.id === vh.id
                        const engines   = enginesFor(vh.id)

                        return (
                          <div key={vh.id} className="topo-node">

                            {/* ── VM row ── */}
                            <div
                              className={`topo-row level-1${vEditing ? ' editing' : ''}`}
                              onClick={() => !vEditing && toggleVirtual(vh.id)}
                            >
                              <span className={`topo-chevron${vExpanded ? ' open' : ''}`}>
                                {vExpanded ? '▼' : '▶'}
                              </span>
                              <div className="topo-icon">
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                  <rect x="3" y="3" width="18" height="18" rx="2" />
                                  <path d="M3 9h18M9 21V9" />
                                </svg>
                              </div>
                              <div className="topo-info">
                                <div className="topo-name">{vh.name}</div>
                                <div className="topo-meta">{labelVirtualType(vh.type)} · {vh.ip}</div>
                              </div>
                              <div className="topo-actions">
                                <button
                                  className="topo-action-btn"
                                  title="Edit"
                                  onClick={ev => startEditVirtual(ev, vh)}
                                >✎</button>
                                <button
                                  className="topo-action-btn danger"
                                  title="Delete"
                                  disabled={deletingIds.has(vh.id)}
                                  onClick={ev => void deleteVirtual(ev, vh.id)}
                                >✕</button>
                              </div>
                            </div>

                            {vEditing && (
                              <div className="topo-edit-panel">
                                <FormWrap
                                  title="Edit VM"
                                  error={formError}
                                  submitting={submitting}
                                  onSubmit={() => void handleEditVirtual(vh.id)}
                                  onCancel={cancelForm}
                                  isEdit
                                  onDelete={() => void deleteVirtual(null, vh.id)}
                                  deleting={deletingIds.has(vh.id)}
                                >
                                  <VirtualFormFields form={virtualForm} onChange={setVirtualForm} />
                                </FormWrap>
                              </div>
                            )}

                            {/* ── Expanded: docker engines ── */}
                            {vExpanded && (
                              <div className="topo-children">
                                {engines.length === 0 && addTarget?.kind !== 'docker' && (
                                  <div className="topo-empty-children">No Docker engines configured.</div>
                                )}

                                {engines.map(de => {
                                  const deEditing  = editTarget?.kind === 'docker' && editTarget.id === de.id
                                  const deExpanded = expandedDocker.has(de.id) && !deEditing
                                  const counts     = containerCounts[de.id]

                                  return (
                                    <div key={de.id} className="topo-node">
                                      <div
                                        className={`topo-row level-2${deEditing ? ' editing' : ''}${deExpanded ? ' expanded' : ''}`}
                                        onClick={ev => {
                                          ev.stopPropagation()
                                          if (!deEditing) toggleDocker(de.id)
                                        }}
                                      >
                                        <span className={`topo-chevron${deExpanded ? ' open' : ''}`}>
                                          {deExpanded ? '▼' : '▶'}
                                        </span>
                                        <div className="topo-icon">
                                          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                            <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
                                            <path d="M12 22V12M3.27 6.96 12 12.01l8.73-5.05M12 2.1V12" />
                                          </svg>
                                        </div>
                                        <div className="topo-info">
                                          <div className="topo-name">{de.name}</div>
                                          <div className="topo-meta">{de.socket_type} · {de.socket_path}</div>
                                        </div>
                                        {counts && (
                                          <span className="de-count-badge">
                                            {counts.total} container{counts.total !== 1 ? 's' : ''} · {counts.unlinked} unlinked
                                          </span>
                                        )}
                                        <div className="topo-actions">
                                          <button
                                            className="topo-action-btn"
                                            title="Edit"
                                            onClick={ev => startEditDocker(ev, de)}
                                          >✎</button>
                                          <button
                                            className="topo-action-btn danger"
                                            title="Delete"
                                            disabled={deletingIds.has(de.id)}
                                            onClick={ev => void deleteDocker(ev, de.id)}
                                          >✕</button>
                                        </div>
                                      </div>

                                      {deEditing && (
                                        <div className="topo-edit-panel">
                                          <FormWrap
                                            title="Edit Docker Engine"
                                            error={formError}
                                            submitting={submitting}
                                            onSubmit={() => void handleEditDocker(de.id)}
                                            onCancel={cancelForm}
                                            isEdit
                                            onDelete={() => void deleteDocker(null, de.id)}
                                            deleting={deletingIds.has(de.id)}
                                          >
                                            <DockerFormFields form={dockerForm} onChange={setDockerForm} />
                                          </FormWrap>
                                        </div>
                                      )}

                                      {deExpanded && (
                                        <DockerEngineDetail
                                          engineId={de.id}
                                          onCountsLoaded={(total, unlinked) =>
                                            handleCountsLoaded(de.id, total, unlinked)
                                          }
                                        />
                                      )}
                                    </div>
                                  )
                                })}

                                {addTarget?.kind === 'docker' && addTarget.virtualId === vh.id && (
                                  <FormWrap
                                    title="Add Docker Engine"
                                    error={formError}
                                    submitting={submitting}
                                    onSubmit={() => void handleAddDocker(vh.id)}
                                    onCancel={cancelForm}
                                  >
                                    <DockerFormFields form={dockerForm} onChange={setDockerForm} />
                                  </FormWrap>
                                )}

                                <div className="topo-add-row">
                                  <button className="topo-add-btn" onClick={ev => startAddDocker(ev, vh.id)}>
                                    + Add Docker Engine
                                  </button>
                                </div>
                              </div>
                            )}

                          </div>
                        )
                      })}

                      {addTarget?.kind === 'virtual' && addTarget.physicalId === ph.id && (
                        <FormWrap
                          title="Add VM"
                          error={formError}
                          submitting={submitting}
                          onSubmit={() => void handleAddVirtual(ph.id)}
                          onCancel={cancelForm}
                        >
                          <VirtualFormFields form={virtualForm} onChange={setVirtualForm} />
                        </FormWrap>
                      )}

                      <div className="topo-add-row">
                        <button className="topo-add-btn" onClick={ev => startAddVirtual(ev, ph.id)}>
                          + Add VM
                        </button>
                      </div>
                    </div>
                  )}

                </div>
              )
            })}
          </div>
        )}

      </div>
    </>
  )
}
