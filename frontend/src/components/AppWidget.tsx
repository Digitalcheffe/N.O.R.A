import { useState } from 'react'
import type { AppSummary } from '../api/types'

function monogram(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

interface Props {
  app: AppSummary
  onClick: () => void
}

export function AppWidget({ app, onClick }: Props) {
  const [iconFailed, setIconFailed] = useState(false)

  const showIcon = app.icon_url && !iconFailed

  const dotClass =
    app.status === 'online' ? 'green'
    : app.status === 'warn' ? 'yellow'
    : app.status === 'down' ? 'red'
    : 'grey'

  const statusText =
    app.status === 'online' ? 'Online'
    : app.status === 'warn' ? 'Warn'
    : app.status === 'down' ? 'Down'
    : 'Unknown'

  const hasChecks   = app.checks_total > 0
  const allUp       = app.checks_up === app.checks_total
  const checksColor = !hasChecks ? '' : allUp ? 'ok' : app.status === 'down' ? 'down' : 'warn'

  return (
    <div
      className={`app-row${app.status !== 'online' ? ` ${app.status}` : ''}`}
      onClick={onClick}
    >
      <div className="ar-icon">
        {showIcon ? (
          <img
            src={app.icon_url}
            alt={app.name}
            className="ar-icon-img"
            onError={() => setIconFailed(true)}
          />
        ) : (
          monogram(app.name)
        )}
      </div>

      <div className="ar-body">
        <div className="ar-top">
          <span className="ar-name">{app.name}</span>
          <span className={`ar-dot ${dotClass}`} title={statusText} />
        </div>

        <div className="ar-event">
          {app.last_event_text ?? <span className="ar-event-empty">No recent events</span>}
        </div>

        {hasChecks && (
          <div className={`ar-checks ${checksColor}`}>
            ✓ {app.checks_up}/{app.checks_total}
          </div>
        )}
      </div>
    </div>
  )
}
