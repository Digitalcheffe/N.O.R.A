import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { DetailPageLayout } from '../components/DetailPageLayout'
import { discovery, apps as appsApi } from '../api/client'
import type { ContainerDetail as ContainerDetailType, App } from '../api/types'
import './InfraComponentDetail.css'
import './ContainerDetail.css'

// ── helpers ────────────────────────────────────────────────────────────────────

function sourceLabel(s: string) {
  if (s === 'docker_engine') return 'Docker'
  if (s === 'portainer')     return 'Portainer'
  return s || '—'
}

// Regex for env var keys that likely contain sensitive values.
const SENSITIVE_RE = /password|passwd|pwd|secret|token|_key|apikey|api_key|auth(?!or)|credential|private/i

function isSensitive(key: string): boolean {
  return SENSITIVE_RE.test(key)
}

interface PortBinding { PrivatePort: number; PublicPort?: number; Type: string; IP?: string }
interface NetworkEntry  { name: string; ip?: string }
interface VolumeEntry   { Source?: string; Destination: string; Mode?: string; Type?: string; RW?: boolean }
interface EnvEntry      { key: string; value: string }

function parsePorts(raw: string | null): PortBinding[] {
  if (!raw) return []
  try { return JSON.parse(raw) as PortBinding[] } catch { return [] }
}
function parseNetworks(raw: string | null): NetworkEntry[] {
  if (!raw) return []
  try { return JSON.parse(raw) as NetworkEntry[] } catch { return [] }
}
function parseVolumes(raw: string | null): VolumeEntry[] {
  if (!raw) return []
  try { return JSON.parse(raw) as VolumeEntry[] } catch { return [] }
}
function parseEnvVars(raw: string | null): EnvEntry[] {
  if (!raw) return []
  try {
    const arr = JSON.parse(raw) as string[]
    return arr.map(entry => {
      const idx = entry.indexOf('=')
      if (idx === -1) return { key: entry, value: '' }
      return { key: entry.slice(0, idx), value: entry.slice(idx + 1) }
    })
  } catch { return [] }
}
function parseLabels(raw: string | null): [string, string][] {
  if (!raw) return []
  try {
    const obj = JSON.parse(raw) as Record<string, string>
    return Object.entries(obj).sort(([a], [b]) => a.localeCompare(b))
  } catch { return [] }
}

// ── Component ─────────────────────────────────────────────────────────────────

export function ContainerDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()

  const [ctr, setCtr] = useState<ContainerDetailType | null>(null)
  const [allApps, setAllApps] = useState<App[]>([])
  const [loading, setLoading] = useState(true)
  const [showAllEnv, setShowAllEnv] = useState(false)
  const [showAllLabels, setShowAllLabels] = useState(false)

  const [linkAppId, setLinkAppId] = useState('')
  const [linkBusy, setLinkBusy] = useState(false)
  const [linkError, setLinkError] = useState('')

  const [deleteConfirm, setDeleteConfirm] = useState(false)
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [deleteError, setDeleteError] = useState('')

  useEffect(() => {
    if (!id) return
    discovery.getContainer(id)
      .then(c => { setCtr(c); setLoading(false) })
      .catch(() => navigate('/infrastructure'))
    appsApi.list()
      .then(res => setAllApps(res.data))
      .catch(() => {})
  }, [id, navigate, tick])

  async function handleLink() {
    if (!id || !linkAppId) return
    setLinkBusy(true)
    setLinkError('')
    try {
      await discovery.linkContainerApp(id, { mode: 'existing', app_id: linkAppId })
      const updated = await discovery.getContainer(id)
      setCtr(updated)
      setLinkAppId('')
    } catch {
      setLinkError('Failed to link app')
    } finally {
      setLinkBusy(false)
    }
  }

  async function handleUnlink() {
    if (!id) return
    setLinkBusy(true)
    setLinkError('')
    try {
      await discovery.unlinkContainerApp(id)
      const updated = await discovery.getContainer(id)
      setCtr(updated)
    } catch {
      setLinkError('Failed to unlink app')
    } finally {
      setLinkBusy(false)
    }
  }

  // handleDelete hard-removes the discovered_containers row. This doesn't touch
  // the actual container on the Docker engine — it just forgets about it.
  // Useful for pruning ghosts left behind after a container was removed from
  // its source (e.g. docker-compose down leftovers, old image rollovers).
  async function handleDelete() {
    if (!id) return
    setDeleteBusy(true)
    setDeleteError('')
    try {
      await discovery.deleteContainer(id)
      navigate('/infrastructure?view=containers')
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Failed to delete container')
      setDeleteBusy(false)
    }
  }

  if (loading || !ctr) {
    return null
  }

  const ports    = parsePorts(ctr.ports)
  const networks = parseNetworks(ctr.networks)
  const volumes  = parseVolumes(ctr.volumes)
  const envVars  = parseEnvVars(ctr.env_vars)
  const labels   = parseLabels(ctr.labels)

  const ENV_PREVIEW  = 10
  const LABEL_PREVIEW = 8
  const visibleEnv    = showAllEnv    ? envVars : envVars.slice(0, ENV_PREVIEW)
  const visibleLabels = showAllLabels ? labels  : labels.slice(0, LABEL_PREVIEW)

  const linkedApp = allApps.find(a => a.id === ctr.app_id)
  const available = allApps.filter(a => a.id !== ctr.app_id)

  const dplStatus = ctr.status === 'running' ? 'online'
    : (ctr.status === 'stopped' || ctr.status === 'exited') ? 'offline'
    : 'unknown'

  const keyDataPoints = [
    { label: 'Source', value: sourceLabel(ctr.source_type) },
    { label: 'Image',  value: ctr.image.length > 40 ? ctr.image.slice(0, 40) + '…' : ctr.image },
    ...(ctr.restart_policy ? [{ label: 'Restart', value: ctr.restart_policy }] : []),
  ]

  const linkedAppExtra = (
    <div className="ctr-det-linked-section">
      {linkedApp ? (
        <div className="ctr-det-linked-row">
          <span className="ctr-det-linked-label">Linked App</span>
          <span
            className="ctr-det-linked-name"
            onClick={() => navigate(`/apps/${linkedApp.id}`)}
          >
            {linkedApp.name}
          </span>
          <button
            className="ctr-det-unlink-btn"
            onClick={() => void handleUnlink()}
            disabled={linkBusy}
          >
            {linkBusy ? 'Unlinking…' : 'Unlink'}
          </button>
        </div>
      ) : (
        <div className="ctr-det-link-row">
          <span className="ctr-det-linked-label">Link App</span>
          <select
            className="ctr-det-select"
            value={linkAppId}
            onChange={e => setLinkAppId(e.target.value)}
          >
            <option value="">— select app —</option>
            {available.map(a => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </select>
          <button
            className="ctr-det-link-btn"
            onClick={() => void handleLink()}
            disabled={!linkAppId || linkBusy}
          >
            {linkBusy ? 'Linking…' : 'Link'}
          </button>
        </div>
      )}
      {linkError && <div className="ctr-det-error">{linkError}</div>}
    </div>
  )

  const deleteAction = (
    <div className="ctr-det-delete-wrap">
      {!deleteConfirm ? (
        <button
          className="ctr-det-delete-btn"
          onClick={() => { setDeleteConfirm(true); setDeleteError('') }}
          title="Remove this container record from NORA"
        >
          Delete
        </button>
      ) : (
        <div className="ctr-det-delete-confirm">
          <span className="ctr-det-delete-warn">Delete this record?</span>
          <button
            className="ctr-det-delete-cancel"
            onClick={() => setDeleteConfirm(false)}
            disabled={deleteBusy}
          >
            Cancel
          </button>
          <button
            className="ctr-det-delete-confirm-btn"
            onClick={() => void handleDelete()}
            disabled={deleteBusy}
          >
            {deleteBusy ? 'Deleting…' : 'Confirm'}
          </button>
        </div>
      )}
      {deleteError && <span className="ctr-det-error">{deleteError}</span>}
    </div>
  )

  return (
    <DetailPageLayout
      breadcrumb="Infrastructure"
      breadcrumbPath="/infrastructure"
      name={ctr.container_name}
      status={{ status: dplStatus, label: ctr.status }}
      keyDataPoints={keyDataPoints}
      headerExtra={linkedAppExtra}
      actions={deleteAction}
      sourceId={ctr.id}
    >
      {/* ── Image ── */}
      <div className="icd-section">
        <div className="icd-section-title">Image</div>
        <div className="ctr-det-fields">
          <div className="ctr-det-field">
            <span className="ctr-det-label">Name</span>
            <span className="ctr-det-value ctr-det-mono">{ctr.image}</span>
          </div>
          <div className="ctr-det-field">
            <span className="ctr-det-label">Update</span>
            <span className="ctr-det-value">
              {ctr.image_last_checked_at === null
                ? <span className="ctr-det-dim">Not checked yet</span>
                : ctr.image_update_available
                  ? <span className="de-update-badge">Update available</span>
                  : <span className="de-uptodate-badge">Up to date</span>
              }
            </span>
          </div>
          {ctr.image_last_checked_at && (
            <div className="ctr-det-field">
              <span className="ctr-det-label">Checked</span>
              <span className="ctr-det-value ctr-det-dim">
                {new Date(ctr.image_last_checked_at).toLocaleString()}
              </span>
            </div>
          )}
          {ctr.image_digest && (
            <div className="ctr-det-field">
              <span className="ctr-det-label">Local digest</span>
              <span className="ctr-det-value ctr-det-mono ctr-det-digest">{ctr.image_digest}</span>
            </div>
          )}
          {ctr.registry_digest && (
            <div className="ctr-det-field">
              <span className="ctr-det-label">Registry digest</span>
              <span className="ctr-det-value ctr-det-mono ctr-det-digest">{ctr.registry_digest}</span>
            </div>
          )}
        </div>
      </div>

      {/* ── Runtime ── */}
      <div className="icd-section">
        <div className="icd-section-title">Runtime</div>
        <div className="ctr-det-fields">
          <div className="ctr-det-field">
            <span className="ctr-det-label">Container ID</span>
            <span className="ctr-det-value ctr-det-mono ctr-det-digest">{ctr.container_id || '—'}</span>
          </div>
          {ctr.docker_created_at && (
            <div className="ctr-det-field">
              <span className="ctr-det-label">Created</span>
              <span className="ctr-det-value ctr-det-dim">
                {new Date(ctr.docker_created_at).toLocaleString()}
              </span>
            </div>
          )}
          <div className="ctr-det-field">
            <span className="ctr-det-label">Last seen</span>
            <span className="ctr-det-value ctr-det-dim">
              {new Date(ctr.last_seen_at).toLocaleString()}
            </span>
          </div>
        </div>
      </div>

      {/* ── Ports ── */}
      {ports.length > 0 && (
        <div className="icd-section">
          <div className="icd-section-title">Ports</div>
          <div className="ctr-port-pills">
            {ports.map((p, i) => (
              <span key={i} className={`ctr-port-pill ctr-port-${p.Type}`}>
                {p.PublicPort ? `${p.PublicPort}:${p.PrivatePort}` : String(p.PrivatePort)}
                <span className="ctr-port-type">{p.Type}</span>
              </span>
            ))}
          </div>
        </div>
      )}

      {/* ── Networks ── */}
      {networks.length > 0 && (
        <div className="icd-section">
          <div className="icd-section-title">Networks</div>
          <div className="ctr-kv-grid">
            {networks.map((n, i) => (
              <div key={i} className="ctr-kv-row">
                <span className="ctr-kv-key">{n.name}</span>
                <span className="ctr-kv-val ctr-det-dim">{n.ip || '—'}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ── Mounts ── */}
      {volumes.length > 0 && (
        <div className="icd-section">
          <div className="icd-section-title">Mounts</div>
          <div className="ctr-mount-list">
            {volumes.map((v, i) => (
              <div key={i} className="ctr-mount-row">
                <div className="ctr-mount-dest">
                  <span className="ctr-mount-arrow">→</span>
                  <span className="ctr-det-mono">{v.Destination}</span>
                </div>
                {v.Source && (
                  <div className="ctr-mount-src ctr-det-mono ctr-det-dim">{v.Source}</div>
                )}
                <div className="ctr-mount-badges">
                  {v.Type && <span className="ctr-mount-badge">{v.Type}</span>}
                  {v.Mode && v.Mode !== 'z' && <span className="ctr-mount-badge">{v.Mode}</span>}
                  {v.RW === false && <span className="ctr-mount-badge ctr-mount-ro">ro</span>}
                  {v.RW === true  && <span className="ctr-mount-badge ctr-mount-rw">rw</span>}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ── Environment ── */}
      {envVars.length > 0 && (
        <div className="icd-section">
          <div className="icd-section-title">
            Environment
            <span className="ctr-section-count">{envVars.length}</span>
          </div>
          <div className="ctr-env-table">
            {visibleEnv.map((ev, i) => {
              const masked = isSensitive(ev.key)
              return (
                <div key={i} className="ctr-env-row">
                  <span className="ctr-env-key">{ev.key}</span>
                  <span className={`ctr-env-val${masked ? ' ctr-env-masked' : ''}`}>
                    {masked ? '••••••••' : (ev.value || <span className="ctr-det-dim">(empty)</span>)}
                  </span>
                </div>
              )
            })}
          </div>
          {envVars.length > ENV_PREVIEW && (
            <button className="ctr-show-more" onClick={() => setShowAllEnv(v => !v)}>
              {showAllEnv
                ? `▲ Show fewer`
                : `▼ Show ${envVars.length - ENV_PREVIEW} more`}
            </button>
          )}
        </div>
      )}

      {/* ── Labels ── */}
      {labels.length > 0 && (
        <div className="icd-section">
          <div className="icd-section-title">
            Labels
            <span className="ctr-section-count">{labels.length}</span>
          </div>
          <div className="ctr-env-table">
            {visibleLabels.map(([k, v], i) => (
              <div key={i} className="ctr-env-row">
                <span className="ctr-env-key">{k}</span>
                <span className="ctr-env-val">{v || <span className="ctr-det-dim">(empty)</span>}</span>
              </div>
            ))}
          </div>
          {labels.length > LABEL_PREVIEW && (
            <button className="ctr-show-more" onClick={() => setShowAllLabels(v => !v)}>
              {showAllLabels
                ? `▲ Show fewer`
                : `▼ Show ${labels.length - LABEL_PREVIEW} more`}
            </button>
          )}
        </div>
      )}

    </DetailPageLayout>
  )
}
