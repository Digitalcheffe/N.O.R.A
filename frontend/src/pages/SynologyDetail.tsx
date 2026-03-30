import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { infrastructure as infraApi, synology as synologyApi } from '../api/client'
import type {
  InfrastructureComponent,
  SynologyDetail,
  SynologyVolume,
  SynologyDisk,
} from '../api/types'
import './SynologyDetail.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const tb = bytes / 1_099_511_627_776
  if (tb >= 1) return `${tb.toFixed(1)} TB`
  const gb = bytes / 1_073_741_824
  if (gb >= 1) return `${gb.toFixed(1)} GB`
  const mb = bytes / 1_048_576
  return `${mb.toFixed(0)} MB`
}

function timeAgo(iso: string | null | undefined): string {
  if (!iso) return '—'
  const secs = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (secs < 60) return `${secs}s ago`
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`
  return `${Math.floor(secs / 86400)}d ago`
}

function tempColor(c: number): string {
  if (c > 55) return 'var(--red)'
  if (c > 45) return 'var(--yellow, #eab308)'
  return 'var(--green)'
}

function barFillColor(pct: number): string {
  if (pct >= 90) return 'var(--red)'
  if (pct >= 70) return 'var(--yellow, #eab308)'
  return 'var(--accent, #3b82f6)'
}

function statusDotClass(status: string): string {
  switch (status.toLowerCase()) {
    case 'normal': return 'syn-dot normal'
    case 'degraded':
    case 'warning': return 'syn-dot warn'
    case 'crashed':
    case 'critical': return 'syn-dot crit'
    default: return 'syn-dot unknown'
  }
}

function statusLabel(status: string): string {
  if (!status) return '—'
  return status.charAt(0).toUpperCase() + status.slice(1)
}

function hostStatusClass(s: string): string {
  if (s === 'online') return 'online'
  if (s === 'degraded') return 'degraded'
  if (s === 'offline' || s === 'down') return 'offline'
  return 'unknown'
}

// ── Sub-components ────────────────────────────────────────────────────────────

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <div className="syn-section-label">{children}</div>
}

function ResourceBar({
  label, value, color, extra,
}: { label: string; value: number; color: string; extra?: string }) {
  return (
    <div className="syn-res-row">
      <div className="syn-res-label">{label}</div>
      <div className="syn-res-track">
        <div
          className="syn-res-fill"
          style={{ width: `${Math.min(Math.max(value, 0), 100)}%`, background: color }}
        />
      </div>
      <div className="syn-res-pct" style={{ color }}>{value > 0 ? `${Math.round(value)}%` : '0%'}</div>
      {extra && <div className="syn-res-extra">{extra}</div>}
    </div>
  )
}

function VolumeRow({ vol }: { vol: SynologyVolume }) {
  const accentClass =
    vol.status === 'crashed' ? 'syn-row-crit' :
    vol.status === 'degraded' ? 'syn-row-warn' : ''
  const color = barFillColor(vol.percent)
  return (
    <div className={`syn-vol-row ${accentClass}`}>
      <div className="syn-vol-path">{vol.path}</div>
      <div className="syn-res-track syn-vol-bar">
        <div
          className="syn-res-fill"
          style={{ width: `${Math.min(Math.max(vol.percent, 0), 100)}%`, background: color }}
        />
      </div>
      <div className="syn-res-pct" style={{ color }}>{Math.round(vol.percent)}%</div>
      <div className="syn-vol-size">{formatBytes(vol.used_bytes)} / {formatBytes(vol.total_bytes)}</div>
      <div className="syn-vol-status">
        <span className={statusDotClass(vol.status)} />
        {statusLabel(vol.status)}
      </div>
    </div>
  )
}

function DiskRow({ disk }: { disk: SynologyDisk }) {
  const accentClass =
    disk.status === 'critical' ? 'syn-row-crit' :
    disk.status === 'warning' ? 'syn-row-warn' : ''
  return (
    <div className={`syn-disk-row ${accentClass}`}>
      <div className="syn-disk-slot">Disk {disk.slot}</div>
      <div className="syn-disk-model">{disk.model || '—'}</div>
      <div className="syn-disk-temp" style={{ color: tempColor(disk.temperature_c) }}>
        {disk.temperature_c > 0 ? `${disk.temperature_c}°C` : '—'}
      </div>
      <div className="syn-disk-status">
        <span className={statusDotClass(disk.status)} />
        {statusLabel(disk.status)}
      </div>
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function SynologyDetail() {
  const { componentId } = useParams<{ componentId: string }>()
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()

  const [component, setComponent] = useState<InfrastructureComponent | null>(null)
  const [detail, setDetail] = useState<SynologyDetail | null>(null)
  const [noData, setNoData] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!componentId) return
    try {
      const [comp, detailResp] = await Promise.all([
        infraApi.get(componentId),
        synologyApi.detail(componentId),
      ])
      setComponent(comp)
      setDetail(detailResp.data)
      setNoData(detailResp.no_data === true)
      setError(null)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to load Synology data')
    } finally {
      setLoading(false)
    }
  }, [componentId])

  useEffect(() => { load() }, [load, tick])

  if (loading) {
    return (
      <>
        <Topbar title="Synology NAS" />
        <div className="content"><div className="syn-loading">Loading…</div></div>
      </>
    )
  }

  if (error || !component) {
    return (
      <>
        <Topbar title="Synology NAS" />
        <div className="content">
          <div className="syn-error">{error ?? 'Component not found'}</div>
          <button className="syn-back-btn" onClick={() => navigate('/topology')}>← Back</button>
        </div>
      </>
    )
  }

  const d = detail
  const statusCls = hostStatusClass(component.last_status)

  return (
    <>
      <Topbar title={component.name} />
      <div className="content">

        {/* Header */}
        <div className="syn-header">
          <div className="syn-header-left">
            <button className="syn-back-btn" onClick={() => navigate('/topology')}>← Infrastructure</button>
            <h1 className="syn-title">{component.name}</h1>
          </div>
          <div className="syn-header-right">
            <span className={`syn-status-dot ${statusCls}`} />
            <span className="syn-status-label">{component.last_status}</span>
            <span className="syn-type-badge">Synology NAS</span>
            {component.ip && <span className="syn-ip">{component.ip}</span>}
            {d?.polled_at && (
              <span className="syn-polled-at">polled {timeAgo(d.polled_at)}</span>
            )}
          </div>
        </div>

        {/* Card */}
        <div className="syn-card">

          {/* Section 1 — System Info */}
          <div className="syn-section">
            <SectionLabel>System Info</SectionLabel>
            <div className="syn-info-grid">
              <span className="syn-info-key">Model</span>
              <span className="syn-info-val">{d?.model || '—'}</span>
              <span className="syn-info-key">DSM</span>
              <span className="syn-info-val">{d?.dsm_version || '—'}</span>
              <span className="syn-info-key">Hostname</span>
              <span className="syn-info-val">{d?.hostname || '—'}</span>
              <span className="syn-info-key">Uptime</span>
              <span className="syn-info-val">{d?.uptime || '—'}</span>
              <span className="syn-info-key">Temp</span>
              <span
                className="syn-info-val"
                style={d?.temperature_c ? { color: tempColor(d.temperature_c) } : undefined}
              >
                {d?.temperature_c ? `${d.temperature_c}°C` : '—'}
              </span>
            </div>
          </div>

          <div className="syn-divider" />

          {/* Section 2 — CPU & Memory */}
          <div className="syn-section">
            <SectionLabel>CPU &amp; Memory</SectionLabel>
            <div className="syn-res-list">
              <ResourceBar
                label="CPU"
                value={d?.cpu_percent ?? 0}
                color={barFillColor(d?.cpu_percent ?? 0)}
              />
              <ResourceBar
                label="MEM"
                value={d?.memory?.percent ?? 0}
                color={barFillColor(d?.memory?.percent ?? 0)}
                extra={
                  d?.memory?.total_bytes
                    ? `${formatBytes(d.memory.used_bytes)} / ${formatBytes(d.memory.total_bytes)}`
                    : undefined
                }
              />
            </div>
          </div>

          <div className="syn-divider" />

          {/* Section 3 — Volumes */}
          <div className="syn-section">
            <SectionLabel>Volumes</SectionLabel>
            {!d || d.volumes.length === 0 ? (
              <div className="syn-pending-row">Pending first scan</div>
            ) : (
              <div className="syn-vol-list">
                {d.volumes.map(vol => (
                  <VolumeRow key={vol.path} vol={vol} />
                ))}
              </div>
            )}
          </div>

          <div className="syn-divider" />

          {/* Section 4 — Disks */}
          <div className="syn-section">
            <SectionLabel>Disks</SectionLabel>
            {!d || d.disks.length === 0 ? (
              <div className="syn-pending-row">Pending first scan</div>
            ) : (
              <div className="syn-disk-list">
                {d.disks.map(disk => (
                  <DiskRow key={disk.slot} disk={disk} />
                ))}
              </div>
            )}
          </div>

          <div className="syn-divider" />

          {/* Section 5 — Updates */}
          <div className="syn-section">
            <SectionLabel>Updates</SectionLabel>
            <div className="syn-update-row">
              <span className="syn-update-label">DSM</span>
              {!d ? (
                <span className="syn-update-value muted">—</span>
              ) : d.update?.available ? (
                <>
                  <span className="syn-update-arrow">↑</span>
                  <span className="syn-update-value available">{d.update.version} available</span>
                  {d.dsm_version && (
                    <span className="syn-update-current">(currently {d.dsm_version})</span>
                  )}
                </>
              ) : (
                <>
                  <span className="syn-update-check">✓</span>
                  <span className="syn-update-value">Up to date</span>
                  {d.dsm_version && (
                    <span className="syn-update-current muted">{d.dsm_version}</span>
                  )}
                </>
              )}
            </div>
          </div>

          {/* Pending note */}
          {noData && (
            <div className="syn-awaiting">Awaiting first scan</div>
          )}

        </div>
      </div>
    </>
  )
}
