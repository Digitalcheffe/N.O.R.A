# NORA — Project Architecture Specification
**Full name:** Nexus Operations Recon & Alerts  
**Version:** 0.2  
**Date:** 2026-03-24  
**Status:** In Design

---

## Product Definition

**Type:** Self-hosted homelab monitoring, event capture, and notification platform  
**Deployment:** Single Docker image  
**Target user:** Homelabbers and small self-hosted teams  

**Core value:** One tool that actively monitors your stack, captures events from your apps, stores what matters, notifies you when something breaks, and summarizes what happened — without requiring you to build or maintain a monitoring pipeline.

### What It Does

| Capability | Description |
|---|---|
| Monitor | Actively checks hosts, services, and SSL certificates on a schedule |
| Capture | Receives webhook events from apps that support them |
| Observe | Connects to Docker socket for container-level visibility |
| Measure | Collects CPU, memory, and disk from containers and hosts. Stores raw readings, rolls up to hourly and daily summaries |
| Store | Retains events by severity with indefinite monthly rollups |
| Notify | Fires Web Push notifications when rules match (v2) |
| Summarize | Delivers a monthly digest of what happened across your stack |

### What It Is Not
- Not a log aggregator — no query language, no log shipping pipeline required
- Not Grafana — resource metrics are captured and summarized automatically, not from pipelines you build and maintain yourself
- Not just a notification tool — it remembers what your apps told it and shows you the trend that led up to a problem
- Not just an uptime monitor — active checks are one input alongside events and resource data

### Design Principles
1. Store what matters. Surface what is important. Stay out of the way.
2. Simple enough that you do not have to struggle.
3. If a feature makes it harder cut it. If it makes it easier keep it.

---

## Tech Stack

| Layer | Choice | Notes |
|---|---|---|
| Backend | Go | Single binary, zero runtime deps, minimal memory footprint |
| Database | SQLite | Single file, zero ops. DuckDB documented upgrade path if load demands it |
| Frontend | React + Vite | PWA support, Web Push notifications |
| Push Notifications | Web Push / VAPID | Browser-native, no app store, no third-party dependency |
| Deployment | Single Docker image | Volume mount for data persistence |

```bash
docker run -d \
  -p 8080:8080 \
  -v ./data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e LOGRELAY_SECRET=changeme \
  ghcr.io/digitalcheffe/nora:latest
```

---

## Data Model

### users
```
id              uuid          PK
email           text          unique
password_hash   text
role            text          admin | member
created_at      timestamp
```

### physical_hosts
```
id              uuid          PK
name            text
ip              text
type            text          bare_metal | proxmox_node
notes           text
created_at      timestamp
```

### virtual_hosts
```
id              uuid          PK
physical_host_id uuid         FK → physical_hosts (nullable)
name            text
ip              text
type            text          vm | lxc | wsl
created_at      timestamp
```

### docker_engines
```
id              uuid          PK
virtual_host_id  uuid         FK → virtual_hosts (nullable)
name            text
socket_type     text          local | remote_proxy
socket_path     text          /var/run/docker.sock or remote URL
created_at      timestamp
```

### apps
```
id              uuid          PK
name            text
token           text          unique — ingest auth
profile_id      text          FK → profile library (nullable for custom)
docker_engine_id uuid         FK → docker_engines (nullable)
config          json          user-provided values: base_url, api_key, etc.
rate_limit      int           max events/minute, default 100
created_at      timestamp
```

### events
```
id              uuid          PK
app_id          uuid          FK → apps
received_at     timestamp
severity        text          debug | info | warn | error | critical
display_text    text          pre-computed summary from profile display_template
raw_payload     json          full original payload, never modified
fields          json          flat key/value map of extracted fields
```

### monitor_checks
```
id              uuid          PK
app_id          uuid          FK → apps (nullable)
name            text
type            text          ping | url | ssl
target          text          IP or URL
interval_secs   int
expected_status int           for URL checks
ssl_warn_days   int           default 30
ssl_crit_days   int           default 7
enabled         bool
last_checked_at timestamp
last_status     text          up | warn | down
last_result     json          raw check result
created_at      timestamp
```

### rollups
```
app_id          uuid          FK → apps
year            int
month           int
event_type      text
severity        text
count           int
PRIMARY KEY (app_id, year, month, event_type, severity)
```

### metrics
```
app_id              uuid      FK → apps
period              timestamp hourly bucket
events_per_hour     int
avg_payload_bytes   int
peak_per_minute     int
```

### resource_readings
```
id              uuid          PK
source_id       uuid          FK → apps or monitor_checks
source_type     text          docker_container | host | vm
metric          text          cpu_percent | mem_percent | mem_bytes | disk_percent
value           float
recorded_at     timestamp
```
Raw readings retained for 7 days then purged. Threshold breaches generate events in the events table.

### resource_rollups
```
source_id       uuid          FK → source
source_type     text          docker_container | host | vm
metric          text
period_type     text          hour | day
period_start    timestamp
avg             float
min             float
max             float
PRIMARY KEY (source_id, metric, period_type, period_start)
```
Hourly rollups retained 90 days. Daily rollups kept indefinitely.

### alert_rules (schema stubbed — v2 implementation)
```
id              uuid          PK
app_id          uuid          FK → apps (nullable = any app)
name            text
conditions      json          [{field, operator, value}]
condition_logic text          AND | OR | custom expression
notif_title     text          template string
notif_body      text          template string
enabled         bool
created_at      timestamp
```

---

## Storage Retention

Raw events retained by severity:

| Severity | Raw Retention |
|---|---|
| critical / error | 90 days |
| warn | 30 days |
| info | 7 days |
| debug | 24 hours |

Monthly rollups are kept indefinitely. One row per app / event type / severity / month. Raw events age out, summary counts never do.

Rate limiting per app: default 100 events/minute. Configurable. Excess events are dropped and the drop count is logged.

### Plain Language Data Approach

**Events** are the core thing. Every time an app sends a webhook — Sonarr downloading a show, n8n completing a workflow, Duplicati finishing a backup — that's one event. NORA stores the full original JSON payload, a one-line summary, and a severity level. Events don't live forever. Errors stick around 90 days, warnings 30 days, info 7 days. After that they're gone because you probably never needed them anyway.

**Rollups** are how NORA remembers things after events expire. Once a month NORA collapses all events for that month into a single summary row — "Sonarr had 143 downloads in March, 2 health warnings." That row lives forever. So even two years from now you can answer "how many downloads did I have in March 2026" without storing two years of raw JSON.

**Resource readings** are the CPU, memory, and disk numbers NORA collects from Docker containers and hosts. Raw readings live 7 days. Every hour NORA collapses them into an hourly average, min, and max — those live 90 days. Every day it collapses hourly data into a daily average — those live forever. You always have the trend, you never drown in raw numbers.

**Monitor check results** store only the latest state per check — up or down, when it last checked, what it found. No full history. When a check fails it creates an event and that event follows normal severity-based retention.

**Metrics** are NORA watching itself. Events per hour per app, payload sizes, peak burst rates. Surfaced in the UI so you can see "OPNsense is sending 400 events per minute" before it becomes a problem.

```
Recent raw data    →  events and readings, kept short term
Long term summary  →  rollups kept forever, tiny storage cost
Current state      →  monitor check results, latest only
Self awareness     →  metrics table, NORA watching itself
```

---

## Ingest Sources

### 1. Webhook (Primary)
```
POST /ingest/{token}
Content-Type: application/json
{ ...any JSON payload... }
→ 202 Accepted
```
Any app that can fire an HTTP POST. Token identifies the source app.

### 2. Docker Socket
- Local: mount /var/run/docker.sock
- Remote: tecnativa/docker-socket-proxy on remote node

Provides per-container: up/down state, restart events, image update available.

### 3. Active Monitoring
NORA polls on a schedule. No agent required.

**Ping** — ICMP, is host alive  
**URL Check** — HTTP, expected status code  
**SSL Check** — cert validity and expiry warning at 30 / 7 days

---

## Host Topology Model

Apps optionally belong to a topology chain:

```
physical_host  →  virtual_host  →  docker_engine  →  app
```

Topology is always optional. An app works without it. Users are nudged to add topology after app creation but never blocked.

When topology is defined the dashboard can show layered impact:

```
⚠️  proxmox-node1 — no ping response
     └─ rocky-vm01 — unreachable
          └─ docker-engine-01 — unreachable
               ├─ Sonarr — affected
               ├─ Radarr — affected
               └─ n8n — affected
```

When topology is not defined apps show as standalone with their own status only.

---

## App Profile Library

Every pre-built app ships with a YAML profile. The profile drives setup instructions, field extraction, display formatting, monitoring config, and digest categories automatically.

### Profile Schema
```yaml
meta:
  name: string
  category: string          Media | Automation | Network | Infrastructure | Storage | Security
  logo: string              filename in /profiles/logos/
  description: string
  capability: string        full | webhook_only | monitor_only | docker_only | limited

webhook:
  setup_instructions: string
  recommended_events: [string]
  not_recommended: [string]
  field_mappings:
    {tag_name}: "$.json.path"
  display_template: string   "{field_name} — {other_field}"
  severity_mapping:
    {event_value}: debug | info | warn | error | critical

monitor:
  check_type: url | ping | ssl
  check_url: string          supports {base_url} variable
  auth_header: string        supports {api_key} variable
  healthy_status: int        default 200
  check_interval: string     1m | 5m | 15m | 1h

digest:
  categories:
    - label: string
      match_field: string
      match_value: string
      match_severity: string
```

### Capability Tiers

| Tier | Description | Examples |
|---|---|---|
| full | Webhook + active monitoring | Sonarr, Radarr, n8n |
| webhook_only | Events only, no health check | DIUN, Watchtower |
| monitor_only | Health check only, no events | Generic hosts |
| docker_only | Container socket only | Databases |
| limited | Listed in library, URL check only | Homepage, Mealie |

---

## Custom App Profiles

Users create custom profiles without uploading files.

### v1 — In-Browser YAML Editor
- Syntax highlighting
- Live validation with inline error messages
- Preview of app card and display template output
- Save directly — no file upload required

### v2 — Visual Profile Builder
- Enter base URL and auth headers
- Test button fires a live request against the target
- Response JSON rendered as interactive tree
- Click any field to map it — NORA writes the JSONPath automatically
- Dot-notation path generated behind the scenes
- Save as profile when satisfied
- No manual JSONPath syntax required

---

## API Surface

```
# Auth
POST   /auth/login
POST   /auth/register
POST   /auth/logout

# Users
GET    /users
POST   /users
PUT    /users/{id}
DELETE /users/{id}

# Apps
GET    /apps
POST   /apps
GET    /apps/{id}
PUT    /apps/{id}
DELETE /apps/{id}
POST   /apps/{id}/token/regenerate

# Topology
GET    /hosts/physical
POST   /hosts/physical
PUT    /hosts/physical/{id}
DELETE /hosts/physical/{id}
GET    /hosts/virtual
POST   /hosts/virtual
PUT    /hosts/virtual/{id}
DELETE /hosts/virtual/{id}
GET    /docker-engines
POST   /docker-engines
PUT    /docker-engines/{id}
DELETE /docker-engines/{id}

# Ingest — public, token auth
POST   /ingest/{token}

# Events
GET    /events                    filter by app_id, severity, time range
GET    /events/{id}               single event + raw payload
GET    /apps/{id}/events          events scoped to one app

# Monitor Checks
GET    /checks
POST   /checks
GET    /checks/{id}
PUT    /checks/{id}
DELETE /checks/{id}
POST   /checks/{id}/run           manual trigger

# Dashboard
GET    /dashboard/summary         status roll-up + counts, current week
GET    /dashboard/digest/{period} rollup data for period — YYYY-MM

# Profile Library
GET    /profiles
GET    /profiles/{id}

# Metrics
GET    /metrics                   instance-wide
GET    /apps/{id}/metrics         per-app

# Alert Rules — v2, endpoints stubbed
GET    /rules
POST   /rules
PUT    /rules/{id}
DELETE /rules/{id}
```

---

## UI Structure

### Dashboard Rendering Philosophy

The dashboard is data-driven. It renders only what the user has actually configured. No empty sections, no placeholder widgets, no assumed app categories.

**Rules:**
- Sections appear only when they have content
- Summary bar categories are derived from profile digest definitions — only categories with events this period are shown
- App widgets, host widgets, monitor checks, and bookmarks are all opt-in
- First-time empty state shows setup prompts, not an empty dashboard

**Empty state (new install):**
```
No apps yet

[ + Add your first app ]
[ + Add a monitor check ]  
[ + Add a bookmark ]
```

**Sparse state example (n8n + OPNsense only):**
```
Summary bar:   Workflows · Errors · Uptime
Apps:          n8n widget
Infrastructure: OPNsense (ping)
Monitor Checks: OPNsense ping · n8n URL check
Recent Events:  n8n events only
```

**Full state example (arr stack + infrastructure):**
```
Summary bar:   Downloads · Errors · Backups · Updates · Uptime
Apps:          Sonarr · Radarr · Lidarr · n8n · Duplicati · DIUN
Infrastructure: Proxmox node · Rocky Linux VM · Synology NAS
Monitor Checks: All configured checks
SSL Panel:     All monitored domains
Recent Events: All apps
Bookmarks:     If any added
```

The dashboard is a reflection of what the user told it about. Nothing more, nothing less.

---

### Home Screen Layout

**Top bar:** Global status badge · Time filter (Day / Week / Month) · Add app button · Notifications

**Summary bar:** Dynamic category cards with count and sparkline. Categories sourced from profile digest definitions. Only populated categories shown.

**Main content — two column:**

Left column (primary):
- Apps section — app widgets, auto-populated from configured apps
- Infrastructure section — host widgets with resource bars, shown if any hosts configured
- Recent events panel — last N events across all apps

Right column (secondary):
- Monitor checks — all active ping / URL / SSL checks
- SSL certificates panel — all monitored domains sorted by days remaining
- Bookmarks — if any configured

**App widget contains:**
- App name, icon, status dot
- Key counts for the current period (sourced from profile digest categories)
- Sparkline chart for event volume
- Last event summary line
- Click → App Detail screen

**Host widget contains:**
- Host name, type, IP
- CPU / memory / disk resource bars
- Status dot
- Click → Host Detail screen

---

### App Detail Screen
- Status and last event timestamp
- This period counts by event category with sparkline charts
- Event list — timestamp, display_text, severity badge
- Click event to expand full raw JSON
- Save as rule button on expanded event — disabled in v1, active in v2
- Time filter — Day / Week / Month
- Launch button — opens app URL in new tab if configured

### Monitor Checks Screen
- List of all active checks with current status
- Last checked timestamp and result
- Add and edit check form — type, target, interval, thresholds

### Host Topology Screen
- Physical hosts list
- Expand to show virtual hosts, docker engines, apps
- Add and edit at each layer
- Fully optional — users who skip this see nothing broken

### Monthly Digest
- Rendered summary by app category
- Event counts, uptime, errors
- Delivered via configurable webhook — n8n or direct HTTP POST
- Mobile responsive inline CSS email template

### Settings
- User management — admin only
- Notification delivery config — webhook URL for digest
- Instance metrics — database size, events per day, peak load
- Web Push subscription management

---

## Notification Rules — v2

Rules evaluate against incoming events in real time. Created from real events via Save as Rule — not from blank forms.

```
conditions:     [{field, operator, value}]
operators:      is | is_not | contains | does_not_contain | gt | lt
logic:          AND (default) | OR | custom "{1} AND ({2} OR {3})"

notification:
  title:        template string — supports {field_name} variables
  body:         template string — supports {field_name} variables
  delivery:     web_push (v1) | webhook (v2)
```

Schema stubbed in DB from day one. UI shows placeholder. Implementation is v2.

---

## v1 Scope

### In Scope
- Multi-user — admin and member roles
- App profile library — Sonarr, Radarr, Lidarr, Prowlarr, n8n, DIUN, Watchtower, Duplicati, Uptime Kuma, Tautulli, Traefik, Matrix, Seerr, OPNsense, Proxmox
- Webhook ingest
- Docker socket integration — local only
- Active monitoring — ping, URL check, SSL certificate expiry
- Host topology model — physical to virtual to docker to app, fully optional
- Severity-based event retention with monthly rollups kept indefinitely
- Event stream — click to expand JSON
- Dashboard — counts with sparkline charts
- Monthly digest — SMTP email delivery
- Per-app rate limiting — default 100 events per minute
- Built-in metrics table
- Custom app profiles — in-browser YAML editor
- PWA — mobile ready, Web Push notifications

### Out of Scope — v1
- Notification rules engine — schema stubbed, UI placeholder only
- Visual profile builder — v2
- PostgreSQL support
- SSO / OAuth
- Community profile import workflow

---

## Open Questions

1. **Name** — Working name NORA. Candidates: labsignal, labscope, labsentinel, vibe.watch, homelabhealth

## Resolved Decisions

| Decision | Resolution |
|---|---|
| Digest delivery | SMTP email for v1. NORA sends directly, no webhook dependency. SMTP config in settings. |
| Remote Docker socket | Out of scope. Local Docker socket only for v1. Remote nodes are a future consideration. |
| Profile contribution | File upload to a GitHub issue or discussion. Community drops a YAML file, gets reviewed, merged into profiles folder. No custom tooling needed. |
| Topology nudge UX | Deferred. UX detail resolved during build, not an architecture decision. |

---

*Updated: 2026-03-24*  
*Next: Finalize v1 profile list → begin build*
