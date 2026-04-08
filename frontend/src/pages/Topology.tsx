import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import { topology as topoApi } from '../api/client'
import type { TopologyNode } from '../api/types'
import { InfraTypeIcon } from '../components/CheckTypeIcon'
import './Topology.css'

// ── Type labels ───────────────────────────────────────────────────────────────

const TYPE_LABELS: Record<string, string> = {
  proxmox_node:  'Proxmox',
  bare_metal:    'Bare Metal',
  linux_host:    'Linux',
  windows_host:  'Windows',
  generic_host:  'Host',
  vm:            'VM',
  vm_linux:      'Linux VM',
  vm_windows:    'Windows VM',
  vm_other:      'VM',
  lxc:           'LXC',
  wsl:           'WSL',
  docker_engine: 'Docker',
  portainer:     'Portainer',
  traefik:       'Traefik',
  synology:      'Synology',
  opnsense:      'OPNsense',
  container:     'Container',
}

function typeLabel(type: string) {
  return TYPE_LABELS[type] ?? type
}

// ── App icon (real icon or letter fallback) ───────────────────────────────────

function AppIcon({ iconUrl, name, size }: { iconUrl?: string; name?: string; size: number }) {
  const [failed, setFailed] = useState(false)
  const letter = name ? name[0].toUpperCase() : '?'
  if (iconUrl && !failed) {
    return (
      <img
        src={iconUrl}
        alt={name}
        width={size}
        height={size}
        style={{ borderRadius: 4, objectFit: 'contain' }}
        onError={() => setFailed(true)}
      />
    )
  }
  return (
    <span
      style={{
        display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
        width: size, height: size, borderRadius: 4,
        background: 'rgba(99,179,237,0.15)', color: '#63b3ed',
        fontSize: size * 0.55, fontWeight: 700,
      }}
    >
      {letter}
    </span>
  )
}

function statusDotClass(status?: string): string {
  if (!status) return 'topo-dot--dim'
  const s = status.toLowerCase()
  if (['online', 'running'].includes(s)) return 'topo-dot--green'
  if (['offline', 'stopped', 'error'].includes(s)) return 'topo-dot--red'
  if (['warn', 'degraded'].includes(s)) return 'topo-dot--yellow'
  return 'topo-dot--dim'
}

// ── Rollup stats ──────────────────────────────────────────────────────────────

function computeStats(node: TopologyNode) {
  let vms = 0, engines = 0, containers = 0
  for (const child of node.children) {
    if (child.type === 'container') {
      containers++
    } else if (['docker_engine', 'portainer'].includes(child.type)) {
      engines++
      containers += computeStats(child).containers
    } else if (['vm', 'vm_linux', 'vm_windows', 'vm_other', 'lxc', 'wsl'].includes(child.type)) {
      vms++
      const s = computeStats(child)
      engines += s.engines
      containers += s.containers
    }
  }
  return { vms, engines, containers, apps: node.apps.length }
}

// ── Single card ───────────────────────────────────────────────────────────────

interface NodeCardProps {
  node: TopologyNode
  selected: boolean
  onClick: () => void
}

function NodeCard({ node, selected, onClick }: NodeCardProps) {
  const stats = computeStats(node)

  return (
    <div
      className={`topo-card topo-card--clickable${selected ? ' topo-card--selected' : ''}`}
      onClick={onClick}
    >
      <div className="topo-card-header">
        <span className="topo-card-icon"><InfraTypeIcon type={node.type} size={22} /></span>
        <div className="topo-card-title">
          <span className="topo-card-name">{node.name}</span>
          <span className="topo-card-type">{typeLabel(node.type)}</span>
        </div>
        <span className={`topo-dot ${statusDotClass(node.status)}`} />
      </div>
      <div className="topo-card-stats">
        {node.ip && <span className="topo-stat topo-stat--ip">{node.ip}</span>}
        {stats.vms > 0        && <span className="topo-stat">{stats.vms} VM{stats.vms !== 1 ? 's' : ''}</span>}
        {stats.engines > 0    && <span className="topo-stat">{stats.engines} engine{stats.engines !== 1 ? 's' : ''}</span>}
        {stats.containers > 0 && <span className="topo-stat">{stats.containers} container{stats.containers !== 1 ? 's' : ''}</span>}
        {stats.apps > 0       && <span className="topo-stat topo-stat--app">{stats.apps} app{stats.apps !== 1 ? 's' : ''}</span>}
        {!node.ip && !stats.vms && !stats.engines && !stats.containers && !stats.apps && (
          <span className="topo-stat topo-stat--empty">No children</span>
        )}
      </div>
    </div>
  )
}

// ── Container pills ───────────────────────────────────────────────────────────

interface ContainerRowProps {
  containers: TopologyNode[]
  selected: TopologyNode | null
  onSelect: (c: TopologyNode) => void
}

function ContainerRow({ containers, selected, onSelect }: ContainerRowProps) {
  return (
    <div className="topo-level">
      <div className="topo-level-label">
        <span className="topo-level-connector" />
        <span className="topo-level-title">{containers.length} container{containers.length !== 1 ? 's' : ''}</span>
      </div>
      <div className="topo-container-grid">
        {containers.map(c => (
          <div
            key={c.id}
            className={`topo-container-pill${c.app_id ? ' topo-container-pill--linked' : ''}${selected?.id === c.id ? ' topo-container-pill--selected' : ''}`}
            onClick={() => onSelect(c)}
          >
            <span className={`topo-dot ${statusDotClass(c.status)}`} />
            <span className="topo-container-name">{c.name}</span>
            {c.app_id && (
              <AppIcon iconUrl={c.app_icon_url} name={c.app_name} size={16} />
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

// ── App layer ─────────────────────────────────────────────────────────────────

function AppLayer({ container }: { container: TopologyNode }) {
  const navigate = useNavigate()

  return (
    <div className="topo-level">
      <div className="topo-level-label">
        <span className="topo-level-connector" />
        <span className="topo-level-title">{container.name} · Linked App</span>
      </div>
      <div className="topo-grid">
        <div
          className="topo-card topo-card--clickable topo-card--app"
          onClick={() => navigate(`/apps/${container.app_id}`)}
        >
          <div className="topo-card-header">
            <span className="topo-card-icon">
              <AppIcon iconUrl={container.app_icon_url} name={container.app_name} size={24} />
            </span>
            <div className="topo-card-title">
              <span className="topo-card-name">{container.app_name}</span>
              <span className="topo-card-type">Application</span>
            </div>
            <span className="topo-app-arrow">→</span>
          </div>
          <div className="topo-card-stats">
            <span className="topo-stat topo-stat--app">View app</span>
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export function TopologyPage() {
  const [tree,               setTree]               = useState<TopologyNode[]>([])
  const [loading,            setLoading]            = useState(true)
  const [error,              setError]              = useState<string | null>(null)
  // selections[i] = the infra node selected at level i
  const [selections,         setSelections]         = useState<TopologyNode[]>([])
  // selected container pill (at the leaf container row)
  const [selectedContainer,  setSelectedContainer]  = useState<TopologyNode | null>(null)

  useEffect(() => {
    topoApi.getTree()
      .then(data => { setTree(data); setError(null) })
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load topology'))
      .finally(() => setLoading(false))
  }, [])

  function selectAt(level: number, node: TopologyNode) {
    setSelectedContainer(null)
    setSelections(prev => {
      const next = prev.slice(0, level)
      // Toggle off if already selected.
      if (prev[level]?.id === node.id) return next
      return [...next, node]
    })
  }

  function selectContainer(c: TopologyNode) {
    setSelectedContainer(prev => prev?.id === c.id ? null : c)
  }

  // Build the list of infra levels to render.
  // Level 0: root nodes
  // Level k: non-container children of selections[k-1]
  const levels: { nodes: TopologyNode[]; selected: TopologyNode | null }[] = [
    { nodes: tree, selected: selections[0] ?? null },
  ]
  for (let i = 0; i < selections.length; i++) {
    const children = selections[i].children.filter(c => c.type !== 'container')
    if (children.length > 0) {
      levels.push({ nodes: children, selected: selections[i + 1] ?? null })
    }
  }

  // Container leaf row: containers under the deepest selected infra node
  // that has containers but no further infra children.
  const deepest = selections[selections.length - 1]
  const containerLeaf = deepest
    ? deepest.children.filter(c => c.type === 'container')
    : []
  const showContainerLeaf =
    deepest !== undefined &&
    containerLeaf.length > 0 &&
    deepest.children.filter(c => c.type !== 'container').length === 0

  // App layer: shown when a container with a linked app is selected.
  const showAppLayer = selectedContainer && selectedContainer.app_id

  return (
    <>
      <Topbar title="Topology" />
      <div className="topo-page">

        {/* Page header */}
        <div className="topo-page-header">
          <h2 className="topo-page-title">Network Map</h2>
          <p className="topo-page-sub">
            {loading
              ? 'Loading…'
              : `${tree.length} root component${tree.length !== 1 ? 's' : ''} — click a card to drill down`}
          </p>
          {selections.length > 0 && (
            <div className="topo-breadcrumb">
              <button className="topo-crumb" onClick={() => { setSelections([]); setSelectedContainer(null) }}>
                All
              </button>
              {selections.map((node, i) => (
                <span key={node.id} className="topo-crumb-wrap">
                  <span className="topo-crumb-arrow">→</span>
                  <button
                    className={`topo-crumb${i === selections.length - 1 ? ' topo-crumb--active' : ''}`}
                    onClick={() => { setSelections(prev => prev.slice(0, i + 1)); setSelectedContainer(null) }}
                  >
                    {node.name}
                  </button>
                </span>
              ))}
              {selectedContainer && (
                <span className="topo-crumb-wrap">
                  <span className="topo-crumb-arrow">→</span>
                  <button className="topo-crumb topo-crumb--active">{selectedContainer.name}</button>
                </span>
              )}
            </div>
          )}
        </div>

        {error && <p className="topo-error">{error}</p>}

        {!loading && !error && (
          <div className="topo-chain">

            {levels.map((level, i) => (
              <div key={i}>
                {/* Level connector label (except for root) */}
                {i > 0 && (
                  <div className="topo-level-label">
                    <span className="topo-level-connector" />
                    <span className="topo-level-title">{selections[i - 1].name}</span>
                  </div>
                )}

                {/* Card grid */}
                <div className="topo-level">
                  <div className="topo-grid">
                    {level.nodes.map(node => (
                      <NodeCard
                        key={node.id}
                        node={node}
                        selected={level.selected?.id === node.id}
                        onClick={() => selectAt(i, node)}
                      />
                    ))}
                  </div>
                </div>

              </div>
            ))}

            {/* Container pill row */}
            {showContainerLeaf && (
              <ContainerRow
                containers={containerLeaf}
                selected={selectedContainer}
                onSelect={selectContainer}
              />
            )}

            {/* App layer */}
            {showAppLayer && <AppLayer container={selectedContainer!} />}

          </div>
        )}
      </div>
    </>
  )
}
