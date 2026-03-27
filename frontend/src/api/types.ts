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
export type SSLSource = 'traefik' | 'standalone'

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
  ssl_source: SSLSource | null
  integration_id: string | null
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
  ssl_source?: SSLSource
  integration_id?: string
  enabled?: boolean
}

// ── Infrastructure Integrations ───────────────────────────────────────────────

export type IntegrationType = 'traefik'
export type IntegrationStatus = 'ok' | 'error'

export interface InfraIntegration {
  id: string
  type: IntegrationType
  name: string
  api_url: string
  api_key?: string | null
  enabled: boolean
  last_synced_at: string | null
  last_status: IntegrationStatus | null
  last_error: string | null
  created_at: string
}

export interface CreateIntegrationInput {
  type: IntegrationType
  name: string
  api_url: string
  api_key?: string | null
}

export interface TraefikCert {
  id: string
  integration_id: string
  domain: string
  issuer: string | null
  expires_at: string | null
  sans: string[]
  last_seen_at: string
}

export interface SyncResult {
  status: string
  certs_found: number
  synced_at: string
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

// ── Host Resources ────────────────────────────────────────────────────────────

export interface HostResources {
  cpu: number
  mem: number
  disk: number
}

// ── Dashboard ─────────────────────────────────────────────────────────────

export interface SummaryBarItem {
  label: string
  count: number
  sub: string
  sparkline: number[]
}

export interface AppStat {
  label: string
  value: string
  color?: string
}

export interface AppSummary {
  id: string
  name: string
  profile_id: string
  status: 'online' | 'warn' | 'down'
  last_event_at: string | null
  last_event_text: string | null
  stats: AppStat[] | null
  sparkline: number[]
}

export interface CheckSummary {
  id: string
  name: string
  type: string
  target: string
  status: string
  uptime_pct: number
  last_checked_at?: string
}

export interface SSLCert {
  domain: string
  days_remaining: number
  expires_at: string
  status: string
}

export interface DashboardSummaryResponse {
  status: 'normal' | 'warn' | 'down'
  period: string
  summary_bar: SummaryBarItem[]
  apps: AppSummary[]
  checks: CheckSummary[]
  ssl_certs: SSLCert[]
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

// ── Custom Profiles ───────────────────────────────────────────────────────────

export interface ValidationResult {
  valid: boolean
  errors: string[]
}

export interface CustomProfile {
  id: string
  name: string
  yaml_content: string
  created_at: string
}

// ── Metrics ──────────────────────────────────────────────────────────────────

export interface AppMetric {
  app_id: string
  period: string
  events_per_hour: number
  avg_payload_bytes: number
  peak_per_minute: number
}
