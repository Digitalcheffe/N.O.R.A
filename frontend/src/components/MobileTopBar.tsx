import { useLocation } from 'react-router-dom'
import { useEnvStatus } from '../context/EnvStatusContext'
import './MobileTopBar.css'

const ROUTE_TITLES: Record<string, string> = {
  '/':          'Overview',
  '/apps':      'Apps',
  '/checks':    'Monitor',
  '/events':    'Events',
  '/infrastructure':  'Hosts',
  '/settings':  'Settings',
  '/profile':   'Profile',
}

function pageTitle(pathname: string): string {
  if (pathname.startsWith('/apps/')) return 'App Detail'
  return ROUTE_TITLES[pathname] ?? 'NORA'
}

export function MobileTopBar() {
  const location = useLocation()
  const status = useEnvStatus()
  const title = pageTitle(location.pathname)

  return (
    <div className="mobile-topbar">
      <div className="mobile-topbar-logo" title="NORA">N</div>

      <span className="mobile-topbar-title">{title}</span>

      <div className="mobile-topbar-right">
        <div className={`mobile-status-pill status-${status}`}>
          <div className={`mobile-status-dot${status !== 'ok' ? ` ${status}` : ''}`} />
        </div>

        <button className="mobile-bell" title="Notifications">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
            <path d="M13.73 21a2 2 0 0 1-3.46 0" />
          </svg>
        </button>
      </div>
    </div>
  )
}
