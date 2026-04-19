# NORA Features

The complete feature catalog. The [README](../README.md) covers the headlines; this doc covers everything that ships.

---

## Monitoring

- **Ping checks** — ICMP reachability on a schedule
- **URL checks** — HTTP/HTTPS with status code verification
- **SSL checks** — certificate expiry with configurable warning and critical thresholds
- **DNS checks** — query validation with per-record-type baseline + one-click baseline reset
- Per-check run-now, pause/resume, and event history
- Automatic URL + SSL checks created for every Traefik-discovered route

## Event capture

- `POST /api/v1/ingest/{token}` accepts webhook payloads from any app
- Token-based auth per app — compromise one token, the rest are safe
- App profiles normalize payloads into NORA's event model
- Compound match (`match_field` + `and_field`) lets one card count events that satisfy multiple conditions without OR logic
- Full event log with severity filter, source filter, date range, text search, and an event-volume chart

## Dashboard & app views

- Category + widget tiles driven by the digest registry — rows you deactivate disappear from the UI immediately
- Per-app detail page with live metrics, event counts by severity, linked monitor checks, and an infrastructure chain visualization
- Discover Now button per app runs API polling + infra scan + every linked check in one action

## Infrastructure integrations

| Integration | What NORA collects |
|---|---|
| **Docker engine** | Container inventory, resource metrics, health state, image-update detection |
| **Proxmox VE** | Node status, VMs and LXC guests, storage pools, recent task failures, uptime |
| **Portainer** | Multi-endpoint container inventory, per-container CPU/memory |
| **Traefik** | Routers, services, TLS certificates — auto-creates URL + SSL checks |
| **Synology** | System status, storage volumes, uptime |
| **SNMP** | Generic metrics and health from any SNMP-capable host |

## Relationships

Cross-cutting link view between apps, containers, monitors, and infrastructure:

- Link a discovered container to an app
- Link a Traefik route to an app
- Link a monitor check to an app
- Assign a parent to an infra component (enforced topology rules: VMs under Proxmox, Docker under a host, etc.)

## Notifications

- **Web Push** — browser-native push to desktop and mobile, no app store required
- **Alert Rules** — conditions on any event field (severity, source, payload values), AND / OR logic, fire push or email
- **Digest email** — scheduled summary via SMTP, configured in-app (frequency, day of week / month, send hour, timezone)
- VAPID keys auto-generated on first run and persisted under `/data`

## App profiles

- 29 built-in profiles; see [docs/examples](examples/) for profile YAML references and per-app webhook configs
- Custom profile editor — map any webhook payload to NORA's event model from the Settings UI
- **Reload jobs** — edit a custom profile in `/data/templates/custom/` and reload without a container restart
- Compound match and synthesized event_type keys for apps whose payloads don't carry an event name (Ghost, etc.)

## Jobs

Every background job is also a manual button on **Settings → Jobs**:

- **Monitor** — Run All Monitors
- **Scan Engine** — Metrics Scan, Snapshot Scan, Discovery Scan
- **Data** — Daily Resource Rollup, Event Retention Purge, Monthly Rollup, Digest
- **Profiles** — Reload App Templates, Reload Digest Registry
- **Cleanup** — Clean Stale Registry Entries, Clean Stopped Containers (both show a preview modal listing what will be deleted before firing)

## User management

- Admin and Member roles
- Admin-controlled user creation with optional invite email
- Per-user password management with configurable policy
- **Two-Factor Authentication (TOTP)** — any authenticator app (Google Authenticator, Authy, etc.)
- Global MFA enforcement with grace login and per-user exempt flag
- Disable TOTP without losing enrollment

## Storage & retention

- Single SQLite file under `/data`
- Event retention by severity with configurable thresholds
- Hourly resource rollups → daily rollups → monthly rollups kept forever

## Deployment

- Single Docker image (~50 MB final)
- 3-stage build: frontend → Go binary → `alpine:3.19` final
- PWA installable on mobile and desktop
- `nora-cli` tool for admin operations that don't require the server (password reset, etc.)

---

See [ARCHITECTURE.md](ARCHITECTURE.md) for repository layout, data flow, and deployment internals.
