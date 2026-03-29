import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { infrastructure as infraApi } from '../api/client'
import type {
  InfrastructureComponent,
  TraefikComponentDetail,
  TraefikCertWithCheck,
  TraefikRoute,
} from '../api/types'
import './TraefikComponentDetail.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function daysUntil(iso: string | null | undefined): number | null {
  if (!iso) return null
  return Math.floor((new Date(iso).getTime() - Date.now()) / 86_400_000)
}

function expiryClass(days: number | null): string {
  if (days === null) return ''
  if (days <= 7) return 'tcd-cert-crit'
  if (days <= 30) return 'tcd-cert-warn'
  return ''
}

function checkStatusClass(status: string): string {
  if (status === 'up') return 'tcd-check-up'
  if (status === 'warn') return 'tcd-check-warn'
  if (status === 'down' || status === 'critical') return 'tcd-check-down'
  return 'tcd-check-unknown'
}

function checkStatusLabel(status: string): string {
  if (status === 'up') return 'UP'
  if (status === 'warn') return 'WARN'
  if (status === 'down') return 'DOWN'
  if (status === 'critical') return 'CRIT'
  return '—'
}

// ── Cert table ────────────────────────────────────────────────────────────────

function CertTable({ certs }: { certs: TraefikCertWithCheck[] }) {
  if (certs.length === 0) {
    return <div className="tcd-empty">No certificates discovered yet. Check that the Traefik API URL is reachable.</div>
  }
  return (
    <table className="tcd-table">
      <thead>
        <tr>
          <th>Domain</th>
          <th>Issuer</th>
          <th>Expires</th>
          <th>Days</th>
          <th>SSL Check</th>
        </tr>
      </thead>
      <tbody>
        {certs.map(cert => {
          const days = daysUntil(cert.expires_at)
          const cls = expiryClass(days)
          return (
            <tr key={cert.id} className={cls}>
              <td className="tcd-cert-domain">{cert.domain}</td>
              <td className="tcd-cert-issuer">{cert.issuer ?? '—'}</td>
              <td className="tcd-cert-expires">
                {cert.expires_at ? new Date(cert.expires_at).toLocaleDateString() : '—'}
              </td>
              <td className="tcd-cert-days">
                {days !== null ? (
                  <span className={`tcd-days-badge${cls ? ' ' + cls : ''}`}>{days}d</span>
                ) : '—'}
              </td>
              <td>
                {cert.check_status ? (
                  <span className={`tcd-check-badge ${checkStatusClass(cert.check_status)}`}>
                    {checkStatusLabel(cert.check_status)}
                  </span>
                ) : (
                  <span className="tcd-check-badge tcd-check-unknown">—</span>
                )}
              </td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

// ── Routes table ──────────────────────────────────────────────────────────────

function RoutesTable({ routes }: { routes: TraefikRoute[] }) {
  if (routes.length === 0) {
    return <div className="tcd-empty">No HTTP routes discovered yet.</div>
  }
  return (
    <table className="tcd-table">
      <thead>
        <tr>
          <th>Name</th>
          <th>Rule</th>
          <th>Service</th>
          <th>Status</th>
        </tr>
      </thead>
      <tbody>
        {routes.map(route => (
          <tr key={route.id}>
            <td className="tcd-route-name">{route.name}</td>
            <td className="tcd-route-rule">{route.rule}</td>
            <td className="tcd-route-service">{route.service}</td>
            <td>
              <span className={`tcd-route-status ${route.status === 'enabled' ? 'enabled' : 'disabled'}`}>
                {route.status}
              </span>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function TraefikComponentDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const [component, setComponent]   = useState<InfrastructureComponent | null>(null)
  const [detail,    setDetail]      = useState<TraefikComponentDetail | null>(null)
  const [loading,   setLoading]     = useState(true)
  const [error,     setError]       = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    Promise.all([
      infraApi.get(id),
      infraApi.traefikDetail(id),
    ])
      .then(([comp, det]) => {
        setComponent(comp)
        setDetail(det)
      })
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load'))
      .finally(() => setLoading(false))
  }, [id])

  function statusClass(s: string) {
    if (s === 'online') return 'online'
    if (s === 'degraded') return 'degraded'
    if (s === 'offline') return 'offline'
    return 'unknown'
  }

  if (loading) {
    return (
      <>
        <Topbar title="Traefik Component" />
        <div className="content"><div className="tcd-loading">Loading…</div></div>
      </>
    )
  }

  if (error || !component || !detail) {
    return (
      <>
        <Topbar title="Traefik Component" />
        <div className="content">
          <div className="tcd-error">{error ?? 'Component not found'}</div>
          <button className="tcd-back-btn" onClick={() => navigate('/topology')}>← Back</button>
        </div>
      </>
    )
  }

  return (
    <>
      <Topbar title={component.name} />
      <div className="content">

        {/* Header */}
        <div className="tcd-header">
          <button className="tcd-back-btn" onClick={() => navigate('/topology')}>← Infrastructure</button>
          <div className="tcd-header-meta">
            <span className={`tcd-status-dot ${statusClass(component.last_status)}`} />
            <span className="tcd-status-label">{component.last_status}</span>
            {component.ip && <span className="tcd-ip">{component.ip}</span>}
          </div>
        </div>

        {/* Summary stats */}
        <div className="tcd-stats-row">
          <div className="tcd-stat">
            <div className="tcd-stat-value">{detail.cert_count}</div>
            <div className="tcd-stat-label">Certs</div>
          </div>
          {detail.crit_count > 0 && (
            <div className="tcd-stat crit">
              <div className="tcd-stat-value">{detail.crit_count}</div>
              <div className="tcd-stat-label">Critical</div>
            </div>
          )}
          {detail.warn_count > 0 && (
            <div className="tcd-stat warn">
              <div className="tcd-stat-value">{detail.warn_count}</div>
              <div className="tcd-stat-label">Expiring Soon</div>
            </div>
          )}
          <div className="tcd-stat">
            <div className="tcd-stat-value">{detail.routes.length}</div>
            <div className="tcd-stat-label">Routes</div>
          </div>
        </div>

        {/* SSL Certificates */}
        <div className="tcd-section">
          <div className="tcd-section-title">SSL Certificates</div>
          <CertTable certs={detail.certs} />
        </div>

        {/* HTTP Routes */}
        <div className="tcd-section">
          <div className="tcd-section-title">HTTP Routes</div>
          <RoutesTable routes={detail.routes} />
        </div>

      </div>
    </>
  )
}
