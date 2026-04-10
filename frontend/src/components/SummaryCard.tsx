import type { SummaryBarItem } from '../api/types'

function valueColorClass(label: string): string {
  const l = label.toLowerCase()
  if (l.includes('error') || l.includes('fail') || l.includes('down')) return 'summary-value red'
  if (l.includes('uptime') || l.includes('backup') || l.includes('success')) return 'summary-value green'
  return 'summary-value'
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
