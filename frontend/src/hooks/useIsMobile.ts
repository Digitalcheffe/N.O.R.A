// Returns true when viewport width < 768px.
// Uses ResizeObserver so it reacts to window resize without a full re-render storm.
import { useState, useEffect } from 'react'

const MOBILE_BREAKPOINT = 768

export function useIsMobile(): boolean {
  const [isMobile, setIsMobile] = useState<boolean>(
    () => window.innerWidth < MOBILE_BREAKPOINT
  )

  useEffect(() => {
    const el = document.documentElement
    const observer = new ResizeObserver(() => {
      setIsMobile(window.innerWidth < MOBILE_BREAKPOINT)
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  return isMobile
}
