import { useNavigate } from 'react-router-dom'
import { Topbar } from './Topbar'
import { EventFeed } from './EventFeed'
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
  /** Required for EventFeed at the bottom */
  sourceId: string
  /** Optional custom title for the EventFeed section (defaults to "RECENT EVENTS") */
  eventFeedTitle?: string
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
  children,
}: DetailPageLayoutProps) {
  const navigate = useNavigate()

  return (
    <>
      <Topbar title={name} />
      <div className="content">

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

        {/* ── Divider ── */}
        <div className="dpl-divider" />

        {/* ── Event feed — always at the bottom, not configurable ── */}
        <EventFeed sourceType={sourceType} sourceId={sourceId} title={eventFeedTitle} />

      </div>
    </>
  )
}
