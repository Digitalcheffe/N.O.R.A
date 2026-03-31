import { createContext, useContext, useEffect, useRef, useState } from 'react'

export type RefreshInterval = 0 | 5 | 10 | 30

interface AutoRefreshContextValue {
  interval: RefreshInterval
  setInterval: (v: RefreshInterval) => void
  tick: number
}

const AutoRefreshContext = createContext<AutoRefreshContextValue>({
  interval: 0,
  setInterval: () => {},
  tick: 0,
})

export function AutoRefreshProvider({ children }: { children: React.ReactNode }) {
  const [interval, setIntervalValue] = useState<RefreshInterval>(0)
  const [tick, setTick] = useState(0)
  const timerRef = useRef<ReturnType<typeof globalThis.setInterval> | null>(null)

  useEffect(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current)
      timerRef.current = null
    }
    if (interval > 0) {
      timerRef.current = globalThis.setInterval(() => {
        setTick(t => t + 1)
      }, interval * 1000)
    }
    return () => {
      if (timerRef.current) clearInterval(timerRef.current)
    }
  }, [interval])

  return (
    <AutoRefreshContext.Provider value={{ interval, setInterval: setIntervalValue, tick }}>
      {children}
    </AutoRefreshContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAutoRefresh(): AutoRefreshContextValue {
  return useContext(AutoRefreshContext)
}
