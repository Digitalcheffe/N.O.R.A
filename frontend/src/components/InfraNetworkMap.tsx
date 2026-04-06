import { memo, useState, useCallback, useContext, createContext, useEffect, useRef } from 'react'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  useReactFlow,
  Handle,
  Position,
  BackgroundVariant,
  MarkerType,
  type Node,
  type Edge,
  type NodeProps,
  type OnNodeDrag,
  type NodeTypes,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import type {
  InfrastructureComponent,
  ComponentType,
  ComponentLink,
  App,
  DiscoveredContainer,
  MonitorCheck,
} from '../api/types'

// ── Constants ─────────────────────────────────────────────────────────────────

const POSITIONS_KEY = 'nora-topology-map-v2'

const TYPE_COLOR: Record<string, string> = {
  proxmox_node:    '#3b82f6',
  synology:        '#a855f7',
  vm_linux:        '#64748b',
  vm_windows:      '#64748b',
  vm_other:        '#64748b',
  linux_host:      '#64748b',
  windows_host:    '#64748b',
  generic_host:    '#94a3b8',
  docker_engine:   '#14b8a6',
  traefik:         '#f97316',
  portainer:       '#13bef9',
  traefik_router:  '#fb923c',
  traefik_service: '#fdba74',
}

const TYPE_LABEL: Record<string, string> = {
  proxmox_node:    'Proxmox VE',
  synology:        'Synology NAS',
  vm_linux:        'VM · Linux',
  vm_windows:      'VM · Windows',
  vm_other:        'VM · Other',
  linux_host:      'Linux Host',
  windows_host:    'Windows Host',
  generic_host:    'Generic Host',
  docker_engine:   'Docker Engine',
  traefik:         'Traefik',
  portainer:       'Portainer',
  traefik_router:  'Traefik Router',
  traefik_service: 'Traefik Service',
}

const CONTAINER_COLOR = '#06b6d4'
const APP_COLOR       = '#8b5cf6'
const MONITOR_COLOR   = '#10b981'

// Edge colors per relationship tier — visible but not garish
const EDGE_INFRA      = '#3b6eb5'
const EDGE_CONTAINER  = '#0891b2'
const EDGE_APP        = '#7c3aed'
const EDGE_MONITOR    = '#059669'

const HANDLE_STYLE = {
  background: '#1e2530',
  border: '1px solid #334155',
  width: 8,
  height: 8,
}

function statusDotColor(status?: string | null): string {
  if (!status) return '#4a5568'
  if (status === 'online' || status === 'up' || status === 'running') return '#22c55e'
  if (status === 'degraded' || status === 'warn') return '#eab308'
  if (status === 'offline' || status === 'down' || status === 'exited') return '#ef4444'
  return '#4a5568'
}

// ── Context ───────────────────────────────────────────────────────────────────

const EditContext = createContext<(c: InfrastructureComponent) => void>(() => {})

// ── Tree layout ───────────────────────────────────────────────────────────────

const NODE_W = 220  // min horizontal space per leaf
const NODE_H = 200  // vertical gap between layers

function computeTreeLayout(
  nodes: { id: string }[],
  edges: { source: string; target: string }[],
): Record<string, { x: number; y: number }> {
  if (nodes.length === 0) return {}
  const nodeIds = new Set(nodes.map(n => n.id))
  const children = new Map<string, string[]>()
  const hasParent = new Set<string>()

  for (const e of edges) {
    if (!nodeIds.has(e.source) || !nodeIds.has(e.target)) continue
    const arr = children.get(e.source) ?? []
    arr.push(e.target)
    children.set(e.source, arr)
    hasParent.add(e.target)
  }

  const roots = [...nodeIds].filter(id => !hasParent.has(id))

  function subtreeLeaves(id: string): number {
    const kids = children.get(id) ?? []
    return kids.length === 0 ? 1 : kids.reduce((s, k) => s + subtreeLeaves(k), 0)
  }

  const positions: Record<string, { x: number; y: number }> = {}

  function place(id: string, cx: number, depth: number) {
    positions[id] = { x: cx - 90, y: depth * NODE_H }
    const kids = children.get(id) ?? []
    if (!kids.length) return
    const total = kids.reduce((s, k) => s + subtreeLeaves(k), 0)
    let left = cx - (total * NODE_W) / 2
    for (const kid of kids) {
      const w = subtreeLeaves(kid)
      place(kid, left + (w * NODE_W) / 2, depth + 1)
      left += w * NODE_W
    }
  }

  const totalLeaves = roots.reduce((s, id) => s + subtreeLeaves(id), 0)
  let left = -(totalLeaves * NODE_W) / 2
  for (const id of roots) {
    const w = subtreeLeaves(id)
    place(id, left + (w * NODE_W) / 2, 0)
    left += w * NODE_W
  }

  return positions
}

// ── Persistence ───────────────────────────────────────────────────────────────

function loadPositions(): Record<string, { x: number; y: number }> {
  try { return JSON.parse(localStorage.getItem(POSITIONS_KEY) ?? '{}') } catch { return {} }
}
function savePositions(p: Record<string, { x: number; y: number }>) {
  localStorage.setItem(POSITIONS_KEY, JSON.stringify(p))
}

// ── Icons ─────────────────────────────────────────────────────────────────────

function TypeIcon({ type, color }: { type: string; color: string }) {
  switch (type as ComponentType) {
    case 'proxmox_node':
    case 'linux_host':
      return (
        <svg width="18" height="18" viewBox="0 0 20 20" fill="none">
          <rect x="2" y="4" width="16" height="12" rx="2" stroke={color} strokeWidth="1.5" />
          <rect x="4" y="7" width="3" height="2" rx="0.5" fill={color} />
          <line x1="9" y1="8" x2="14" y2="8" stroke={color} strokeWidth="1" />
          <line x1="9" y1="10.5" x2="14" y2="10.5" stroke={color} strokeWidth="1" />
        </svg>
      )
    case 'synology':
      return (
        <svg width="18" height="18" viewBox="0 0 20 20" fill="none">
          <rect x="3" y="3" width="14" height="14" rx="2" stroke={color} strokeWidth="1.5" />
          <circle cx="10" cy="7.5" r="2" stroke={color} strokeWidth="1" />
          <circle cx="10" cy="12.5" r="2" stroke={color} strokeWidth="1" />
          <circle cx="14.5" cy="7.5" r="0.8" fill={color} />
          <circle cx="14.5" cy="12.5" r="0.8" fill={color} />
        </svg>
      )
    case 'vm_linux':
    case 'vm_windows':
    case 'vm_other':
      return (
        <svg width="18" height="18" viewBox="0 0 20 20" fill="none">
          <rect x="3" y="4" width="14" height="12" rx="2" stroke={color} strokeWidth="1.5" />
          <polyline points="7,9 10,6 13,9" stroke={color} strokeWidth="1" fill="none" />
          <polyline points="7,11 10,14 13,11" stroke={color} strokeWidth="1" fill="none" />
        </svg>
      )
    case 'windows_host':
      return (
        <svg width="18" height="18" viewBox="0 0 20 20" fill="none">
          <rect x="2"    y="3"    width="7.5" height="6.5" rx="0.5" fill={color} opacity="0.8" />
          <rect x="10.5" y="3"    width="7.5" height="6.5" rx="0.5" fill={color} opacity="0.8" />
          <rect x="2"    y="10.5" width="7.5" height="6.5" rx="0.5" fill={color} opacity="0.8" />
          <rect x="10.5" y="10.5" width="7.5" height="6.5" rx="0.5" fill={color} opacity="0.8" />
        </svg>
      )
    case 'docker_engine':
      return (
        <svg width="18" height="18" viewBox="0 0 20 20" fill="none">
          <rect x="2"  y="9" width="3" height="3" rx="0.5" stroke={color} strokeWidth="1.2" />
          <rect x="6"  y="9" width="3" height="3" rx="0.5" stroke={color} strokeWidth="1.2" />
          <rect x="10" y="9" width="3" height="3" rx="0.5" stroke={color} strokeWidth="1.2" />
          <rect x="6"  y="5" width="3" height="3" rx="0.5" stroke={color} strokeWidth="1.2" />
          <rect x="10" y="5" width="3" height="3" rx="0.5" stroke={color} strokeWidth="1.2" />
          <path d="M14.5 10.5 Q18 8.5 16.5 5.5" stroke={color} strokeWidth="1.2" fill="none" />
        </svg>
      )
    case 'traefik':
    case 'traefik_router':
    case 'traefik_service':
      return (
        <svg width="18" height="18" viewBox="0 0 20 20" fill="none">
          <polyline points="3,10 8,5 13,10 8,15" stroke={color} strokeWidth="1.4" fill="none" />
          <line x1="8" y1="5" x2="17" y2="5" stroke={color} strokeWidth="1.2" />
          <line x1="8" y1="10" x2="17" y2="10" stroke={color} strokeWidth="1.2" />
          <line x1="8" y1="15" x2="17" y2="15" stroke={color} strokeWidth="1.2" />
        </svg>
      )
    case 'portainer':
      return (
        <svg width="18" height="18" viewBox="0 0 20 20" fill="none">
          <rect x="3" y="3" width="14" height="14" rx="2" stroke={color} strokeWidth="1.5" />
          <circle cx="10" cy="10" r="4" stroke={color} strokeWidth="1.2" />
          <circle cx="10" cy="10" r="1.5" fill={color} />
        </svg>
      )
    default:
      return (
        <svg width="18" height="18" viewBox="0 0 20 20" fill="none">
          <circle cx="10" cy="10" r="7" stroke={color} strokeWidth="1.5" />
          <circle cx="10" cy="10" r="2.5" fill={color} />
        </svg>
      )
  }
}

// ── Node components ───────────────────────────────────────────────────────────

type InfraNodeData = { component: InfrastructureComponent }

const InfraNode = memo(function InfraNode({ data }: NodeProps) {
  const onEdit = useContext(EditContext)
  const { component } = data as InfraNodeData
  const color = TYPE_COLOR[component.type] ?? '#64748b'
  const dotColor = statusDotColor(component.last_status)

  return (
    <div
      style={{
        background: '#0d1117',
        border: `1.5px solid ${color}`,
        borderRadius: 8,
        padding: '10px 12px',
        minWidth: 175,
        fontFamily: '"JetBrains Mono", monospace',
        cursor: 'default',
        userSelect: 'none',
      }}
      onDoubleClick={() => onEdit(component)}
    >
      <Handle type="target" position={Position.Top}    style={HANDLE_STYLE} />

      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
        <TypeIcon type={component.type} color={color} />
        <div style={{ overflow: 'hidden' }}>
          <div style={{ fontSize: 12, fontWeight: 600, color: '#c8d4e0', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 115 }}>
            {component.name}
          </div>
          <div style={{ fontSize: 9, color: color, marginTop: 1, letterSpacing: '0.04em', textTransform: 'uppercase' }}>
            {TYPE_LABEL[component.type] ?? component.type}
          </div>
        </div>
      </div>

      {component.ip && (
        <div style={{ fontSize: 10, color: '#475569', marginBottom: 6, fontFamily: 'monospace' }}>
          {component.ip}
        </div>
      )}

      <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
        <div style={{ width: 7, height: 7, borderRadius: '50%', background: dotColor, boxShadow: `0 0 5px ${dotColor}66`, flexShrink: 0 }} />
        <span style={{ fontSize: 10, color: dotColor, textTransform: 'capitalize' }}>
          {component.last_status ?? 'unknown'}
        </span>
      </div>

      <Handle type="source" position={Position.Bottom} style={HANDLE_STYLE} />
    </div>
  )
})

// ── Container node ────────────────────────────────────────────────────────────

type ContainerNodeData = { container: DiscoveredContainer }

const ContainerNode = memo(function ContainerNode({ data }: NodeProps) {
  const { container } = data as ContainerNodeData
  const dotColor = statusDotColor(container.status)
  const imgShort = (() => {
    const img = container.image
    if (img.startsWith('sha256:')) return 'sha256:' + img.slice(7, 15) + '…'
    const c = img.lastIndexOf(':')
    if (c === -1) return img.length > 28 ? img.slice(0, 27) + '…' : img
    const name = img.slice(0, c)
    const tag  = img.slice(c + 1)
    const short = name.length > 20 ? name.slice(name.lastIndexOf('/') + 1) : name
    return `${short.length > 18 ? short.slice(0, 17) + '…' : short}:${tag}`
  })()

  return (
    <div style={{
      background: '#0d1117',
      border: `1.5px solid ${CONTAINER_COLOR}`,
      borderRadius: 8,
      padding: '8px 12px',
      minWidth: 165,
      fontFamily: '"JetBrains Mono", monospace',
      cursor: 'default',
      userSelect: 'none',
    }}>
      <Handle type="target" position={Position.Top}    style={HANDLE_STYLE} />

      <div style={{ display: 'flex', alignItems: 'center', gap: 7, marginBottom: 5 }}>
        <svg width="14" height="14" viewBox="0 0 20 20" fill="none" style={{ flexShrink: 0 }}>
          <rect x="3"  y="10" width="3.5" height="3.5" rx="0.5" stroke={CONTAINER_COLOR} strokeWidth="1.2" />
          <rect x="7.5" y="10" width="3.5" height="3.5" rx="0.5" stroke={CONTAINER_COLOR} strokeWidth="1.2" />
          <rect x="12" y="10" width="3.5" height="3.5" rx="0.5" stroke={CONTAINER_COLOR} strokeWidth="1.2" />
          <rect x="7.5" y="5.5" width="3.5" height="3.5" rx="0.5" stroke={CONTAINER_COLOR} strokeWidth="1.2" />
          <rect x="12" y="5.5" width="3.5" height="3.5" rx="0.5" stroke={CONTAINER_COLOR} strokeWidth="1.2" />
        </svg>
        <div style={{ fontSize: 11, fontWeight: 600, color: '#c8d4e0', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 118 }}>
          {container.container_name}
        </div>
      </div>

      <div style={{ fontSize: 9, color: '#475569', marginBottom: 6, fontFamily: 'monospace', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
        {imgShort}
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
        <div style={{ width: 6, height: 6, borderRadius: '50%', background: dotColor, boxShadow: `0 0 4px ${dotColor}66`, flexShrink: 0 }} />
        <span style={{ fontSize: 9, color: dotColor, textTransform: 'capitalize' }}>{container.status}</span>
      </div>

      <Handle type="source" position={Position.Bottom} style={HANDLE_STYLE} />
    </div>
  )
})

// ── App node ──────────────────────────────────────────────────────────────────

type AppNodeData = { app: App }

const AppNode = memo(function AppNode({ data }: NodeProps) {
  const [imgFailed, setImgFailed] = useState(false)
  const { app } = data as AppNodeData

  return (
    <div style={{
      background: '#0d1117',
      border: `1.5px solid ${APP_COLOR}`,
      borderRadius: 8,
      padding: '8px 12px',
      minWidth: 150,
      fontFamily: '"JetBrains Mono", monospace',
      cursor: 'default',
      userSelect: 'none',
    }}>
      <Handle type="target" position={Position.Top}    style={HANDLE_STYLE} />

      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <div style={{
          width: 26, height: 26, borderRadius: 5,
          background: '#151c25', border: `1px solid ${APP_COLOR}40`,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          flexShrink: 0, overflow: 'hidden',
        }}>
          {app.profile_id && !imgFailed ? (
            <img
              src={`/api/v1/icons/${app.profile_id}`}
              alt={app.name}
              onError={() => setImgFailed(true)}
              style={{ width: '100%', height: '100%', objectFit: 'contain', padding: 3 }}
            />
          ) : (
            <span style={{ fontSize: 9, fontWeight: 700, color: APP_COLOR }}>
              {app.name.trim().slice(0, 2).toUpperCase()}
            </span>
          )}
        </div>
        <div>
          <div style={{ fontSize: 11, fontWeight: 600, color: '#c8d4e0', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 100 }}>
            {app.name}
          </div>
          <div style={{ fontSize: 9, color: APP_COLOR, marginTop: 1, letterSpacing: '0.04em', textTransform: 'uppercase' }}>
            Application
          </div>
        </div>
      </div>

      <Handle type="source" position={Position.Bottom} style={HANDLE_STYLE} />
    </div>
  )
})

// ── Monitor node ──────────────────────────────────────────────────────────────

type MonitorNodeData = { monitor: MonitorCheck }

const CHECK_TYPE_COLOR: Record<string, string> = {
  ping: '#a78bfa',
  url:  '#5dade2',
  ssl:  '#fbbf24',
  dns:  '#34d399',
}

const MonitorNode = memo(function MonitorNode({ data }: NodeProps) {
  const { monitor } = data as MonitorNodeData
  const dotColor = statusDotColor(monitor.last_status)
  const typeColor = CHECK_TYPE_COLOR[monitor.type] ?? MONITOR_COLOR

  return (
    <div style={{
      background: '#0d1117',
      border: `1.5px solid ${MONITOR_COLOR}`,
      borderRadius: 8,
      padding: '8px 12px',
      minWidth: 150,
      fontFamily: '"JetBrains Mono", monospace',
      cursor: 'default',
      userSelect: 'none',
    }}>
      <Handle type="target" position={Position.Top} style={HANDLE_STYLE} />

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 5 }}>
        <div style={{ fontSize: 11, fontWeight: 600, color: '#c8d4e0', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 100 }}>
          {monitor.name}
        </div>
        <span style={{
          fontSize: 8, fontWeight: 700, letterSpacing: '0.05em', textTransform: 'uppercase',
          background: `${typeColor}20`, color: typeColor, border: `1px solid ${typeColor}40`,
          borderRadius: 3, padding: '1px 5px', marginLeft: 6, flexShrink: 0,
        }}>
          {monitor.type}
        </span>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
        <div style={{ width: 6, height: 6, borderRadius: '50%', background: dotColor, boxShadow: `0 0 4px ${dotColor}66`, flexShrink: 0 }} />
        <span style={{ fontSize: 9, color: dotColor, textTransform: 'capitalize' }}>
          {monitor.last_status ?? 'unknown'}
        </span>
      </div>
    </div>
  )
})

const nodeTypes: NodeTypes = {
  infraNode:     InfraNode,
  containerNode: ContainerNode,
  appNode:       AppNode,
  monitorNode:   MonitorNode,
}

// ── Inner flow component ──────────────────────────────────────────────────────

interface InnerProps {
  components:  InfrastructureComponent[]
  links:       ComponentLink[]
  containers:  DiscoveredContainer[]
  apps:        App[]
  monitors:    MonitorCheck[]
  onEditComponent: (c: InfrastructureComponent) => void
}

function edgeStyle(color: string) {
  return {
    style: { stroke: color, strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color, width: 10, height: 10 },
  }
}

function InfraNetworkMapInner({ components, links, containers, apps, monitors, onEditComponent }: InnerProps) {
  const { fitView } = useReactFlow()
  const savedPositions = useRef<Record<string, { x: number; y: number }>>(loadPositions())
  const onEditRef = useRef(onEditComponent)
  onEditRef.current = onEditComponent
  const didFitView = useRef(false)

  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])

  // Change key — structural changes only
  const infraIds = components.map(c => c.id).sort().join(',')
  const ctrIds   = containers.map(c => c.id).sort().join(',')
  const appIds   = apps.map(a => a.id).sort().join(',')
  const monIds   = monitors.map(m => m.id).sort().join(',')
  const dataKey  = `${infraIds}|${ctrIds}|${appIds}|${monIds}`
  const prevKey  = useRef('')

  useEffect(() => {
    if (dataKey === prevKey.current) {
      // Status-only — patch without rebuilding
      setNodes(prev => prev.map(node => {
        if (node.id.startsWith('infra_')) {
          const c = components.find(c => `infra_${c.id}` === node.id)
          return c ? { ...node, data: { component: c } } : node
        }
        if (node.id.startsWith('ctr_')) {
          const c = containers.find(c => `ctr_${c.id}` === node.id)
          return c ? { ...node, data: { container: c } } : node
        }
        if (node.id.startsWith('mon_')) {
          const m = monitors.find(m => `mon_${m.id}` === node.id)
          return m ? { ...node, data: { monitor: m } } : node
        }
        return node
      }))
      return
    }
    prevKey.current = dataKey

    // Build ID sets
    const infraIdSet = new Set(components.map(c => c.id))
    const appIdSet   = new Set(apps.map(a => a.id))

    // ── Edges ──────────────────────────────────────────────────────────────────
    // 1) Infra → Infra (via component_links)
    const infraEdges: Edge[] = links
      .filter(l => infraIdSet.has(l.parent_id) && infraIdSet.has(l.child_id))
      .map(l => ({
        id: `e_ii_${l.parent_id}_${l.child_id}`,
        source: `infra_${l.parent_id}`,
        target: `infra_${l.child_id}`,
        ...edgeStyle(EDGE_INFRA),
      }))

    // 2) Infra → Container (via container.infra_component_id)
    const ctrEdges: Edge[] = containers
      .filter(c => c.infra_component_id && infraIdSet.has(c.infra_component_id))
      .map(c => ({
        id: `e_ic_${c.infra_component_id}_${c.id}`,
        source: `infra_${c.infra_component_id}`,
        target: `ctr_${c.id}`,
        ...edgeStyle(EDGE_CONTAINER),
      }))

    // 3) Container → App (via container.app_id)
    const appEdges: Edge[] = containers
      .filter(c => c.app_id && appIdSet.has(c.app_id))
      .map(c => ({
        id: `e_ca_${c.id}_${c.app_id}`,
        source: `ctr_${c.id}`,
        target: `app_${c.app_id}`,
        ...edgeStyle(EDGE_APP),
      }))

    // 4) App → Monitor (via monitor.app_id)
    const monEdges: Edge[] = monitors
      .filter(m => m.app_id && appIdSet.has(m.app_id))
      .map(m => ({
        id: `e_am_${m.app_id}_${m.id}`,
        source: `app_${m.app_id}`,
        target: `mon_${m.id}`,
        ...edgeStyle(EDGE_MONITOR),
      }))

    const allEdges = [...infraEdges, ...ctrEdges, ...appEdges, ...monEdges]

    // ── Layout ─────────────────────────────────────────────────────────────────
    const layoutNodes = [
      ...components.map(c => ({ id: `infra_${c.id}` })),
      ...containers.map(c => ({ id: `ctr_${c.id}` })),
      ...apps.map(a => ({ id: `app_${a.id}` })),
      ...monitors.map(m => ({ id: `mon_${m.id}` })),
    ]
    const layoutEdges = allEdges.map(e => ({ source: e.source as string, target: e.target as string }))
    const auto = computeTreeLayout(layoutNodes, layoutEdges)

    // ── Nodes ──────────────────────────────────────────────────────────────────
    const newNodes: Node[] = [
      ...components.map(c => ({
        id: `infra_${c.id}`,
        type: 'infraNode',
        position: savedPositions.current[`infra_${c.id}`] ?? auto[`infra_${c.id}`] ?? { x: 0, y: 0 },
        data: { component: c },
        draggable: true,
      })),
      ...containers.map(c => ({
        id: `ctr_${c.id}`,
        type: 'containerNode',
        position: savedPositions.current[`ctr_${c.id}`] ?? auto[`ctr_${c.id}`] ?? { x: 0, y: 0 },
        data: { container: c },
        draggable: true,
      })),
      ...apps.map(a => ({
        id: `app_${a.id}`,
        type: 'appNode',
        position: savedPositions.current[`app_${a.id}`] ?? auto[`app_${a.id}`] ?? { x: 0, y: 0 },
        data: { app: a },
        draggable: true,
      })),
      ...monitors.map(m => ({
        id: `mon_${m.id}`,
        type: 'monitorNode',
        position: savedPositions.current[`mon_${m.id}`] ?? auto[`mon_${m.id}`] ?? { x: 0, y: 0 },
        data: { monitor: m },
        draggable: true,
      })),
    ]

    setNodes(newNodes)
    setEdges(allEdges)
    didFitView.current = false
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dataKey, components, containers, apps, monitors])

  useEffect(() => {
    if (!didFitView.current && nodes.length > 0) {
      didFitView.current = true
      requestAnimationFrame(() => { fitView({ padding: 0.15, duration: 500 }) })
    }
  }, [nodes, fitView])

  const onNodeDragStop: OnNodeDrag = useCallback((_: React.MouseEvent, node: Node) => {
    savedPositions.current[node.id] = node.position
    savePositions(savedPositions.current)
  }, [])

  const onNodeDoubleClick = useCallback((_event: React.MouseEvent, node: Node) => {
    if (node.id.startsWith('infra_')) {
      const id = node.id.slice(6)
      const c = components.find(c => c.id === id)
      if (c) onEditRef.current(c)
    }
  }, [components])

  return (
    <EditContext.Provider value={(c) => onEditRef.current(c)}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeDragStop={onNodeDragStop}
        onNodeDoubleClick={onNodeDoubleClick}
        nodeTypes={nodeTypes}
        nodesConnectable={false}
        deleteKeyCode={null}
        style={{ background: '#080b0f' }}
        fitView
      >
        <Background variant={BackgroundVariant.Dots} gap={24} size={1} color="#1a2030" />
        <Controls
          position="bottom-right"
          style={{ background: '#0d1117', border: '1px solid #1e2530', borderRadius: 6 }}
        />
        <MiniMap
          position="bottom-left"
          style={{ background: '#080b0f', border: '1px solid #1e2530', borderRadius: 6 }}
          nodeColor={(node) => {
            if (node.id.startsWith('infra_')) {
              const d = node.data as InfraNodeData | undefined
              return d ? (TYPE_COLOR[d.component.type] ?? '#445566') : '#445566'
            }
            if (node.id.startsWith('ctr_')) return CONTAINER_COLOR
            if (node.id.startsWith('app_')) return APP_COLOR
            if (node.id.startsWith('mon_')) return MONITOR_COLOR
            return '#445566'
          }}
          maskColor="#08090c99"
        />
      </ReactFlow>
    </EditContext.Provider>
  )
}

// ── Exported component ────────────────────────────────────────────────────────

export interface InfraNetworkMapProps {
  components:  InfrastructureComponent[]
  links?:      ComponentLink[]
  containers?: DiscoveredContainer[]
  apps?:       App[]
  monitors?:   MonitorCheck[]
  onEditComponent: (c: InfrastructureComponent) => void
}

export function InfraNetworkMap({
  components,
  links      = [],
  containers = [],
  apps       = [],
  monitors   = [],
  onEditComponent,
}: InfraNetworkMapProps) {
  const total = components.length + containers.length + apps.length + monitors.length
  if (total === 0) {
    return (
      <div className="infra-map-empty">
        No infrastructure configured yet. Add a component to see the topology map.
      </div>
    )
  }

  return (
    <div className="infra-map-container">
      <ReactFlowProvider>
        <InfraNetworkMapInner
          components={components}
          links={links}
          containers={containers}
          apps={apps}
          monitors={monitors}
          onEditComponent={onEditComponent}
        />
      </ReactFlowProvider>
    </div>
  )
}
