import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AutoRefreshProvider } from './context/AutoRefreshContext'
import { AuthProvider } from './context/AuthContext'
import { EnvStatusProvider } from './context/EnvStatusContext'
import { AuthGuard } from './components/AuthGuard'
import { Layout } from './components/Layout'
import { Login } from './pages/Login'
import { Setup } from './pages/Setup'
import { Dashboard } from './pages/Dashboard'
import { Events } from './pages/Events'
import { Checks } from './pages/Checks'
import { CheckDetail } from './pages/CheckDetail'
import { Apps } from './pages/Apps'
import { AppDetail } from './pages/AppDetail'
import { Infrastructure } from './pages/Infrastructure'
import { InfraComponentDetail } from './pages/InfraComponentDetail'
import { SynologyDetail } from './pages/SynologyDetail'
import { TraefikDetail } from './pages/TraefikDetail'
import { Settings } from './pages/Settings'
import { AppTemplateEditor } from './pages/AppTemplateEditor'
import { Profile } from './pages/Profile'

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AutoRefreshProvider>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route path="/setup" element={<Setup />} />
            <Route element={
              <AuthGuard>
                <EnvStatusProvider>
                  <Layout />
                </EnvStatusProvider>
              </AuthGuard>
            }>
              <Route index element={<Dashboard />} />
              <Route path="events" element={<Events />} />
              <Route path="checks" element={<Checks />} />
              <Route path="checks/:id" element={<CheckDetail />} />
              <Route path="apps" element={<Apps />} />
              <Route path="apps/:id" element={<AppDetail />} />
              <Route path="infrastructure" element={<Infrastructure />} />
              <Route path="infrastructure/synology/:componentId" element={<SynologyDetail />} />
              <Route path="infrastructure/traefik/:componentId" element={<TraefikDetail />} />
<Route path="infrastructure/:id" element={<InfraComponentDetail />} />
              <Route path="settings" element={<Settings />} />
              <Route path="profile" element={<Profile />} />
              <Route path="app-templates/new" element={<AppTemplateEditor />} />
              <Route path="app-templates/:id/edit" element={<AppTemplateEditor />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Route>
          </Routes>
        </AutoRefreshProvider>
      </AuthProvider>
    </BrowserRouter>
  )
}
