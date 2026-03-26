import type { SummaryBarItem } from '../api/types'

function sparklineColor(label: string): string {
  const l = label.toLowerCase()
  if (l.includes('error') || l.includes('fail') || l.includes('down')) return 'var(--red)'
  if (l.includes('uptime') || l.includes('backup') || l.includes('success')) return 'var(--green)'
  return 'var(--accent)'
}

function valueColorClass(label: string): string {
  const l = label.toLowerCase()
  if (l.includes('error') || l.includes('fail') || l.includes('down')) return 'summary-value red'
  if (l.includes('uptime') || l.includes('backup') || l.includes('success')) return 'summary-value green'
  return 'summary-value'
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
  item: SummaryBarItem
}

export function SummaryCard({ item }: Props) {
  const color = sparklineColor(item.label)
  const pts = sparklinePoints(item.sparkline, 120, 24)
  const closedPts = pts ? `${pts} 120,24 0,24` : ''

  return (
    <div className="summary-card">
      <div className="summary-label">{item.label}</div>
      <div className={valueColorClass(item.label)}>{item.count}</div>
      <div className="summary-sub">{item.sub}</div>
      {pts && (
        <svg className="sparkline" viewBox="0 0 120 24" preserveAspectRatio="none">
          <polyline points={pts} fill="none" stroke={color} strokeWidth="1.5" opacity="0.8" />
          <polyline points={closedPts} fill={color} stroke="none" opacity="0.08" />
        </svg>
      )}
    </div>
  )
}
