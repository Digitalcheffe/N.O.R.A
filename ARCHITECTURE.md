# NORA Architecture

This document describes how NORA is structured — the layers, how data moves through them, and the decisions behind the design.

---

## Overview

NORA is a single Go binary that serves the frontend, runs all background jobs, and exposes a REST API. There is no separate worker process, no message queue, and no external dependencies beyond the Docker socket (optional) and an SMTP server (optional). The database is SQLite.

```
┌──────────────────────────────────────────────────────┐
│                    Browser / PWA                     │
└────────────────────────┬─────────────────────────────┘
                         │ HTTP / REST
┌────────────────────────▼─────────────────────────────┐
│                   Go HTTP Server                     │
│  ┌──────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │  Static SPA  │  │   REST API  │  │  Ingest API │ │
│  │  (embedded)  │  │  /api/v1/*  │  │  /api/v1/   │ │
│  └──────────────┘  └──────┬──────┘  │  ingest/:t  │ │
│                           │         └──────┬────────┘ │
│  ┌────────────────────────▼──────────────▼────────┐  │
│  │                   Service Layer                │  │
│  │  Auth · Ingest · Notify · Digest · Enrichment  │  │
│  └────────────────────────┬───────────────────────┘  │
│                           │                          │
│  ┌────────────────────────▼───────────────────────┐  │
│  │                  Repo Layer                    │  │
│  │  Apps · Events · Checks · Infra · Discovery    │  │
│  └────────────────────────┬───────────────────────┘  │
│                           │ sqlx / SQLite            │
│  ┌────────────────────────▼───────────────────────┐  │
│  │                   SQLite DB                    │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  ┌──────────────────────────────────────────────────┐ │
│  │               Background Scheduler               │ │
│  │  Monitor · Discovery · Snapshot · Enrichment     │ │
│  │  Digest · Daily image scan                       │ │
│  └──────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
```

---

## Repository Layout

```
cmd/
  nora/              — main entry point; wires everything together
  nora-cli/          — optional CLI for admin tasks

internal/
  api/               — HTTP handlers (one file per domain area)
  apptemplate/       — app profile loader and field mapper
  auth/              — JWT, session management, TOTP
  config/            — environment variable parsing
  crypto/            — VAPID key generation, hashing helpers
  frontend/          — embeds the compiled React bundle
  icons/             — serves app profile icons
  infra/             — integration clients and enrichment workers
    portainer.go     — Portainer API client
    portainer_enrichment.go
    proxmox_detail.go
    docker_enrichment.go
  ingest/            — webhook payload parsing and normalization
  jobs/              — high-level job orchestration (discover, digest, enrich)
  models/            — shared struct definitions (no DB logic)
  monitor/           — ping / URL / SSL / DNS check runners
  profile/           — user profile and password policy
  push/              — Web Push / VAPID notification dispatch
  repo/              — all database access (one interface per entity type)
  rules/             — alert rule evaluation
  scanner/
    discovery/       — Docker and Portainer container discovery
    snapshot/        — resource metric collection (CPU/mem/disk)
    daily/           — once-per-day jobs (image update check)
    intervals.go     — per-integration poll intervals
    scheduler.go     — schedules and owns all scanner goroutines

frontend/
  src/
    api/             — typed fetch wrappers for every API endpoint
    components/      — shared UI components (Topbar, Layout, SlidePanel…)
    context/         — React contexts (auth, auto-refresh, env status)
    pages/           — one file per route
    styles/          — shared CSS (modal primitives, variables)
    utils/           — formatting helpers

migrations/          — numbered SQL migration files (applied at startup)
```

---

## Data Flow

### Webhook Ingest

```
App → POST /api/v1/ingest/{token}
        │
        ▼
  Token lookup → App resolved
        │
        ▼
  Profile applied (field mapping, severity, title extraction)
        │
        ▼
  Event written to events table
        │
        ▼
  Alert rules evaluated → Web Push fired if matched
```

### Infrastructure Discovery

The scheduler runs discovery on a per-integration cadence (typically 60–300 s):

```
Scheduler tick
  │
  ├── Docker discovery
  │     → List containers → Upsert discovered_containers
  │     → Collect CPU/mem/disk → Write resource_readings
  │
  ├── Portainer discovery
  │     → Enumerate endpoints → List containers per endpoint
  │     → Inspect each container (image, env vars, restart policy)
  │     → Upsert discovered_containers, resource_readings
  │
  ├── Proxmox snapshot
  │     → Node metrics, VM/LXC list, storage
  │     → Write resource_readings
  │
  └── Traefik snapshot
        → Routes, services, SSL certs
        → Write to traefik_routes, traefik_services, traefik_certs
```

### Daily Image Update Scan

Runs once per day on startup + cron:

```
List all discovered containers with a known image digest
  │
  ▼
For each container: pull registry digest for the image tag
  │
  ▼
Compare local digest vs registry digest
  │
  ▼
UpdateContainerImageCheck → update_available flag + registry_digest stored
```

### Monitor Checks

Each enabled check runs on its own ticker:

```
Scheduler → check runner (ping / url / ssl / dns)
  │
  ▼
Result written to monitor_results
  │
  ▼
last_status + last_checked_at updated on monitor_checks row
  │
  ▼
If status changed → event emitted → alert rules evaluated
```

---

## Database

NORA uses SQLite via `mattn/go-sqlite3`. The schema is applied at startup from numbered SQL files in `migrations/`. Migrations are applied in order and skipped if already applied (tracked in a `schema_migrations` table).

WAL mode is enabled. There is one write connection and one read connection pool to avoid lock contention.

Key tables:

| Table | Purpose |
|---|---|
| `apps` | Registered apps with webhook tokens and profile assignments |
| `events` | All ingested and system-generated events |
| `monitor_checks` | Check configuration (type, target, interval) |
| `monitor_results` | Per-run check results and latency |
| `infra_components` | Infrastructure integrations (Docker, Proxmox, Portainer, etc.) |
| `discovered_containers` | Containers found by discovery, with current state |
| `resource_readings` | Time-series CPU/mem/disk samples |
| `resource_rollups` | Monthly aggregated rollups (kept indefinitely) |
| `traefik_routes` | Routes discovered from Traefik API |
| `traefik_services` | Backend services behind routes |
| `traefik_certs` | SSL certificates from Traefik |
| `component_links` | Manual and discovered relationships between components |
| `alert_rules` | User-defined notification conditions |
| `push_subscriptions` | Web Push subscriber endpoints |
| `users` | User accounts, hashed passwords, TOTP secrets |

---

## API

All endpoints are under `/api/v1/`. The API is REST-ish JSON. Authentication uses a JWT in the `Authorization: Bearer` header. Tokens are issued at login and have a configurable TTL.

The ingest endpoint (`/api/v1/ingest/{token}`) is unauthenticated — the token is the credential.

Handlers live in `internal/api/` with one file per domain (e.g. `apps.go`, `events.go`, `checks.go`, `docker_discovery.go`).

---

## Frontend

The frontend is a React SPA built with Vite and embedded into the Go binary at compile time via `embed.FS`. All routes are handled client-side; the server returns `index.html` for any non-API path.

State management is local React state — no Redux or Zustand. The auto-refresh context drives periodic re-fetches. All API calls go through typed wrappers in `frontend/src/api/client.ts`.

The frontend is a PWA: it has a service worker, a web app manifest, and can be installed to the home screen on desktop and mobile.

---

## Authentication

- JWT-based. Tokens are signed with `NORA_SECRET`.
- TOTP (RFC 6238) via `pquerna/otp`. Secrets are stored encrypted. TOTP can be globally enforced or per-user exempt.
- Sessions are stateless — no session table. Logout invalidates on the client.
- Password policy (length, complexity) is enforced server-side and configurable in Settings.

---

## Notifications

### Web Push
NORA generates VAPID keys on first run (or accepts them via env vars). When an alert rule matches, the rule evaluator queries `push_subscriptions` and dispatches a push message to each subscriber via the Web Push protocol. No third-party push service is involved.

### Digest Email
A scheduled job (cron, configurable) assembles a summary of events, check results, and infrastructure status over the configured period and sends it via SMTP. SMTP credentials are stored in the database, configured in Settings.

---

## Deployment

NORA ships as a single Docker image built in three stages:

1. **Frontend build** (`node:22-alpine`) — `npm ci && vite build`
2. **Go build** (`golang:1.24-alpine`) — `CGO_ENABLED=1 go build`
3. **Final image** (`alpine:3.19`) — copies binary + embedded frontend only

The final image is ~50 MB. There is no Node, no Go toolchain, and no source code in the runtime image.

The binary listens on `NORA_PORT` (default `8081`) and serves everything: static assets, API, and the ingest endpoint.

Data lives in a single directory (`/data` by default):
- `nora.db` — SQLite database
- `templates/` — app profile YAML files
- `icons/` — custom icon overrides

Mount `/data` as a volume to persist data across container restarts.
