import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import './Layout.css'

export function Layout() {
  return (
    <div className="shell">
      <Sidebar />
      <div className="main">
        <Outlet />
      </div>
    </div>
  )
}
