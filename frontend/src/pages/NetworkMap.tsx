import { useState, useEffect, useCallback } from 'react'
import { Topbar } from '../components/Topbar'
import { InfraNetworkMap } from '../components/InfraNetworkMap'
import { InfraEditModal } from './InfraEditModal'
import {
  infrastructure as infraApi,
  links as linksApi,
  discovery,
  apps as appsApi,
  checks as checksApi,
} from '../api/client'
import type {
  InfrastructureComponent,
  ComponentLink,
  DiscoveredContainer,
  App,
  MonitorCheck,
} from '../api/types'

export function NetworkMap() {
  const [components,  setComponents]  = useState<InfrastructureComponent[]>([])
  const [links,       setLinks]       = useState<ComponentLink[]>([])
  const [containers,  setContainers]  = useState<DiscoveredContainer[]>([])
  const [apps,        setApps]        = useState<App[]>([])
  const [monitors,    setMonitors]    = useState<MonitorCheck[]>([])
  const [loading,     setLoading]     = useState(true)

  const [modalOpen,         setModalOpen]         = useState(false)
  const [openKey,           setOpenKey]           = useState(0)
  const [editingComponent,  setEditingComponent]  = useState<InfrastructureComponent | null>(null)
  const [editingHasCreds,   setEditingHasCreds]   = useState(false)

  useEffect(() => {
    Promise.all([
      infraApi.list(),
      linksApi.list(),
      discovery.allContainers(),
      appsApi.list(),
      checksApi.list(),
    ])
      .then(([infraRes, linkRes, ctrRes, appRes, monRes]) => {
        setComponents(infraRes.data)
        setLinks(linkRes.data)
        setContainers(ctrRes.data)
        setApps(appRes.data)
        setMonitors(monRes.data)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  const openEdit = useCallback((c: InfrastructureComponent) => {
    setEditingComponent(c)
    setEditingHasCreds(c.has_credentials ?? false)
    setOpenKey(k => k + 1)
    setModalOpen(true)
  }, [])

  function closeModal() {
    setModalOpen(false)
    setEditingComponent(null)
    setEditingHasCreds(false)
  }

  return (
    <>
      <Topbar title="Network Map" />
      <div className="content" style={{ display: 'flex', flexDirection: 'column', minHeight: 0, flex: 1 }}>
        {loading ? (
          <div style={{ padding: 40, textAlign: 'center', fontFamily: 'var(--mono)', fontSize: 12, color: 'var(--text3)' }}>
            Loading…
          </div>
        ) : (
          <InfraNetworkMap
            components={components}
            links={links}
            containers={containers}
            apps={apps}
            monitors={monitors}
            onEditComponent={openEdit}
          />
        )}
      </div>

      <InfraEditModal
        key={openKey}
        open={modalOpen}
        component={editingComponent ?? undefined}
        components={components}
        hasCreds={editingHasCreds}
        onSave={async (payload) => {
          if (editingComponent) {
            const updated = await infraApi.update(editingComponent.id, payload)
            setComponents(prev => prev.map(c => c.id === editingComponent.id ? updated : c))
          }
        }}
        onClose={closeModal}
        onDelete={editingComponent ? async () => {
          await infraApi.delete(editingComponent.id)
          setComponents(prev => prev.filter(c => c.id !== editingComponent.id))
        } : undefined}
      />
    </>
  )
}
