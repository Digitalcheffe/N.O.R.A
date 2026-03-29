import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from './components/Layout'
import { Dashboard } from './pages/Dashboard'
import { Events } from './pages/Events'
import { Checks } from './pages/Checks'
import { CheckDetail } from './pages/CheckDetail'
import { Apps } from './pages/Apps'
import { AppDetail } from './pages/AppDetail'
import { Infrastructure } from './pages/Infrastructure'
import { Settings } from './pages/Settings'
import { AppTemplateEditor } from './pages/AppTemplateEditor'
import { Profile } from './pages/Profile'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="events" element={<Events />} />
          <Route path="checks" element={<Checks />} />
          <Route path="checks/:id" element={<CheckDetail />} />
          <Route path="apps" element={<Apps />} />
          <Route path="apps/:id" element={<AppDetail />} />
          <Route path="topology" element={<Infrastructure />} />
          <Route path="settings" element={<Settings />} />
          <Route path="profile" element={<Profile />} />
          <Route path="app-templates/new" element={<AppTemplateEditor />} />
          <Route path="app-templates/:id/edit" element={<AppTemplateEditor />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
