import { InfraTypeIcon } from '../components/CheckTypeIcon'
import { TYPE_LABEL, COLLECTION_METHOD } from './InfraEditModal'
import type { ComponentType } from '../api/types'
import './Integrations.css'

// ── Per-type metadata for component type cards ────────────────────────────────
// Add an entry here when you need custom description/caps for a new type.
// Any type in TYPE_LABEL without an entry gets sensible defaults from COLLECTION_METHOD.

const TYPE_META: Partial<Record<ComponentType, { description: string; capabilities: string[] }>> = {
  proxmox_node:  { description: 'Proxmox hypervisor node — polls VMs and resources via Proxmox API.', capabilities: ['resource metrics', 'VM status', 'API polling'] },
  synology:      { description: 'Synology NAS — volume health and resource metrics via DSM API.',      capabilities: ['resource metrics', 'volume health', 'API polling'] },
  docker_engine: { description: 'Docker Engine — container discovery and metrics via Docker socket.',  capabilities: ['container discovery', 'resource metrics', 'socket polling'] },
  traefik:       { description: 'Traefik reverse proxy — route and SSL cert discovery via API.',       capabilities: ['route discovery', 'SSL monitoring', 'API polling'] },
  portainer:     { description: 'Portainer — multi-endpoint container management via REST API.',       capabilities: ['container discovery', 'image update detection', 'API polling'] },
  vm_linux:      { description: 'Linux VM — resource metrics via SNMP.',                              capabilities: ['ping', 'SNMP polling', 'resource metrics'] },
  vm_windows:    { description: 'Windows VM — resource metrics via SNMP.',                            capabilities: ['ping', 'SNMP polling', 'resource metrics'] },
  vm_other:      { description: 'VM of unknown OS — availability monitoring only.',                   capabilities: ['ping', 'availability monitoring'] },
  linux_host:    { description: 'Linux server or bare-metal host — resource metrics via SNMP.',       capabilities: ['ping', 'SNMP polling', 'resource metrics'] },
  windows_host:  { description: 'Windows server or workstation — resource metrics via SNMP.',         capabilities: ['ping', 'SNMP polling', 'resource metrics'] },
  generic_host:  { description: 'Any network-reachable device — routers, switches, appliances.',      capabilities: ['ping', 'availability monitoring'] },
}

function defaultMeta(type: ComponentType): { description: string; capabilities: string[] } {
  const method = COLLECTION_METHOD[type]
  const caps = method === 'snmp'          ? ['ping', 'SNMP polling', 'resource metrics']
             : method === 'docker_socket' ? ['container discovery', 'resource metrics']
             : method === 'proxmox_api'   ? ['resource metrics', 'API polling']
             : method === 'traefik_api'   ? ['route discovery', 'API polling']
             : method === 'portainer_api' ? ['container discovery', 'API polling']
             : method === 'synology_api'  ? ['resource metrics', 'API polling']
             : ['ping', 'availability monitoring']
  return { description: `${TYPE_LABEL[type]} component.`, capabilities: caps }
}

// ── Cards ─────────────────────────────────────────────────────────────────────

interface DriverCardProps { name: string; label: string; capabilities: string[] }

function DriverCard({ name, label, capabilities }: DriverCardProps) {
  return (
    <div className="int-driver-card">
      <div className="int-driver-name">
        <InfraTypeIcon type={name} size={18} />
        {label}
      </div>
      <div className="int-driver-caps">
        {capabilities.map(c => (
          <span key={c} className="int-cap-pill">{c}</span>
        ))}
      </div>
    </div>
  )
}

// ── Main exported component ───────────────────────────────────────────────────

export function InfraIntegrations() {
  // Component type cards are driven from TYPE_LABEL — adding a new ComponentType
  // automatically adds a card here without touching this file.
  const componentTypeCards = (Object.keys(TYPE_LABEL) as ComponentType[]).map(type => {
    const meta = TYPE_META[type] ?? defaultMeta(type)
    return { name: type, label: TYPE_LABEL[type], ...meta }
  }).sort((a, b) => a.label.localeCompare(b.label))

  return (
    <div className="int-section">
      <div className="int-section-header">
        <span className="int-section-title">Infrastructure Integrations</span>
      </div>
      <div className="int-driver-grid">
        {componentTypeCards.map(d => (
          <DriverCard key={d.name} name={d.name} label={d.label} capabilities={d.capabilities} />
        ))}
      </div>
    </div>
  )
}
