import './Integrations.css'

// ── Static driver definitions ─────────────────────────────────────────────────

const DRIVERS = [
  {
    name: 'docker',
    label: 'Docker',
    description: 'Container discovery and resource metrics via Docker socket.',
    capabilities: ['container discovery', 'resource metrics', 'app linking', 'socket polling'],
  },
  {
    name: 'traefik',
    label: 'Traefik',
    description: 'SSL cert discovery and routing visibility via Traefik API.',
    capabilities: ['SSL discovery', 'network map node', 'API polling'],
  },
  {
    name: 'proxmox',
    label: 'Proxmox',
    description: 'Node and VM status plus resource metrics via Proxmox REST API.',
    capabilities: ['resource metrics', 'VM/CT status', 'API polling'],
  },
  {
    name: 'opnsense',
    label: 'OPNsense',
    description: 'Network status and availability via OPNsense API.',
    capabilities: ['network status', 'firmware alerts', 'API polling'],
  },
  {
    name: 'synology',
    label: 'Synology',
    description: 'NAS resource metrics and volume health via Synology DSM API.',
    capabilities: ['resource metrics', 'volume health', 'API polling'],
  },
  {
    name: 'snmp',
    label: 'SNMP',
    description: 'Generic host polling via SNMP v2c/v3 for devices without a dedicated API.',
    capabilities: ['resource metrics', 'ping baseline', 'generic host support'],
  },
]

// ── Driver card ───────────────────────────────────────────────────────────────

function DriverCard({ label, capabilities }: typeof DRIVERS[number]) {
  return (
    <div className="int-driver-card">
      <div className="int-driver-name">{label}</div>
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
  return (
    <div className="int-section">
      <div className="int-section-header">
        <span className="int-section-title">Infrastructure Integrations</span>
      </div>
      <div className="int-driver-grid">
        {DRIVERS.map(d => <DriverCard key={d.name} {...d} />)}
      </div>
    </div>
  )
}
