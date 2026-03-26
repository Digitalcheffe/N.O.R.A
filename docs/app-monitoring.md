# LogRelay — App Monitoring Capabilities
**Version:** 0.1  
**Date:** 2026-03-24  
**Status:** In Design

---

## Overview

Every app in the LogRelay profile library falls into one or more monitoring tiers. The goal is to understand what data is actually available per app — webhook events, API polling, or just a basic health check — so we build the right profile for each one.

**Monitoring Tiers:**
- **Webhook** — app pushes events to LogRelay on its own
- **API Poll** — LogRelay calls the app's API on a schedule to pull state/data
- **Ping** — ICMP ping, is the host alive
- **URL Check** — HTTP check, is the service responding with expected status
- **SSL Check** — certificate validity and expiry warning
- **Docker** — container state via Docker socket, no app involvement

---

## Media

### Sonarr
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Health, Download, Grab, Rename, Delete, Series Add |
| API Poll | ✓ | `/api/v3/health` — returns health issues. `/api/v3/queue` — download queue |
| URL Check | ✓ | `/api/v3/health` with API key header |
| SSL Check | ✓ | If exposed via HTTPS |
| Docker | ✓ | Container state |

**Best approach:** Webhook for events + URL check for uptime. API poll for queue depth if desired.

---

### Radarr
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Same event model as Sonarr |
| API Poll | ✓ | `/api/v3/health`, `/api/v3/queue` |
| URL Check | ✓ | `/api/v3/health` |
| SSL Check | ✓ | If exposed via HTTPS |
| Docker | ✓ | Container state |

**Best approach:** Same as Sonarr.

---

### Lidarr
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Same *arr webhook model |
| API Poll | ✓ | `/api/v1/health`, `/api/v1/queue` |
| URL Check | ✓ | `/api/v1/health` |
| SSL Check | ✓ | If exposed via HTTPS |
| Docker | ✓ | Container state |

---

### Prowlarr
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Health, indexer events |
| API Poll | ✓ | `/api/v1/health` |
| URL Check | ✓ | `/api/v1/health` |
| Docker | ✓ | Container state |

---

### Plex Media Server
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Playback events, library updates — requires Plex Pass |
| API Poll | ✓ | `/status/sessions` — active streams. `/library/sections` — library stats |
| URL Check | ✓ | `/:32400/identity` |
| SSL Check | ✓ | If using custom domain |
| Docker | ✓ | Container state |

**Note:** Webhooks require Plex Pass subscription.

---

### Jellyfin
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Via Jellyfin webhook plugin — playback, library events |
| API Poll | ✓ | `/health` endpoint, `/Sessions` for active streams |
| URL Check | ✓ | `/health` |
| SSL Check | ✓ | If exposed via HTTPS |
| Docker | ✓ | Container state |

---

### Bazarr
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook support |
| API Poll | ✓ | `/api/system/status` — system health |
| URL Check | ✓ | `/api/system/status` |
| Docker | ✓ | Container state |

**Best approach:** URL check for uptime + Docker for container state.

---

## Download Clients

### NZBGet
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook — uses script extensions |
| API Poll | ✓ | JSON-RPC API — `/jsonrpc` — status, queue, history |
| URL Check | ✓ | `/jsonrpc` with credentials |
| Docker | ✓ | Container state |

**Best approach:** API poll for queue depth and download status. Interesting data: active downloads, failed downloads, speed.

---

### SABnzbd
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | `/api?mode=queue` — queue status, speed, ETA |
| URL Check | ✓ | `/api?mode=version` |
| Docker | ✓ | Container state |

---

### qBittorrent
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | `/api/v2/torrents/info` — torrent list and status |
| URL Check | ✓ | `/api/v2/app/version` |
| Docker | ✓ | Container state |

---

### Transmission
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | RPC API — session stats, torrent list |
| URL Check | ✓ | RPC endpoint |
| Docker | ✓ | Container state |

---

## Automation & Monitoring

### n8n
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Workflow execution results, errors via webhook node |
| API Poll | ✓ | `/api/v1/workflows` — workflow list and status |
| URL Check | ✓ | `/healthz` |
| SSL Check | ✓ | If exposed via HTTPS |
| Docker | ✓ | Container state |

**Best approach:** Webhook for workflow events + URL check for uptime.

---

### Uptime Kuma
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Status change notifications — up/down events |
| API Poll | ✗ | No public REST API |
| URL Check | ✓ | Base URL health check |
| Docker | ✓ | Container state |

**Note:** Uptime Kuma is itself a monitoring tool — its webhooks firing into LogRelay means LogRelay gets Uptime Kuma's monitoring data as events.

---

### Home Assistant
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Automation triggers can POST to LogRelay |
| API Poll | ✓ | `/api/states` — entity states. Rich API. |
| URL Check | ✓ | `/api/` with long-lived token |
| SSL Check | ✓ | If exposed via HTTPS |
| Docker | ✓ | Container state |

**Best approach:** Webhook for automation events + API poll for specific entity states.

---

## Backup & Updates

### Duplicati
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | HTTP report destination — fires on backup completion/failure |
| API Poll | ✓ | `/api/v1/backups` — backup job list and last result |
| URL Check | ✓ | Base URL |
| Docker | ✓ | Container state |

**Best approach:** Webhook for backup events — success and failure are the critical ones.

---

### Watchtower
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | HTTP notification on container update |
| API Poll | ✗ | No API |
| URL Check | ✗ | No HTTP interface |
| Docker | ✓ | Container state |

**Best approach:** Webhook only. Fires when it updates a container image.

---

### DIUN (Docker Image Update Notifier)
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Native webhook — fires on new image version available |
| API Poll | ✗ | No API |
| URL Check | ✗ | No HTTP interface |
| Docker | ✓ | Container state |

**Best approach:** Webhook only. Clean payload, great for update notifications.

---

## Security & Identity

### Bitwarden / Vaultwarden
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook (Vaultwarden has some admin events) |
| API Poll | ✓ | `/api/alive` — health check |
| URL Check | ✓ | `/alive` or `/api/alive` |
| SSL Check | ✓ | Critical — password manager must have valid cert |
| Docker | ✓ | Container state |

**Note:** SSL check is especially important here. A Vaultwarden cert expiry is a very bad day.

---

### Authelia
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | `/api/health` |
| URL Check | ✓ | `/api/health` |
| SSL Check | ✓ | Auth proxy — cert expiry breaks everything downstream |
| Docker | ✓ | Container state |

---

## Network & Infrastructure

### OPNsense
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | Rich REST API — `/api/core/firmware/status`, `/api/diagnostics/interface/getInterfaceStatistics` |
| Ping | ✓ | LAN IP ping — is the firewall alive |
| URL Check | ✓ | `/api/core/firmware/status` with API key |
| SSL Check | ✓ | If admin UI is HTTPS |

**Best approach:** Ping for basic uptime + API poll for firmware update status and interface health. This is the one that causes sleepless nights — monitor it properly.

---

### Unifi Controller / Network Application
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | REST API — site stats, device health, client counts |
| URL Check | ✓ | Controller base URL |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | If running containerized |

---

### AdGuard Home
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | `/control/status` — filtering status, stats |
| URL Check | ✓ | `/control/status` |
| Docker | ✓ | Container state |

**Interesting data:** queries blocked per day, top blocked domains.

---

### Pi-hole
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | `/admin/api.php` — stats, status |
| URL Check | ✓ | `/admin/api.php?status` |
| Docker | ✓ | Container state |

---

### Nginx Proxy Manager
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | `/api/` — proxy hosts, SSL cert status |
| URL Check | ✓ | `/api/` |
| SSL Check | ✓ | Every proxied domain is a candidate |
| Docker | ✓ | Container state |

**Note:** NPM manages certs for everything — its API can tell you cert expiry for all proxied hosts. Goldmine for SSL monitoring.

---

## Storage & Files

### Synology DSM
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | Synology API — `/webapi/entry.cgi` — volume health, S.M.A.R.T, backup tasks |
| Ping | ✓ | NAS IP |
| URL Check | ✓ | DSM web interface |
| SSL Check | ✓ | If using custom cert |

**Best approach:** Ping for uptime + API poll for storage health. Volume degraded or disk failing are critical events.

---

### TrueNAS
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook for external systems |
| API Poll | ✓ | REST API — pool status, alerts, disk health |
| Ping | ✓ | NAS IP |
| URL Check | ✓ | Web UI |
| SSL Check | ✓ | If HTTPS |

---

## Databases

### MariaDB / MySQL
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook capability |
| API Poll | ✗ | No HTTP API — requires DB connection |
| Ping | ✓ | Host ping |
| URL Check | ✗ | No HTTP interface |
| Docker | ✓ | Container state — best available option |

**Best approach:** Docker socket for container health. If it's running the container is alive. Deep DB health requires a dedicated DB monitoring tool.

---

### PostgreSQL
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook |
| API Poll | ✗ | No HTTP API |
| Ping | ✓ | Host ping |
| Docker | ✓ | Container state |

---

### Redis
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook |
| API Poll | ✗ | No HTTP API natively |
| URL Check | ✓ | If RedisInsight or similar is running |
| Docker | ✓ | Container state |

---

## Recipe & Home Management

### Mealie
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook support |
| API Poll | ✓ | `/api/app/about` — version and health |
| URL Check | ✓ | `/api/app/about` |
| Docker | ✓ | Container state |

**Note:** Limited visibility. Docker + URL check is the best we can do. No interesting event data available.

---

### Grocy
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook |
| API Poll | ✓ | `/api/system/info` |
| URL Check | ✓ | `/api/system/info` |
| Docker | ✓ | Container state |

---

## Finance

### Actual Budget
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook |
| API Poll | ✗ | No public API |
| URL Check | ✓ | Base URL health |
| Docker | ✓ | Container state |

---

### Firefly III
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Webhooks on transaction events |
| API Poll | ✓ | REST API — account balances, transaction counts |
| URL Check | ✓ | Base URL |
| Docker | ✓ | Container state |

---

## Operating Systems & Host Management

### Rocky Linux / RHEL / CentOS
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | If Cockpit is installed — `/cockpit/login` health, system metrics via Cockpit API |
| Ping | ✓ | Host IP — is the machine alive |
| URL Check | ✓ | Cockpit web UI on port 9090 if enabled |
| SSL Check | ✓ | If Cockpit is using HTTPS |
| Docker | ✗ | Host OS — Docker socket goes the other way here |

**Best approach:** Ping for basic host uptime. Cockpit URL check if installed. For deeper OS metrics (CPU, memory, disk) Cockpit API is the cleanest path without installing an agent.

**Interesting data via Cockpit API:** CPU load, memory usage, disk utilization, systemd service state, recent journal entries, package updates available.

**Note:** Rocky Linux is likely the host running your Docker stack. If this host goes down everything on it goes with it. Ping monitoring here is critical.

---

### Cockpit (Linux Web Console)
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook support |
| API Poll | ✓ | REST API on port 9090 — system info, service state, storage, network |
| URL Check | ✓ | `https://{host}:9090` — standard port |
| SSL Check | ✓ | Self-signed by default, can use custom cert |

**Best approach:** URL check for availability + API poll for host metrics. Cockpit gives you OS-level visibility without any agent — it ships with Rocky Linux and is enabled by default on many installs.

**Interesting data:** Systemd service up/down, disk space per volume, memory pressure, failed journal entries, pending OS updates.

**Note:** Cockpit runs on every major Linux distro. If someone runs Ubuntu, Debian, or Fedora hosts the same profile applies.

---

### Windows Host / Windows Server
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | If Windows Admin Center is installed — REST API for system metrics |
| Ping | ✓ | Host IP |
| URL Check | ✓ | Windows Admin Center on port 6516 if installed |
| SSL Check | ✓ | If Windows Admin Center uses HTTPS |

**Best approach:** Ping for basic uptime. Windows Admin Center URL check if installed. Without WAC, ping is the only agentless option.

**Interesting data via Windows Admin Center:** CPU, memory, disk usage, running services, event log entries, Windows Update status.

**Note:** Windows hosts in a homelab are common for gaming rigs that double as servers, Hyper-V hosts, or dedicated media/storage machines. Ping monitoring catches the most common failure mode — someone accidentally shut it down.

**Limitation:** Windows is significantly harder to monitor agentlessly than Linux. Without Windows Admin Center or a management API, ping + URL check is the ceiling. This is an honest limitation worth surfacing in the profile.

---

### Windows Admin Center
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook |
| API Poll | ✓ | REST API — node inventory, system metrics per managed host |
| URL Check | ✓ | `https://{host}:6516` |
| SSL Check | ✓ | HTTPS by default |

**Note:** If someone runs Windows Admin Center it becomes a proxy for monitoring multiple Windows hosts through a single API endpoint. One integration, multiple hosts visible.

---

## Physical Devices & Network Gear

### Generic Host / Device
| Method | Available | Notes |
|---|---|---|
| Ping | ✓ | Any IP-addressable device |
| URL Check | ✓ | Any device with an HTTP interface |
| SSL Check | ✓ | Any HTTPS endpoint |

**Covers:** Printers, smart switches, Raspberry Pis, any device on the network.

---

### Proxmox VE
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | Rich REST API — node status, VM/CT state, storage, tasks |
| URL Check | ✓ | Web UI / API |
| SSL Check | ✓ | If using custom cert |
| Ping | ✓ | Node IP |

**Best approach:** API poll is very rich here — VM running state, resource usage, recent tasks. High value target.

---

### Portainer
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Stack webhook for redeploy events |
| API Poll | ✓ | `/api/status` — environment health |
| URL Check | ✓ | `/api/status` |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | Container state |

## Notifications & Push

### Gotify
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | Gotify IS a notification tool — it doesn't send webhooks out |
| API Poll | ✓ | `/health` — service health. `/application` — registered apps |
| URL Check | ✓ | `/health` |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | Container state |

**Note:** Gotify is the predecessor to what LogRelay is becoming. If someone runs both during migration, LogRelay monitors Gotify's health via URL check.

---

## DNS & Certificates

### Cloudflare DDNS (cf-ddns)
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | Cloudflare API — verify DNS records are current vs external IP |
| URL Check | ✗ | No HTTP interface |
| Docker | ✓ | Container state |

**Best approach:** Docker for container health + Cloudflare API poll to verify records are actually updating. Container staying alive does not mean DNS is updating.

---

## Dashboards & Portals

### Homepage
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook — it's a dashboard |
| API Poll | ✗ | No public API |
| URL Check | ✓ | Base URL |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | Container state |

**Note:** Limited visibility. Docker + URL check is the ceiling.

---

## Reverse Proxy

### Traefik
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | `/api/http/routers` — active routes. `/api/overview` — summary. `/api/http/services` — service health |
| URL Check | ✓ | `/ping` — built-in health endpoint |
| SSL Check | ✓ | Manages ACME certs — monitor all proxied domains |
| Docker | ✓ | Container state |

**Best approach:** `/ping` URL check for uptime + API poll for router health. Traefik is your reverse proxy — if it goes down everything behind it goes with it. High priority.

---

## Media Requests

### Seerr (Jellyseerr / Overseerr)
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Native webhook — request approved, declined, media available events |
| API Poll | ✓ | `/api/v1/status` — health. `/api/v1/request` — pending request counts |
| URL Check | ✓ | `/api/v1/status` |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | Container state |

**Best approach:** Webhook for request events + URL check for uptime. Users want to know when their requests are approved or available.

---

### Tautulli
| Method | Available | Notes |
|---|---|---|
| Webhook | ✓ | Native webhook — playback events, user activity, library updates |
| API Poll | ✓ | `/api/v2?apikey={key}&cmd=get_activity` — current stream count |
| URL Check | ✓ | Base URL |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | Container state |

**Interesting data:** Active stream count, transcoding vs direct play, user activity, library stats.

---

### Tube Sync / TubeArchivist
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native webhook |
| API Poll | ✓ | REST API — `/api/task/` — task queue and status |
| URL Check | ✓ | Base URL |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | Container state |

**Best approach:** URL check for uptime + API poll for active download task status.

---

## Communication & Bots

### Matrix (Synapse)
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No native outbound webhook |
| API Poll | ✓ | `/_matrix/client/versions` — server alive. Admin API for room/user stats |
| URL Check | ✓ | `/_matrix/client/versions` |
| SSL Check | ✓ | Critical — Matrix federation requires valid cert |
| Docker | ✓ | Container state |

**Best approach:** URL check + SSL check. Matrix federation breaks immediately on cert expiry — this is non-negotiable.

---

### Matrix Admin (Synapse Admin UI)
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | UI tool only |
| API Poll | ✗ | Calls Synapse API directly — no own API |
| URL Check | ✓ | Base URL |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | Container state |

**Note:** Monitor Synapse directly for meaningful data. Matrix Admin URL check just confirms the UI is accessible.

---

### Maubot
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No outbound webhook |
| API Poll | ✓ | `/_matrix/maubot/v1/` — plugin status, instance health |
| URL Check | ✓ | `/_matrix/maubot/v1/` |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | Container state |

**Interesting data:** Plugin instances running/stopped, recent errors per plugin.

---

## Network & VPN

### WG-Easy (WireGuard)
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook |
| API Poll | ✓ | REST API — `/api/wireguard/client` — peer list and connection status |
| URL Check | ✓ | Base URL / API |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | Container state |

**Interesting data:** Connected peers, last handshake time per peer, data transferred.

---

## Smart Home

### Z-Wave JS to MQTT (zwavejs2mqtt)
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No HTTP webhook — MQTT based |
| API Poll | ✓ | REST API — `/api/health` — gateway health |
| URL Check | ✓ | `/api/health` or base UI |
| SSL Check | ✓ | If HTTPS |
| Docker | ✓ | Container state |

**Interesting data:** Node count, failed nodes, controller status.

---

## Log Forwarding

### Syslog-ng
| Method | Available | Notes |
|---|---|---|
| Webhook | ✗ | No webhook |
| API Poll | ✗ | No HTTP API |
| Docker | ✓ | Container state |

**Note:** Syslog-ng's value to LogRelay is as a data source — it forwards physical device logs to LogRelay's webhook endpoint. Monitor the container is running, that's it.

---


---

## Summary — Monitoring Method by App

| App | Webhook | API Poll | Ping | URL | SSL | Docker |
|---|---|---|---|---|---|---|
| Sonarr | ✓ | ✓ | | ✓ | ✓ | ✓ |
| Radarr | ✓ | ✓ | | ✓ | ✓ | ✓ |
| Lidarr | ✓ | ✓ | | ✓ | ✓ | ✓ |
| Prowlarr | ✓ | ✓ | | ✓ | | ✓ |
| Plex | ✓* | ✓ | | ✓ | ✓ | ✓ |
| Jellyfin | ✓ | ✓ | | ✓ | ✓ | ✓ |
| Bazarr | | ✓ | | ✓ | | ✓ |
| NZBGet | | ✓ | | ✓ | | ✓ |
| SABnzbd | | ✓ | | ✓ | | ✓ |
| qBittorrent | | ✓ | | ✓ | | ✓ |
| n8n | ✓ | ✓ | | ✓ | ✓ | ✓ |
| Uptime Kuma | ✓ | | | ✓ | | ✓ |
| Home Assistant | ✓ | ✓ | | ✓ | ✓ | ✓ |
| Duplicati | ✓ | ✓ | | ✓ | | ✓ |
| Watchtower | ✓ | | | | | ✓ |
| DIUN | ✓ | | | | | ✓ |
| Vaultwarden | | | | ✓ | ✓ | ✓ |
| Authelia | | | | ✓ | ✓ | ✓ |
| OPNsense | | ✓ | ✓ | ✓ | ✓ | |
| Unifi | | ✓ | | ✓ | ✓ | ✓ |
| AdGuard Home | | ✓ | | ✓ | | ✓ |
| Pi-hole | | ✓ | | ✓ | | ✓ |
| Nginx Proxy Manager | | ✓ | | ✓ | ✓ | ✓ |
| Synology | | ✓ | ✓ | ✓ | ✓ | |
| TrueNAS | | ✓ | ✓ | ✓ | ✓ | |
| MariaDB | | | ✓ | | | ✓ |
| PostgreSQL | | | ✓ | | | ✓ |
| Proxmox | | ✓ | ✓ | ✓ | ✓ | |
| Rocky Linux | | ✓ | ✓ | ✓ | ✓ | |
| Cockpit | | ✓ | | ✓ | ✓ | |
| Windows Host | | | ✓ | ✓ | | |
| Windows Admin Center | | ✓ | | ✓ | ✓ | |
| Portainer | ✓ | ✓ | | ✓ | ✓ | ✓ |
| Mealie | | | | ✓ | | ✓ |
| Firefly III | ✓ | ✓ | | ✓ | | ✓ |
| Gotify | | ✓ | | ✓ | ✓ | ✓ |
| CF-DDNS | | ✓ | | | | ✓ |
| Sewer | | | | | ✓ | ✓ |
| Homepage | | | | ✓ | ✓ | ✓ |
| Traefik | | ✓ | | ✓ | ✓ | ✓ |
| Tautulli | ✓ | ✓ | | ✓ | ✓ | ✓ |
| Tube Sync | | ✓ | | ✓ | ✓ | ✓ |
| Matrix (Synapse) | | ✓ | | ✓ | ✓ | ✓ |
| Matrix Admin | | | | ✓ | ✓ | ✓ |
| Maubot | | ✓ | | ✓ | ✓ | ✓ |
| WG-Easy | | ✓ | | ✓ | ✓ | ✓ |
| zwavejs2mqtt | | ✓ | | ✓ | ✓ | ✓ |
| Syslog-ng | | | | | | ✓ |

*Plex webhook requires Plex Pass

---

## Key Observations

1. **Almost everything has a URL check.** Universal fallback — if it has an HTTP interface, LogRelay can check it's alive.

2. **API polling is underutilized in the homelab space.** Traefik, Proxmox, Synology, NZBGet — all have excellent APIs surfacing real state without any webhook config.

3. **Databases are the blind spot.** MariaDB, PostgreSQL, Redis — no HTTP, no webhook. Docker socket is the only agentless option.

4. **SSL is non-negotiable for reverse proxies and Matrix.** Traefik, NPM, and Matrix break hard on cert expiry. 30-day warning is the minimum.

5. **Physical devices need ping + URL.** OPNsense, Synology, Proxmox — not containerized but highly monitorable via API. These hurt most when undetected.

6. **Syslog-ng is a data source, not just a monitored app.** Its value is as a log forwarder for physical devices feeding LogRelay — not something LogRelay monitors.

7. **CF-DDNS failure is subtle.** Container stays running but stops updating DNS. API polling against Cloudflare to verify record freshness is the only way to catch it.

8. **Matrix SSL is non-negotiable.** Federation breaks immediately on cert expiry. SSL check with 30-day warning is the most critical Matrix profile setting.

---

*Document generated 2026-03-24*  
*Next: Build profiles for v1 launch apps — Sonarr, Radarr, Lidarr, n8n, DIUN, Watchtower, Duplicati, Uptime Kuma, OPNsense, Proxmox, Traefik, Matrix*
