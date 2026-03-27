import { useState, useEffect } from 'react'
import { integrations as integrationsApi } from '../api/client'
import type { InfraIntegration, CreateIntegrationInput } from '../api/types'
import './Integrations.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function statusDotClass(integration: InfraIntegration): string {
  if (!integration.last_status) return 'int-dot grey'
  if (integration.last_status === 'ok') return 'int-dot green'
  return 'int-dot red'
}

function statusLabel(integration: InfraIntegration): string {
  if (!integration.last_status) return 'Never synced'
  if (integration.last_status === 'ok') return 'Connected'
  return 'Error'
}

function formatTimeAgo(iso?: string | null): string {
  if (!iso) return '—'
  const diffMs = Date.now() - new Date(iso).getTime()
  const mins = Math.floor(diffMs / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}

// ── Add form ──────────────────────────────────────────────────────────────────

interface AddFormProps {
  onCreated: (integration: InfraIntegration) => void
  onCancel: () => void
}

function AddTraefikForm({ onCreated, onCancel }: AddFormProps) {
  const [name, setName] = useState('Traefik')
  const [apiUrl, setApiUrl] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [testResult, setTestResult] = useState<string | null>(null)

  async function handleCreate() {
    if (!apiUrl.trim()) { setError('API URL is required'); return }
    setSubmitting(true)
    setError(null)
    try {
      const input: CreateIntegrationInput = {
        type: 'traefik',
        name: name.trim() || 'Traefik',
        api_url: apiUrl.trim(),
        api_key: apiKey.trim() || null,
      }
      const created = await integrationsApi.create(input)
      onCreated(created)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to create integration')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleTest() {
    if (!apiUrl.trim()) { setError('API URL is required'); return }
    setSubmitting(true)
    setError(null)
    setTestResult(null)
    try {
      const input: CreateIntegrationInput = {
        type: 'traefik',
        name: name.trim() || 'Traefik',
        api_url: apiUrl.trim(),
        api_key: apiKey.trim() || null,
      }
      const created = await integrationsApi.create(input)
      const result = await integrationsApi.sync(created.id)
      setTestResult(`Connected — ${result.certs_found} cert${result.certs_found !== 1 ? 's' : ''} discovered`)
      onCreated(created)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Connection test failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="int-add-form">
      <div className="int-form-title">Add Traefik Integration</div>
      <div className="int-form-field">
        <div className="int-form-label">Name</div>
        <input
          className="int-form-input"
          value={name}
          onChange={e => setName(e.target.value)}
          placeholder="Traefik"
        />
      </div>
      <div className="int-form-field">
        <div className="int-form-label">API URL</div>
        <input
          className="int-form-input"
          value={apiUrl}
          onChange={e => setApiUrl(e.target.value)}
          placeholder="http://traefik:8080"
        />
      </div>
      <div className="int-form-field">
        <div className="int-form-label">API Key (optional)</div>
        <input
          className="int-form-input"
          type="password"
          value={apiKey}
          onChange={e => setApiKey(e.target.value)}
          placeholder="Leave blank if dashboard auth is disabled"
        />
      </div>
      {error && <div className="int-form-error">{error}</div>}
      {testResult && <div className="int-form-success">{testResult}</div>}
      <div className="int-form-actions">
        <button className="int-btn primary" onClick={handleCreate} disabled={submitting}>
          {submitting ? 'Saving…' : 'Add Integration'}
        </button>
        <button className="int-btn secondary" onClick={handleTest} disabled={submitting}>
          Test connection
        </button>
        <button className="int-btn ghost" onClick={onCancel}>Cancel</button>
      </div>
    </div>
  )
}

// ── Integration card ──────────────────────────────────────────────────────────

interface CardProps {
  integration: InfraIntegration
  onUpdated: (integration: InfraIntegration) => void
  onDeleted: (id: string) => void
}

function IntegrationCard({ integration, onUpdated, onDeleted }: CardProps) {
  const [syncing, setSyncing] = useState(false)
  const [syncMsg, setSyncMsg] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [current, setCurrent] = useState(integration)

  async function handleSync() {
    setSyncing(true)
    setSyncMsg(null)
    try {
      const result = await integrationsApi.sync(current.id)
      setSyncMsg(`${result.certs_found} cert${result.certs_found !== 1 ? 's' : ''} discovered`)
      // Refresh integration state
      const updated = await integrationsApi.get(current.id)
      setCurrent(updated)
      onUpdated(updated)
    } catch (e: unknown) {
      setSyncMsg(e instanceof Error ? e.message : 'Sync failed')
    } finally {
      setSyncing(false)
    }
  }

  async function handleDelete() {
    if (!window.confirm(`Delete integration "${current.name}"?`)) return
    setDeleting(true)
    try {
      await integrationsApi.delete(current.id)
      onDeleted(current.id)
    } catch {
      setDeleting(false)
    }
  }

  return (
    <div className="int-card">
      <div className="int-card-header">
        <div className="int-card-left">
          <div className="int-card-icon">↔</div>
          <div>
            <div className="int-card-name">{current.name}</div>
            <div className="int-card-url">{current.api_url}</div>
          </div>
        </div>
        <div className={statusDotClass(current)} title={statusLabel(current)} />
      </div>

      <div className="int-card-meta">
        <span>Last sync: {formatTimeAgo(current.last_synced_at)}</span>
        {syncMsg && <span className="int-sync-msg">{syncMsg}</span>}
      </div>

      {current.last_status === 'error' && current.last_error && (
        <div className="int-card-error">{current.last_error}</div>
      )}

      <div className="int-card-actions">
        <button className="int-btn secondary" onClick={handleSync} disabled={syncing}>
          {syncing ? 'Syncing…' : 'Sync now'}
        </button>
        <button className="int-btn danger" onClick={handleDelete} disabled={deleting}>
          {deleting ? 'Removing…' : 'Remove'}
        </button>
      </div>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function InfraIntegrations() {
  const [list, setList] = useState<InfraIntegration[]>([])
  const [loading, setLoading] = useState(true)
  const [showAdd, setShowAdd] = useState(false)

  useEffect(() => {
    integrationsApi.list()
      .then(res => setList(res.data))
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  function handleCreated(integration: InfraIntegration) {
    setList(prev => [...prev.filter(i => i.id !== integration.id), integration])
    setShowAdd(false)
  }

  function handleUpdated(integration: InfraIntegration) {
    setList(prev => prev.map(i => i.id === integration.id ? integration : i))
  }

  function handleDeleted(id: string) {
    setList(prev => prev.filter(i => i.id !== id))
  }

  return (
    <div className="int-section">
      <div className="int-section-header">
        <span className="int-section-title">Infrastructure Integrations</span>
        {!showAdd && (
          <button className="int-btn secondary" onClick={() => setShowAdd(true)}>
            + Add Traefik
          </button>
        )}
      </div>

      {showAdd && (
        <AddTraefikForm
          onCreated={handleCreated}
          onCancel={() => setShowAdd(false)}
        />
      )}

      {loading ? (
        <div className="int-empty">Loading…</div>
      ) : list.length === 0 && !showAdd ? (
        <div className="int-empty">No integrations configured. Add Traefik to enable SSL cert discovery.</div>
      ) : (
        <div className="int-list">
          {list.map(i => (
            <IntegrationCard
              key={i.id}
              integration={i}
              onUpdated={handleUpdated}
              onDeleted={handleDeleted}
            />
          ))}
        </div>
      )}
    </div>
  )
}
