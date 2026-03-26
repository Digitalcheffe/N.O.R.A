/* ── NORA API Types ──────────────────────────────────────────────────────────
   TypeScript interfaces mirroring the Go models in /internal/models/models.go.
   ──────────────────────────────────────────────────────────────────────────── */

// ── Shared ──────────────────────────────────────────────────────────────────

export interface ListResponse<T> {
  data: T[]
  total: number
}

// ── Auth ────────────────────────────────────────────────────────────────────

export interface LoginInput {
  email: string
  password: string
}

export interface AuthUser {
  id: string
  email: string
  role: 'admin' | 'member'
}

// ── Users ───────────────────────────────────────────────────────────────────

export interface User {
  id: string
  email: string
  role: 'admin' | 'member'
  created_at: string
}

export interface CreateUserInput {
  email: string
  password: string
  role: 'admin' | 'member'
}

// ── Apps ────────────────────────────────────────────────────────────────────

export interface App {
  id: string
  name: string
  token: string
  profile_id: string | null
  docker_engine_id: string | null
  config: Record<string, unknown>
  rate_limit: number
  created_at: string
}

export interface CreateAppInput {
  name: string
  profile_id?: string
  docker_engine_id?: string
  config?: Record<string, unknown>
  rate_limit?: number
}

// ── Events ──────────────────────────────────────────────────────────────────

export type Severity = 'debug' | 'info' | 'warn' | 'error' | 'critical'

export interface Event {
  id: string
  app_id: string
  received_at: string
  severity: Severity
  display_text: string
  raw_payload: Record<string, unknown>
  fields: Record<string, unknown>
}

export interface EventFilter {
  app_id?: string
  severity?: Severity
  from?: string
  to?: string
  limit?: number
  offset?: number
}

// ── Monitor Checks ──────────────────────────────────────────────────────────

export type CheckType = 'ping' | 'url' | 'ssl'
export type CheckStatus = 'up' | 'warn' | 'down'

export interface MonitorCheck {
  id: string
  app_id: string | null
  name: string
  type: CheckType
  target: string
  interval_secs: number
  expected_status: number | null
  ssl_warn_days: number
  ssl_crit_days: number
  enabled: boolean
  last_checked_at: string | null
  last_status: CheckStatus | null
  last_result: string | null
  created_at: string
}

export interface CreateCheckInput {
  app_id?: string
  name: string
  type: CheckType
  target: string
  interval_secs?: number
  expected_status?: number
  ssl_warn_days?: number
  ssl_crit_days?: number
  enabled?: boolean
}

// ── Topology ─────────────────────────────────────────────────────────────────

export type PhysicalHostType = 'bare_metal' | 'proxmox_node'
export type VirtualHostType = 'vm' | 'lxc' | 'wsl'
export type SocketType = 'local' | 'remote_proxy'

export interface PhysicalHost {
  id: string
  name: string
  ip: string
  type: PhysicalHostType
  notes: string | null
  created_at: string
}

export interface VirtualHost {
  id: string
  physical_host_id: string | null
  name: string
  ip: string
  type: VirtualHostType
  created_at: string
}

export interface DockerEngine {
  id: string
  virtual_host_id: string | null
  name: string
  socket_type: SocketType
  socket_path: string
  created_at: string
}

// ── Dashboard ─────────────────────────────────────────────────────────────

export interface DashboardSummary {
  status: 'ok' | 'warn' | 'down'
  categories: SummaryCategory[]
  checks_total: number
  checks_up: number
  checks_warn: number
  checks_down: number
}

export interface SummaryCategory {
  label: string
  count: number
  trend: number[]
}

// ── Profile Library ──────────────────────────────────────────────────────────

export type ProfileCapability = 'full' | 'webhook_only' | 'monitor_only' | 'docker_only' | 'limited'

export interface Profile {
  id: string
  name: string
  category: string
  description: string
  capability: ProfileCapability
}

// ── Metrics ──────────────────────────────────────────────────────────────────

export interface AppMetric {
  app_id: string
  period: string
  events_per_hour: number
  avg_payload_bytes: number
  peak_per_minute: number
}
