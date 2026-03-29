import { useAutoRefresh, type RefreshInterval } from '../context/AutoRefreshContext'
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
  ok: 'Healthy',
  warn: 'Warning',
  down: 'Alert',
}

const REFRESH_OPTIONS: { value: RefreshInterval; label: string }[] = [
  { value: 0,  label: 'Off' },
  { value: 5,  label: '5s' },
  { value: 10, label: '10s' },
  { value: 30, label: '30s' },
]

export function Topbar({
  title,
  status = 'ok',
  statusLabel,
  timeFilter = 'week',
  onTimeFilter,
  onAdd,
}: TopbarProps) {
  const { interval, setInterval } = useAutoRefresh()

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

        <div className="auto-refresh">
          <svg className="auto-refresh-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
            <path d="M3 3v5h5" />
          </svg>
          <select
            className="auto-refresh-select"
            value={interval}
            onChange={e => setInterval(Number(e.target.value) as RefreshInterval)}
            title="Auto refresh interval"
          >
            {REFRESH_OPTIONS.map(o => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </div>
      </div>
    </div>
  )
}
