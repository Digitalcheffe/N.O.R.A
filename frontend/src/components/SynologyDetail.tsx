import { useState, useEffect, useCallback } from 'react'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { synology as synologyApi } from '../api/client'
import { formatBytes } from '../utils/format'
import type {
  InfrastructureComponent,
  SynologyDetail,
  SynologyVolume,
  SynologyDisk,
} from '../api/types'
import '../pages/InfraComponentDetail.css'
import './SynologyDetail.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

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
    case 'normal':   return 'syn-dot normal'
    case 'degraded':
    case 'warning':  return 'syn-dot warn'
    case 'crashed':
    case 'critical': return 'syn-dot crit'
    default:         return 'syn-dot unknown'
  }
}

function statusLabel(status: string): string {
  if (!status) return '—'
  return status.charAt(0).toUpperCase() + status.slice(1)
}

// ── Sub-components ────────────────────────────────────────────────────────────

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
    vol.status === 'crashed'  ? 'syn-row-crit' :
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
    disk.status === 'warning'  ? 'syn-row-warn' : ''
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

// ── SynologyContent ───────────────────────────────────────────────────────────
// Content-only component rendered inside InfraComponentDetail's DetailPageLayout
// shell. Returns both the JSX children and the key data points via callback so
// the parent shell can display them in the header.

interface SynologyContentProps {
  component: InfrastructureComponent
  onDetailLoaded?: (detail: SynologyDetail | null, noData: boolean) => void
}

export function SynologyContent({ component, onDetailLoaded }: SynologyContentProps) {
  const { tick } = useAutoRefresh()
  const [detail, setDetail] = useState<SynologyDetail | null>(null)
  const [noData, setNoData] = useState(false)

  const load = useCallback(async () => {
    try {
      const resp = await synologyApi.detail(component.id)
      setDetail(resp.data)
      setNoData(resp.no_data === true)
      onDetailLoaded?.(resp.data, resp.no_data === true)
    } catch {
      setDetail(null)
      onDetailLoaded?.(null, true)
    }
  }, [component.id, onDetailLoaded])

  useEffect(() => { void load() }, [load, tick])

  const d = detail

  return (
    <>
      {/* Section 1 — System Info */}
      <div className="icd-section">
        <div className="icd-section-title">System Info</div>
        {noData ? (
          <div className="snmp-pending">Awaiting first scan</div>
        ) : (
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
        )}
      </div>

      {/* Section 2 — CPU & Memory */}
      <div className="icd-section">
        <div className="icd-section-title">CPU &amp; Memory</div>
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

      {/* Section 3 — Volumes */}
      <div className="icd-section">
        <div className="icd-section-title">Volumes</div>
        {!d || d.volumes.length === 0 ? (
          <div className="snmp-pending">Pending first scan</div>
        ) : (
          <div className="syn-vol-list">
            {d.volumes.map(vol => (
              <VolumeRow key={vol.path} vol={vol} />
            ))}
          </div>
        )}
      </div>

      {/* Section 4 — Disks */}
      <div className="icd-section">
        <div className="icd-section-title">Disks</div>
        {!d || d.disks.length === 0 ? (
          <div className="snmp-pending">Pending first scan</div>
        ) : (
          <div className="syn-disk-list">
            {d.disks.map(disk => (
              <DiskRow key={disk.slot} disk={disk} />
            ))}
          </div>
        )}
      </div>

      {/* Section 5 — Updates */}
      <div className="icd-section">
        <div className="icd-section-title">Updates</div>
        <div className="syn-update-row">
          <span className="syn-update-label">DSM</span>
          {!d || !d.update?.checked ? (
            <span className="syn-update-value muted">—</span>
          ) : d.update.available ? (
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
    </>
  )
}

// Backward compat alias
export { SynologyContent as SynologyDetail }
