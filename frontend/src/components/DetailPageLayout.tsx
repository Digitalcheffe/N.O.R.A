import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Topbar } from './Topbar'
import { events as eventsApi } from '../api/client'
import './DetailPageLayout.css'

// ── Types ─────────────────────────────────────────────────────────────────────

export interface KeyDataPoint {
  label: string
  value: string
}

export interface StatusIndicator {
  status: 'online' | 'offline' | 'unknown' | 'warning'
  label?: string
}

export interface DetailPageLayoutProps {
  /** e.g. "Infrastructure" — rendered as "← Infrastructure" link */
  breadcrumb: string
  /** Route to navigate to on breadcrumb click */
  breadcrumbPath: string
  /** Entity name — large heading and Topbar title */
  name: string
  /** Optional icon rendered inline next to the name */
  icon?: React.ReactNode
  /** Badges rendered under the name */
  keyDataPoints?: KeyDataPoint[]
  /** Online/offline/unknown dot + label, top right */
  status?: StatusIndicator
  /** "Last polled Xs ago" — top right, next to status */
  lastPolled?: string
  /** Top right action area (e.g. Discover Now button) */
  actions?: React.ReactNode
  /** Extra content rendered between the KDP row and the first divider (e.g. compact linked apps) */
  headerExtra?: React.ReactNode
  /** Optional source_type filter for EventFeed at the bottom */
  sourceType?: string
  /** Optional — when provided, renders an EventFeed at the bottom */
  sourceId?: string
  /** Optional custom title for the EventFeed section (defaults to "RECENT EVENTS") */
  eventFeedTitle?: string
  /** Time filter value passed to Topbar (day/week/month) */
  timeFilter?: 'day' | 'week' | 'month'
  /** When provided, shows Day/Week/Month tabs in the Topbar */
  onTimeFilter?: (f: 'day' | 'week' | 'month') => void
  /** Unique content section — rendered between dividers */
  children: React.ReactNode
}

// ── Status dot colors ─────────────────────────────────────────────────────────

const STATUS_DOT_CLASS: Record<StatusIndicator['status'], string> = {
  online:  'dpl-dot-online',
  offline: 'dpl-dot-offline',
  unknown: 'dpl-dot-unknown',
  warning: 'dpl-dot-warning',
}

// ── Component ─────────────────────────────────────────────────────────────────

const EVENT_LEVELS = ['info', 'warn', 'error', 'critical'] as const
const EVENT_META = {
  info:     { label: 'Info',     color: 'var(--accent)',  icon: <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><circle cx="10" cy="10" r="8"/><path d="M10 9v5M10 7h.01"/></svg> },
  warn:     { label: 'Warn',     color: 'var(--yellow)',  icon: <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M10 3L18.5 17H1.5L10 3z"/><path d="M10 9v4M10 15h.01"/></svg> },
  error:    { label: 'Error',    color: 'var(--red)',     icon: <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><circle cx="10" cy="10" r="8"/><path d="M7 7l6 6M13 7l-6 6"/></svg> },
  critical: { label: 'Critical', color: 'var(--red)',     icon: <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M10 2c0 4-4 5-4 9a4 4 0 0 0 8 0c0-4-4-5-4-9z"/><path d="M8 17.5a2 2 0 0 0 4 0"/></svg> },
}

export function DetailPageLayout({
  breadcrumb,
  breadcrumbPath,
  name,
  icon,
  keyDataPoints,
  status,
  lastPolled,
  actions,
  headerExtra,
  sourceType,
  sourceId,
  eventFeedTitle,
  timeFilter,
  onTimeFilter,
  children,
}: DetailPageLayoutProps) {
  const navigate = useNavigate()
  const [eventCounts, setEventCounts] = useState<Record<string, number> | null>(null)

  useEffect(() => {
    if (!sourceId) return
    Promise.all(
      EVENT_LEVELS.map(level =>
        eventsApi.list({ source_type: sourceType, source_id: sourceId, level, limit: 1 })
          .then(res => [level, res.total] as const)
          .catch(() => [level, 0] as const)
      )
    ).then(results => setEventCounts(Object.fromEntries(results)))
  }, [sourceId, sourceType])

  return (
    <>
      <Topbar title={name} timeFilter={timeFilter} onTimeFilter={onTimeFilter} />
      <div className="content dpl-page">

        {/* ── Header row ── */}
        <div className="dpl-header">
          <div className="dpl-header-left">
            <button
              className="dpl-breadcrumb"
              onClick={() => navigate(breadcrumbPath)}
            >
              ← {breadcrumb}
            </button>
            <div className="dpl-name-row">
              {icon && <span className="dpl-name-icon">{icon}</span>}
              <h1 className="dpl-name">{name}</h1>
            </div>
          </div>

          <div className="dpl-header-right">
            {status && (
              <>
                <span className={`dpl-status-dot ${STATUS_DOT_CLASS[status.status]}`} />
                <span className="dpl-status-label">
                  {status.label ?? status.status}
                </span>
              </>
            )}
            {lastPolled && (
              <span className="dpl-last-polled">{lastPolled}</span>
            )}
            {actions && (
              <div className="dpl-actions">{actions}</div>
            )}
          </div>
        </div>

        {/* ── Key data points ── */}
        {keyDataPoints && keyDataPoints.length > 0 && (
          <div className="dpl-kdp-row">
            {keyDataPoints.map((pt, i) => (
              <span key={i} className="dpl-kdp-badge">
                {pt.label && <span className="dpl-kdp-label">{pt.label}</span>}
                <span className="dpl-kdp-value">{pt.value}</span>
              </span>
            ))}
          </div>
        )}

        {/* ── Header extra (e.g. compact linked apps) ── */}
        {headerExtra && (
          <div className="dpl-header-extra">{headerExtra}</div>
        )}

        {/* ── Divider ── */}
        <div className="dpl-divider" />

        {/* ── Unique content ── */}
        <div className="dpl-content">{children}</div>

        {/* ── Event summary — only rendered when a sourceId is provided ── */}
        {sourceId && eventCounts !== null && (
          <>
            <div className="dpl-divider" />
            <div className="dpl-event-section">
              <div className="dpl-event-section-title">{eventFeedTitle ?? 'Events'}</div>
              <div className="dpl-event-list">
                {EVENT_LEVELS.map(level => {
                  const { label, color, icon: lvlIcon } = EVENT_META[level]
                  const count = eventCounts[level] ?? 0
                  const params = new URLSearchParams({ level })
                  if (sourceType) params.set('source_type', sourceType)
                  if (sourceId)   params.set('source_id', sourceId)
                  return (
                    <div key={level} className="appdetail-event-row"
                      onClick={() => navigate(`/events?${params.toString()}`)}>
                      <span className="appdetail-event-icon" style={{ color }}>{lvlIcon}</span>
                      <span className="appdetail-event-level" style={{ color }}>{label}</span>
                      <span className="appdetail-event-count" style={{ color }}>{count}</span>
                      <span className="appdetail-event-link">View →</span>
                    </div>
                  )
                })}
              </div>
            </div>
          </>
        )}

      </div>
    </>
  )
}
