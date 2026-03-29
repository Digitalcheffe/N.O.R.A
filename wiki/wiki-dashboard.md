# Dashboard Behavior

NORA's dashboard is data-driven. It only renders what you've configured — no empty sections, no placeholder widgets, no assumed categories.

---

## What Shows Up and When

| Section | Appears when |
|---|---|
| Summary bar | At least one app has events in the current period |
| Summary bar categories | A digest category has at least one matching event this period |
| Apps section | At least one app is configured |
| Infrastructure section | At least one infrastructure component is configured |
| Monitor Checks panel | At least one check is configured |
| SSL Certificates panel | At least one SSL check exists or Traefik integration is active |
| Recent Events feed | At least one event has been received |
| Bookmarks | At least one bookmark is configured |

**Empty install:** Shows a setup prompt with CTAs to add your first app, add a monitor check, and add a bookmark. Nothing else.

---

## Summary Bar

The summary bar shows aggregate counts for the current time period with a sparkline for each category.

Categories are derived from app profile digest definitions — if your profiles define "Downloads" and "Errors" as digest categories, and there are events matching those categories this period, those cards appear. If no events match a category this period, that card is omitted.

**Time filter:** Day / Week / Month buttons in the top bar re-fetch all summary data for the selected period. The sparklines update accordingly.

---

## App Widgets

Each configured app renders as a widget card showing:

- App name and status (Online / Warning / Down)
- Stats grid — profile-defined metrics (e.g., downloads today, workflows run)
- A sparkline of event volume
- Last event summary

Widget border color signals status at a glance:

| Border | Meaning |
|---|---|
| None (hover only) | Online — all good |
| Yellow (always visible) | Warning — something worth checking |
| Red (always visible) | Down — needs attention |

Clicking a widget navigates to the app's detail page.

---

## Infrastructure Widgets

Infrastructure components appear as resource bar widgets showing CPU, memory, and disk utilization. Resource data comes from the configured collection method (Proxmox API, Synology DSM API, or SNMP).

If no resource data is available for a component (e.g., collection isn't configured yet), the bars render at 0% with muted coloring.

The Infrastructure section is absent from the dashboard entirely when no components are configured.

---

## Cascade Impact Display

When topology relationships are configured (physical host → VM → Docker engine → app), the dashboard can show downstream impact when a host goes unreachable:

```
⚠️  proxmox-node1 — no ping response
     └─ rocky-vm01 — unreachable
          └─ docker-engine-01 — unreachable
               ├─ Sonarr — affected
               ├─ Radarr — affected
               └─ n8n — affected
```

This only appears when topology is defined. Apps with no topology chain show as standalone — their status is independent and based only on their own checks and Docker health.

---

## Event Feed

The Recent Events feed shows the last N events across all apps, most recent first. Click any event row to expand inline and view the raw JSON payload. No navigation required — events expand in place.

Events are color-coded by severity:

| Severity | Color |
|---|---|
| info | Blue dot |
| warn | Yellow dot |
| error | Red dot |
| critical | Red dot (persistent highlight) |

---

## Bookmarks

Bookmarks are quick-access links to services in your homelab. They appear in the right column as icon cards. No data, no status — just a fast way to jump to a service from the NORA UI.

Add bookmarks from **Settings → Bookmarks**.
