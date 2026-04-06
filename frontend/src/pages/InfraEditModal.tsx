import { useState } from 'react'
import type { ComponentType, CollectionMethod, InfrastructureComponent, InfrastructureComponentInput } from '../api/types'
import { SlidePanel } from '../components/SlidePanel'
import './Infrastructure.css'
import '../components/CheckForm.css'

// ── Constants ─────────────────────────────────────────────────────────────────

export const COLLECTION_METHOD: Record<ComponentType, CollectionMethod> = {
  proxmox_node:  'proxmox_api',
  synology:      'synology_api',
  vm_linux:      'snmp',
  vm_windows:    'snmp',
  vm_other:      'none',
  linux_host:    'snmp',
  windows_host:  'snmp',
  generic_host:  'none',
  docker_engine: 'docker_socket',
  traefik:       'traefik_api',
  portainer:     'portainer_api',
}

export const TYPE_LABEL: Record<ComponentType, string> = {
  proxmox_node:  'Proxmox Node',
  synology:      'Synology NAS',
  vm_linux:      'VM Linux',
  vm_windows:    'VM Windows',
  vm_other:      'VM Other',
  linux_host:    'Linux Host',
  windows_host:  'Windows Host',
  generic_host:  'Generic Host',
  docker_engine: 'Docker Engine',
  traefik:       'Traefik',
  portainer:     'Portainer',
}

export const SNMP_TYPES = new Set<ComponentType>(['vm_linux', 'vm_windows', 'linux_host', 'windows_host'])

// ── Form state ────────────────────────────────────────────────────────────────

export interface InfraForm {
  name: string
  ip: string
  type: ComponentType
  parent_id: string
  notes: string
  enabled: boolean
  proxmox_base_url: string
  proxmox_token_id: string
  proxmox_token_secret: string
  proxmox_verify_tls: boolean
  synology_base_url: string
  synology_username: string
  synology_password: string
  synology_verify_tls: boolean
  snmp_version: '2c' | '3'
  snmp_community: string
  snmp_port: string
  snmp_auth_protocol: string
  snmp_auth_passphrase: string
  snmp_priv_protocol: string
  snmp_priv_passphrase: string
  docker_socket_type: 'local' | 'remote_proxy'
  docker_socket_path: string
  traefik_api_url: string
  traefik_api_key: string
  portainer_base_url: string
  portainer_api_key: string
}

export const DEFAULT_FORM: InfraForm = {
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
  traefik_api_url: '',
  traefik_api_key: '',
  portainer_base_url: '',
  portainer_api_key: '',
}

// ── Helpers ───────────────────────────────────────────────────────────────────

export function formToPayload(form: InfraForm, isEdit: boolean): InfrastructureComponentInput {
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
    const hasCredFields = form.proxmox_base_url || form.proxmox_token_id || form.proxmox_token_secret
    if (!isEdit || hasCredFields) {
      payload.credentials = JSON.stringify({
        base_url:     form.proxmox_base_url,
        token_id:     form.proxmox_token_id,
        token_secret: form.proxmox_token_secret,
        verify_tls:   form.proxmox_verify_tls,
      })
    }
  } else if (form.type === 'synology') {
    const hasCredFields = form.synology_base_url || form.synology_username || form.synology_password
    if (!isEdit || hasCredFields) {
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
  } else if (form.type === 'traefik') {
    const hasCredFields = !!form.traefik_api_url
    if (!isEdit || hasCredFields) {
      payload.credentials = JSON.stringify({
        api_url: form.traefik_api_url,
        api_key: form.traefik_api_key,
      })
    }
  } else if (form.type === 'portainer') {
    const hasCredFields = !!form.portainer_base_url
    if (!isEdit || hasCredFields) {
      payload.credentials = JSON.stringify({
        base_url: form.portainer_base_url,
        api_key:  form.portainer_api_key,
      })
    }
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

export function componentToForm(c: InfrastructureComponent): InfraForm {
  const form: InfraForm = {
    ...DEFAULT_FORM,
    name:      c.name,
    ip:        c.ip,
    type:      c.type,
    parent_id: '',
    notes:     c.notes,
    enabled:   c.enabled,
  }

  const m = c.credential_meta
  if (m) {
    if (c.type === 'proxmox_node') {
      form.proxmox_base_url   = (m.base_url   as string)  ?? ''
      form.proxmox_token_id   = (m.token_id   as string)  ?? ''
      form.proxmox_verify_tls = (m.verify_tls as boolean) ?? false
    } else if (c.type === 'synology') {
      form.synology_base_url   = (m.base_url   as string)  ?? ''
      form.synology_username   = (m.username   as string)  ?? ''
      form.synology_verify_tls = (m.verify_tls as boolean) ?? false
    } else if (c.type === 'docker_engine') {
      form.docker_socket_type = (m.socket_type as 'local' | 'remote_proxy') ?? 'local'
      form.docker_socket_path = (m.socket_path as string) ?? '/var/run/docker.sock'
    } else if (c.type === 'traefik') {
      form.traefik_api_url = (m.api_url as string) ?? ''
    } else if (c.type === 'portainer') {
      form.portainer_base_url = (m.base_url as string) ?? ''
    }
  }

  if (c.snmp_config) {
    try {
      const s = JSON.parse(c.snmp_config) as Record<string, unknown>
      form.snmp_version         = (s.version         as '2c' | '3') ?? '2c'
      form.snmp_community       = (s.community       as string)     ?? 'public'
      form.snmp_port            = String(s.port ?? 161)
      form.snmp_auth_protocol   = (s.auth_protocol   as string)     ?? 'SHA'
      form.snmp_auth_passphrase = (s.auth_passphrase as string)     ?? ''
      form.snmp_priv_protocol   = (s.priv_protocol   as string)     ?? 'AES'
      form.snmp_priv_passphrase = (s.priv_passphrase as string)     ?? ''
    } catch { /* keep defaults */ }
  }

  return form
}

// ── Sub-components ────────────────────────────────────────────────────────────

function SectionHeading({ children }: { children: React.ReactNode }) {
  return <div className="infra-form-section">{children}</div>
}

function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <label className="infra-toggle-row">
      <span className="infra-toggle-label">{label}</span>
      <span className={`infra-toggle${checked ? ' on' : ''}`} onClick={() => onChange(!checked)}>
        <span className="infra-toggle-thumb" />
      </span>
    </label>
  )
}

// ── InfraEditModal ────────────────────────────────────────────────────────────

export interface InfraEditModalProps {
  open: boolean
  /** The component to edit. Undefined = add mode. */
  component?: InfrastructureComponent
  /** All components, for the parent selector. */
  components: InfrastructureComponent[]
  /** Whether the component has saved credentials (shows hint text). */
  hasCreds?: boolean
  /** Pre-selected parent ID for add mode. */
  initialParentId?: string
  /**
   * Called with the fully-built payload on save.
   * Should make the API call and update local state.
   * The panel closes itself after this resolves.
   * Throw to display an error and keep the panel open.
   */
  onSave: (payload: InfrastructureComponentInput) => Promise<void>
  /** Called when the panel should close (cancel or after success). */
  onClose: () => void
  /**
   * Optional delete handler shown as a Delete button in edit mode.
   * Should make the API call and update local state.
   * The panel closes itself after this resolves.
   */
  onDelete?: () => Promise<void>
}

export function InfraEditModal({
  open,
  component,
  components,
  hasCreds = false,
  initialParentId,
  onSave,
  onClose,
  onDelete,
}: InfraEditModalProps) {
  const isEdit = !!component

  const [form, setFormState] = useState<InfraForm>(() => {
    if (component) return componentToForm(component)
    const f = { ...DEFAULT_FORM }
    if (initialParentId) f.parent_id = initialParentId
    return f
  })
  const [formError, setFormError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  function setField<K extends keyof InfraForm>(key: K, value: InfraForm[K]) {
    setFormState(prev => ({ ...prev, [key]: value }))
  }

  async function handleSubmit() {
    if (!form.name.trim()) { setFormError('Name is required'); return }
    setSubmitting(true)
    setFormError(null)
    try {
      await onSave(formToPayload(form, isEdit))
      onClose()
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleDelete() {
    if (!onDelete) return
    setDeleting(true)
    try {
      await onDelete()
      onClose()
    } finally {
      setDeleting(false)
    }
  }

  const parentOptions = components
    .filter(c => !component || c.id !== component.id)
    .sort((a, b) => a.name.localeCompare(b.name))

  const panelTitle = isEdit
    ? `Edit ${TYPE_LABEL[form.type]}`
    : `Add ${TYPE_LABEL[form.type]}`

  const footer = (
    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
      {isEdit && onDelete && (
        confirmingDelete ? (
          <>
            <span style={{ fontSize: 13, color: 'var(--text2)' }}>Are you sure?</span>
            <button
              className="sp-btn sp-btn--danger"
              onClick={() => void handleDelete()}
              disabled={deleting}
            >
              {deleting ? 'Deleting…' : 'Confirm'}
            </button>
            <button
              className="sp-btn sp-btn--secondary"
              onClick={() => setConfirmingDelete(false)}
              disabled={deleting}
            >
              Back
            </button>
          </>
        ) : (
          <button
            className="sp-btn sp-btn--danger"
            onClick={() => setConfirmingDelete(true)}
          >
            Delete
          </button>
        )
      )}
      <button
        className="sp-btn sp-btn--primary"
        onClick={() => void handleSubmit()}
        disabled={submitting}
      >
        {submitting ? 'Saving…' : isEdit ? 'Save Changes' : 'Add Component'}
      </button>
    </div>
  )

  return (
    <SlidePanel
      open={open}
      onClose={onClose}
      title={panelTitle}
      footer={footer}
    >
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
            {(Object.entries(TYPE_LABEL) as [ComponentType, string][])
              .sort((a, b) => a[1].localeCompare(b[1]))
              .map(([value, label]) => (
                <option key={value} value={value}>{label}</option>
              ))
            }
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
            {parentOptions.map(c => (
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
          <SectionHeading>
            Proxmox Credentials{' '}
            {isEdit && hasCreds
              ? <span className="infra-cred-saved">Credentials saved</span>
              : isEdit && <span className="infra-optional">(leave blank to keep existing)</span>}
          </SectionHeading>
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
                placeholder={hasCreds ? 'leave blank to keep saved secret' : '••••••••'} />
            </div>
          </div>
          <Toggle checked={form.proxmox_verify_tls} onChange={v => setField('proxmox_verify_tls', v)} label="Verify TLS" />
        </>
      )}

      {/* ── Synology credentials ── */}
      {form.type === 'synology' && (
        <>
          <SectionHeading>
            Synology Credentials{' '}
            {isEdit && hasCreds
              ? <span className="infra-cred-saved">Credentials saved</span>
              : isEdit && <span className="infra-optional">(leave blank to keep existing)</span>}
          </SectionHeading>
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
                placeholder={hasCreds ? 'leave blank to keep saved password' : '••••••••'} />
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

      {/* ── Traefik credentials ── */}
      {form.type === 'traefik' && (
        <>
          <SectionHeading>
            Traefik API{' '}
            {isEdit && hasCreds
              ? <span className="infra-cred-saved">Credentials saved</span>
              : isEdit && <span className="infra-optional">(leave blank to keep existing)</span>}
          </SectionHeading>
          <div className="form-fields" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
            <div className="form-field form-field-full">
              <div className="form-label">API URL</div>
              <input className="form-input" value={form.traefik_api_url}
                onChange={e => setField('traefik_api_url', e.target.value)}
                placeholder="http://traefik.local:8080" />
            </div>
            <div className="form-field form-field-full">
              <div className="form-label">API Key <span className="infra-optional">(optional)</span></div>
              <input className="form-input" type="password" value={form.traefik_api_key}
                onChange={e => setField('traefik_api_key', e.target.value)}
                placeholder={hasCreds ? 'leave blank to keep saved key' : 'Bearer ••••••••'} />
            </div>
          </div>
          <div className="infra-hint">
            NORA will poll the Traefik API every 5 minutes to discover SSL certs and HTTP routes.
            SSL checks are auto-created per cert and shown on the Checks page.
          </div>
        </>
      )}

      {/* ── Portainer credentials ── */}
      {form.type === 'portainer' && (
        <>
          <SectionHeading>
            Portainer API{' '}
            {isEdit && hasCreds
              ? <span className="infra-cred-saved">Credentials saved</span>
              : isEdit && <span className="infra-optional">(leave blank to keep existing)</span>}
          </SectionHeading>
          <div className="form-fields" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
            <div className="form-field form-field-full">
              <div className="form-label">Base URL</div>
              <input className="form-input" value={form.portainer_base_url}
                onChange={e => setField('portainer_base_url', e.target.value)}
                placeholder="http://portainer.local:9000" />
            </div>
            <div className="form-field form-field-full">
              <div className="form-label">API Key</div>
              <input className="form-input" type="password" value={form.portainer_api_key}
                onChange={e => setField('portainer_api_key', e.target.value)}
                placeholder={hasCreds ? 'leave blank to keep saved key' : '••••••••'} />
            </div>
          </div>
          <div className="infra-hint">
            Generate an API key in Portainer under My Account → Access Tokens.
            NORA polls every 15 minutes to enrich container image update status.
          </div>
        </>
      )}

      {formError && <div className="form-error">{formError}</div>}
    </SlidePanel>
  )
}
