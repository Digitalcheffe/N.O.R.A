import { memo, useCallback, useContext, createContext, useEffect, useRef } from 'react'
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
import type { InfrastructureComponent, ComponentType } from '../api/types'

// ── Constants ─────────────────────────────────────────────────────────────────

const POSITIONS_KEY = 'nora-infra-map-positions'

const TYPE_COLOR: Record<ComponentType, string> = {
  proxmox_node:  '#3b82f6',
  synology:      '#a855f7',
  vm:            '#6b7280',
  lxc:           '#6b7280',
  bare_metal:    '#6b7280',
  windows_host:  '#6b7280',
  docker_engine: '#14b8a6',
  traefik:       '#f97316',
}

const TYPE_LABEL: Record<ComponentType, string> = {
  proxmox_node:  'Proxmox Node',
  synology:      'Synology NAS',
  vm:            'VM',
  lxc:           'LXC',
  bare_metal:    'Bare Metal',
  windows_host:  'Windows Host',
  docker_engine: 'Docker Engine',
  traefik:       'Traefik',
}

const STATUS_COLOR: Record<string, string> = {
  online:   '#22c55e',
  degraded: '#eab308',
  offline:  '#ef4444',
}

const STATUS_LABEL: Record<string, string> = {
  online:   'Online',
  degraded: 'Degraded',
  offline:  'Offline',
}

// ── Context ───────────────────────────────────────────────────────────────────

const EditContext = createContext<(c: InfrastructureComponent) => void>(() => {})

// ── Layout ────────────────────────────────────────────────────────────────────

const NODE_W = 240
const NODE_H = 180

function computeAutoLayout(components: InfrastructureComponent[]): Record<string, { x: number; y: number }> {
  if (components.length === 0) return {}

  const idSet = new Set(components.map(c => c.id))
  const childrenOf = new Map<string, string[]>()
  const roots: string[] = []

  for (const c of components) {
    if (!c.parent_id || !idSet.has(c.parent_id)) {
      roots.push(c.id)
    } else {
      const arr = childrenOf.get(c.parent_id) ?? []
      arr.push(c.id)
      childrenOf.set(c.parent_id, arr)
    }
  }

  const positions: Record<string, { x: number; y: number }> = {}
  const seen = new Set<string>()
  let layer = roots
  let y = 0

  while (layer.length > 0) {
    const totalW = layer.length * NODE_W
    layer.forEach((id, i) => {
      if (seen.has(id)) return
      seen.add(id)
      positions[id] = {
        x: i * NODE_W - totalW / 2 + NODE_W / 2,
        y,
      }
    })
    const next: string[] = []
    for (const id of layer) {
      next.push(...(childrenOf.get(id) ?? []))
    }
    layer = next
    y += NODE_H
  }

  return positions
}

// ── Persistence ───────────────────────────────────────────────────────────────

function loadPositions(): Record<string, { x: number; y: number }> {
  try {
    return JSON.parse(localStorage.getItem(POSITIONS_KEY) ?? '{}') as Record<string, { x: number; y: number }>
  } catch {
    return {}
  }
}

function savePositions(p: Record<string, { x: number; y: number }>) {
  localStorage.setItem(POSITIONS_KEY, JSON.stringify(p))
}

// ── Node icons ────────────────────────────────────────────────────────────────

function TypeIcon({ type, color }: { type: ComponentType; color: string }) {
  switch (type) {
    case 'proxmox_node':
    case 'bare_metal':
      return (
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <rect x="2" y="4" width="16" height="12" rx="2" stroke={color} strokeWidth="1.5" />
          <rect x="4" y="7" width="3" height="2" rx="0.5" fill={color} />
          <line x1="9" y1="8" x2="14" y2="8" stroke={color} strokeWidth="1" />
          <line x1="9" y1="10.5" x2="14" y2="10.5" stroke={color} strokeWidth="1" />
        </svg>
      )
    case 'synology':
      return (
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <rect x="3" y="3" width="14" height="14" rx="2" stroke={color} strokeWidth="1.5" />
          <circle cx="10" cy="7.5" r="2" stroke={color} strokeWidth="1" />
          <circle cx="10" cy="12.5" r="2" stroke={color} strokeWidth="1" />
          <circle cx="14.5" cy="7.5" r="0.8" fill={color} />
          <circle cx="14.5" cy="12.5" r="0.8" fill={color} />
        </svg>
      )
    case 'vm':
    case 'lxc':
      return (
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <rect x="3" y="4" width="14" height="12" rx="2" stroke={color} strokeWidth="1.5" />
          <polyline points="7,9 10,6 13,9" stroke={color} strokeWidth="1" fill="none" />
          <polyline points="7,11 10,14 13,11" stroke={color} strokeWidth="1" fill="none" />
        </svg>
      )
    case 'windows_host':
      return (
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <rect x="2"  y="3"  width="7.5" height="6.5" rx="0.5" fill={color} opacity="0.8" />
          <rect x="10.5" y="3"  width="7.5" height="6.5" rx="0.5" fill={color} opacity="0.8" />
          <rect x="2"  y="10.5" width="7.5" height="6.5" rx="0.5" fill={color} opacity="0.8" />
          <rect x="10.5" y="10.5" width="7.5" height="6.5" rx="0.5" fill={color} opacity="0.8" />
        </svg>
      )
    case 'docker_engine':
      return (
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <rect x="2"  y="9" width="3" height="3" rx="0.5" stroke={color} strokeWidth="1.2" />
          <rect x="6"  y="9" width="3" height="3" rx="0.5" stroke={color} strokeWidth="1.2" />
          <rect x="10" y="9" width="3" height="3" rx="0.5" stroke={color} strokeWidth="1.2" />
          <rect x="6"  y="5" width="3" height="3" rx="0.5" stroke={color} strokeWidth="1.2" />
          <rect x="10" y="5" width="3" height="3" rx="0.5" stroke={color} strokeWidth="1.2" />
          <path d="M14.5 10.5 Q18 8.5 16.5 5.5" stroke={color} strokeWidth="1.2" fill="none" />
        </svg>
      )
  }
}

// ── Custom node ───────────────────────────────────────────────────────────────

type InfraNodeData = { component: InfrastructureComponent }

const InfraNode = memo(function InfraNode({ data }: NodeProps) {
  const onEdit = useContext(EditContext)
  const { component } = data as InfraNodeData
  const borderColor = TYPE_COLOR[component.type]
  const statusColor = STATUS_COLOR[component.last_status] ?? '#445566'
  const statusText  = STATUS_LABEL[component.last_status]  ?? 'Unknown'

  return (
    <div
      style={{
        background: '#0f1215',
        border: `1.5px solid ${borderColor}`,
        borderRadius: 8,
        padding: '10px 12px',
        minWidth: 160,
        fontFamily: '"JetBrains Mono", monospace',
        cursor: 'default',
        userSelect: 'none',
      }}
      onDoubleClick={() => onEdit(component)}
    >
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: '#1e2530', border: '1px solid #252d38', width: 8, height: 8 }}
      />

      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
        <TypeIcon type={component.type} color={borderColor} />
        <div style={{ overflow: 'hidden' }}>
          <div style={{
            fontSize: 12,
            fontWeight: 500,
            color: '#c8d4e0',
            lineHeight: 1.2,
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            maxWidth: 120,
          }}>
            {component.name}
          </div>
          <div style={{ fontSize: 10, color: '#7a8fa8', marginTop: 2 }}>
            {TYPE_LABEL[component.type]}
          </div>
        </div>
      </div>

      <div style={{ fontSize: 10, color: '#7a8fa8', marginBottom: 8, fontFamily: 'monospace' }}>
        {component.ip || '—'}
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
        <div style={{
          width: 7,
          height: 7,
          borderRadius: '50%',
          background: statusColor,
          flexShrink: 0,
          boxShadow: `0 0 4px ${statusColor}44`,
        }} />
        <span style={{ fontSize: 10, color: statusColor }}>{statusText}</span>
      </div>

      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: '#1e2530', border: '1px solid #252d38', width: 8, height: 8 }}
      />
    </div>
  )
})

const nodeTypes: NodeTypes = { infraNode: InfraNode }

// ── Inner flow component ──────────────────────────────────────────────────────

interface InnerProps {
  components: InfrastructureComponent[]
  onEditComponent: (c: InfrastructureComponent) => void
}

function InfraNetworkMapInner({ components, onEditComponent }: InnerProps) {
  const { fitView } = useReactFlow()
  const savedPositions = useRef<Record<string, { x: number; y: number }>>(loadPositions())
  const prevIdsRef = useRef('')
  const onEditRef = useRef(onEditComponent)
  onEditRef.current = onEditComponent
  const didFitView = useRef(false)

  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])

  const componentIds = components.map(c => c.id).sort().join(',')

  useEffect(() => {
    if (components.length === 0) {
      setNodes([])
      setEdges([])
      prevIdsRef.current = ''
      didFitView.current = false
      return
    }

    if (componentIds === prevIdsRef.current) {
      // Status-only update — patch data without moving nodes
      setNodes(prev =>
        prev.map(node => {
          const c = components.find(c => c.id === node.id)
          return c ? { ...node, data: { component: c } } : node
        })
      )
      return
    }

    // Structural change — rebuild nodes and edges
    prevIdsRef.current = componentIds
    const auto = computeAutoLayout(components)

    const newNodes: Node[] = components.map(c => ({
      id: c.id,
      type: 'infraNode',
      position: savedPositions.current[c.id] ?? auto[c.id] ?? { x: 0, y: 0 },
      data: { component: c } as InfraNodeData,
      draggable: true,
    }))

    const newEdges: Edge[] = components
      .filter(c => c.parent_id)
      .map(c => ({
        id: `e-${c.parent_id}-${c.id}`,
        source: c.parent_id!,
        target: c.id,
        style: {
          stroke: '#252d38',
          strokeDasharray: '5 5',
          strokeWidth: 1.5,
          opacity: 0.7,
        },
        markerEnd: {
          type: MarkerType.Arrow,
          color: '#252d38',
          width: 12,
          height: 12,
        },
      }))

    setNodes(newNodes)
    setEdges(newEdges)
    didFitView.current = false
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [componentIds, components])

  // Fit view after initial layout
  useEffect(() => {
    if (!didFitView.current && nodes.length > 0) {
      didFitView.current = true
      requestAnimationFrame(() => {
        fitView({ padding: 0.2, duration: 400 })
      })
    }
  }, [nodes, fitView])

  const onNodeDragStop: OnNodeDrag = useCallback((_: React.MouseEvent, node: Node) => {
    savedPositions.current[node.id] = node.position
    savePositions(savedPositions.current)
  }, [])

  const onNodeDoubleClick = useCallback((_event: React.MouseEvent, node: Node) => {
    const c = components.find(c => c.id === node.id)
    if (c) onEditRef.current(c)
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
        style={{ background: '#0f1215' }}
        fitView
      >
        <Background
          variant={BackgroundVariant.Dots}
          gap={20}
          size={1}
          color="#1e2530"
        />
        <Controls
          position="bottom-right"
          style={{
            background: '#151920',
            border: '1px solid #1e2530',
            borderRadius: 6,
          }}
        />
        <MiniMap
          position="bottom-left"
          style={{
            background: '#0a0c0f',
            border: '1px solid #1e2530',
            borderRadius: 6,
          }}
          nodeColor={(node) => {
            const d = node.data as InfraNodeData | undefined
            return d ? TYPE_COLOR[d.component.type] : '#445566'
          }}
          maskColor="#0a0c0f99"
        />
      </ReactFlow>
    </EditContext.Provider>
  )
}

// ── Exported component ────────────────────────────────────────────────────────

export interface InfraNetworkMapProps {
  components: InfrastructureComponent[]
  onEditComponent: (c: InfrastructureComponent) => void
}

export function InfraNetworkMap({ components, onEditComponent }: InfraNetworkMapProps) {
  if (components.length < 2) {
    return (
      <div className="infra-map-empty">
        Add more components to see the network map.
      </div>
    )
  }

  return (
    <div className="infra-map-container">
      <ReactFlowProvider>
        <InfraNetworkMapInner components={components} onEditComponent={onEditComponent} />
      </ReactFlowProvider>
    </div>
  )
}
