import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { MobileTopBar } from './MobileTopBar'
import { BottomNav } from './BottomNav'
import { useIsMobile } from '../hooks/useIsMobile'
import './Layout.css'

export function Layout() {
  const isMobile = useIsMobile()

  if (isMobile) {
    return (
      <>
        <MobileTopBar />
        <main className="mobile-scroll-area">
          <Outlet />
        </main>
        <BottomNav />
      </>
    )
  }

  return (
    <div className="shell">
      <Sidebar />
      <div className="main">
        <Outlet />
      </div>
    </div>
  )
}
