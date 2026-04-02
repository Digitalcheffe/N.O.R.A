# N.O.R.A
### Nexus Operations Recon & Alerts

> Know what's happening in your homelab without it becoming a project.

NORA is a self-hosted monitoring, event capture, and notification platform built for homelabbers and small self-hosted teams. One Docker image. No pipelines to build. No dashboards to configure. It just works.

---

## The Problem

Every homelabber knows they *should* have better visibility into their stack. They know they should get notified before things break instead of after. But standing up Grafana + Prometheus + Loki + Alertmanager is a project, not a solution. So it never happens. And they stay blind.

NORA is what you get when you commit to the thing none of those tools committed to: **visibility without the work.**

---

## What It Does

| | |
|---|---|
| **Monitor** | Actively checks hosts, services, SSL certificates, and DNS on a schedule — no agent required |
| **Capture** | Receives webhook events from apps that support them (Sonarr, n8n, Duplicati, and more) |
| **Observe** | Connects to Docker, Proxmox, Portainer, Traefik, Synology, and SNMP targets for deep visibility |
| **Measure** | Collects CPU, memory, and disk from containers, VMs, and hosts automatically |
| **Store** | Retains events by severity with configurable retention — monthly rollups kept forever |
| **Alert** | Fires rules-based Web Push notifications to any subscribed browser or mobile device |
| **Summarize** | Delivers a scheduled digest email of what happened across your stack |

---

## What It Is Not

- **Not Grafana** — no metrics pipelines, no dashboards you have to build yourself
- **Not Splunk** — no query language, no complex field extraction, no enterprise pricing
- **Not Uptime Kuma** — not just uptime; has event history, context, resource trends, and integrations
- **Not Gotify** — not just notifications; it remembers what your apps told it

---

## Quick Start

```bash
docker run -d \
  -p 8080:8080 \
  -v ./data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e NORA_SECRET=your-secret-here \
  ghcr.io/digitalcheffe/nora:latest
```

Open `http://localhost:8080` — create your admin account and add your first app.

---

## Features

### Monitoring
- **Ping checks** — ICMP reachability on a schedule
- **URL checks** — HTTP/HTTPS with status code verification
- **SSL checks** — certificate expiry detection with configurable warning thresholds
- **DNS checks** — query validation with record type support
- Manual run, baseline reset, and per-check event history

### Event Capture
- Receive webhooks from any app via `POST /api/v1/ingest/{token}`
- Token-based auth per app — compromise one token, the rest are safe
- App profiles normalize payloads into NORA's event model automatically

### Infrastructure Integrations

| Integration | What NORA collects |
|---|---|
| **Docker** | Container discovery, resource metrics, image update detection |
| **Proxmox** | Node status, VMs and LXC guests, storage, task failures |
| **Portainer** | Endpoints, container inventory, image status |
| **Traefik** | Routes, services, SSL certificates |
| **Synology** | System status, storage information |
| **SNMP** | Generic metrics and health from any SNMP-capable device |

### Notifications
- **Web Push** — browser-native push to desktop and mobile, no app store required
- **Alert Rules** — define conditions on any event field, fire notifications when they match
- **Digest Email** — scheduled summary of what happened across your stack via SMTP
- VAPID keys auto-generated on first run

### User Management
- Admin and Member roles
- Admin-controlled user creation with optional invite email on creation
- Per-user password management with configurable policy enforcement
- **Two-Factor Authentication (TOTP)** — time-based codes via any authenticator app (Google Authenticator, Authy, etc.)
- Global MFA enforcement with grace login and per-user exempt flag
- Disable TOTP without losing enrollment — re-enable without re-scanning

### Dashboard
- Summary counts, sparklines, and status rollup across apps, checks, and infrastructure
- Event timeline, check status, and resource trends in one view
- Clickable event cards with full payload detail

### Alert Rules
- Define conditions on any event field (severity, source, app, message, etc.)
- Combine conditions with AND / OR logic
- Fire Web Push or email notifications when a rule matches
- Enable, disable, or delete rules from the Settings → Notify Rules tab

### Topology
- Visual network map of your infrastructure, containers, apps, and routes
- Clickable nodes drill into component detail
- Automatically populated from Docker, Traefik, and infrastructure discovery

### App Library

NORA ships with 29 pre-built profiles. Pick your app and NORA already knows how to handle its events.

| Category | Apps |
|---|---|
| Media | Plex · Sonarr · Radarr · Lidarr · Prowlarr · Tautulli · Overseerr · Tubesync · NZBGet |
| Automation | n8n · Home Assistant · Mealie |
| Infrastructure | Traefik · Unifi · WG-Easy |
| Security & DNS | AdGuard Home · Cloudflare DDNS · Vaultwarden |
| Backup & Updates | Duplicati · Watchtower · DIUN |
| Notifications & Comms | Gotify · Ghost · Matrix · Maubot |
| Other | Uptime Kuma · Homepage · Zwavejs2mqtt |

Don't see your app? The custom profile editor lets you map any webhook payload to NORA's event model.

---

## Configuration

### Environment Variables

| Variable | Description | Default | Required |
|---|---|---|---|
| `NORA_SECRET` | JWT signing secret | — | Yes |
| `NORA_DB_PATH` | Path to SQLite database file | `/data/nora.db` | No |
| `NORA_PORT` | HTTP port | `8080` | No |
| `NORA_DEV_MODE` | Inject hardcoded admin session, skip auth | `false` | No |
| `NORA_SMTP_HOST` | SMTP server hostname | — | No |
| `NORA_SMTP_PORT` | SMTP port | `587` | No |
| `NORA_SMTP_USER` | SMTP username | — | No |
| `NORA_SMTP_PASS` | SMTP password | — | No |
| `NORA_SMTP_FROM` | From address for outbound email | — | No |
| `NORA_DIGEST_SCHEDULE` | Cron expression for digest email | `0 8 1 * *` | No |
| `NORA_VAPID_PUBLIC` | VAPID public key (auto-generated if absent) | — | No |
| `NORA_VAPID_PRIVATE` | VAPID private key (auto-generated if absent) | — | No |
| `NORA_ADMIN_EMAIL` | Bootstrap admin email | — | Required (first run) |
| `NORA_ADMIN_PASSWORD` | Bootstrap admin password | — | Required (first run) |

### In-App Settings
- SMTP configuration and test email
- Password policy (minimum length, uppercase, numbers, special characters)
- Global MFA requirement
- Digest email schedule

---

## Stack

| Layer | Choice |
|---|---|
| Backend | Go — single binary, zero runtime dependencies |
| Database | SQLite — single file, zero ops |
| Frontend | React + Vite — PWA, installable |
| Push | Web Push / VAPID — browser-native, no third party |
| Deployment | Single Docker image (~50 MB) |

3-stage Docker build: frontend → Go binary → `alpine:3.19` final image. No node_modules, no Go toolchain, no source in the final image.

---

## Roadmap

### v2 — Intelligence
- **Visual profile builder** — point-and-click field mapping from live API responses, no JSONPath required
- **Remote Docker nodes** — monitor containers across multiple hosts via socket proxy
- **Deeper API polling** — richer Proxmox, Synology, and OPNsense integration

### v3 — Scale
- PostgreSQL support for larger installations
- SSO / OAuth login
- Community profile library — import profiles contributed by other users

---

## Design Principles

1. Store what matters. Surface what's important. Stay out of the way.
2. Simple enough that you don't have to struggle.
3. If a feature makes it harder — cut it. If it makes it easier — keep it.

---

## Contributing

Profile contributions are welcome. Drop a YAML file in a GitHub issue or discussion — see the profile schema in `/docs/architecture.md` for the format. Profiles are reviewed and merged into the library.

Code contributions: open an issue first so we can align on approach before you build.

---

## License

MIT
