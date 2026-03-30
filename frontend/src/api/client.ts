/* ── NORA API Client ─────────────────────────────────────────────────────────
   All API calls go through this file. Components never call fetch() directly.
   In dev mode (VITE_DEV_MODE=true) auth headers are skipped.
   ──────────────────────────────────────────────────────────────────────────── */

import type {
  App,
  AppMetric,
  AppTemplate,
  AuthUser,
  CreateAppInput,
  CreateCheckInput,
  CreateIntegrationInput,
  CreateUserInput,
  CustomProfile,
  DashboardSummaryResponse,
  DigestSchedule,
  DiscoveredContainer,
  DiscoveredRoute,
  DockerEngine,
  Event,
  EventFilter,
  HostResources,
  InfraIntegration,
  InfrastructureComponent,
  InfrastructureComponentInput,
  InstanceMetrics,
  IntegrationDriver,
  LinkAppInput,
  ListResponse,
  LoginInput,
  MonitorCheck,
  PhysicalHost,
  ProxmoxGuestInfo,
  ProxmoxNodeStatusDetail,
  ProxmoxStoragePool,
  ProxmoxTaskFailure,
  ResourceHistory,
  ResourceSummary,
  SMTPSettings,
  SNMPDetail,
  ScanResult,
  SendNowResult,
  SyncResult,
  TimeseriesBucket,
  TimeseriesFilter,
  TraefikCert,
  TraefikComponentDetail,
  TraefikOverview,
  TraefikServiceDetail,
  User,
  ValidationResult,
  VirtualHost,
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
    const payload = await res.json().catch(() => ({ error: 'Request failed' }))
    throw new Error(payload.error ?? `HTTP ${res.status}`)
  }

  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

// ── Auth ─────────────────────────────────────────────────────────────────────

export const auth = {
  login: (input: LoginInput) =>
    request<AuthUser>('POST', '/auth/login', input),

  register: (input: LoginInput) =>
    request<AuthUser>('POST', '/auth/register', input),

  logout: () =>
    request<void>('POST', '/auth/logout'),
}

// ── Users ─────────────────────────────────────────────────────────────────────

export const users = {
  list: () =>
    request<ListResponse<User>>('GET', '/users'),

  create: (input: CreateUserInput) =>
    request<User>('POST', '/users', input),

  update: (id: string, input: Partial<CreateUserInput>) =>
    request<User>('PUT', `/users/${id}`, input),

  delete: (id: string) =>
    request<void>('DELETE', `/users/${id}`),
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
    if (filter?.severity) params.set('severity', filter.severity)
    if (filter?.from) params.set('since', filter.from)
    if (filter?.to) params.set('until', filter.to)
    if (filter?.limit) params.set('limit', String(filter.limit))
    if (filter?.offset) params.set('offset', String(filter.offset))
    const qs = params.toString()
    return request<ListResponse<Event>>('GET', `/apps/${id}/events${qs ? '?' + qs : ''}`)
  },

  metrics: (id: string) =>
    request<ListResponse<AppMetric>>('GET', `/apps/${id}/metrics`),
}

// ── Events ────────────────────────────────────────────────────────────────────

export const events = {
  list: (filter?: EventFilter) => {
    const params = new URLSearchParams()
    if (filter?.app_id) params.set('app_id', filter.app_id)
    if (filter?.severity) params.set('severity', filter.severity)
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
    if (filter.app_id) params.set('app_id', filter.app_id)
    if (filter.severity) params.set('severity', filter.severity)
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

  listEvents: (id: string) =>
    request<ListResponse<Event>>('GET', `/checks/${id}/events`),
}

// ── Topology ──────────────────────────────────────────────────────────────────

export const topology = {
  physicalHosts: {
    list: () =>
      request<ListResponse<PhysicalHost>>('GET', '/hosts/physical'),
    create: (input: Omit<PhysicalHost, 'id' | 'created_at'>) =>
      request<PhysicalHost>('POST', '/hosts/physical', input),
    update: (id: string, input: Partial<Omit<PhysicalHost, 'id' | 'created_at'>>) =>
      request<PhysicalHost>('PUT', `/hosts/physical/${id}`, input),
    delete: (id: string) =>
      request<void>('DELETE', `/hosts/physical/${id}`),
    resources: (id: string, period = 'hour') =>
      request<HostResources>('GET', `/hosts/physical/${id}/resources?period=${period}`),
  },

  virtualHosts: {
    list: () =>
      request<ListResponse<VirtualHost>>('GET', '/hosts/virtual'),
    create: (input: Omit<VirtualHost, 'id' | 'created_at'>) =>
      request<VirtualHost>('POST', '/hosts/virtual', input),
    update: (id: string, input: Partial<Omit<VirtualHost, 'id' | 'created_at'>>) =>
      request<VirtualHost>('PUT', `/hosts/virtual/${id}`, input),
    delete: (id: string) =>
      request<void>('DELETE', `/hosts/virtual/${id}`),
  },

  dockerEngines: {
    list: () =>
      request<ListResponse<DockerEngine>>('GET', '/docker-engines'),
    create: (input: Omit<DockerEngine, 'id' | 'created_at'>) =>
      request<DockerEngine>('POST', '/docker-engines', input),
    update: (id: string, input: Partial<Omit<DockerEngine, 'id' | 'created_at'>>) =>
      request<DockerEngine>('PUT', `/docker-engines/${id}`, input),
    delete: (id: string) =>
      request<void>('DELETE', `/docker-engines/${id}`),
  },
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
}

// ── Infrastructure Integrations ───────────────────────────────────────────────

export const integrations = {
  list: () =>
    request<ListResponse<InfraIntegration>>('GET', '/integrations'),

  get: (id: string) =>
    request<InfraIntegration>('GET', `/integrations/${id}`),

  create: (input: CreateIntegrationInput) =>
    request<InfraIntegration>('POST', '/integrations', input),

  update: (id: string, input: Partial<CreateIntegrationInput>) =>
    request<InfraIntegration>('PUT', `/integrations/${id}`, input),

  delete: (id: string) =>
    request<void>('DELETE', `/integrations/${id}`),

  sync: (id: string) =>
    request<SyncResult>('POST', `/integrations/${id}/sync`),

  certs: (id: string) =>
    request<ListResponse<TraefikCert>>('GET', `/integrations/${id}/certs`),
}

// ── Integration Drivers ───────────────────────────────────────────────────────

export const integrationDrivers = {
  list: () =>
    request<ListResponse<IntegrationDriver>>('GET', '/integration-drivers'),

  configure: (name: string, creds: Record<string, string>) =>
    request<{ configured: boolean }>('PUT', `/integration-drivers/${name}`, creds),

  disconnect: (name: string) =>
    request<void>('DELETE', `/integration-drivers/${name}`),
}

// ── Digest ────────────────────────────────────────────────────────────────────

export const digestSettings = {
  getSchedule: () =>
    request<DigestSchedule>('GET', '/digest/schedule'),

  putSchedule: (s: DigestSchedule) =>
    request<DigestSchedule>('PUT', '/digest/schedule', s),

  sendNow: () =>
    request<SendNowResult>('POST', '/digest/send-now'),
}

export const smtpSettings = {
  get: () =>
    request<SMTPSettings>('GET', '/settings/smtp'),

  put: (s: SMTPSettings) =>
    request<SMTPSettings>('PUT', '/settings/smtp', s),

  test: () =>
    request<{ status: string; to: string }>('POST', '/settings/smtp/test'),
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

  traefikDetail: (id: string) =>
    request<TraefikComponentDetail>('GET', `/infrastructure/${id}/traefik`),

  resourceHistory: (id: string, period: 'hour' | 'day' = 'hour', limit = 24) =>
    request<ResourceHistory>('GET', `/infrastructure/${id}/resources/history?period=${period}&limit=${limit}`),

  scan: (id: string) =>
    request<ScanResult>('POST', `/infrastructure/${id}/scan`),

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
}

// ── Docker Discovery ──────────────────────────────────────────────────────────

export const discovery = {
  containers: (engineId: string) =>
    request<ListResponse<DiscoveredContainer>>('GET', `/infrastructure/${engineId}/containers`),

  deleteContainer: (containerId: string) =>
    request<void>('DELETE', `/discovered-containers/${containerId}`),

  linkContainerApp: (containerId: string, body: LinkAppInput) =>
    request<unknown>('POST', `/discovered-containers/${containerId}/link-app`, body),
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
