import { useState } from 'react'

const BASE = { fill: 'none', stroke: 'currentColor', strokeWidth: 1.5, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const }

export function CheckTypeIcon({ type, size = 15 }: { type: string; size?: number }) {
  const s = { ...BASE, width: size, height: size, flexShrink: 0 as const, opacity: 0.65 }
  switch (type) {
    case 'url':
      return (
        <svg viewBox="0 0 16 16" style={s}>
          <circle cx="8" cy="8" r="6.5" />
          <path d="M8 1.5S5.5 4 5.5 8 8 14.5 8 14.5M8 1.5S10.5 4 10.5 8 8 14.5 8 14.5" />
          <path d="M1.5 8h13" />
        </svg>
      )
    case 'ssl':
      return (
        <svg viewBox="0 0 16 16" style={s}>
          <rect x="3" y="7" width="10" height="7.5" rx="1.5" />
          <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" />
          <circle cx="8" cy="10.5" r="1" fill="currentColor" stroke="none" opacity="1" />
        </svg>
      )
    case 'dns':
      return (
        <svg viewBox="0 0 16 16" style={s}>
          <path d="M8 2v12M5 5l3-3 3 3M5 11l3 3 3-3" />
          <path d="M2 8h3M11 8h3" />
        </svg>
      )
    case 'ping':
      return (
        <svg viewBox="0 0 16 16" style={s}>
          <path d="M1 9h2.5L5 5l3 8 2-5 1.5 2.5H15" />
        </svg>
      )
    default:
      return null
  }
}

const INFRA_CDN_ICONS: Record<string, string> = {
  proxmox_node:  'proxmox',
  synology:      'synology-dsm',
  traefik:       'traefik',
  portainer:     'portainer',
  docker_engine: 'docker',
}

function InfraTypeSVGFallback({ type, size }: { type: string; size: number }) {
  const s = { ...BASE, width: size, height: size, strokeWidth: 1.4 }
  switch (type) {
    case 'vm':
    case 'lxc':
      return (
        <svg viewBox="0 0 22 22" style={s}>
          <rect x="2" y="3" width="18" height="13" rx="2" />
          <path d="M7 19h8M11 16v3" />
        </svg>
      )
    case 'linux_host':
      return (
        <svg viewBox="0 0 22 22" style={s}>
          <ellipse cx="11" cy="8" rx="5" ry="5.5" />
          <path d="M7 13c-3 1-4 4-4 6h16c0-2-1-5-4-6" />
          <circle cx="9" cy="7" r="1" fill="currentColor" stroke="none" />
          <circle cx="13" cy="7" r="1" fill="currentColor" stroke="none" />
          <path d="M9 10.5c.5.7 1.5.7 2 0" />
        </svg>
      )
    case 'windows_host':
      return (
        <svg viewBox="0 0 22 22" style={s}>
          <path d="M3 5.5l7-1v7H3zM11 4.2l8-1.2v8.5H11zM3 12.5h7v7l-7-1zM11 12.5h8v8.5L11 19.8z" />
        </svg>
      )
    default:
      return (
        <svg viewBox="0 0 22 22" style={s}>
          <rect x="2" y="4" width="18" height="5" rx="1.5" />
          <rect x="2" y="11" width="18" height="5" rx="1.5" />
          <circle cx="17" cy="6.5" r="1" fill="currentColor" stroke="none" />
          <circle cx="17" cy="13.5" r="1" fill="currentColor" stroke="none" />
        </svg>
      )
  }
}

export function InfraTypeIcon({ type, size = 24 }: { type: string; size?: number }) {
  const [failed, setFailed] = useState(false)
  const cdnName = INFRA_CDN_ICONS[type]

  if (cdnName && !failed) {
    return (
      <img
        src={`https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/svg/${cdnName}.svg`}
        alt={type}
        className="infra-type-icon"
        style={{ width: size, height: size }}
        onError={() => setFailed(true)}
      />
    )
  }
  return (
    <span className="infra-type-icon infra-type-icon-svg" style={{ color: 'var(--text3)' }}>
      <InfraTypeSVGFallback type={type} size={size} />
    </span>
  )
}
