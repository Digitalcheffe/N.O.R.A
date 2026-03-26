import './Topbar.css'

type TimeFilter = 'day' | 'week' | 'month'
type OverallStatus = 'ok' | 'warn' | 'down'

interface TopbarProps {
  title: string
  status?: OverallStatus
  statusLabel?: string
  timeFilter?: TimeFilter
  onTimeFilter?: (f: TimeFilter) => void
  onAdd?: () => void
}

const STATUS_LABELS: Record<OverallStatus, string> = {
  ok: 'All systems normal',
  warn: 'Degraded',
  down: 'Outage detected',
}

export function Topbar({
  title,
  status = 'ok',
  statusLabel,
  timeFilter = 'week',
  onTimeFilter,
  onAdd,
}: TopbarProps) {
  return (
    <div className="topbar">
      <span className="topbar-title">{title}</span>

      <div className={`topbar-status status-${status}`}>
        <div className={`status-dot${status !== 'ok' ? ` ${status}` : ''}`} />
        <span>{statusLabel ?? STATUS_LABELS[status]}</span>
      </div>

      <div className="topbar-right">
        {onTimeFilter && (
          <div className="time-filter">
            {(['day', 'week', 'month'] as TimeFilter[]).map((f) => (
              <button
                key={f}
                className={`time-btn${timeFilter === f ? ' active' : ''}`}
                onClick={() => onTimeFilter(f)}
              >
                {f.charAt(0).toUpperCase() + f.slice(1)}
              </button>
            ))}
          </div>
        )}

        {onAdd && (
          <button className="icon-btn" title="Add" onClick={onAdd}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
          </button>
        )}
      </div>
    </div>
  )
}
