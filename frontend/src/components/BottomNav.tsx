import { useState } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { useMonitorAlerts } from '../hooks/useMonitorAlerts'
import './BottomNav.css'

export function BottomNav() {
  const hasAlerts = useMonitorAlerts()
  const [sheetOpen, setSheetOpen] = useState(false)
  const navigate = useNavigate()

  return (
    <>
      <nav className="bottom-nav">
        {/* Overview */}
        <NavLink to="/" end className={({ isActive }) => `bottom-tab${isActive ? ' active' : ''}`}>
          <div className="bottom-tab-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <rect x="3" y="3" width="7" height="7" />
              <rect x="14" y="3" width="7" height="7" />
              <rect x="3" y="14" width="7" height="7" />
              <rect x="14" y="14" width="7" height="7" />
            </svg>
          </div>
          <span className="bottom-tab-label">Overview</span>
        </NavLink>

        {/* Apps */}
        <NavLink to="/apps" className={({ isActive }) => `bottom-tab${isActive ? ' active' : ''}`}>
          <div className="bottom-tab-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <rect x="2" y="3" width="20" height="14" rx="2" />
              <line x1="8" y1="21" x2="16" y2="21" />
              <line x1="12" y1="17" x2="12" y2="21" />
            </svg>
          </div>
          <span className="bottom-tab-label">Apps</span>
        </NavLink>

        {/* Monitor */}
        <NavLink to="/checks" className={({ isActive }) => `bottom-tab${isActive ? ' active' : ''}`}>
          <div className="bottom-tab-icon">
            {hasAlerts && <span className="bottom-tab-badge" />}
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
            </svg>
          </div>
          <span className="bottom-tab-label">Monitor</span>
        </NavLink>

        {/* Hosts */}
        <NavLink to="/infrastructure" className={({ isActive }) => `bottom-tab${isActive ? ' active' : ''}`}>
          <div className="bottom-tab-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <rect x="2" y="2" width="20" height="8" rx="2" />
              <rect x="2" y="14" width="20" height="8" rx="2" />
              <line x1="6" y1="6" x2="6.01" y2="6" />
              <line x1="6" y1="18" x2="6.01" y2="18" />
            </svg>
          </div>
          <span className="bottom-tab-label">Hosts</span>
        </NavLink>

        {/* More */}
        <button
          className={`bottom-tab${sheetOpen ? ' active' : ''}`}
          onClick={() => setSheetOpen(true)}
        >
          <div className="bottom-tab-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <circle cx="5" cy="12" r="1" fill="currentColor" />
              <circle cx="12" cy="12" r="1" fill="currentColor" />
              <circle cx="19" cy="12" r="1" fill="currentColor" />
            </svg>
          </div>
          <span className="bottom-tab-label">More</span>
        </button>
      </nav>

      {/* Slide-up sheet */}
      {sheetOpen && (
        <div className="bottom-sheet-backdrop" onClick={() => setSheetOpen(false)}>
          <div className="bottom-sheet" onClick={e => e.stopPropagation()}>
            <div className="bottom-sheet-handle" />
            <div className="bottom-sheet-title">More</div>
            <div className="bottom-sheet-items">
              <button
                className="bottom-sheet-item"
                onClick={() => { navigate('/settings?tab=apps'); setSheetOpen(false) }}
              >
                <div className="bottom-sheet-item-icon">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                    <circle cx="12" cy="12" r="3" />
                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
                  </svg>
                </div>
                <span>Settings</span>
                <svg className="bottom-sheet-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                  <polyline points="9 18 15 12 9 6" />
                </svg>
              </button>

              <button
                className="bottom-sheet-item"
                onClick={() => { navigate('/profile'); setSheetOpen(false) }}
              >
                <div className="bottom-sheet-item-icon">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                    <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
                    <circle cx="12" cy="7" r="4" />
                  </svg>
                </div>
                <span>Profile</span>
                <svg className="bottom-sheet-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                  <polyline points="9 18 15 12 9 6" />
                </svg>
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
