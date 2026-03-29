import { useState, useEffect, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { discovery, apps as appsApi, appTemplates as templatesApi } from '../api/client'
import type { DiscoveredContainer, App, AppTemplate } from '../api/types'

// ── Props ─────────────────────────────────────────────────────────────────────

interface Props {
  engineId: string
  onCountsLoaded: (total: number, unlinked: number) => void
}

// ── Link form state ───────────────────────────────────────────────────────────

type LinkMode = 'create' | 'existing'

interface LinkFormState {
  containerId: string
  mode: LinkMode
  name: string
  profileId: string
  baseUrl: string
  apiKey: string
  appId: string
  submitting: boolean
  error: string | null
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function containerDotClass(status: string): string {
  if (status === 'running') return 'green'
  if (status === 'exited')  return 'red'
  return 'grey'
}

function timeAgo(dateStr: string): string {
  const diff = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000)
  if (diff < 60)   return `${diff}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  return `${Math.floor(diff / 3600)}h ago`
}

// ── Mini resource bar ─────────────────────────────────────────────────────────

function MiniBar({ value, label }: { value: number | null; label: string }) {
  const pct     = value ?? 0
  const noData  = value === null
  const fillCls = noData ? 'no-data' : pct >= 90 ? 'crit' : pct >= 70 ? 'warn' : ''
  return (
    <div className="de-res-row">
      <span className="de-res-label">{label}</span>
      <div className="de-res-track">
        <div
          className={`de-res-fill${fillCls ? ` ${fillCls}` : ''}`}
          style={{ width: noData ? '0%' : `${Math.min(pct, 100)}%` }}
        />
      </div>
      <span className={`de-res-pct${noData ? ' no-data' : ''}`}>
        {noData ? '—' : `${Math.round(pct)}%`}
      </span>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function DockerEngineDetail({ engineId, onCountsLoaded }: Props) {
  const navigate = useNavigate()

  const [containers, setContainers] = useState<DiscoveredContainer[]>([])
  const [apps,       setApps]       = useState<App[]>([])
  const [templates,  setTemplates]  = useState<AppTemplate[]>([])
  const [loading,    setLoading]    = useState(true)
  const [linkForm,   setLinkForm]   = useState<LinkFormState | null>(null)

  // Keep a stable ref to onCountsLoaded so it never triggers a re-fetch
  const onCountsLoadedRef = useRef(onCountsLoaded)
  onCountsLoadedRef.current = onCountsLoaded

  const load = useCallback(() => {
    setLoading(true)
    Promise.all([
      discovery.containers(engineId),
      appsApi.list(),
      templatesApi.list(),
    ])
      .then(([ctrs, appList, tmplList]) => {
        setContainers(ctrs.data)
        setApps(appList.data)
        setTemplates(tmplList.data)
        const unlinked = ctrs.data.filter(c => !c.app_id).length
        onCountsLoadedRef.current(ctrs.data.length, unlinked)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [engineId]) // engineId only — onCountsLoaded via ref to avoid re-fetch loop

  useEffect(() => { load() }, [load])

  // App IDs already linked to a discovered container (exclude from "use existing")
  const linkedAppIds = new Set(
    containers.filter(c => c.app_id).map(c => c.app_id as string)
  )

  // ── Link form handlers ──────────────────────────────────────────────────────

  function openLinkForm(container: DiscoveredContainer) {
    setLinkForm({
      containerId: container.id,
      mode:        container.profile_suggestion ? 'create' : 'create',
      name:        container.container_name.replace(/^\//, ''),
      profileId:   container.profile_suggestion ?? '',
      baseUrl:     '',
      apiKey:      '',
      appId:       '',
      submitting:  false,
      error:       null,
    })
  }

  function closeLinkForm() {
    setLinkForm(null)
  }

  async function submitLink() {
    if (!linkForm) return
    setLinkForm(prev => prev && { ...prev, submitting: true, error: null })
    try {
      if (linkForm.mode === 'existing') {
        if (!linkForm.appId) {
          setLinkForm(prev => prev && { ...prev, submitting: false, error: 'Select an app' })
          return
        }
        await discovery.linkContainerApp(linkForm.containerId, {
          mode:   'existing',
          app_id: linkForm.appId,
        })
      } else {
        if (!linkForm.name.trim()) {
          setLinkForm(prev => prev && { ...prev, submitting: false, error: 'App name is required' })
          return
        }
        if (!linkForm.profileId.trim()) {
          setLinkForm(prev => prev && { ...prev, submitting: false, error: 'Profile is required' })
          return
        }
        const cfg: Record<string, unknown> = {}
        if (linkForm.baseUrl.trim()) cfg.base_url = linkForm.baseUrl.trim()
        if (linkForm.apiKey.trim())  cfg.api_key  = linkForm.apiKey.trim()
        await discovery.linkContainerApp(linkForm.containerId, {
          mode:       'create',
          name:       linkForm.name.trim(),
          profile_id: linkForm.profileId.trim(),
          config:     Object.keys(cfg).length > 0 ? cfg : undefined,
        })
      }
      setLinkForm(null)
      load()
    } catch (err: unknown) {
      setLinkForm(prev => prev && {
        ...prev,
        submitting: false,
        error: err instanceof Error ? err.message : 'Failed to link',
      })
    }
  }

  // ── Render ──────────────────────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="de-detail">
        <div className="de-loading">Loading containers…</div>
      </div>
    )
  }

  if (containers.length === 0) {
    return (
      <div className="de-detail">
        <div className="de-empty">No containers discovered yet.</div>
      </div>
    )
  }

  return (
    <div className="de-detail">
      {containers.map(c => {
        const isLinked   = !!c.app_id
        const hasSugg    = !isLinked && !!c.profile_suggestion
        const isUnlinked = !isLinked && !c.profile_suggestion
        const isFormOpen = linkForm?.containerId === c.id
        const dotCls     = containerDotClass(c.status)
        const linkedApp  = isLinked ? apps.find(a => a.id === c.app_id) : undefined

        return (
          <div key={c.id} className="de-container-block">

            {/* ── Container row ── */}
            <div className={`de-container-row${isFormOpen ? ' form-open' : ''}`}>
              <span className={`de-dot ${dotCls}`} />
              <span className="de-name">{c.container_name}</span>

              {isLinked && (
                <>
                  <button
                    className="de-app-chip"
                    onClick={() => navigate(`/apps/${c.app_id}`)}
                  >
                    {linkedApp?.name ?? c.app_id}
                  </button>
                  <div className="de-res-mini">
                    <MiniBar value={c.cpu_percent} label="CPU" />
                    <MiniBar value={c.mem_percent} label="MEM" />
                  </div>
                  <span className="de-last-seen">{timeAgo(c.last_seen_at)}</span>
                </>
              )}

              {hasSugg && (
                <>
                  <span className="de-suggestion-badge">Looks like {c.profile_suggestion}</span>
                  <div className="de-res-mini">
                    <MiniBar value={c.cpu_percent} label="CPU" />
                    <MiniBar value={c.mem_percent} label="MEM" />
                  </div>
                  <button
                    className="de-link-btn accent"
                    onClick={() => isFormOpen ? closeLinkForm() : openLinkForm(c)}
                  >
                    {isFormOpen ? 'Cancel' : 'Add App'}
                  </button>
                </>
              )}

              {isUnlinked && (
                <>
                  <span className="de-image">{c.image}</span>
                  <div className="de-res-mini">
                    <MiniBar value={c.cpu_percent} label="CPU" />
                    <MiniBar value={c.mem_percent} label="MEM" />
                  </div>
                  <button
                    className="de-link-btn"
                    onClick={() => isFormOpen ? closeLinkForm() : openLinkForm(c)}
                  >
                    {isFormOpen ? 'Cancel' : 'Link Manually'}
                  </button>
                </>
              )}
            </div>

            {/* ── Inline link form ── */}
            {isFormOpen && linkForm && (
              <div className="de-link-form">

                {/* Tab switcher */}
                <div className="de-link-tabs">
                  <button
                    className={`de-link-tab${linkForm.mode === 'create' ? ' active' : ''}`}
                    onClick={() => setLinkForm(prev => prev && { ...prev, mode: 'create' })}
                  >
                    Create New
                  </button>
                  <button
                    className={`de-link-tab${linkForm.mode === 'existing' ? ' active' : ''}`}
                    onClick={() => setLinkForm(prev => prev && { ...prev, mode: 'existing' })}
                  >
                    Use Existing
                  </button>
                </div>

                {/* Create new fields */}
                {linkForm.mode === 'create' && (
                  <div className="de-link-fields">
                    <div className="de-link-field">
                      <div className="de-link-label">App Name</div>
                      <input
                        className="de-link-input"
                        value={linkForm.name}
                        onChange={e => setLinkForm(prev => prev && { ...prev, name: e.target.value })}
                        placeholder="e.g. my-app"
                      />
                    </div>
                    <div className="de-link-field">
                      <div className="de-link-label">Profile</div>
                      {templates.length > 0 ? (
                        <select
                          className="de-link-input"
                          value={linkForm.profileId}
                          onChange={e => setLinkForm(prev => prev && { ...prev, profileId: e.target.value })}
                        >
                          <option value="">— select profile —</option>
                          {templates.map(t => (
                            <option key={t.id} value={t.id}>{t.name}</option>
                          ))}
                        </select>
                      ) : (
                        <input
                          className="de-link-input"
                          value={linkForm.profileId}
                          onChange={e => setLinkForm(prev => prev && { ...prev, profileId: e.target.value })}
                          placeholder="profile id"
                        />
                      )}
                    </div>
                    <div className="de-link-field">
                      <div className="de-link-label">
                        Base URL <span className="de-link-optional">(optional)</span>
                      </div>
                      <input
                        className="de-link-input"
                        value={linkForm.baseUrl}
                        onChange={e => setLinkForm(prev => prev && { ...prev, baseUrl: e.target.value })}
                        placeholder="https://…"
                      />
                    </div>
                    <div className="de-link-field">
                      <div className="de-link-label">
                        API Key <span className="de-link-optional">(optional)</span>
                      </div>
                      <input
                        className="de-link-input"
                        type="password"
                        value={linkForm.apiKey}
                        onChange={e => setLinkForm(prev => prev && { ...prev, apiKey: e.target.value })}
                        placeholder="••••••••"
                      />
                    </div>
                  </div>
                )}

                {/* Use existing fields */}
                {linkForm.mode === 'existing' && (
                  <div className="de-link-fields">
                    <div className="de-link-field de-link-field-full">
                      <div className="de-link-label">App</div>
                      <select
                        className="de-link-input"
                        value={linkForm.appId}
                        onChange={e => setLinkForm(prev => prev && { ...prev, appId: e.target.value })}
                      >
                        <option value="">— select app —</option>
                        {apps
                          .filter(a => !linkedAppIds.has(a.id))
                          .map(a => (
                            <option key={a.id} value={a.id}>{a.name}</option>
                          ))}
                      </select>
                    </div>
                  </div>
                )}

                {linkForm.error && (
                  <div className="de-link-error">{linkForm.error}</div>
                )}

                <div className="de-link-actions">
                  <button
                    className="de-link-submit"
                    onClick={() => void submitLink()}
                    disabled={linkForm.submitting}
                  >
                    {linkForm.submitting ? 'Linking…' : 'Link'}
                  </button>
                  <button className="de-link-cancel" onClick={closeLinkForm}>
                    Cancel
                  </button>
                </div>
              </div>
            )}

          </div>
        )
      })}
    </div>
  )
}
