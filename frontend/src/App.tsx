import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from './components/Layout'
import { Dashboard } from './pages/Dashboard'
import { Events } from './pages/Events'
import { Checks } from './pages/Checks'
import { Apps } from './pages/Apps'
import { AppDetail } from './pages/AppDetail'
import { Topology } from './pages/Topology'
import { Settings } from './pages/Settings'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="events" element={<Events />} />
          <Route path="checks" element={<Checks />} />
          <Route path="apps" element={<Apps />} />
          <Route path="apps/:id" element={<AppDetail />} />
          <Route path="topology" element={<Topology />} />
          <Route path="settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
