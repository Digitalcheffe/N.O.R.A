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
| **Monitor** | Actively checks hosts, services, and SSL certificates on a schedule — no agent required |
| **Capture** | Receives webhook events from apps that support them (Sonarr, n8n, Duplicati, and more) |
| **Observe** | Connects to the Docker socket for container-level visibility across your stack |
| **Measure** | Collects CPU, memory, and disk from containers and hosts automatically |
| **Store** | Retains events by severity — errors for 90 days, info for 7 — with monthly rollups kept forever |
| **Notify** | Fires Web Push notifications to any subscribed browser or mobile device |
| **Summarize** | Delivers a monthly digest email of what happened across your stack |

---

## What It Is Not

- **Not Grafana** — no metrics pipelines, no dashboards you have to build yourself
- **Not Splunk** — no query language, no complex field extraction, no enterprise pricing
- **Not Uptime Kuma** — not just uptime; has event history, context, and resource trends
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

## App Library

NORA ships with pre-built profiles for the most common homelab apps. Pick your app from the library and NORA already knows how to handle its events, what to monitor, and how to summarize it.

**Media** — Sonarr · Radarr · Lidarr · Prowlarr · Tautulli · Overseerr  
**Automation** — n8n · Home Assistant  
**Infrastructure** — Proxmox · OPNsense · Traefik · Matrix  
**Backup & Updates** — Duplicati · Watchtower · DIUN · Uptime Kuma  

Don't see your app? The custom profile editor lets you map any webhook payload to NORA's event model in minutes.

---

## Roadmap

### v1 — Foundation *(in development)*
- Webhook ingest from any app that can fire an HTTP POST
- Active monitoring — ping, URL checks, SSL certificate expiry
- Docker socket integration — container events and resource metrics
- App profile library — 15 apps at launch
- Dashboard with counts, sparklines, and status rollup
- Monthly digest email via SMTP
- Web Push notifications (browser-native, no app store)
- Single Docker image, SQLite, zero external dependencies

### v2 — Intelligence
- **Notification rules engine** — define conditions on any event field, fire push notifications when they match. Rules created from real events you're looking at, not blank forms.
- **Visual profile builder** — point-and-click field mapping from live API responses, no JSONPath required
- **Remote Docker nodes** — monitor containers across multiple hosts via socket proxy

### v3 — Scale
- PostgreSQL support for larger installations
- SSO / OAuth login
- Community profile library — import profiles contributed by other users
- API polling for apps without webhook support (deeper Proxmox, Synology, OPNsense integration)
- Multi-instance federation

---

## Stack

| Layer | Choice |
|---|---|
| Backend | Go — single binary, zero runtime dependencies |
| Database | SQLite — single file, zero ops |
| Frontend | React + Vite — PWA, installable, works offline |
| Push | Web Push / VAPID — browser-native, no third party |
| Deployment | Single Docker image |

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