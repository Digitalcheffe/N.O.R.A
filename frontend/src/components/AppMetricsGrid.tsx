import type { AppMetricSnapshot } from '../api/types'
import { AppMetricCard, AppMetricCardSkeleton } from './AppMetricCard'
import './AppMetricsGrid.css'

interface Props {
  metrics: AppMetricSnapshot[]
  loading?: boolean
}

export function AppMetricsGrid({ metrics, loading }: Props) {
  if (!loading && metrics.length === 0) return null

  return (
    <div className="amg-section">
      {!loading && (
        <div className="amg-label">Live Metrics</div>
      )}
      <div className="amg-grid">
        {loading ? (
          [0, 1, 2].map(i => <AppMetricCardSkeleton key={i} />)
        ) : (
          metrics.map(m => <AppMetricCard key={m.id} metric={m} />)
        )}
      </div>
    </div>
  )
}
