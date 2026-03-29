# Adding Apps

Apps are the core of NORA. Each app can receive webhook events, be actively monitored, and have its Docker container watched — depending on what it supports.

---

## From the App Library

NORA ships with pre-built profiles for common homelab apps. The profile handles everything: webhook field extraction, display formatting, monitoring config, and digest categories.

**To add an app from the library:**

1. Click **+ Add App** on the dashboard or navigate to **Apps → Add**
2. Select your app from the library grid
3. Enter the app's base URL and any required API key
4. Follow the webhook setup instructions shown on screen (these are profile-specific)
5. Save — the app appears on the dashboard immediately

**Library apps at v1 launch:**

| Category | Apps |
|---|---|
| Media | Sonarr · Radarr · Lidarr · Prowlarr · Tautulli · Overseerr |
| Automation | n8n · Home Assistant |
| Infrastructure | Proxmox · OPNsense · Traefik · Matrix |
| Backup & Updates | Duplicati · Watchtower · DIUN · Uptime Kuma |

---

## Custom Apps

If your app isn't in the library, use the custom profile editor to map any webhook payload to NORA's event model.

See [Custom App Profiles](wiki-custom-profiles) for the full walkthrough.

---

## Capability Tiers

Not every app supports every feature. NORA uses capability tiers to set expectations:

| Tier | What it means | Examples |
|---|---|---|
| **full** | Webhook events + active monitoring | Sonarr, Radarr, n8n |
| **webhook_only** | Events only, no meaningful health check | DIUN, Watchtower |
| **monitor_only** | Health check only, no events | Generic hosts |
| **docker_only** | Container state via Docker socket | MariaDB, PostgreSQL |
| **limited** | URL check only — minimal visibility | Homepage, Mealie |

The profile card in the library shows the capability tier so you know what you're getting before you add it.

---

## Optional: Linking to Infrastructure

After adding an app, NORA will prompt you to link it to a topology chain:

```
physical_host → virtual_host → docker_engine → app
```

This is always optional. Linking enables cascading impact display on the dashboard — if a Proxmox node goes unreachable, NORA can show all apps downstream as affected. If you skip it, apps show as standalone with their own status only.

You can add topology later from the app's detail page.

---

## Webhook Endpoint

Every app gets its own ingest URL:

```
POST http://your-nora-host:8080/api/v1/ingest/{app-id}
```

The app ID is shown on the app's detail page and in the webhook setup instructions. No authentication is required for the ingest endpoint — use network-level controls (firewall, internal network) to restrict access if needed.
