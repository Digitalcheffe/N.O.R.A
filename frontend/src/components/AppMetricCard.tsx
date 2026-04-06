import type { AppMetricSnapshot } from '../api/types'
import './AppMetricCard.css'

// Returns null for invalid or Go zero-value timestamps ("0001-01-01...").
function parsePollDate(dateStr: string): Date | null {
  const d = new Date(dateStr)
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return null
  return d
}

function formatRelativeTime(dateStr: string): string {
  const d = parsePollDate(dateStr)
  if (!d) return 'pending'
  const diff = Date.now() - d.getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'updated just now'
  if (mins < 60) return `updated ${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `updated ${hrs}h ago`
  return `updated ${Math.floor(hrs / 24)}d ago`
}

function isStale(dateStr: string): boolean {
  const d = parsePollDate(dateStr)
  if (!d) return false
  return Date.now() - d.getTime() > 2 * 60 * 60 * 1000
}

function displayValue(metric: AppMetricSnapshot): string {
  if (!metric.value) return '—'
  if (metric.value_type === 'list') {
    try {
      const parsed: unknown = JSON.parse(metric.value)
      if (Array.isArray(parsed)) return String(parsed.length)
    } catch {
      // fall through to raw value
    }
  }
  return metric.value
}

interface Props {
  metric: AppMetricSnapshot
}

export function AppMetricCard({ metric }: Props) {
  const val = displayValue(metric)
  const isEmpty = !metric.value
  const stale = isStale(metric.polled_at)
  const relTime = formatRelativeTime(metric.polled_at)

  return (
    <div className="amc-card">
      <div
        className="amc-value"
        title={isEmpty ? 'Waiting for first poll' : undefined}
      >
        {val}
      </div>
      <div className="amc-label">{metric.label}</div>
      <div className={`amc-timestamp${stale ? ' amc-timestamp--stale' : ''}`}>
        {relTime}
      </div>
    </div>
  )
}

export function AppMetricCardSkeleton() {
  return <div className="amc-card amc-card--skeleton skeleton" />
}
