import type { CheckSummary } from '../api/types'
import { formatEventTime } from '../utils/formatTime'

function statusBlockLabel(check: CheckSummary): string {
  if (check.type === 'ssl') return 'SSL'
  if (check.status === 'up') return 'UP'
  if (check.status === 'down') return 'DOWN'
  if (check.status === 'warn') return 'WARN'
  return '?'
}

function statusBlockClass(status: string): string {
  if (status === 'up') return 'monitor-status-block up'
  if (status === 'warn') return 'monitor-status-block warn'
  if (status === 'down') return 'monitor-status-block down'
  return 'monitor-status-block unknown'
}

function uptimeClass(status: string): string {
  if (status === 'down') return 'monitor-uptime down'
  if (status === 'warn') return 'monitor-uptime warn'
  return 'monitor-uptime'
}

interface Props {
  check: CheckSummary
  onClick?: () => void
}

export function MonitorWidget({ check, onClick }: Props) {
  return (
    <div className="monitor-widget" style={onClick ? { cursor: 'pointer' } : undefined} onClick={onClick}>
      <div className={statusBlockClass(check.status)}>{statusBlockLabel(check)}</div>
      <div className="monitor-info">
        <div className="monitor-name">{check.name}</div>
        <div className="monitor-target">{check.target} · {check.type}</div>
      </div>
      <div className="monitor-meta">
        <div className={uptimeClass(check.status)}>
          {check.uptime_pct.toFixed(1)}%
        </div>
        <div className="monitor-last">{formatEventTime(check.last_checked_at)}</div>
      </div>
    </div>
  )
}
