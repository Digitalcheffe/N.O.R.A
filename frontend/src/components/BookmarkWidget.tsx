import { useState } from 'react'
import type { AppSummary } from '../api/types'

function monogram(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

const CAPABILITY_LABEL: Record<string, string> = {
  monitor_only: 'Monitor',
  docker_only: 'Docker',
  limited: 'Limited',
}

interface Props {
  app: AppSummary
  onClick: () => void
}

export function BookmarkWidget({ app, onClick }: Props) {
  const [iconFailed, setIconFailed] = useState(false)

  const dotClass =
    app.status === 'online' ? 'green'
    : app.status === 'warn' ? 'yellow'
    : app.status === 'down' ? 'red'
    : 'grey'

  const showIcon = app.icon_url && !iconFailed
  const capLabel = CAPABILITY_LABEL[app.capability ?? ''] ?? 'Service'

  return (
    <div className="bookmark-widget" onClick={onClick}>
      <div className="bookmark-icon">
        {showIcon ? (
          <img
            src={app.icon_url}
            alt={app.name}
            className="app-icon-img"
            onError={() => setIconFailed(true)}
          />
        ) : (
          <span className="bookmark-monogram">{monogram(app.name)}</span>
        )}
      </div>
      <div className="bookmark-info">
        <div className="bookmark-name">{app.name}</div>
        <div className="bookmark-url">{capLabel}</div>
      </div>
      <div className={`dot ${dotClass}`} style={{ marginLeft: 'auto', flexShrink: 0 }} />
    </div>
  )
}
