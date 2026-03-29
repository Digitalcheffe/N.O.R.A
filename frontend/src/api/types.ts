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
  profile_id?: string  // empty string clears the profile, undefined leaves it unchanged
  docker_engine_id?: string
  config?: Record<string, unknown>
  rate_limit?: number
}

// ── Events ──────────────────────────────────────────────────────────────────

export type Severity = 'debug' | 'info' | 'warn' | 'error' | 'critical'

export interface Event {
  id: string
  app_id: string
  app_name: string
  received_at: string
  severity: Severity
  display_text: string
  raw_payload: Record<string, unknown>
  fields: Record<string, unknown>
}

export type EventSort = 'newest' | 'oldest' | 'severity_desc' | 'severity_asc'

export interface EventFilter {
  app_id?: string
  severity?: Severity
  from?: string
  to?: string
  limit?: number
  offset?: number
  sort?: EventSort
}

export interface TimeseriesBucket {
  time: string
  count: number
}

export interface TimeseriesFilter {
  since: string
  until: string
  granularity: 'hour' | 'day'
  app_id?: string
  severity?: string
}

// ── Monitor Checks ──────────────────────────────────────────────────────────

export type CheckType = 'ping' | 'url' | 'ssl'
export type CheckStatus = 'up' | 'warn' | 'down' | 'critical'
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
  source_component_id: string | null
  skip_tls_verify: boolean
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
  skip_tls_verify?: boolean
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

// ── App Template Library ──────────────────────────────────────────────────────

export type AppTemplateCapability = 'full' | 'webhook_only' | 'monitor_only' | 'docker_only' | 'limited'

export interface AppTemplate {
  id: string
  name: string
  category: string
  description: string
  capability: AppTemplateCapability
}

// ── Custom App Templates ───────────────────────────────────────────────────────

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

// ── Integration Drivers ───────────────────────────────────────────────────────

export interface IntegrationDriver {
  name: string
  label: string
  description: string
  capabilities: string[]
  configured: boolean
}

// ── Settings ─────────────────────────────────────────────────────────────────

export type DigestFrequency = 'daily' | 'weekly' | 'monthly'

export interface DigestSchedule {
  frequency: DigestFrequency
  day_of_week: number    // 0–6, used when frequency=weekly
  day_of_month: number   // 1–28, used when frequency=monthly
  send_hour?: number     // 0–23, defaults to 8 when absent
}

export interface SMTPSettings {
  host: string
  port: number
  user: string
  pass: string
  from: string
  to: string
}

export interface SendNowResult {
  status: string
  period: string
}

// ── Infrastructure Components ─────────────────────────────────────────────────

export type ComponentType =
  | 'proxmox_node'
  | 'synology'
  | 'vm'
  | 'lxc'
  | 'bare_metal'
  | 'linux_host'
  | 'windows_host'
  | 'generic_host'
  | 'docker_engine'
  | 'traefik'

export type CollectionMethod =
  | 'proxmox_api'
  | 'synology_api'
  | 'snmp'
  | 'docker_socket'
  | 'traefik_api'
  | 'none'

export interface InfrastructureComponent {
  id: string
  name: string
  ip: string
  type: ComponentType
  collection_method: CollectionMethod
  parent_id?: string | null
  snmp_config?: string | null
  notes: string
  enabled: boolean
  last_polled_at?: string | null
  last_status: string
  created_at: string
  has_credentials?: boolean
  credential_meta?: Record<string, unknown>
}

export interface InfrastructureComponentInput {
  name: string
  ip: string
  type: ComponentType
  collection_method: CollectionMethod
  parent_id?: string | null
  credentials?: string | null
  snmp_config?: string | null
  notes?: string
  enabled?: boolean
}

export interface VolumeResource {
  name: string
  percent: number
}

export interface ScanResult {
  component_id: string
  status: string
  last_polled_at: string
  message?: string
  error?: string
}

// ── Traefik Component ─────────────────────────────────────────────────────────

export interface TraefikRoute {
  id: string
  component_id: string
  name: string
  rule: string
  service: string
  status: string
  updated_at: string
}

export interface TraefikCertWithCheck {
  id: string
  domain: string
  issuer?: string | null
  expires_at?: string | null
  sans: string[]
  last_seen_at: string
  check_status: string
  check_id?: string
}

export interface TraefikComponentDetail {
  component_id: string
  cert_count: number
  warn_count: number
  crit_count: number
  certs: TraefikCertWithCheck[]
  routes: TraefikRoute[]
}

export interface ResourceSummary {
  component_id: string
  period: string
  cpu_percent: number
  mem_percent: number
  disk_percent: number
  volumes?: VolumeResource[]
  recorded_at?: string
  no_data?: boolean
}

export interface ResourceRollupPoint {
  period_start: string
  metric: string
  avg: number
  min: number
  max: number
}

export interface ResourceHistory {
  component_id: string
  period: string
  data: ResourceRollupPoint[]
  total: number
}

// ── Docker Discovery ─────────────────────────────────────────────────────────

export interface DiscoveredContainer {
  id: string
  container_name: string
  image: string
  status: string
  app_id: string | null
  profile_suggestion: string | null
  suggestion_confidence: number | null
  cpu_percent: number | null
  mem_percent: number | null
  last_seen_at: string
}

export interface LinkAppInput {
  mode: 'existing' | 'create'
  app_id?: string
  profile_id?: string
  name?: string
  config?: Record<string, unknown>
}

// ── Metrics ──────────────────────────────────────────────────────────────────

export interface AppMetric {
  app_id: string
  period: string
  events_per_hour: number
  avg_payload_bytes: number
  peak_per_minute: number
}

export interface TopAppItem {
  app_id: string
  app_name: string
  events_per_hour: number
}

export interface AppEventItem {
  app_id: string
  app_name: string
  count: number
}

export interface InstanceMetrics {
  db_size_bytes: number
  events_last_24h: number
  uptime_seconds: number
  top_apps: TopAppItem[]
  app_events_24h: AppEventItem[]
}
