import { useState, useEffect } from 'react'
import { integrationDrivers } from '../api/client'
import type { IntegrationDriver } from '../api/types'
import './Integrations.css'

// ── Form field schema ─────────────────────────────────────────────────────────

interface FieldDef {
  key: string
  label: string
  type: 'text' | 'password' | 'select'
  options?: string[]
  optional?: boolean
  showWhen?: (formData: Record<string, string>) => boolean
}

const DRIVER_FIELDS: Record<string, FieldDef[]> = {
  traefik: [
    { key: 'api_url', label: 'API URL', type: 'text' },
    { key: 'api_token', label: 'API Token', type: 'password', optional: true },
  ],
  proxmox: [
    { key: 'host_url', label: 'Host URL', type: 'text' },
    { key: 'token_id', label: 'API Token ID', type: 'text' },
    { key: 'token_secret', label: 'API Token Secret', type: 'password' },
  ],
  opnsense: [
    { key: 'host_url', label: 'Host URL', type: 'text' },
    { key: 'api_key', label: 'API Key', type: 'text' },
    { key: 'api_secret', label: 'API Secret', type: 'password' },
  ],
  synology: [
    { key: 'host_url', label: 'Host URL', type: 'text' },
    { key: 'username', label: 'Username', type: 'text' },
    { key: 'password', label: 'Password', type: 'password' },
  ],
  snmp: [
    { key: 'version', label: 'SNMP Version', type: 'select', options: ['v2c', 'v3'] },
    {
      key: 'community',
      label: 'Community String',
      type: 'text',
      showWhen: (f) => f['version'] !== 'v3',
    },
    {
      key: 'username',
      label: 'Username',
      type: 'text',
      showWhen: (f) => f['version'] === 'v3',
    },
    {
      key: 'auth_password',
      label: 'Auth Password',
      type: 'password',
      showWhen: (f) => f['version'] === 'v3',
    },
    {
      key: 'priv_password',
      label: 'Priv Password',
      type: 'password',
      showWhen: (f) => f['version'] === 'v3',
    },
  ],
}

function defaultFormData(name: string): Record<string, string> {
  const fields = DRIVER_FIELDS[name] ?? []
  const data: Record<string, string> = {}
  for (const f of fields) {
    data[f.key] = f.key === 'version' ? 'v2c' : ''
  }
  return data
}

// ── Inline configure form ─────────────────────────────────────────────────────

interface ConfigFormProps {
  name: string
  onSaved: () => void
  onCancel: () => void
}

function ConfigForm({ name, onSaved, onCancel }: ConfigFormProps) {
  const [formData, setFormData] = useState<Record<string, string>>(() => defaultFormData(name))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const fields = DRIVER_FIELDS[name] ?? []

  async function handleSave() {
    setSaving(true)
    setError(null)
    try {
      await integrationDrivers.configure(name, formData)
      onSaved()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  function setField(key: string, value: string) {
    setFormData(prev => ({ ...prev, [key]: value }))
  }

  return (
    <div className="int-add-form">
      {fields.map(f => {
        if (f.showWhen && !f.showWhen(formData)) return null
        if (f.type === 'select' && f.options) {
          return (
            <div key={f.key} className="int-form-field">
              <div className="int-form-label">{f.label}</div>
              <select
                className="int-form-input"
                value={formData[f.key] ?? ''}
                onChange={e => setField(f.key, e.target.value)}
              >
                {f.options.map(o => <option key={o} value={o}>{o}</option>)}
              </select>
            </div>
          )
        }
        return (
          <div key={f.key} className="int-form-field">
            <div className="int-form-label">
              {f.label}{f.optional ? ' (optional)' : ''}
            </div>
            <input
              className="int-form-input"
              type={f.type === 'password' ? 'password' : 'text'}
              value={formData[f.key] ?? ''}
              onChange={e => setField(f.key, e.target.value)}
            />
          </div>
        )
      })}
      {error && <div className="int-form-error">{error}</div>}
      <div className="int-form-actions">
        <button className="int-btn primary" onClick={handleSave} disabled={saving}>
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button className="int-btn ghost" onClick={onCancel}>Cancel</button>
      </div>
    </div>
  )
}

// ── Driver card ───────────────────────────────────────────────────────────────

interface DriverCardProps {
  driver: IntegrationDriver
  onChanged: (name: string, configured: boolean) => void
}

function DriverCard({ driver, onChanged }: DriverCardProps) {
  const [editOpen, setEditOpen] = useState(false)
  const [disconnecting, setDisconnecting] = useState(false)

  async function handleDisconnect() {
    setDisconnecting(true)
    try {
      await integrationDrivers.disconnect(driver.name)
      onChanged(driver.name, false)
    } catch {
      // leave state unchanged on error
    } finally {
      setDisconnecting(false)
    }
  }

  return (
    <div className="int-card">
      <div className="int-card-header">
        <div className="int-card-left">
          <div>
            <div className="int-card-name">{driver.label}</div>
            <div className="int-card-url">{driver.description}</div>
            <div className="int-capabilities">
              Capabilities: {driver.capabilities.join(' · ')}
            </div>
          </div>
        </div>
        <div className="int-badge-row">
          <span className={`int-dot ${driver.configured ? 'green' : 'grey'}`} />
          <span className="int-badge-label">
            {driver.configured ? 'Configured' : 'Not configured'}
          </span>
        </div>
      </div>

      {editOpen && (
        <ConfigForm
          name={driver.name}
          onSaved={() => { setEditOpen(false); onChanged(driver.name, true) }}
          onCancel={() => setEditOpen(false)}
        />
      )}

      {!editOpen && (
        <div className="int-card-actions">
          {driver.configured ? (
            <>
              <button className="int-btn secondary" onClick={() => setEditOpen(true)}>
                Edit
              </button>
              <button
                className="int-btn danger"
                onClick={handleDisconnect}
                disabled={disconnecting}
              >
                {disconnecting ? 'Disconnecting…' : 'Disconnect'}
              </button>
            </>
          ) : (
            <button className="int-btn secondary" onClick={() => setEditOpen(true)}>
              Configure
            </button>
          )}
        </div>
      )}
    </div>
  )
}

// ── Main exported component ───────────────────────────────────────────────────

export function InfraIntegrations() {
  const [drivers, setDrivers] = useState<IntegrationDriver[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    integrationDrivers.list()
      .then(res => setDrivers(res.data))
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  function handleChanged(name: string, configured: boolean) {
    setDrivers(prev => prev.map(d => d.name === name ? { ...d, configured } : d))
  }

  return (
    <div className="int-section">
      <div className="int-section-header">
        <span className="int-section-title">Infrastructure Integrations</span>
      </div>

      {loading ? (
        <div className="int-empty">Loading…</div>
      ) : (
        <div className="int-list">
          {drivers.map(d => (
            <DriverCard
              key={d.name}
              driver={d}
              onChanged={handleChanged}
            />
          ))}
        </div>
      )}
    </div>
  )
}
