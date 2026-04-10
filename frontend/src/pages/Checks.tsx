import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAutoRefresh } from '../context/AutoRefreshContext'
import { Topbar } from '../components/Topbar'
import { checks as checksApi, apps as appsApi, appTemplates as appTemplatesApi } from '../api/client'
import type { App, MonitorCheck } from '../api/types'
import { CheckForm } from '../components/CheckForm'
import { SlidePanel } from '../components/SlidePanel'
import {
  type FormFields,
  defaultForm,
  validateForm,
  formToInput,
} from '../components/checkFormHelpers'
import { formatEventTime } from '../utils/formatTime'
import './Checks.css'

// ── Helpers ───────────────────────────────────────────────────────────────────

function sslDaysFromResult(lastResult: string | null): number | null {
  if (!lastResult) return null
  try {
    const r = JSON.parse(lastResult) as { days_remaining?: number }
    return r.days_remaining ?? null
  } catch {
    return null
  }
}

function statusLabel(check: MonitorCheck): string {
  if (check.type === 'ssl') {
    const days = sslDaysFromResult(check.last_result)
    if (days != null) return `${days}d`
    return 'SSL'
  }
  if (check.last_status === 'up') return 'UP'
  if (check.last_status === 'down') return 'DOWN'
  if (check.last_status === 'warn') return 'WARN'
  return '—'
}

function statusClass(status: string | null): string {
  if (status === 'up') return 'monitor-status-block up'
  if (status === 'warn') return 'monitor-status-block warn'
  if (status === 'down' || status === 'critical') return 'monitor-status-block down'
  return 'monitor-status-block unknown'
}


// ── Main page ─────────────────────────────────────────────────────────────────

type StatusFilter = 'all' | 'up' | 'down' | 'paused'

export function Checks() {
  const navigate = useNavigate()
  const { tick } = useAutoRefresh()
  const [checkList, setCheckList] = useState<MonitorCheck[]>([])
  const [loading, setLoading] = useState(true)

  const [appList, setAppList] = useState<App[]>([])

  // Filter state
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [typeFilter, setTypeFilter] = useState<string>('all')

  // Add form
  const [showAddForm, setShowAddForm] = useState(false)
  const [addForm, setAddForm] = useState<FormFields>(defaultForm)
  const [addError, setAddError] = useState<string | null>(null)
  const [addSubmitting, setAddSubmitting] = useState(false)
  const [targetSuggestion, setTargetSuggestion] = useState<string>('')

  // Action state
  const [runningIds, setRunningIds] = useState<Set<string>>(new Set())

  function closeAddForm() {
    setShowAddForm(false)
    setAddForm(defaultForm)
    setAddError(null)
    setTargetSuggestion('')
  }

  useEffect(() => {
    checksApi.list()
      .then(res => setCheckList(res.data))
      .catch(() => {})
      .finally(() => setLoading(false))

    appsApi.list()
      .then(res => setAppList(res.data))
      .catch(() => {})
  }, [tick])

  // ── Add form ──

  function handleAddChange(field: keyof FormFields, value: string) {
    setAddForm(prev => ({ ...prev, [field]: value }))
    setAddError(null)

    if (field === 'app_id') {
      setTargetSuggestion('')
      if (!value) return
      const app = appList.find(a => a.id === value)
      if (!app?.profile_id) return
      appTemplatesApi.get(app.profile_id)
        .then(tmpl => {
          if (!tmpl.monitor?.check_url) return
          const baseUrl = typeof app.config?.base_url === 'string' ? app.config.base_url : ''
          const resolved = baseUrl
            ? tmpl.monitor.check_url.replace('{base_url}', baseUrl)
            : tmpl.monitor.check_url
          setTargetSuggestion(resolved)
        })
        .catch(() => {})
    }
  }

  async function handleAddSubmit() {
    const err = validateForm(addForm)
    if (err) { setAddError(err); return }
    setAddSubmitting(true)
    try {
      const created = await checksApi.create(formToInput(addForm))
      setCheckList(prev => [created, ...prev])
      setShowAddForm(false)
      setAddForm(defaultForm)
    } catch (e: unknown) {
      setAddError(e instanceof Error ? e.message : 'Failed to create check')
    } finally {
      setAddSubmitting(false)
    }
  }

  // ── Toggle enabled ──

  async function handleToggleEnabled(check: MonitorCheck) {
    const newEnabled = !check.enabled
    setCheckList(prev => prev.map(c => c.id === check.id ? { ...c, enabled: newEnabled } : c))
    try {
      const updated = await checksApi.update(check.id, { enabled: newEnabled })
      setCheckList(prev => prev.map(c => c.id === check.id ? updated : c))
    } catch {
      setCheckList(prev => prev.map(c => c.id === check.id ? check : c))
    }
  }

  // ── Run ──

  async function handleRun(id: string) {
    setRunningIds(prev => new Set(prev).add(id))
    try {
      await checksApi.run(id)
      const updated = await checksApi.get(id)
      setCheckList(prev => prev.map(c => c.id === id ? updated : c))
    } catch {
      // noop
    } finally {
      setRunningIds(prev => { const next = new Set(prev); next.delete(id); return next })
    }
  }

  return (
    <>
      <Topbar title="Monitor Checks" />
      <div className="content">

        <div className="section-header">
          <span className="section-title">Active Checks</span>
          {!loading && checkList.length > 0 && (() => {
            const up     = checkList.filter(c => c.last_status === 'up').length
            const down   = checkList.filter(c => c.last_status === 'down' || c.last_status === 'warn').length
            const paused = checkList.filter(c => !c.enabled).length
            const byType = (['url','ssl','dns','ping'] as const).map(t => ({ t, n: checkList.filter(c => c.type === t).length })).filter(x => x.n > 0)
            return (
              <span className="checks-header-stats">
                <span style={{ color: 'var(--green)' }}>{up} up</span>
                {down > 0 && <><span className="checks-header-dot" /><span style={{ color: 'var(--red)' }}>{down} down</span></>}
                {paused > 0 && <><span className="checks-header-dot" /><span style={{ color: 'var(--text3)' }}>{paused} paused</span></>}
                <span className="checks-header-sep" />
                {byType.map(({ t, n }) => <span key={t}>{n} {t.toUpperCase()}</span>)}
              </span>
            )
          })()}
          <button className="section-action" onClick={() => setShowAddForm(prev => !prev)}>
            + Add check
          </button>
        </div>

        {/* ── Filter bar ── */}
        {!loading && checkList.length > 0 && (
          <div className="checks-filter-bar">
            <input
              className="checks-search"
              placeholder="Search name or target…"
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
            <div className="checks-filter-pills">
              {(['all', 'up', 'down', 'paused'] as const).map(s => (
                <button key={s} className={`checks-filter-pill${statusFilter === s ? ' active' : ''}`}
                  onClick={() => setStatusFilter(s)}>
                  {s === 'all' ? 'All' : s.charAt(0).toUpperCase() + s.slice(1)}
                </button>
              ))}
              <span style={{ width: 1, background: 'var(--border)', alignSelf: 'stretch', margin: '0 4px' }} />
              {(['url', 'ssl', 'dns', 'ping'] as const).map(t => (
                <button key={t} className={`checks-filter-pill${typeFilter === t ? ' active' : ''}`}
                  onClick={() => setTypeFilter(cur => cur === t ? 'all' : t)}>
                  {t.toUpperCase()}
                </button>
              ))}
            </div>
          </div>
        )}

        {loading ? (
          <div className="checks-empty"><span>Loading…</span></div>
        ) : checkList.length === 0 ? (
          <div className="checks-empty"><span>No monitor checks configured yet.</span></div>
        ) : (() => {
          const q = search.toLowerCase()
          const filtered = checkList.filter(c => {
            if (typeFilter !== 'all' && c.type !== typeFilter) return false
            if (statusFilter === 'up' && c.last_status !== 'up') return false
            if (statusFilter === 'down' && c.last_status !== 'down' && c.last_status !== 'warn') return false
            if (statusFilter === 'paused' && c.enabled) return false
            if (q && !c.name.toLowerCase().includes(q) && !c.target.toLowerCase().includes(q)) return false
            return true
          })
          if (filtered.length === 0) return (
            <div className="checks-empty"><span>No checks match your filters.</span></div>
          )
          return (
            <div className="checks-table-wrap">
              <table className="checks-table">
                <thead>
                  <tr>
                    <th>Status</th>
                    <th>Name</th>
                    <th>Target</th>
                    <th>Type</th>
                    <th>Interval</th>
                    <th>Last Checked</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map(check => (
                    <tr key={check.id} style={{ cursor: 'pointer' }} onClick={() => navigate(`/checks/${check.id}`)}>
                      <td><div className={statusClass(check.last_status)} style={{ display: 'inline-block' }}>{statusLabel(check)}</div></td>
                      <td className="checks-table-name">{check.name}{!check.enabled && <span className="check-paused-badge" style={{ marginLeft: 6 }}>PAUSED</span>}</td>
                      <td className="checks-table-target" title={check.target}>{check.target}</td>
                      <td><span className={`check-type-badge check-type-${check.type}`}>{check.type.toUpperCase()}</span></td>
                      <td>{check.interval_secs}s</td>
                      <td>{formatEventTime(check.last_checked_at)}</td>
                      <td>
                        <div className="checks-table-actions" onClick={e => e.stopPropagation()}>
                          <button className={`check-run-btn${runningIds.has(check.id) ? ' running' : ''}`} title="Run now"
                            onClick={() => void handleRun(check.id)} disabled={runningIds.has(check.id)}>
                            {runningIds.has(check.id) ? <span className="check-spinner" /> : '▶'}
                          </button>
                          <button className={`check-toggle-btn${check.enabled ? ' enabled' : ' paused'}`}
                            title={check.enabled ? 'Pause' : 'Resume'}
                            onClick={() => void handleToggleEnabled(check)}>
                            {check.enabled ? '⏸' : '▶'}
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        })()}


      </div>

      <SlidePanel
        open={showAddForm}
        onClose={closeAddForm}
        title="Add Check"
        footer={
          <button
            className="sp-btn sp-btn--primary"
            onClick={() => void handleAddSubmit()}
            disabled={addSubmitting}
          >
            {addSubmitting ? 'Saving…' : 'Add Check'}
          </button>
        }
      >
        <CheckForm
          form={addForm}
          onChange={handleAddChange}
          onSubmit={handleAddSubmit}
          onCancel={closeAddForm}
          error={addError}
          submitting={addSubmitting}
          title=""
          submitLabel="Add Check"
          apps={appList.map(a => ({ id: a.id, name: a.name }))}
          targetSuggestion={targetSuggestion}
          hideActions
        />
      </SlidePanel>
    </>
  )
}
