import type { AppSummary } from '../api/types'

function monogram(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

function sparklinePoints(data: number[], width: number, height: number): string {
  if (!data || data.length < 2) return ''
  const max = Math.max(...data, 1)
  const n = data.length
  return data
    .map((v, i) => {
      const x = ((i / (n - 1)) * width).toFixed(1)
      const y = (height - 2 - (v / max) * (height - 4)).toFixed(1)
      return `${x},${y}`
    })
    .join(' ')
}

interface Props {
  app: AppSummary
  onClick: () => void
}

export function AppWidget({ app, onClick }: Props) {
  const pts = sparklinePoints(app.sparkline, 180, 28)
  const closedPts = pts ? `${pts} 180,28 0,28` : ''

  const sparkColor =
    app.status === 'warn'
      ? 'var(--yellow)'
      : app.status === 'down'
      ? 'var(--red)'
      : 'var(--accent)'

  const dotClass =
    app.status === 'online' ? 'green' : app.status === 'warn' ? 'yellow' : app.status === 'down' ? 'red' : 'grey'

  const statusText =
    app.status === 'online' ? 'Online' : app.status === 'warn' ? 'Warn' : app.status === 'down' ? 'Down' : 'Unknown'

  const lastEventStyle: Record<string, string> =
    app.status === 'warn' ? { color: 'var(--yellow)' } : app.status === 'down' ? { color: 'var(--red)' } : {}

  return (
    <div
      className={`app-widget${app.status !== 'online' ? ` ${app.status}` : ''}`}
      onClick={onClick}
    >
      <div className="app-widget-header">
        <div className="app-icon">{monogram(app.name)}</div>
        <div className="app-name">{app.name}</div>
        <div className={`app-status ${app.status}`}>
          <div className={`dot ${dotClass}`} />
          {statusText}
        </div>
      </div>

      {app.stats && app.stats.length > 0 && (
        <div className="app-widget-stats">
          {app.stats.slice(0, 4).map(stat => (
            <div key={stat.label} className="stat-item">
              <div className="stat-label">{stat.label}</div>
              <div
                className="stat-value"
                style={stat.color ? { color: stat.color } : {}}
              >
                {stat.value}
              </div>
            </div>
          ))}
        </div>
      )}

      {pts && (
        <svg className="app-widget-sparkline" viewBox="0 0 180 28" preserveAspectRatio="none">
          <polyline points={pts} fill="none" stroke={sparkColor} strokeWidth="1.5" opacity="0.7" />
          <polyline points={closedPts} fill={sparkColor} stroke="none" opacity="0.07" />
        </svg>
      )}

      {app.last_event_text && (
        <div className="app-last-event" style={lastEventStyle}>
          {app.last_event_text}
        </div>
      )}
    </div>
  )
}
