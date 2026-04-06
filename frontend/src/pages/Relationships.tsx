import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import {
  apps as appsApi,
  infrastructure as infraApi,
  links as linksApi,
} from '../api/client'
import type { App, ComponentLink, InfrastructureComponent } from '../api/types'
import './Relationships.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

const TYPE_LABEL: Record<string, string> = {
  proxmox_node:   'Proxmox Node',
  vm_linux:       'Linux VM',
  vm_windows:     'Windows VM',
  vm_other:       'VM',
  linux_host:     'Linux Host',
  windows_host:   'Windows Host',
  generic_host:   'Generic Host',
  synology:       'Synology NAS',
  docker_engine:  'Docker Engine',
  traefik:        'Traefik',
  portainer:      'Portainer',
}

function StatusDot({ status }: { status?: string }) {
  const s = status ?? 'unknown'
  const cls =
    s === 'online'  ? 'rel-dot rel-dot--online'  :
    s === 'offline' ? 'rel-dot rel-dot--offline' :
                      'rel-dot rel-dot--unknown'
  return <span className={cls} title={s} />
}

function AppCell({ app }: { app: App }) {
  const [imgFailed, setImgFailed] = useState(false)
  useEffect(() => { setImgFailed(false) }, [app.profile_id])

  return (
    <div className="rel-app-cell">
      <span className="rel-app-icon">
        {app.profile_id && !imgFailed ? (
          <img
            src={`/api/v1/icons/${app.profile_id}`}
            alt={app.name}
            onError={() => setImgFailed(true)}
          />
        ) : (
          <span className="rel-app-monogram">
            {app.name.trim().slice(0, 2).toUpperCase()}
          </span>
        )}
      </span>
      <span className="rel-app-name">{app.name}</span>
    </div>
  )
}

// ── Row type ─────────────────────────────────────────────────────────────────

interface Row {
  app: App
  link?: ComponentLink
  component?: InfrastructureComponent
}

// ── Link selector inline cell ─────────────────────────────────────────────────

interface LinkSelectorProps {
  appId: string
  allComponents: InfrastructureComponent[]
  onLinked: (parentType: string, parentId: string) => void
}

function LinkSelector({ appId, allComponents, onLinked }: LinkSelectorProps) {
  const [open,    setOpen]    = useState(false)
  const [saving,  setSaving]  = useState(false)

  async function handleSelect(c: InfrastructureComponent) {
    setSaving(true)
    try {
      await linksApi.setParent(c.type, c.id, 'app', appId)
      onLinked(c.type, c.id)
    } finally {
      setSaving(false)
      setOpen(false)
    }
  }

  if (!open) {
    return (
      <button
        className="rel-link-btn"
        onClick={e => { e.stopPropagation(); setOpen(true) }}
        disabled={saving}
      >
        + Link
      </button>
    )
  }

  return (
    <div className="rel-link-dropdown" onClick={e => e.stopPropagation()}>
      <div className="rel-link-dropdown-header">
        <span className="rel-dim">Select component</span>
        <button className="rel-link-close" onClick={() => setOpen(false)}>✕</button>
      </div>
      {allComponents.length === 0 ? (
        <div className="rel-link-empty rel-dim">No components available</div>
      ) : (
        <div className="rel-link-options">
          {allComponents.map(c => (
            <button
              key={c.id}
              className="rel-link-option"
              onClick={() => void handleSelect(c)}
              disabled={saving}
            >
              <span className="rel-link-option-name">{c.name}</span>
              <span className="rel-dim">{TYPE_LABEL[c.type] ?? c.type}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export function Relationships() {
  const navigate = useNavigate()
  const [rows,       setRows]       = useState<Row[]>([])
  const [allComponents, setAllComponents] = useState<InfrastructureComponent[]>([])
  const [loading,    setLoading]    = useState(true)
  const [error,      setError]      = useState('')
  const [filter,     setFilter]     = useState<'all' | 'linked' | 'unlinked'>('all')
  const [unlinking,  setUnlinking]  = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      setError('')
      try {
        const [appRes, linkRes, infraRes] = await Promise.all([
          appsApi.list(),
          linksApi.list(),
          infraApi.list(),
        ])
        if (cancelled) return

        const linksByApp = new Map<string, ComponentLink>()
        for (const l of linkRes.data) {
          if (l.child_type === 'app') linksByApp.set(l.child_id, l)
        }

        const infraById = new Map<string, InfrastructureComponent>()
        for (const c of infraRes.data) infraById.set(c.id, c)

        const built: Row[] = appRes.data.map(app => {
          const link = linksByApp.get(app.id)
          const component = link ? infraById.get(link.parent_id) : undefined
          return { app, link, component }
        })

        built.sort((a, b) => a.app.name.localeCompare(b.app.name))
        setRows(built)
        setAllComponents(infraRes.data)
      } catch (e) {
        if (!cancelled) setError(String(e))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    load()
    return () => { cancelled = true }
  }, [])

  function handleLinked(appId: string, parentType: string, parentId: string) {
    const component = allComponents.find(c => c.id === parentId)
    setRows(prev => prev.map(r => {
      if (r.app.id !== appId) return r
      const link: ComponentLink = {
        parent_type: parentType,
        parent_id:   parentId,
        child_type:  'app',
        child_id:    appId,
        created_at:  new Date().toISOString(),
      }
      return { ...r, link, component }
    }))
  }

  async function handleUnlink(e: React.MouseEvent, appId: string) {
    e.stopPropagation()
    setUnlinking(appId)
    try {
      await linksApi.removeParent('app', appId)
      setRows(prev => prev.map(r =>
        r.app.id === appId ? { app: r.app } : r
      ))
    } finally {
      setUnlinking(null)
    }
  }

  const visible = rows.filter(r => {
    if (filter === 'linked')   return !!r.link
    if (filter === 'unlinked') return !r.link
    return true
  })

  const linkedCount   = rows.filter(r => !!r.link).length
  const unlinkedCount = rows.filter(r => !r.link).length

  return (
    <>
      <Topbar title="Relationships" />
      <div className="content">

      <div className="rel-controls">
        <div className="rel-filter-group">
          {(['all', 'linked', 'unlinked'] as const).map(f => (
            <button
              key={f}
              className={`rel-filter-btn${filter === f ? ' active' : ''}`}
              onClick={() => setFilter(f)}
            >
              {f === 'all'      && `All (${rows.length})`}
              {f === 'linked'   && `Linked (${linkedCount})`}
              {f === 'unlinked' && `Unlinked (${unlinkedCount})`}
            </button>
          ))}
        </div>
      </div>

      {loading && (
        <div className="rel-state">Loading…</div>
      )}

      {error && (
        <div className="rel-state rel-state--error">{error}</div>
      )}

      {!loading && !error && visible.length === 0 && (
        <div className="rel-state">
          {rows.length === 0 ? 'No apps found.' : 'No apps match this filter.'}
        </div>
      )}

      {!loading && !error && visible.length > 0 && (
        <div className="rel-table-wrap">
          <table className="rel-table">
            <thead>
              <tr>
                <th>App</th>
                <th>Profile</th>
                <th>Infrastructure</th>
                <th>Type</th>
                <th>Status</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {visible.map(({ app, link, component }) => (
                <tr
                  key={app.id}
                  className="rel-row"
                  onClick={() => navigate(`/apps/${app.id}`)}
                  title={`Open ${app.name}`}
                >
                  <td><AppCell app={app} /></td>

                  <td className="rel-mono">
                    {app.profile_id
                      ? <span className="rel-profile-badge">{app.profile_id}</span>
                      : <span className="rel-dim">—</span>
                    }
                  </td>

                  <td>
                    {component ? (
                      <button
                        className="rel-infra-btn"
                        onClick={e => {
                          e.stopPropagation()
                          navigate(`/infrastructure/${component.id}`)
                        }}
                      >
                        {component.name}
                      </button>
                    ) : link ? (
                      <span className="rel-dim">{link.parent_id}</span>
                    ) : (
                      <LinkSelector
                        appId={app.id}
                        allComponents={allComponents}
                        onLinked={(pt, pid) => handleLinked(app.id, pt, pid)}
                      />
                    )}
                  </td>

                  <td className="rel-mono rel-type">
                    {component
                      ? (TYPE_LABEL[component.type] ?? component.type)
                      : <span className="rel-dim">—</span>
                    }
                  </td>

                  <td>
                    <div className="rel-status-cell">
                      <StatusDot status={component?.last_status} />
                      <span className="rel-status-label rel-dim">
                        {component?.last_status ?? '—'}
                      </span>
                    </div>
                  </td>

                  <td className="rel-action-cell">
                    {link && (
                      <button
                        className="rel-unlink-btn"
                        title="Unlink"
                        disabled={unlinking === app.id}
                        onClick={e => void handleUnlink(e, app.id)}
                      >
                        {unlinking === app.id ? '…' : 'Unlink'}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      </div>
    </>
  )
}
