/* ── NORA API Client ─────────────────────────────────────────────────────────
   All API calls go through this file. Components never call fetch() directly.
   ──────────────────────────────────────────────────────────────────────────── */

import type {
  App,
  AppChainResponse,
  ComponentLink,
  AppMetricSnapshot,
  AppTemplate,
  AuthUser,
  ChangePasswordInput,
  CreateAppInput,
  CreateCheckInput,
  CreateRuleInput,
  CreateUserInput,
  UpdateUserInput,
  CustomProfile,
  DashboardSummaryResponse,
  DigestRegistryListResponse,
  DigestSchedule,
  DiscoverResult,
  ContainerDetail,
  DiscoveredContainer,
  DiscoveredRoute,
  Event,
  EventFilter,
  InfrastructureComponent,
  InfrastructureComponentInput,
  InstanceMetrics,
  Job,
  JobRunResult,
  PortainerEndpoint,
  PortainerEndpointSummary,
  DockerEngineSummary,
  PortainerContainerResource,
  LinkAppInput,
  ListResponse,
  LoginInput,
  LoginResponse,
  MFARequiredResponse,
  TOTPSetupResponse,
  TOTPVerifyInput,
  MonitorCheck,
  TopologyNode,
  ProxmoxGuestInfo,
  ProxmoxNodeStatusDetail,
  ProxmoxStoragePool,
  ProxmoxTaskFailure,
  ProxmoxBackupJob,
  ProxmoxBackupFile,
  ResourceSummary,
  Rule,
  RuleSourcesResponse,
  PasswordPolicy,
  SMTPSettings,
  SNMPDetail,
  ScanResult,
  SynologyDetail,
  SendNowResult,
  TimeseriesBucket,
  TimeseriesFilter,
  TraefikOverview,
  TraefikServiceDetail,
  User,
  ValidationResult,
} from './types'

// ── Base request ─────────────────────────────────────────────────────────────

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }

  const res = await fetch(`/api/v1${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (!res.ok) {
    if (res.status === 401 && !path.startsWith('/auth/')) {
      window.dispatchEvent(new CustomEvent('nora:session-expired'))
    }
    let errMsg = `HTTP ${res.status}`
    const text = await res.text().catch(() => '')
    if (text.trim()) {
      try { errMsg = (JSON.parse(text) as { error?: string }).error ?? text.trim() }
      catch { errMsg = text.trim() }
    }
    throw new Error(errMsg)
  }

  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

// ── Auth ─────────────────────────────────────────────────────────────────────

export const auth = {
  me: () =>
    request<AuthUser>('GET', '/auth/me'),

  // login returns either a full session (200) or an MFA challenge (202).
  login: async (input: LoginInput): Promise<LoginResponse | MFARequiredResponse> => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    const res = await fetch('/api/v1/auth/login', {
      method: 'POST',
      headers,
      body: JSON.stringify(input),
    })
    if (res.status === 200 || res.status === 202) {
      return res.json()
    }
    const payload = await res.json().catch(() => ({ error: 'Login failed' }))
    throw new Error(payload.error ?? `HTTP ${res.status}`)
  },

  register: (input: LoginInput) =>
    request<AuthUser>('POST', '/auth/register', input),

  logout: () =>
    request<void>('POST', '/auth/logout'),

  setupRequired: () =>
    request<{ required: boolean }>('GET', '/auth/setup-required'),
}

// ── TOTP ──────────────────────────────────────────────────────────────────────

export const totp = {
  // Initiate TOTP setup — generates secret, returns URI + raw secret for display.
  setup: () =>
    request<TOTPSetupResponse>('GET', '/auth/totp/setup'),

  // Confirm the first code to activate TOTP.
  confirm: (code: string) =>
    request<User>('POST', '/auth/totp/confirm', { code }),

  // Second-step verification during login (uses the mfa_token from auth.login 202).
  verify: (input: TOTPVerifyInput) =>
    request<LoginResponse>('POST', '/auth/totp/verify', input),

  // User disables their own TOTP (requires current code as confirmation).
  disableOwn: (code: string) =>
    request<void>('DELETE', '/auth/totp/self', { code }),

  // User re-enables TOTP using the existing secret (no new QR needed).
  enableOwn: () =>
    request<void>('PUT', '/auth/totp/self/enable'),

  // Admin: disable TOTP for any user.
  adminDisable: (id: string) =>
    request<void>('DELETE', `/users/${id}/totp`),

  // Admin: restore a user's grace login.
  adminResetGrace: (id: string) =>
    request<void>('PUT', `/users/${id}/totp/grace`),
}

// ── Users ─────────────────────────────────────────────────────────────────────

export const users = {
  list: () =>
    request<ListResponse<User>>('GET', '/users'),

  create: (input: CreateUserInput) =>
    request<User>('POST', '/users', input),

  update: (id: string, input: UpdateUserInput) =>
    request<User>('PUT', `/users/${id}`, input),

  setTOTPExempt: (id: string, exempt: boolean) =>
    request<void>('PUT', `/users/${id}/totp/exempt`, { exempt }),

  delete: (id: string) =>
    request<void>('DELETE', `/users/${id}`),

  setPassword: (id: string, password: string) =>
    request<void>('PUT', `/users/${id}/password`, { password }),

  changePassword: (input: ChangePasswordInput) =>
    request<void>('PUT', '/users/me/password', input),
}

// ── Apps ──────────────────────────────────────────────────────────────────────

export const apps = {
  list: () =>
    request<ListResponse<App>>('GET', '/apps'),

  get: (id: string) =>
    request<App>('GET', `/apps/${id}`),

  create: (input: CreateAppInput) =>
    request<App>('POST', '/apps', input),

  update: (id: string, input: Partial<CreateAppInput>) =>
    request<App>('PUT', `/apps/${id}`, input),

  delete: (id: string) =>
    request<void>('DELETE', `/apps/${id}`),

  regenerateToken: (id: string) =>
    request<{ token: string }>('POST', `/apps/${id}/token/regenerate`),

  events: (id: string, filter?: EventFilter) => {
    const params = new URLSearchParams()
    if (filter?.level) params.set('level', filter.level)
    if (filter?.from) params.set('since', filter.from)
    if (filter?.to) params.set('until', filter.to)
    if (filter?.limit) params.set('limit', String(filter.limit))
    if (filter?.offset) params.set('offset', String(filter.offset))
    const qs = params.toString()
    return request<ListResponse<Event>>('GET', `/apps/${id}/events${qs ? '?' + qs : ''}`)
  },

  metrics: (id: string) =>
    request<ListResponse<AppMetricSnapshot>>('GET', `/apps/${id}/metrics`),

  pollNow: (id: string) =>
    request<void>('POST', `/apps/${id}/poll`),

  chain: (id: string) =>
    request<AppChainResponse>('GET', `/apps/${id}/chain`),
}

// ── Events ────────────────────────────────────────────────────────────────────

export const events = {
  list: (filter?: EventFilter) => {
    const params = new URLSearchParams()
    if (filter?.source_type) params.set('source_type', filter.source_type)
    if (filter?.source_id) params.set('source_id', filter.source_id)
    if (filter?.search) params.set('search', filter.search)
    if (filter?.level) params.set('level', filter.level)
    if (filter?.from) params.set('since', filter.from)
    if (filter?.to) params.set('until', filter.to)
    if (filter?.limit) params.set('limit', String(filter.limit))
    if (filter?.offset) params.set('offset', String(filter.offset))
    if (filter?.sort) params.set('sort', filter.sort)
    const qs = params.toString()
    return request<ListResponse<Event>>('GET', `/events${qs ? '?' + qs : ''}`)
  },

  get: (id: string) =>
    request<Event>('GET', `/events/${id}`),

  timeseries: (filter: TimeseriesFilter) => {
    const params = new URLSearchParams()
    params.set('since', filter.since)
    params.set('until', filter.until)
    params.set('granularity', filter.granularity)
    if (filter.severity) params.set('level', filter.severity)
    return request<{ data: TimeseriesBucket[] }>('GET', `/events/timeseries?${params.toString()}`)
  },
}

// ── Monitor Checks ────────────────────────────────────────────────────────────

export const checks = {
  list: () =>
    request<ListResponse<MonitorCheck>>('GET', '/checks'),

  get: (id: string) =>
    request<MonitorCheck>('GET', `/checks/${id}`),

  create: (input: CreateCheckInput) =>
    request<MonitorCheck>('POST', '/checks', input),

  update: (id: string, input: Partial<CreateCheckInput>) =>
    request<MonitorCheck>('PUT', `/checks/${id}`, input),

  delete: (id: string) =>
    request<void>('DELETE', `/checks/${id}`),

  run: (id: string) =>
    request<MonitorCheck>('POST', `/checks/${id}/run`),

  resetBaseline: (id: string) =>
    request<MonitorCheck>('POST', `/checks/${id}/reset-baseline`),

  listEvents: (id: string) =>
    request<ListResponse<Event>>('GET', `/checks/${id}/events`),
}

// ── Topology ──────────────────────────────────────────────────────────────────

export const topology = {
  getTree: () =>
    request<TopologyNode[]>('GET', '/topology'),
}

// ── Dashboard ─────────────────────────────────────────────────────────────────

export const dashboard = {
  summary: (period: string = 'week') =>
    request<DashboardSummaryResponse>('GET', `/dashboard/summary?period=${period}`),

  digest: (period: string) =>
    request<unknown>('GET', `/dashboard/digest/${period}`),
}

// ── App Template Library ──────────────────────────────────────────────────────

export const appTemplates = {
  list: () =>
    request<ListResponse<AppTemplate>>('GET', '/app-templates'),

  get: (id: string) =>
    request<AppTemplate>('GET', `/app-templates/${id}`),

  validate: (yamlContent: string) =>
    request<ValidationResult>('POST', '/app-templates/validate', { yaml: yamlContent }),

  createCustom: (yamlContent: string) =>
    request<CustomProfile>('POST', '/app-templates/custom', { yaml: yamlContent }),

  listCustom: () =>
    request<ListResponse<CustomProfile>>('GET', '/app-templates/custom'),

  deleteCustom: (id: string) =>
    request<void>('DELETE', `/app-templates/custom/${id}`),

  reload: () =>
    request<{ loaded: number }>('POST', '/app-templates/reload'),

  getRaw: (id: string) =>
    request<{ yaml: string }>('GET', `/app-templates/${id}/raw`),
}

// ── Digest ────────────────────────────────────────────────────────────────────

export const digestSettings = {
  getSchedule: () =>
    request<DigestSchedule>('GET', '/digest/schedule'),

  putSchedule: (s: DigestSchedule) =>
    request<DigestSchedule>('PUT', '/digest/schedule', s),

  sendNow: (period?: string) =>
    request<SendNowResult>('POST', `/digest/send-now${period ? `?period=${encodeURIComponent(period)}` : ''}`),
}

export const smtpSettings = {
  get: () =>
    request<SMTPSettings>('GET', '/settings/smtp'),

  put: (s: SMTPSettings) =>
    request<SMTPSettings>('PUT', '/settings/smtp', s),

  test: () =>
    request<{ status: string; to: string }>('POST', '/settings/smtp/test'),
}

export const passwordPolicy = {
  get: () =>
    request<PasswordPolicy>('GET', '/settings/password-policy'),

  put: (p: PasswordPolicy) =>
    request<PasswordPolicy>('PUT', '/settings/password-policy', p),
}

export const mfaSettings = {
  get: () =>
    request<{ required: boolean }>('GET', '/settings/mfa-required'),

  put: (required: boolean) =>
    request<{ required: boolean }>('PUT', '/settings/mfa-required', { required }),
}

export const digestReport = {
  url: (period?: string) =>
    `/api/v1/digest/report${period ? `?period=${encodeURIComponent(period)}` : ''}`,
}

// ── Infrastructure Components ─────────────────────────────────────────────────

export const infrastructure = {
  list: () =>
    request<ListResponse<InfrastructureComponent>>('GET', '/infrastructure'),

  get: (id: string) =>
    request<InfrastructureComponent>('GET', `/infrastructure/${id}`),

  create: (data: InfrastructureComponentInput) =>
    request<InfrastructureComponent>('POST', '/infrastructure', data),

  update: (id: string, data: InfrastructureComponentInput) =>
    request<InfrastructureComponent>('PUT', `/infrastructure/${id}`, data),

  delete: (id: string) =>
    request<void>('DELETE', `/infrastructure/${id}`),

  resources: (id: string, period = 'hour') =>
    request<ResourceSummary>('GET', `/infrastructure/${id}/resources?period=${period}`),

  dockerSummary: (id: string) =>
    request<DockerEngineSummary>('GET', `/infrastructure/${id}/docker-summary`),

  scan: (id: string) =>
    request<ScanResult>('POST', `/infrastructure/${id}/scan`),

  discover: (id: string) =>
    request<DiscoverResult>('POST', `/infrastructure/${id}/discover`),

  events: (id: string, filter?: EventFilter) => {
    const params = new URLSearchParams()
    if (filter?.level) params.set('level', filter.level)
    if (filter?.from) params.set('since', filter.from)
    if (filter?.to) params.set('until', filter.to)
    if (filter?.limit) params.set('limit', String(filter.limit))
    if (filter?.offset) params.set('offset', String(filter.offset))
    if (filter?.sort) params.set('sort', filter.sort)
    if (filter?.search) params.set('search', filter.search)
    const qs = params.toString()
    return request<ListResponse<Event>>('GET', `/infrastructure/${id}/events${qs ? '?' + qs : ''}`)
  },

  snmpDetail: (id: string) =>
    request<SNMPDetail>('GET', `/infrastructure/${id}/snmp`),

  children: (id: string) =>
    request<ListResponse<InfrastructureComponent>>('GET', `/infrastructure/${id}/children`),

  linkedApps: (id: string) =>
    request<ListResponse<App>>('GET', `/infrastructure/${id}/apps`),

  linkApp: (componentId: string, appId: string) =>
    request<void>('POST', `/infrastructure/${componentId}/apps/${appId}`),

  unlinkApp: (componentId: string, appId: string) =>
    request<void>('DELETE', `/infrastructure/${componentId}/apps/${appId}`),
}

// ── Component Links ───────────────────────────────────────────────────────────

export const links = {
  list: () =>
    request<ListResponse<ComponentLink>>('GET', '/links'),
  setParent: (parentType: string, parentId: string, childType: string, childId: string) =>
    request<void>('POST', '/links', { parent_type: parentType, parent_id: parentId, child_type: childType, child_id: childId }),
  removeParent: (childType: string, childId: string) =>
    request<void>('DELETE', `/links?child_type=${encodeURIComponent(childType)}&child_id=${encodeURIComponent(childId)}`),
}

// ── Synology Detail ───────────────────────────────────────────────────────────

export const synology = {
  detail: (id: string) =>
    request<{ data: SynologyDetail; no_data?: boolean }>('GET', `/infrastructure/${id}/synology`),
}

// ── Proxmox Detail ────────────────────────────────────────────────────────────

export const proxmox = {
  storage: (id: string) =>
    request<ListResponse<ProxmoxStoragePool>>('GET', `/infrastructure/proxmox/${id}/storage`),

  guests: (id: string) =>
    request<ListResponse<ProxmoxGuestInfo>>('GET', `/infrastructure/proxmox/${id}/guests`),

  nodeStatus: (id: string) =>
    request<ListResponse<ProxmoxNodeStatusDetail>>('GET', `/infrastructure/proxmox/${id}/status`),

  taskFailures: (id: string) =>
    request<ListResponse<ProxmoxTaskFailure>>('GET', `/infrastructure/proxmox/${id}/tasks`),

  backupJobs: (id: string) =>
    request<ListResponse<ProxmoxBackupJob>>('GET', `/infrastructure/proxmox/${id}/backups`),

  backupFiles: (id: string) =>
    request<ListResponse<ProxmoxBackupFile>>('GET', `/infrastructure/proxmox/${id}/backup-files`),
}

// ── Docker Discovery ──────────────────────────────────────────────────────────

export const discovery = {
  containers: (engineId: string) =>
    request<ListResponse<DiscoveredContainer>>('GET', `/infrastructure/${engineId}/containers`),

  allContainers: () =>
    request<ListResponse<DiscoveredContainer>>('GET', `/containers`),

  getContainer: (id: string) =>
    request<ContainerDetail>('GET', `/discovered-containers/${id}`),

  deleteContainer: (containerId: string) =>
    request<void>('DELETE', `/discovered-containers/${containerId}`),

  linkContainerApp: (containerId: string, body: LinkAppInput) =>
    request<unknown>('POST', `/discovered-containers/${containerId}/link-app`, body),

  unlinkContainerApp: (containerId: string) =>
    request<void>('DELETE', `/discovered-containers/${containerId}/link-app`),

  allRoutes: () =>
    request<ListResponse<DiscoveredRoute>>('GET', `/routes`),

  linkRouteApp: (routeId: string, appId: string) =>
    request<unknown>('POST', `/discovered-routes/${routeId}/link-app`, { mode: 'existing', app_id: appId }),

  unlinkRouteApp: (routeId: string) =>
    request<void>('DELETE', `/discovered-routes/${routeId}/link-app`),
}

// ── Traefik expanded (Infra-10/11) ────────────────────────────────────────────

export const traefik = {
  getOverview: (id: string) =>
    request<TraefikOverview>('GET', `/infrastructure/${id}/traefik/overview`),

  getRouters: (id: string) =>
    request<ListResponse<DiscoveredRoute>>('GET', `/infrastructure/${id}/traefik/routers`),

  getServices: (id: string) =>
    request<ListResponse<TraefikServiceDetail>>('GET', `/infrastructure/${id}/traefik/services`),
}

// ── Metrics ───────────────────────────────────────────────────────────────────

export const metrics = {
  instance: () =>
    request<InstanceMetrics>('GET', '/metrics'),
}

// ── Notification Rules ────────────────────────────────────────────────────────

export const notifyRules = {
  sources: () =>
    request<RuleSourcesResponse>('GET', '/rules/sources'),

  list: () =>
    request<ListResponse<Rule>>('GET', '/rules'),

  get: (id: string) =>
    request<Rule>('GET', `/rules/${id}`),

  create: (input: CreateRuleInput) =>
    request<Rule>('POST', '/rules', input),

  update: (id: string, input: CreateRuleInput) =>
    request<Rule>('PUT', `/rules/${id}`, input),

  delete: (id: string) =>
    request<void>('DELETE', `/rules/${id}`),

  toggle: (id: string) =>
    request<Rule>('PATCH', `/rules/${id}/toggle`),
}

// ── Web Push ──────────────────────────────────────────────────────────────────

interface PushKeys {
  p256dh: string
  auth: string
}

interface PushSubscribeInput {
  endpoint: string
  keys: PushKeys
}

export interface PushSubscription {
  id: string
  user_id: string
  endpoint: string
  created_at: string
}

export const push = {
  vapidPublicKey: () =>
    request<{ public_key: string }>('GET', '/push/vapid-public-key'),

  listSubscriptions: () =>
    request<{ data: PushSubscription[]; total: number }>('GET', '/push/subscriptions'),

  subscribe: (input: PushSubscribeInput) =>
    request<void>('POST', '/push/subscribe', input),

  unsubscribe: (input: { endpoint: string }) =>
    request<void>('DELETE', '/push/subscribe', input),

  removeSubscription: (id: string) =>
    request<void>('DELETE', `/push/subscriptions/${id}`),

  test: () =>
    request<{ status: string }>('POST', '/push/test'),
}

// ── Jobs ──────────────────────────────────────────────────────────────────────

export const jobsApi = {
  list: () =>
    request<{ data: Job[] }>('GET', '/jobs'),

  run: (id: string) =>
    request<JobRunResult>('POST', `/jobs/${id}/run`),
}

// ── Portainer (DD-8) ──────────────────────────────────────────────────────────

export const portainer = {
  listEndpoints: (componentId: string) =>
    request<ListResponse<PortainerEndpoint>>('GET', `/integrations/portainer/${componentId}/endpoints`),

  endpointSummary: (componentId: string, endpointId: number) =>
    request<PortainerEndpointSummary>('GET', `/integrations/portainer/${componentId}/endpoints/${endpointId}/summary`),

  endpointContainers: (componentId: string, endpointId: number) =>
    request<ListResponse<PortainerContainerResource>>('GET', `/integrations/portainer/${componentId}/endpoints/${endpointId}/containers`),
}

// ── Digest Registry ───────────────────────────────────────────────────────────

export const digestRegistry = {
  list: () =>
    request<DigestRegistryListResponse>('GET', '/digest-registry'),

  setActive: (id: string, active: boolean) =>
    request<void>('PUT', `/digest-registry/${id}/active`, { active }),

  delete: (id: string) =>
    request<void>('DELETE', `/digest-registry/${id}`),
}
