// Returns true when any monitor check has status 'warn' or 'down'.
// Polls the dashboard summary every 60 seconds.
import { useState, useEffect } from 'react'
import { dashboard as dashboardApi } from '../api/client'

export function useMonitorAlerts(): boolean {
  const [hasAlerts, setHasAlerts] = useState(false)

  useEffect(() => {
    function fetchAlerts() {
      dashboardApi
        .summary('week')
        .then(data => {
          const alert = data.checks.some(
            c => c.status === 'warn' || c.status === 'down'
          )
          setHasAlerts(alert)
        })
        .catch(() => {/* silently ignore */})
    }

    fetchAlerts()
    const id = setInterval(fetchAlerts, 60_000)
    return () => clearInterval(id)
  }, [])

  return hasAlerts
}
