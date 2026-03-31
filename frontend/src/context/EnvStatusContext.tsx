import { createContext, useContext, useState, useEffect } from 'react'
import { useAutoRefresh } from './AutoRefreshContext'
import { dashboard as dashboardApi } from '../api/client'

type EnvStatus = 'ok' | 'warn' | 'down'

const EnvStatusContext = createContext<EnvStatus>('ok')

export function EnvStatusProvider({ children }: { children: React.ReactNode }) {
  const { tick } = useAutoRefresh()
  const [status, setStatus] = useState<EnvStatus>('ok')

  useEffect(() => {
    void (async () => {
      try {
        const data = await dashboardApi.summary('week')
        setStatus(data.status === 'normal' ? 'ok' : data.status)
      } catch {
        // Keep previous status on error — don't flash red on a blip
      }
    })()
  }, [tick])

  return (
    <EnvStatusContext.Provider value={status}>
      {children}
    </EnvStatusContext.Provider>
  )
}

export function useEnvStatus(): EnvStatus {
  return useContext(EnvStatusContext)
}
