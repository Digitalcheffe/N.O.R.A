# NORA Wiki

> **Nexus Operations Recon & Alerts** — a self-hosted homelab visibility hub.  
> One install. One UI. No agents. No cloud dependency.

---

## Contents

- [Adding Your First App](wiki-adding-apps)
- [Infrastructure Components](wiki-infrastructure)
- [Monitor Checks](wiki-monitor-checks)
- [SSL Certificate Monitoring](wiki-ssl-checks)
- [Webhooks](wiki-webhooks)
- [Custom App Profiles](wiki-custom-profiles)
- [Dashboard Behavior](wiki-dashboard)

---

## Quick Start

1. Deploy NORA using Docker — see the [README](../README.md) for the compose snippet
2. Open the UI (default: `http://your-host:8080`)
3. Add an app from the library or create a custom profile
4. Optionally add infrastructure components (Proxmox, Synology, etc.)
5. Add monitor checks (ping, URL, SSL) for anything you care about staying up
6. Configure Web Push or SMTP for notifications

NORA only shows what you've configured. An empty install shows setup prompts — not empty widgets.
