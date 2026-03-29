import { useState, useEffect, useCallback } from 'react'
import { Topbar } from '../components/Topbar'
import { infrastructure as infraApi } from '../api/client'
import type {
  ComponentType,
  CollectionMethod,
  InfrastructureComponent,
  InfrastructureComponentInput,
  ResourceSummary,
  VolumeResource,
} from '../api/types'
import './Infrastructure.css'
import '../components/CheckForm.css'

// ── Constants ─────────────────────────────────────────────────────────────────

type ActiveTab = 'components' | 'map'

const COLLECTION_METHOD: Record<ComponentType, CollectionMethod> = {
  proxmox_node:  'proxmox_api',
  synology:      'synology_api',
  vm:            'snmp',
  lxc:           'none',
  bare_metal:    'snmp',
  windows_host:  'snmp',
  docker_engine: 'docker_socket',
}

const TYPE_LABEL: Record<ComponentType, string> = {
  proxmox_node:  'Proxmox Node',
  synology:      'Synology NAS',
  vm:            'VM',
  lxc:           'LXC',
  bare_metal:    'Bare Metal',
  windows_host:  'Windows Host',
  docker_engine: 'Docker Engine',
}

const CAN_HAVE_CHILDREN = new Set<ComponentType>(['proxmox_node', 'bare_metal', 'vm'])

const SNMP_TYPES = new Set<ComponentType>(['vm', 'bare_metal', 'windows_host'])

// ── Form state ────────────────────────────────────────────────────────────────

interface InfraForm {
  // Basic
  name: string
  ip: string
  type: ComponentType
  parent_id: string
  notes: string
  enabled: boolean
  // Proxmox credentials
  proxmox_base_url: string
  proxmox_token_id: string
  proxmox_token_secret: string
  proxmox_verify_tls: boolean
  // Synology credentials
  synology_base_url: string
  synology_username: string
  synology_password: string
  synology_verify_tls: boolean
  // SNMP config
  snmp_version: '2c' | '3'
  snmp_community: string
  snmp_port: string
  snmp_auth_protocol: string
  snmp_auth_passphrase: string
  snmp_priv_protocol: string
  snmp_priv_passphrase: string
  // Docker config
  docker_socket_type: 'local' | 'remote_proxy'
  docker_socket_path: string
}

const DEFAULT_FORM: InfraForm = {
  name: '',
  ip: '',
  type: 'proxmox_node',
  parent_id: '',
  notes: '',
  enabled: true,
  proxmox_base_url: '',
  proxmox_token_id: '',
  proxmox_token_secret: '',
  proxmox_verify_tls: false,
  synology_base_url: '',
  synology_username: '',
  synology_password: '',
  synology_verify_tls: false,
  snmp_version: '2c',
  snmp_community: 'public',
  snmp_port: '161',
  snmp_auth_protocol: 'SHA',
  snmp_auth_passphrase: '',
  snmp_priv_protocol: 'AES',
  snmp_priv_passphrase: '',
  docker_socket_type: 'local',
  docker_socket_path: '/var/run/docker.sock',
}

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

function formToPayload(form: InfraForm, isEdit: boolean): InfrastructureComponentInput {
  const payload: InfrastructureComponentInput = {
    name:              form.name.trim(),
    ip:                form.ip.trim(),
    type:              form.type,
    collection_method: COLLECTION_METHOD[form.type],
    parent_id:         form.parent_id || null,
    notes:             form.notes.trim(),
    enabled:           form.enabled,
  }

  if (form.type === 'proxmox_node') {
    const hasNewCreds = form.proxmox_token_id || form.proxmox_token_secret || form.proxmox_base_url
    if (!isEdit || hasNewCreds) {
      payload.credentials = JSON.stringify({
        base_url:     form.proxmox_base_url,
        token_id:     form.proxmox_token_id,
        token_secret: form.proxmox_token_secret,
        verify_tls:   form.proxmox_verify_tls,
      })
    }
  } else if (form.type === 'synology') {
    const hasNewCreds = form.synology_username || form.synology_password || form.synology_base_url
    if (!isEdit || hasNewCreds) {
      payload.credentials = JSON.stringify({
        base_url:   form.synology_base_url,
        username:   form.synology_username,
        password:   form.synology_password,
        verify_tls: form.synology_verify_tls,
      })
    }
  } else if (form.type === 'docker_engine') {
    payload.credentials = JSON.stringify({
      socket_type: form.docker_socket_type,
      socket_path: form.docker_socket_path,
    })
  }

  if (SNMP_TYPES.has(form.type)) {
    payload.snmp_config = JSON.stringify({
      version:         form.snmp_version,
      community:       form.snmp_community,
      port:            parseInt(form.snmp_port, 10) || 161,
      auth_protocol:   form.snmp_auth_protocol,
      auth_passphrase: form.snmp_auth_passphrase,
      priv_protocol:   form.snmp_priv_protocol,
      priv_passphrase: form.snmp_priv_passphrase,
    })
  }

  return payload
}

function componentToForm(c: InfrastructureComponent): InfraForm {
  const form: InfraForm = {
    ...DEFAULT_FORM,
    name:      c.name,
    ip:        c.ip,
    type:      c.type,
    parent_id: c.parent_id ?? '',
    notes:     c.notes,
    enabled:   c.enabled,
  }
  if (c.snmp_config) {
    try {
      const s = JSON.parse(c.snmp_config) as Record<string, unknown>
      form.snmp_version       = (s.version as '2c' | '3') ?? '2c'
      form.snmp_community     = (s.community as string) ?? 'public'
      form.snmp_port          = String(s.port ?? 161)
      form.snmp_auth_protocol  = (s.auth_protocol as string) ?? 'SHA'
      form.snmp_auth_passphrase = (s.auth_passphrase as string) ?? ''
      form.snmp_priv_protocol  = (s.priv_protocol as string) ?? 'AES'
      form.snmp_priv_passphrase = (s.priv_passphrase as string) ?? ''
    } catch { /* keep defaults */ }
  }
  return form
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

// ── Form section heading ──────────────────────────────────────────────────────

function SectionHeading({ children }: { children: React.ReactNode }) {
  return <div className="infra-form-section">{children}</div>
}

// ── Toggle switch ─────────────────────────────────────────────────────────────

function Toggle({
  checked, onChange, label,
}: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <label className="infra-toggle-row">
      <span className="infra-toggle-label">{label}</span>
      <span
        className={`infra-toggle${checked ? ' on' : ''}`}
        onClick={() => onChange(!checked)}
      >
        <span className="infra-toggle-thumb" />
      </span>
    </label>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function Infrastructure() {
  const [components,   setComponents]   = useState<InfrastructureComponent[]>([])
  const [resourcesMap, setResourcesMap] = useState<Record<string, ResourceSummary>>({})
  const [lastPolledAt, setLastPolledAt] = useState<Date | null>(null)
  const [loading,      setLoading]      = useState(true)
  const [activeTab,    setActiveTab]    = useState<ActiveTab>('components')
  const [tick,         setTick]         = useState(0)

  // Modal state
  const [modalOpen,  setModalOpen]  = useState(false)
  const [editingId,  setEditingId]  = useState<string | null>(null)
  const [form,       setForm]       = useState<InfraForm>(DEFAULT_FORM)
  const [formError,  setFormError]  = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)

  // ── Polling ─────────────────────────────────────────────────────────────────

  const pollAll = useCallback(async (compList: InfrastructureComponent[]) => {
    if (compList.length === 0) return
    const results = await Promise.allSettled(
      compList.map(c =>
        infraApi.resources(c.id, 'hour').then(r => ({ id: c.id, data: r }))
      )
    )
    const map: Record<string, ResourceSummary> = {}
    for (const r of results) {
      if (r.status === 'fulfilled') {
        map[r.value.id] = r.value.data
      }
    }
    setResourcesMap(prev => ({ ...prev, ...map }))
    setLastPolledAt(new Date())
  }, [])

  // Initial load
  useEffect(() => {
    infraApi.list()
      .then(res => {
        setComponents(res.data)
        return pollAll(res.data)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [pollAll])

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
    setEditingId(null)
    const f = { ...DEFAULT_FORM }
    if (parentId) f.parent_id = parentId
    setForm(f)
    setFormError(null)
    setModalOpen(true)
  }

  function openEdit(c: InfrastructureComponent) {
    setEditingId(c.id)
    setForm(componentToForm(c))
    setFormError(null)
    setModalOpen(true)
  }

  function closeModal() {
    setModalOpen(false)
    setEditingId(null)
    setFormError(null)
  }

  function setField<K extends keyof InfraForm>(key: K, value: InfraForm[K]) {
    setForm(prev => ({ ...prev, [key]: value }))
  }

  // ── Submit ───────────────────────────────────────────────────────────────────

  async function handleSubmit() {
    if (!form.name.trim()) { setFormError('Name is required'); return }
    setSubmitting(true)
    setFormError(null)
    try {
      const payload = formToPayload(form, editingId !== null)
      if (editingId) {
        const updated = await infraApi.update(editingId, payload)
        setComponents(prev => prev.map(c => c.id === editingId ? updated : c))
      } else {
        const created = await infraApi.create(payload)
        setComponents(prev => [...prev, created])
        void pollAll([created])
      }
      closeModal()
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleDelete(id: string) {
    setDeletingId(id)
    try {
      await infraApi.delete(id)
      setComponents(prev => prev.filter(c => c.id !== id))
      setResourcesMap(prev => { const n = { ...prev }; delete n[id]; return n })
      if (editingId === id) closeModal()
    } catch { /* keep in list */ } finally {
      setDeletingId(null)
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

  function renderCard(c: InfrastructureComponent) {
    const res = resourcesMap[c.id]
    const isDeleting = deletingId === c.id

    return (
      <div key={c.id} className="infra-card">
        <div className="infra-card-header">
          <div className="infra-card-title-group">
            <div className="infra-card-name">{c.name}</div>
            <div className="infra-card-meta">
              {TYPE_LABEL[c.type]} · {c.ip}
            </div>
          </div>
          <div className="infra-card-status-group">
            <span className={`infra-status-dot ${statusClass(c.last_status)}`} />
            <span className="infra-status-label">{statusLabel(c.last_status)}</span>
          </div>
        </div>

        {renderResourceBars(c)}

        <div className="infra-card-footer">
          {lastPolledAt && (
            <span className="infra-last-updated">
              Last updated: {timeAgo(lastPolledAt)}
              {res?.recorded_at ? ` · data from ${new Date(res.recorded_at).toLocaleTimeString()}` : ''}
            </span>
          )}
          <div className="infra-card-actions">
            <button
              className="infra-card-btn"
              onClick={() => openEdit(c)}
              disabled={isDeleting}
            >
              Edit
            </button>
            <button
              className="infra-card-btn danger"
              onClick={() => void handleDelete(c.id)}
              disabled={isDeleting}
            >
              {isDeleting ? 'Deleting…' : 'Delete'}
            </button>
            {CAN_HAVE_CHILDREN.has(c.type) && (
              <button
                className="infra-card-btn accent"
                onClick={() => openAdd(c.id)}
                disabled={isDeleting}
              >
                + Add Child
              </button>
            )}
          </div>
        </div>
      </div>
    )
  }

  // ── Form fields ──────────────────────────────────────────────────────────────

  function renderFormFields() {
    return (
      <>
        {/* ── Basic fields ── */}
        <div className="form-fields" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
          <div className="form-field form-field-full">
            <div className="form-label">Name</div>
            <input
              className="form-input"
              value={form.name}
              onChange={e => setField('name', e.target.value)}
              placeholder="e.g. proxmox-node1"
            />
          </div>

          <div className="form-field">
            <div className="form-label">Type</div>
            <select
              className="form-input"
              value={form.type}
              onChange={e => setField('type', e.target.value as ComponentType)}
            >
              <option value="proxmox_node">Proxmox Node</option>
              <option value="synology">Synology NAS</option>
              <option value="vm">VM</option>
              <option value="lxc">LXC</option>
              <option value="bare_metal">Bare Metal</option>
              <option value="windows_host">Windows Host</option>
              <option value="docker_engine">Docker Engine</option>
            </select>
          </div>

          <div className="form-field">
            <div className="form-label">IP Address</div>
            <input
              className="form-input"
              value={form.ip}
              onChange={e => setField('ip', e.target.value)}
              placeholder="e.g. 192.168.1.10"
            />
          </div>

          <div className="form-field form-field-full">
            <div className="form-label">Parent Component <span className="infra-optional">(optional)</span></div>
            <select
              className="form-input"
              value={form.parent_id}
              onChange={e => setField('parent_id', e.target.value)}
            >
              <option value="">None</option>
              {components
                .filter(c => c.id !== editingId)
                .map(c => (
                  <option key={c.id} value={c.id}>
                    {c.name} ({TYPE_LABEL[c.type]})
                  </option>
                ))}
            </select>
          </div>

          <div className="form-field form-field-full">
            <div className="form-label">Notes <span className="infra-optional">(optional)</span></div>
            <textarea
              className="form-input infra-textarea"
              value={form.notes}
              onChange={e => setField('notes', e.target.value)}
              placeholder="Optional notes"
              rows={2}
            />
          </div>
        </div>

        <Toggle checked={form.enabled} onChange={v => setField('enabled', v)} label="Enabled" />

        {/* ── Proxmox credentials ── */}
        {form.type === 'proxmox_node' && (
          <>
            <SectionHeading>Proxmox Credentials {editingId && <span className="infra-optional">(leave blank to keep existing)</span>}</SectionHeading>
            <div className="form-fields" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
              <div className="form-field form-field-full">
                <div className="form-label">Base URL</div>
                <input className="form-input" value={form.proxmox_base_url}
                  onChange={e => setField('proxmox_base_url', e.target.value)}
                  placeholder="https://proxmox.local:8006" />
              </div>
              <div className="form-field">
                <div className="form-label">Token ID</div>
                <input className="form-input" value={form.proxmox_token_id}
                  onChange={e => setField('proxmox_token_id', e.target.value)}
                  placeholder="user@pam!token-name" />
              </div>
              <div className="form-field">
                <div className="form-label">Token Secret</div>
                <input className="form-input" type="password" value={form.proxmox_token_secret}
                  onChange={e => setField('proxmox_token_secret', e.target.value)}
                  placeholder="••••••••" />
              </div>
            </div>
            <Toggle checked={form.proxmox_verify_tls} onChange={v => setField('proxmox_verify_tls', v)} label="Verify TLS" />
          </>
        )}

        {/* ── Synology credentials ── */}
        {form.type === 'synology' && (
          <>
            <SectionHeading>Synology Credentials {editingId && <span className="infra-optional">(leave blank to keep existing)</span>}</SectionHeading>
            <div className="form-fields" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
              <div className="form-field form-field-full">
                <div className="form-label">Base URL</div>
                <input className="form-input" value={form.synology_base_url}
                  onChange={e => setField('synology_base_url', e.target.value)}
                  placeholder="https://synology.local:5001" />
              </div>
              <div className="form-field">
                <div className="form-label">Username</div>
                <input className="form-input" value={form.synology_username}
                  onChange={e => setField('synology_username', e.target.value)}
                  placeholder="admin" />
              </div>
              <div className="form-field">
                <div className="form-label">Password</div>
                <input className="form-input" type="password" value={form.synology_password}
                  onChange={e => setField('synology_password', e.target.value)}
                  placeholder="••••••••" />
              </div>
            </div>
            <Toggle checked={form.synology_verify_tls} onChange={v => setField('synology_verify_tls', v)} label="Verify TLS" />
          </>
        )}

        {/* ── SNMP config ── */}
        {SNMP_TYPES.has(form.type) && (
          <>
            <SectionHeading>SNMP Configuration</SectionHeading>
            <div className="form-fields" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
              <div className="form-field">
                <div className="form-label">SNMP Version</div>
                <select className="form-input" value={form.snmp_version}
                  onChange={e => setField('snmp_version', e.target.value as '2c' | '3')}>
                  <option value="2c">v2c</option>
                  <option value="3">v3</option>
                </select>
              </div>
              <div className="form-field">
                <div className="form-label">Port</div>
                <input className="form-input" value={form.snmp_port}
                  onChange={e => setField('snmp_port', e.target.value)}
                  placeholder="161" />
              </div>
              {form.snmp_version === '2c' && (
                <div className="form-field">
                  <div className="form-label">Community String</div>
                  <input className="form-input" value={form.snmp_community}
                    onChange={e => setField('snmp_community', e.target.value)}
                    placeholder="public" />
                </div>
              )}
              {form.snmp_version === '3' && (
                <>
                  <div className="form-field">
                    <div className="form-label">Auth Protocol</div>
                    <select className="form-input" value={form.snmp_auth_protocol}
                      onChange={e => setField('snmp_auth_protocol', e.target.value)}>
                      <option value="MD5">MD5</option>
                      <option value="SHA">SHA</option>
                    </select>
                  </div>
                  <div className="form-field">
                    <div className="form-label">Auth Passphrase</div>
                    <input className="form-input" type="password" value={form.snmp_auth_passphrase}
                      onChange={e => setField('snmp_auth_passphrase', e.target.value)}
                      placeholder="••••••••" />
                  </div>
                  <div className="form-field">
                    <div className="form-label">Priv Protocol</div>
                    <select className="form-input" value={form.snmp_priv_protocol}
                      onChange={e => setField('snmp_priv_protocol', e.target.value)}>
                      <option value="DES">DES</option>
                      <option value="AES">AES</option>
                    </select>
                  </div>
                  <div className="form-field">
                    <div className="form-label">Priv Passphrase</div>
                    <input className="form-input" type="password" value={form.snmp_priv_passphrase}
                      onChange={e => setField('snmp_priv_passphrase', e.target.value)}
                      placeholder="••••••••" />
                  </div>
                </>
              )}
            </div>
            <div className="infra-hint">
              SNMP must be configured on the host. See documentation for setup instructions.
            </div>
          </>
        )}

        {/* ── Docker Engine config ── */}
        {form.type === 'docker_engine' && (
          <>
            <SectionHeading>Docker Socket</SectionHeading>
            <div className="form-fields" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
              <div className="form-field">
                <div className="form-label">Socket Type</div>
                <select className="form-input" value={form.docker_socket_type}
                  onChange={e => setField('docker_socket_type', e.target.value as 'local' | 'remote_proxy')}>
                  <option value="local">Local</option>
                  <option value="remote_proxy">Remote Proxy</option>
                </select>
              </div>
              <div className="form-field">
                <div className="form-label">Socket Path</div>
                <input className="form-input" value={form.docker_socket_path}
                  onChange={e => setField('docker_socket_path', e.target.value)}
                  placeholder="/var/run/docker.sock" />
              </div>
            </div>
          </>
        )}
      </>
    )
  }

  // ── Render ───────────────────────────────────────────────────────────────────

  return (
    <>
      <Topbar title="Infrastructure" onAdd={() => openAdd()} />
      <div className="content">

        {/* ── Tab toggle + Add button ── */}
        <div className="infra-tab-row">
          <div className="infra-tabs">
            <button
              className={`infra-tab${activeTab === 'components' ? ' active' : ''}`}
              onClick={() => setActiveTab('components')}
            >
              Components
            </button>
            <button
              className={`infra-tab${activeTab === 'map' ? ' active' : ''}`}
              onClick={() => setActiveTab('map')}
            >
              Network Map
            </button>
          </div>
          <button className="infra-add-btn" onClick={() => openAdd()}>
            + Add Component
          </button>
        </div>

        {/* ── Components tab ── */}
        {activeTab === 'components' && (
          <>
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
          </>
        )}

        {/* ── Network Map tab ── */}
        {activeTab === 'map' && (
          <div className="infra-map-placeholder">
            Network map coming soon — relationships between components will be visualised here.
          </div>
        )}

      </div>

      {/* ── Modal ── */}
      {modalOpen && (
        <div className="infra-modal-overlay" onClick={closeModal}>
          <div className="infra-modal" onClick={e => e.stopPropagation()}>

            <div className="infra-modal-header">
              <div className="infra-modal-title">
                {editingId ? 'Edit Component' : 'Add Component'}
              </div>
              <button className="infra-modal-close" onClick={closeModal}>✕</button>
            </div>

            <div className="infra-modal-body">
              {renderFormFields()}
              {formError && <div className="form-error">{formError}</div>}
            </div>

            <div className="infra-modal-footer">
              <button
                className="form-btn primary"
                onClick={() => void handleSubmit()}
                disabled={submitting}
              >
                {submitting ? 'Saving…' : editingId ? 'Save Changes' : 'Add Component'}
              </button>
              <button className="form-btn secondary" onClick={closeModal}>
                Cancel
              </button>
              {editingId && (
                <button
                  className="form-btn danger"
                  style={{ marginLeft: 'auto' }}
                  onClick={() => void handleDelete(editingId)}
                  disabled={deletingId === editingId}
                >
                  {deletingId === editingId ? 'Deleting…' : 'Delete'}
                </button>
              )}
            </div>

          </div>
        </div>
      )}
    </>
  )
}
