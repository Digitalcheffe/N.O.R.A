# Monitor Checks

Monitor checks are NORA's active polling layer. No agent required on the target — NORA reaches out on a schedule and records the result.

---

## Check Types

| Type | What it does | Good for |
|---|---|---|
| **Ping** | ICMP — is the host reachable? | Physical hosts, NAS, routers |
| **URL** | HTTP — does the endpoint return the expected status code? | App health endpoints, admin UIs |
| **SSL** | TLS handshake — is the certificate valid and how long until it expires? | Any HTTPS endpoint |

---

## Adding a Check

Navigate to **Monitor Checks → + Add Check**.

### Ping Check

| Field | Description |
|---|---|
| Name | Display name — e.g., `OPNsense LAN` |
| Target | IP address or hostname |
| Interval | How often to check — 1m / 5m / 15m / 1h |

**Use ping for:** Physical hosts with stable IPs — NAS, router, Proxmox nodes, bare-metal servers.

**Do not use ping for:** Docker containers. Use Docker socket monitoring instead — containers don't always have stable IPs and ICMP tells you nothing about container health.

---

### URL Check

| Field | Description |
|---|---|
| Name | Display name |
| Target URL | Full URL including scheme — e.g., `https://sonarr.home.example.com/api/v3/health` |
| Expected Status | HTTP status code that means "healthy" — default `200` |
| Interval | 1m / 5m / 15m / 1h |

**Useful URL check endpoints for common apps:**

| App | Endpoint |
|---|---|
| Sonarr / Radarr / Lidarr | `/api/v3/health` |
| n8n | `/healthz` |
| Traefik | `/ping` |
| Authelia | `/api/health` |
| Vaultwarden | `/api/alive` |
| Proxmox | `https://{ip}:8006/api2/json/nodes` |
| OPNsense | `https://{ip}/api/core/firmware/status` |
| AdGuard Home | `/control/status` |
| Home Assistant | `/api/` |

> **Tip:** Profile-based apps automatically suggest the best URL check endpoint when you configure the app. The check is pre-populated — you just confirm and save.

---

## Check Results

Each check records:

- **Status** — `UP` · `WARN` · `DOWN`
- **Response time** — in milliseconds (URL checks only)
- **Last checked** — timestamp
- **Uptime %** — rolling percentage over the current period

Checks appear in a list on the **Monitor Checks** screen with status badges:

```
● UP    OPNsense LAN           192.168.1.1          99.9%   2s ago
● UP    Sonarr Health          https://sonarr...     100%    30s ago
● WARN  itegasus.com SSL       itegasus.com          —       5m ago   18d remaining
● DOWN  n8n                    https://n8n...        97.2%   1m ago
```

---

## Manual Run

Each check has a **Run now** button that fires the check immediately, bypassing the interval. Useful when you've just restarted a service and want to confirm it's back up without waiting.

---

## Check Intervals

Choose an interval appropriate for the criticality of what you're monitoring:

| Interval | Good for |
|---|---|
| 1 minute | Critical services — auth proxy, reverse proxy, firewall |
| 5 minutes | Standard apps — Sonarr, n8n, Home Assistant |
| 15 minutes | Low-priority checks — non-critical admin UIs |
| 1 hour | SSL certificate expiry (changes slowly — no need to check constantly) |

---

## Events from Checks

When a check transitions from UP to DOWN (or vice versa), NORA generates an event. These appear in the event feed on the dashboard and count toward the Uptime summary card. You can configure notification rules (v2) to fire a push notification on status changes.
