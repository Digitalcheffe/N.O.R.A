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

export interface LoginResponse {
  token: string
  user: AuthUser
}

export interface ChangePasswordInput {
  current_password: string
  new_password: string
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
  host_component_id?: string | null
  config: Record<string, unknown>
  rate_limit: number
  created_at: string
}

export interface CreateAppInput {
  name: string
  profile_id?: string  // empty string clears the profile, undefined leaves it unchanged
  docker_engine_id?: string
  host_component_id?: string | null
  config?: Record<string, unknown>
  rate_limit?: number
}

// ── Events ──────────────────────────────────────────────────────────────────

export type Severity = 'debug' | 'info' | 'warn' | 'error' | 'critical'

export interface Event {
  id: string
  level: Severity
  source_name: string
  source_type: string
  source_id: string
  title: string
  payload?: Record<string, unknown>
  created_at: string
}

export type EventSort = 'newest' | 'oldest' | 'level_desc' | 'level_asc'

export interface EventFilter {
  source_type?: string
  source_id?: string
  search?: string
  level?: Severity
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

export type CheckType = 'ping' | 'url' | 'ssl' | 'dns'
export type CheckStatus = 'up' | 'warn' | 'down' | 'critical'
export type SSLSource = 'traefik' | 'standalone'
export type DNSRecordType = 'A' | 'AAAA' | 'MX' | 'CNAME' | 'TXT'

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
  dns_record_type: DNSRecordType | null
  dns_expected_value: string | null
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
  dns_record_type?: DNSRecordType
  dns_expected_value?: string
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

export interface DiscoverResult {
  status: string
  discovered: number
  updated: number
  missing: number
  message?: string
  error?: string
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

// ── Proxmox Detail ────────────────────────────────────────────────────────────

export interface ProxmoxStoragePool {
  name: string
  type: string
  used_bytes: number
  total_bytes: number
  used_percent: number
  active: boolean
  node: string
}

export interface ProxmoxGuestInfo {
  vmid: number
  name: string
  guest_type: 'vm' | 'lxc'
  status: 'running' | 'stopped' | 'paused' | string
  cpus: number
  max_mem_bytes: number
  max_disk_bytes: number
  os_type?: string
  network_bridges?: string[]
  tags?: string[]
  onboot: boolean
  uptime: number
  node: string
}

export interface ProxmoxNodeStatusDetail {
  node: string
  cpu_count: number
  total_mem_bytes: number
  uptime: number
  pve_version?: string
  updates_available: number
}

export interface ProxmoxTaskFailure {
  upid: string
  type: string
  object_id?: string
  exit_status: string
  start_time: number
  end_time?: number
  user: string
  node: string
}

// ── Synology Detail ──────────────────────────────────────────────────────────

export interface SynologyMemory {
  used_bytes: number
  total_bytes: number
  percent: number
}

export interface SynologyVolume {
  path: string
  status: string
  used_bytes: number
  total_bytes: number
  percent: number
}

export interface SynologyDisk {
  slot: number
  model: string
  temperature_c: number
  status: string
}

export interface SynologyUpdate {
  available: boolean
  version: string
}

export interface SynologyDetail {
  model: string
  dsm_version: string
  hostname: string
  uptime: string
  uptime_secs: number
  temperature_c: number
  cpu_percent: number
  memory: SynologyMemory
  volumes: SynologyVolume[]
  disks: SynologyDisk[]
  update: SynologyUpdate
  polled_at: string
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
  image_update_available: boolean
  image_last_checked_at: string | null
}

export interface LinkAppInput {
  mode: 'existing' | 'create'
  app_id?: string
  profile_id?: string
  name?: string
  config?: Record<string, unknown>
}

// ── Traefik Expanded (Infra-10/11) ───────────────────────────────────────────

export interface TraefikOverview {
  component_id: string
  version: string
  routers_total: number
  routers_errors: number
  routers_warnings: number
  services_total: number
  services_errors: number
  middlewares_total: number
  updated_at: string | null
}

export interface DiscoveredRoute {
  id: string
  infrastructure_id: string
  router_name: string
  rule: string
  domain: string | null
  backend_service: string | null
  container_id: string | null
  app_id: string | null
  ssl_expiry: string | null
  ssl_issuer: string | null
  last_seen_at: string
  created_at: string
  router_status: string
  provider: string | null
  entry_points: string | null  // JSON array string
  has_tls_resolver: number
  cert_resolver_name: string | null
  service_name: string | null
}

export interface TraefikServiceDetail {
  id: string
  component_id: string
  service_name: string
  service_type: string
  status: string
  server_count: number
  servers_up: number
  servers_down: number
  server_status_json: string  // JSON map: { "http://url:port": "UP"|"DOWN" }
  last_seen: string
}

// ── SNMP Detail ───────────────────────────────────────────────────────────────

export interface SNMPMemory {
  used_bytes: number
  total_bytes: number
  percent: number
}

export interface SNMPDisk {
  label: string
  used_bytes: number
  total_bytes: number
  percent: number
}

export interface SNMPDetail {
  os_description: string
  uptime: string
  hostname: string
  cpu_percent: number
  memory: SNMPMemory
  disks: SNMPDisk[]
  no_data?: boolean
}

// ── Notification Rules ────────────────────────────────────────────────────────

export type RuleOperator = 'is' | 'is_not' | 'contains' | 'does_not_contain'
export type RuleConditionLogic = 'AND' | 'OR'
export type RuleSourceType = 'app' | 'docker' | 'monitor'

export interface RuleCondition {
  field: string
  operator: RuleOperator
  value: string
}

export interface Rule {
  id: string
  name: string
  enabled: boolean
  source_id: string | null
  source_type: RuleSourceType | null
  severity: Severity | null
  conditions: RuleCondition[]
  condition_logic: RuleConditionLogic
  delivery_email: boolean
  delivery_push: boolean
  delivery_webhook: boolean
  webhook_url: string | null
  notif_title: string
  notif_body: string
  created_at: string
  updated_at: string
}

export interface RuleSource {
  id: string | null
  label: string
  type: RuleSourceType | null
}

export interface RuleSourcesResponse {
  sources: RuleSource[]
}

export interface CreateRuleInput {
  name: string
  enabled: boolean
  source_id?: string | null
  source_type?: RuleSourceType | null
  severity?: Severity | null
  conditions: RuleCondition[]
  condition_logic: RuleConditionLogic
  delivery_email: boolean
  delivery_push: boolean
  delivery_webhook: boolean
  webhook_url?: string | null
  notif_title: string
  notif_body: string
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
