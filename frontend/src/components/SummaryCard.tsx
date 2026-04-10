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
  return (
    <div className="summary-card">
      <div className="summary-label">{item.label}</div>
      <div className={valueColorClass(item.label)}>{item.count}</div>
      <div className="summary-sub">{item.sub}</div>
    </div>
  )
}
