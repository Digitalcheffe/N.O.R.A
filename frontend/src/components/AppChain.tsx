import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { ChainNode, ChainTraefikRoute } from '../api/types'
import './AppChain.css'

const CDN = 'https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/svg'

// dashboard-icons CDN slug per infra type
const TYPE_CDN_SLUG: Record<string, string> = {
  portainer:    'portainer',
  docker_engine:'docker',
  proxmox_node: 'proxmox',
  traefik:      'traefik',
  synology:     'synology-dsm',
  vm_linux:     'linux',
  vm_windows:   'windows',
  linux_host:   'linux',
  windows_host: 'windows',
}

const INFRA_TYPES = new Set([
  'portainer', 'docker_engine', 'vm_linux', 'vm_windows', 'vm_other',
  'proxmox_node', 'synology', 'linux_host', 'windows_host', 'generic_host', 'traefik',
])

function nodeHref(node: ChainNode): string | null {
  if (INFRA_TYPES.has(node.type)) return `/infrastructure/${node.id}`
  return null
}

function statusClass(status: string): string {
  const s = status.toLowerCase()
  if (['online', 'running', 'enabled', 'up', 'ok'].includes(s)) return 'chain-dot--green'
  if (['stopped', 'error', 'down', 'disabled', 'offline'].includes(s)) return 'chain-dot--red'
  if (['warn', 'warning', 'degraded'].includes(s)) return 'chain-dot--yellow'
  return 'chain-dot--dim'
}

function statusLabel(status: string): string {
  if (!status) return ''
  return status.charAt(0).toUpperCase() + status.slice(1).toLowerCase()
}

const TYPE_CLASS: Record<string, string> = {
  app:          'chain-badge--app',
  container:    'chain-badge--container',
  portainer:    'chain-badge--portainer',
  docker_engine:'chain-badge--docker',
  vm_linux:     'chain-badge--vm',
  vm_windows:   'chain-badge--vm',
  proxmox_node: 'chain-badge--proxmox',
  synology:     'chain-badge--synology',
  traefik:      'chain-badge--traefik',
}

const TYPE_LABEL: Record<string, string> = {
  app:          'App',
  container:    'Container',
  portainer:    'Portainer',
  docker_engine:'Docker',
  vm_linux:     'Linux VM',
  vm_windows:   'Windows VM',
  proxmox_node: 'Proxmox',
  synology:     'Synology',
  traefik:      'Traefik',
  linux_host:   'Linux',
  windows_host: 'Windows',
  generic_host: 'Host',
}

// Resolves the best icon URL for a node: explicit backend url → CDN by type → null
function resolveIconUrl(node: ChainNode): string | null {
  if (node.icon_url) return node.icon_url
  const slug = TYPE_CDN_SLUG[node.type]
  if (slug) return `${CDN}/${slug}.svg`
  return null
}

function NodeIcon({ node }: { node: ChainNode }) {
  const [failed, setFailed] = useState(false)
  const url = !failed ? resolveIconUrl(node) : null

  if (url) {
    return (
      <img
        src={url}
        alt=""
        className="chain-node-icon"
        onError={() => setFailed(true)}
      />
    )
  }
  return (
    <span className="chain-node-icon chain-node-icon--letter">
      {node.name.charAt(0).toUpperCase()}
    </span>
  )
}

function TraefikBadgeIcon() {
  const [failed, setFailed] = useState(false)
  const url = !failed ? `${CDN}/traefik.svg` : null
  if (url) return <img src={url} alt="" className="chain-badge-icon" onError={() => setFailed(true)} />
  return null
}

interface AppChainProps {
  chain: ChainNode[]
  appStatus?: string
  traefik: ChainTraefikRoute[]
}

export function AppChain({ chain, appStatus, traefik }: AppChainProps) {
  const navigate = useNavigate()

  if (chain.length === 0) return null

  return (
    <div className="app-chain-wrap">
      <div className="app-chain-label">Infrastructure</div>

      <div className="app-chain-row">
        {chain.map((node, i) => {
          const href = nodeHref(node)
          const status = node.type === 'app' ? (appStatus ?? '') : node.status
          const badgeClass = TYPE_CLASS[node.type] ?? 'chain-badge--default'

          return (
            <div key={node.id} className="app-chain-step">
              <div
                className={`app-chain-node${href ? ' app-chain-node--link' : ''}`}
                onClick={href ? () => navigate(href) : undefined}
                role={href ? 'button' : undefined}
                tabIndex={href ? 0 : undefined}
                onKeyDown={href ? (e) => { if (e.key === 'Enter') navigate(href) } : undefined}
              >
                {/* Icon */}
                <div className={`chain-icon-wrap ${badgeClass}`}>
                  <NodeIcon node={node} />
                </div>

                {/* Text content */}
                <div className="chain-node-body">
                  <span className={`chain-type-label ${badgeClass}`}>{TYPE_LABEL[node.type] ?? node.type}</span>
                  <span className="chain-name" title={node.name}>{node.name}</span>
                  {node.detail && (
                    <span className="chain-detail" title={node.detail}>{node.detail}</span>
                  )}
                  {status && (
                    <div className="chain-status-row">
                      <span className={`chain-dot ${statusClass(status)}`} />
                      <span className="chain-status-text">{statusLabel(status)}</span>
                    </div>
                  )}
                </div>
              </div>

              {i < chain.length - 1 && (
                <div className="chain-connector" aria-hidden>
                  <span className="chain-connector-line" />
                  <span className="chain-connector-arrow">›</span>
                </div>
              )}
            </div>
          )
        })}
      </div>

      {traefik.length > 0 && (
        <div className="app-chain-traefik">
          <div className="chain-traefik-header">
            <div className="chain-badge chain-badge--traefik">
              <TraefikBadgeIcon />
              <span>Traefik</span>
            </div>
          </div>
          <div className="app-chain-traefik-routes">
            {traefik.map((r, i) => (
              r.manual_link ? (
                // Manually-linked Traefik component — no routes discovered yet.
                // Show the component name + status without rule/service arrows.
                <div key={i} className="chain-traefik-row chain-traefik-row--manual">
                  <span className="chain-traefik-service">{r.router}</span>
                  <div className="chain-status-row">
                    <span className={`chain-dot ${statusClass(r.status)}`} />
                    <span className="chain-status-text">{statusLabel(r.status) || 'Linked'}</span>
                  </div>
                  <span className="chain-traefik-manual-label">no routes discovered</span>
                </div>
              ) : (
                <div key={i} className="chain-traefik-row">
                  {/* Router status + rule */}
                  <div className="chain-status-row">
                    <span className={`chain-dot ${statusClass(r.status)}`} title={`Router: ${statusLabel(r.status)}`} />
                  </div>
                  <span className="chain-traefik-rule" title={r.rule}>{r.rule}</span>

                  {/* Arrow to service */}
                  {(r.service || r.router) && (
                    <>
                      <div className="chain-connector chain-connector--sm" aria-hidden>
                        <span className="chain-connector-line" />
                        <span className="chain-connector-arrow">›</span>
                      </div>
                      {/* Service name + its own health */}
                      <span className="chain-traefik-service">{r.service || r.router}</span>
                      {r.service_status && (
                        <div className="chain-status-row chain-traefik-svc-status">
                          <span className={`chain-dot ${statusClass(r.service_status)}`} title={`Service: ${statusLabel(r.service_status)}`} />
                          {(r.server_count ?? 0) > 0 && (
                            <span className="chain-traefik-servers">
                              {r.servers_up}/{r.server_count}
                            </span>
                          )}
                        </div>
                      )}
                      {/* First backend host from servers_json */}
                      {(() => {
                        if (!r.servers_json) return null
                        try {
                          const entries = Object.entries(JSON.parse(r.servers_json) as Record<string, string>)
                          if (entries.length === 0) return null
                          const [url, st] = entries[0]
                          const isDown = st.toUpperCase() === 'DOWN'
                          return (
                            <span className={`chain-traefik-backend${isDown ? ' chain-traefik-backend--down' : ''}`}>
                              {url}{entries.length > 1 ? ` +${entries.length - 1}` : ''}
                            </span>
                          )
                        } catch { return null }
                      })()}
                    </>
                  )}
                </div>
              )
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
