export interface ResourceBar {
  label: string
  pct: number
}

export interface HostData {
  id: string
  name: string
  type: string
  ip: string
  status: 'online' | 'warn' | 'down' | 'unknown'
  resources: ResourceBar[]
}

function barFillClass(pct: number): string {
  if (pct >= 90) return 'resource-bar-fill crit'
  if (pct >= 70) return 'resource-bar-fill warn'
  return 'resource-bar-fill'
}

interface Props {
  host: HostData
  onClick?: () => void
}

export function HostWidget({ host, onClick }: Props) {
  const dotClass =
    host.status === 'online'
      ? 'green'
      : host.status === 'warn'
      ? 'yellow'
      : host.status === 'down'
      ? 'red'
      : 'grey'

  return (
    <div className="host-widget" onClick={onClick} style={onClick ? { cursor: 'pointer' } : undefined}>
      <div className="host-widget-header">
        <div className="host-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <rect x="2" y="2" width="20" height="8" rx="2" />
            <rect x="2" y="14" width="20" height="8" rx="2" />
            <line x1="6" y1="6" x2="6.01" y2="6" />
            <line x1="6" y1="18" x2="6.01" y2="18" />
          </svg>
        </div>
        <div className="host-meta">
          <div className="host-name">{host.name}</div>
          <div className="host-type">{host.type} · {host.ip}</div>
        </div>
        <div className={`app-status ${host.status}`}>
          <div className={`dot ${dotClass}`} />
        </div>
      </div>
      <div className="resource-bars">
        {host.resources.map(r => (
          <div key={r.label} className="resource-row">
            <div className="resource-label">{r.label}</div>
            <div className="resource-bar-track">
              <div
                className={barFillClass(r.pct)}
                style={{ width: `${r.pct}%` }}
              />
            </div>
            <div
              className="resource-pct"
              style={r.pct >= 90 ? { color: 'var(--red)' } : r.pct >= 70 ? { color: 'var(--yellow)' } : {}}
            >
              {r.pct}%
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
