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
  totp_enabled: boolean
  totp_enrolled: boolean
  totp_grace: boolean
  totp_exempt: boolean
}

export interface LoginResponse {
  token: string
  user: AuthUser
  mfa_enrollment_required?: boolean
  pw_policy_noncompliant?: boolean
}

export interface MFARequiredResponse {
  mfa_required: true
  mfa_token: string
}

export interface TOTPSetupResponse {
  uri: string
  secret: string
}

export interface TOTPVerifyInput {
  mfa_token: string
  code: string
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
  totp_enabled: boolean
  totp_enrolled: boolean
  totp_grace: boolean
  totp_exempt: boolean
}

export interface CreateUserInput {
  email: string
  password: string
  role: 'admin' | 'member'
}

export interface UpdateUserInput {
  email: string
  role: 'admin' | 'member'
}

// ── Apps ────────────────────────────────────────────────────────────────────

export interface App {
  id: string
  name: string
  token: string
  profile_id: string | null
  config: Record<string, unknown>
  rate_limit: number
  created_at: string
}

// ── Component Links ──────────────────────────────────────────────────────────

export interface ComponentLink {
  parent_type: string
  parent_id: string
  child_type: string
  child_id: string
  created_at: string
}

export interface CreateAppInput {
  name: string
  profile_id?: string  // empty string clears the profile, undefined leaves it unchanged
  config?: Record<string, unknown>
  rate_limit?: number
}

export interface AppMetricSnapshot {
  id: string
  app_id: string
  profile_id: string
  metric_name: string
  label: string
  value: string
  value_type: string
  polled_at: string
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
  source_component_id: string | null
  skip_tls_verify: boolean
  dns_record_type: DNSRecordType | null
  dns_expected_value: string | null
  dns_resolver: string | null
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
  skip_tls_verify?: boolean
  dns_record_type?: DNSRecordType
  dns_expected_value?: string
  dns_resolver?: string
  enabled?: boolean
}

// ── Topology ─────────────────────────────────────────────────────────────────

export interface TopologyApp {
  id: string
  name: string
  icon_url?: string
}

export interface TopologyNode {
  id: string
  name: string
  type: string
  status?: string
  ip?: string
  notes?: string
  meta?: string
  app_id?: string
  app_name?: string
  app_icon_url?: string
  children: TopologyNode[]
  apps: TopologyApp[]
}

// ── Dashboard ─────────────────────────────────────────────────────────────

export interface SummaryBarItem {
  label: string
  count: number
  sub: string
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
  icon_url?: string
  capability?: string
  status: 'online' | 'warn' | 'down'
  last_event_at: string | null
  last_event_text: string | null
  stats: AppStat[] | null
  checks_up: number
  checks_total: number
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

export type AppTemplateCapability = 'full' | 'webhook_only' | 'api_only' | 'monitor_only'

export interface AppTemplate {
  id: string
  name: string
  category: string
  description: string
  capability: AppTemplateCapability
  homepage?: string
  icon?: string  // CDN icon slug override; falls back to id
  monitor?: {
    check_type: string
    check_url: string   // raw template, e.g. "{base_url}/ping" — substitute before use
  }
  // api_polling auth defaults — only present on GET /app-templates/{id}
  auth_type?: AppAuthType
  auth_header?: string
}

export type AppAuthType = 'none' | 'apikey_header' | 'apikey_query' | 'bearer' | 'basic'

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

// ── Settings ─────────────────────────────────────────────────────────────────

export type DigestFrequency = 'daily' | 'weekly' | 'monthly'

export interface DigestSchedule {
  frequency: DigestFrequency
  day_of_week: number    // 0–6, used when frequency=weekly
  day_of_month: number   // 1–28, used when frequency=monthly
  send_hour?: number     // 0–23 in server timezone, defaults to 17 when absent
  timezone?: string      // read-only: IANA timezone name from server config (NORA_TIMEZONE)
}

export interface SMTPSettings {
  host: string
  port: number
  user: string
  pass: string
  from: string
  to: string
}

export interface PasswordPolicy {
  min_length: number
  require_uppercase: boolean
  require_number: boolean
  require_special: boolean
}

export interface SendNowResult {
  status: string
  period: string
}

// ── Infrastructure Components ─────────────────────────────────────────────────

export type ComponentType =
  | 'proxmox_node'
  | 'synology'
  | 'vm_linux'
  | 'vm_windows'
  | 'vm_other'
  | 'linux_host'
  | 'windows_host'
  | 'generic_host'
  | 'docker_engine'
  | 'traefik'
  | 'traefik_router'
  | 'traefik_service'
  | 'portainer'

export type CollectionMethod =
  | 'proxmox_api'
  | 'synology_api'
  | 'snmp'
  | 'docker_socket'
  | 'traefik_api'
  | 'portainer_api'
  | 'none'

export interface InfrastructureComponent {
  id: string
  name: string
  ip: string
  type: ComponentType
  collection_method: CollectionMethod
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
  parent_id?: string | null  // written to component_links server-side
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
  ip?: string
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

export interface ProxmoxBackupJob {
  upid: string
  object_id?: string
  exit_status: string
  start_time: number
  end_time?: number
  node: string
}

export interface ProxmoxBackupFile {
  volid: string
  vmid: number
  ctime: number
  size: number
  format: string
  node: string
  store: string
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
  checked: boolean
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
  infra_component_id: string
  source_type: string
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
  // Enrichment fields (AP-04 / migration 037).
  ports: string | null
  labels: string | null
  volumes: string | null
  networks: string | null
  restart_policy: string | null
  docker_created_at: string | null
}

export interface ContainerDetail {
  id: string
  infra_component_id: string
  source_type: string
  container_id: string
  container_name: string
  image: string
  status: string
  app_id: string | null
  last_seen_at: string
  docker_created_at: string | null
  image_digest: string | null
  registry_digest: string | null
  image_update_available: boolean
  image_last_checked_at: string | null
  ports: string | null
  networks: string | null
  volumes: string | null
  restart_policy: string | null
  labels: string | null
  env_vars: string | null
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
  service_name: string | null
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
  // Service health fields (migration 042 / 049)
  service_status: string | null
  service_type: string | null
  servers_total: number
  servers_up: number
  servers_down: number
  servers_json: string | null  // JSON map: { "http://url:port": "UP"|"DOWN" }
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

export interface DepItem {
  name: string
  version: string
  label: string
}

export interface InstanceMetrics {
  version: string
  go_version: string
  sqlite_version: string
  deps: DepItem[]
  db_size_bytes: number
  events_last_24h: number
  uptime_seconds: number
  top_apps: TopAppItem[]
  app_events_24h: AppEventItem[]
}

// ── Jobs ─────────────────────────────────────────────────────────────────────

export interface Job {
  id: string
  name: string
  description: string
  category: string
  last_run_at: string | null
  last_run_status: string | null
}

export interface JobRunResult {
  status: 'ok' | 'error'
  error?: string
  duration_ms: number
}

// ── Portainer (DD-8) ─────────────────────────────────────────────────────────

export interface PortainerEndpoint {
  id: number
  name: string
  type: number
}

export interface PortainerEndpointSummary {
  containers_running: number
  containers_stopped: number
  images_total: number
  images_dangling: number
  images_disk_bytes: number
  volumes_total: number
  volumes_unused: number
  volumes_disk_bytes: number
  networks_total: number
}

// DockerEngineSummary mirrors PortainerEndpointSummary so both detail pages
// can use the same StatCard layout.
export type DockerEngineSummary = PortainerEndpointSummary

export interface PortainerContainerResource {
  id: string
  name: string
  image: string
  state: string
  cpu_percent: number
  mem_bytes: number
  mem_limit_bytes: number
  mem_percent: number
  image_update_available: boolean
  stack?: string
}


export interface DigestRegistryEntry {
  id: string
  profile_id: string
  source: 'webhook' | 'api'
  entry_type: 'category' | 'widget'
  name: string
  label: string
  config: Record<string, string>
  profile_source: string
  active: boolean
  created_at: string
  updated_at: string
}

export interface DigestRegistryListResponse {
  data: DigestRegistryEntry[]
  total: number
}

// ── App Infrastructure Chain ──────────────────────────────────────────────────

export interface ChainNode {
  type: string
  id: string
  name: string
  status: string
  detail?: string
  icon_url?: string
}

export interface ChainTraefikRoute {
  router: string
  rule: string
  service: string
  status: string
  service_status?: string
  servers_up?: number
  servers_down?: number
  server_count?: number
  servers_json?: string | null  // JSON map: { "http://url:port": "UP"|"DOWN" }
  manual_link?: boolean
}

export interface AppChainResponse {
  chain: ChainNode[]
  traefik: ChainTraefikRoute[]
}

