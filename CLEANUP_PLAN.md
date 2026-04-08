# NORA Schema & Code Cleanup Plan

Notes from review session. Items to investigate, consolidate, or remove.

---

## 1. Retire `infrastructure_integrations` + all cert tables

The old Traefik integration path (`infrastructure_integrations`) has been superseded by `infrastructure_components`. Cert collection exists in two tables (`traefik_certs`, `traefik_component_certs`) but is not surfaced anywhere in the UI — the only cert feature is the SSL monitor which dials directly or reads expiry inline. Cert inventory will be redesigned and built out properly later.

**What to keep:**
- `monitor_checks` with `type = ssl` — direct TLS dial, expiry validation, warn/crit alerting. This is the only cert feature that matters right now.

**What to remove:**
- `infrastructure_integrations` table
- `traefik_certs` table (old integration path)
- `traefik_component_certs` table (component path — data collected but never surfaced)
- `infra/sync.go` — cert sync into `traefik_certs`
- `monitor/ssl.go` — remove `ssl_source = "traefik"` branch; direct TLS dial is the only path
- `monitor_checks.ssl_source` column — no longer needed once Traefik cert cache path is gone
- `monitor_checks.integration_id` column — only used by the Traefik cert cache path
- `scanner/snapshot/ssl.go` — cert cache references
- `repo/infrastructure.go` — cert INSERT/SELECT
- `repo/traefik_component.go` — cert upsert/list methods
- All cert polling code in the Traefik component poller

**Security note for future redesign:**
- Reading ACME JSON files directly exposes private keys — never do this
- Traefik API returns cert metadata only (no private keys) — safe
- Direct TLS dial reads public cert from handshake only — safe
- Future cert inventory should use Traefik API or TLS dial, never filesystem cert files

**Build out later:**
- Proper cert inventory page tied to Traefik component, using API metadata only

---

## 2. Retire `traefik_routes`

The simple `traefik_routes` table has been superseded by `discovered_routes` which carries full service health, container/app linking, entry points, TLS info, and server counts.

**What to remove:**
- `traefik_routes` table
- `repo/traefik_component.go` — manages `traefik_routes`

**What to verify:**
- `api/infra_components.go` Traefik detail page — likely still reading from `traefik_routes`. Needs to switch to `discovered_routes`.

---

## 3. Consolidate `discovered_routes` service name columns

`backend_service` and `service_name` are redundant. Both store the Traefik service name.

| Column | Added | Current value | Problem |
|--------|-------|---------------|---------|
| `backend_service` | 012 | Stripped name e.g. `nora` | Original column, no provider context |
| `service_name` | 017 | Raw from router API e.g. `nora`, `api@internal` | Almost same as backend_service |
| `provider` | 017 | e.g. `docker`, `file`, `internal` | Stored separately |

**Proposal:** Store one authoritative value in `service_name` as `name@provider` (e.g. `nora@docker`, `api@internal`). Derive the stripped name at query time where needed for container cross-referencing. Stop writing to `backend_service`.

**What to update:**
- `traefik_discovery.go` — compose `service_name = rr.ServiceName + "@" + rr.Provider`
- `repo/discovery.go` — stop writing `backend_service`
- `api/apps.go` — reads `service_name` for chain display (already does)
- `api/docker_discovery.go` — returns `backend_service` to frontend, switch to `service_name`

---

## 4. Evaluate `traefik_services` table

Service health data is now written directly onto `discovered_routes` (migration 042 — `service_status`, `service_type`, `servers_up`, `servers_down`, `servers_total`). The `traefik_services` table may now be unused or duplicate.

**What to check:**
- Is anything still writing to `traefik_services`?
- Is anything reading from it that isn't already covered by `discovered_routes`?
- If not, candidate for removal.

---

## 5. Clarify `metrics` naming

The `metrics` table stores **app webhook pipeline throughput** (events/hour, payload bytes, peak rate) — not infrastructure metrics. The name is misleading.

**Proposal:** Rename to `app_event_metrics` or `app_throughput` to distinguish from `resource_readings` which stores actual infrastructure resource utilization (CPU, RAM, disk).

---

## 6. Verify Traefik detail page data source

`api/infra_components.go` likely renders the Traefik component detail page using `traefik_routes`. Now that `discovered_routes` is the rich data source it should be reading from there instead.

**What to check:**
- What does the Traefik detail page API endpoint read?
- Switch to `discovered_routes` if still on `traefik_routes`

---

## 7. Consolidate `traefik_overview` into `infrastructure_components.traefik_meta`

Same pattern as `snmp_meta` and `synology_meta` — store the Traefik overview snapshot as a JSON blob on the component row instead of a dedicated table.

**What to do:**
- Add `traefik_meta TEXT` column to `infrastructure_components`
- Store version, routers_total, routers_errors, routers_warnings, services_total, services_errors, middlewares_total, updated_at as JSON
- Remove `traefik_overview` table
- Update `scanner/metrics/traefik.go` to write to `traefik_meta`
- Update `api/infra_components.go` `GetTraefikOverview` to read from component row
- Remove `TraefikOverviewRepo` from `repo/traefik_expanded.go` and `repo/store.go`

---

## 8. Consolidate `snmp_meta` + `synology_meta` + `traefik_meta` → `meta`

Rather than adding a new `*_meta` column per component type, replace all of them with a single generic `meta TEXT` column on `infrastructure_components`. Each component type writes its own JSON shape into it.

**What to do:**
- Add `meta TEXT` column to `infrastructure_components`
- Migrate existing `snmp_meta` and `synology_meta` data into `meta`
- Drop `snmp_meta` and `synology_meta` columns (via table rebuild — SQLite can't drop columns directly)
- Skip adding `traefik_meta` separately — use `meta` from the start
- Update all readers/writers: SNMP poller, Synology poller, Traefik poller, detail page APIs

**Future:** any new component type with rich metadata just writes to `meta` — no schema change needed.

---

## 9. Store Docker Engine + Portainer summary in `meta`

Currently the Docker Engine detail page cards (images, volumes, networks, disk usage) are fetched live from the Docker socket on every page load via `infraApi.dockerSummary()`. This should instead be captured during the snapshot cycle and stored in `meta` on the component row.

**What to do:**
- Snapshot job writes Docker summary (containers_running, containers_stopped, images_total, images_dangling, images_disk_bytes, volumes_total, volumes_unused, volumes_disk_bytes, networks_total) into `meta`
- Same for Portainer — capture equivalent summary per endpoint into `meta`
- Detail page reads from `meta` instead of making a live Docker API call
- Remove the live `dockerSummary` API endpoint once `meta` covers it

**Result:** Page load is instant, data is consistent with snapshot timing, no on-demand Docker socket calls from the UI.

---

## 10. Retire `docker_engines` table

The `docker_engines` table (4 columns: id, name, socket_type, socket_path) predates `infrastructure_components` and has been superseded by `infrastructure_components` with `type = docker_engine`, which carries polling status, credentials, last_polled_at, etc.

**What to remove:**
- `docker_engines` table
- `repo/topology.go` — DockerEngineRepo CRUD against `docker_engines`
- `api/topology.go` — serves `docker_engines` in the topology response

**What to verify:**
- Anything consuming the topology `docker_engines` array in the frontend needs to switch to reading from `infrastructure_components` filtered by `type = docker_engine`

---

---

## Verification Checklist

UI checks to run after each phase to confirm nothing broke and the old tables are no longer needed.

---

### Phase 1 — Traefik data consolidation

**After step 2 (service_name consolidation):**
- [ ] Navigate to an app detail page → Infrastructure chain shows Traefik section
- [ ] Traefik route rows show: status dot · rule · `›` · service name (e.g. `nora@docker`) · service status dot · `1/1`
- [ ] No route shows a bare stripped name where `@provider` should be present
- [ ] `GET /api/v1/apps/{id}/chain` — `service` field contains `name@provider` format

**After step 3 (retire traefik_routes, switch detail page):**
- [ ] Navigate to Traefik component detail page
- [ ] Routers table shows all routes with rule, domain, entry points, status
- [ ] Services table shows service name, servers up/down, status
- [ ] `GET /api/v1/infrastructure/{id}/traefik/routers` returns data from `discovered_routes`
- [ ] `GET /api/v1/infrastructure/{id}/traefik/services` returns data from `discovered_routes`
- [ ] Trigger a discovery — routes table updates correctly
- [ ] Confirm `traefik_routes` table is empty (no longer written to)

**After step 4 (retire traefik_services):**
- [ ] Traefik detail page services tab still shows server health counts
- [ ] App chain still shows `servers_up / server_count`
- [ ] `GET /api/v1/infrastructure/{id}/traefik/services` still returns correctly (now from `discovered_routes`)
- [ ] Confirm `traefik_services` table is empty (no longer written to)

---

### Phase 2 — Cert cleanup

**After step 5 (retire infrastructure_integrations + cert tables):**
- [ ] SSL monitor checks still work — navigate to Monitor Checks, run an SSL check manually
- [ ] SSL check result shows expiry date, status (up/warn/down)
- [ ] Creating a new SSL check — form only shows "Standalone" mode, no Traefik option
- [ ] No `ssl_source` or `integration_id` fields in the check form
- [ ] `GET /api/v1/infrastructure` — no integrations returned
- [ ] Confirm `infrastructure_integrations`, `traefik_certs`, `traefik_component_certs` tables are empty

---

### Phase 3 — `meta` column

**After steps 6+7 (traefik_overview + snmp_meta + synology_meta → meta):**
- [ ] Traefik detail page overview cards still show: routers total/errors, services total/errors, middlewares, version
- [ ] SNMP component detail page still shows: sysDescr, hostname, CPU%, RAM, disk
- [ ] Synology component detail page still shows: model, DSM version, uptime, temp, CPU, memory, volumes
- [ ] `GET /api/v1/infrastructure/{id}` — response includes `meta` field with correct JSON for each type
- [ ] Trigger a poll cycle — `meta` updates on the component row
- [ ] Confirm `traefik_overview` table is empty
- [ ] `snmp_meta` and `synology_meta` columns removed from `infrastructure_components`

**After step 8 (Docker Engine + Portainer summary → meta):**
- [ ] Docker Engine detail page cards still show: containers running/stopped/total, images total/dangling/disk, volumes total/unused/disk, networks total
- [ ] Portainer detail page endpoint cards still show equivalent data
- [ ] Data is present on first load without a live Docker API call
- [ ] Trigger a snapshot cycle — cards update with fresh data
- [ ] `GET /api/v1/infrastructure/{id}/docker-summary` endpoint removed (or returns from meta)

---

### Phase 4 — Legacy table removal

**After step 9 (retire docker_engines):**
- [ ] Infrastructure list page shows Docker Engine components correctly
- [ ] Docker Engine detail page loads correctly
- [ ] App chain still shows Docker Engine nodes in the horizontal chain
- [ ] `GET /api/v1/topology` — no `docker_engines` array in response (or empty)
- [ ] Confirm `docker_engines` table is empty

---

### Final — All tables retired

Confirm these tables have zero rows and are no longer written to:
- [ ] `infrastructure_integrations`
- [ ] `traefik_certs`
- [ ] `traefik_component_certs`
- [ ] `traefik_routes`
- [ ] `traefik_services`
- [ ] `traefik_overview`
- [ ] `docker_engines`

---

## Order of attack (suggested)

1. Verify Traefik detail page data source (quick check)
2. Consolidate `service_name` in `discovered_routes`
3. Retire `traefik_routes`, switch detail page to `discovered_routes`
4. Evaluate and retire `traefik_services`
5. Retire `infrastructure_integrations` + all cert tables + cert polling code
6. Consolidate `traefik_overview` → `meta` on `infrastructure_components`
7. Consolidate `snmp_meta` + `synology_meta` → single `meta` column (do alongside 6)
8. Store Docker Engine + Portainer summary in `meta`
9. Retire `docker_engines` table
10. Rename `metrics` table (low priority)
