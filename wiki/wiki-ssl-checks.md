# SSL Certificate Monitoring

SSL certificate expiry is a silent killer in a homelab. Everything is fine until it isn't — and then Vaultwarden is unreachable, Authelia breaks, and your entire reverse proxy stack is showing security warnings. NORA watches certificate expiry so you get warned well in advance.

---

## How SSL Checks Work

NORA has two SSL check modes. Which one to use depends on **where NORA is running relative to the service being checked.**

---

### Mode 1: Traefik-Sourced (Recommended for Same-Host Services)

If you're running Traefik as your reverse proxy, NORA can read certificate expiry directly from the Traefik API — **without making any outbound TLS connection to the service.**

This is the right approach for services proxied by Traefik when NORA runs on the same Docker host or network.

**How to enable:**
1. Go to **Settings → Infrastructure → Traefik Integration**
2. Enter your Traefik API URL (e.g., `http://traefik:8080`)
3. Save

Once connected, NORA discovers all domains from Traefik's router list and reads their Let's Encrypt certificate expiry dates. These populate the **SSL Certificates panel** on the dashboard automatically.

**What you get:**
- All Traefik-managed domains, auto-discovered — no manual entry
- Certificate expiry dates pulled directly from Traefik's cert store
- No network connection to the proxied services required
- Works even if services are only reachable via Traefik (which is the whole point)

---

### Mode 2: Standalone SSL Check

For services **not** behind Traefik, or for externally-facing domains you want to verify independently, use a standalone SSL check. NORA makes a direct TLS handshake to the target and reads the certificate from the connection.

**To add a standalone SSL check:**

1. Go to **Monitor Checks → + Add Check**
2. Select **SSL** as the type

| Field | Description |
|---|---|
| Name | Display name — e.g., `itegasus.com external` |
| Target | Domain name only — e.g., `itegasus.com` or `vault.example.com` |
| Warn threshold | Days before expiry to show a warning — default `30` |
| Critical threshold | Days before expiry to show critical — default `7` |
| Interval | How often to check — `1h` is sufficient for certs |

**Status badges:**

| Status | Meaning |
|---|---|
| `UP` (green) | Certificate valid, more than 30 days remaining |
| `WARN` (yellow) | Certificate valid, within warn threshold |
| `DOWN` (red) | Certificate expired, invalid, or unreachable |

---

## ⚠️ Same-Host Warning

This is the most common SSL check mistake in a homelab. **Read this before adding standalone SSL checks.**

**The problem:**

If NORA and the service being checked run on the **same Docker host**, and the service is accessed via a domain name that resolves to the host's IP, Docker's networking may prevent NORA from making a clean outbound TLS connection back to itself. This is called **hairpin NAT**, and it's inconsistent — it works on some Docker networking setups and fails silently on others.

The result: the SSL check appears to work, but it may be checking against Docker's internal networking rather than the real certificate, or it may fail with a connection error that looks like a cert problem.

**When this applies:**

- NORA is containerized and runs on the same host as Traefik
- The target domain resolves to the same host's public or LAN IP
- The service is accessed through a reverse proxy (Traefik, Nginx Proxy Manager, etc.)

**NORA will warn you** when it detects you're adding a standalone SSL check for a domain that appears to be same-host. The warning looks like:

> ⚠️ **Same-host target detected**  
> This domain appears to resolve to the same host NORA is running on. Standalone SSL checks may be unreliable in this configuration due to Docker networking constraints. Consider using **Traefik-sourced SSL monitoring** instead if this service is behind Traefik.

**The fix:**

Use Traefik-sourced SSL monitoring (Mode 1 above). It bypasses the network entirely and reads cert data directly from Traefik's API.

**Exceptions:**

- If NORA is running on a different host than the service, standalone SSL checks work fine
- If you're checking an external domain (not hosted by you), standalone checks are exactly right
- If your Docker setup handles hairpin NAT correctly (some router firmware and host configurations do), standalone checks may work — but the Traefik-sourced approach is always more reliable

---

## SSL Panel on the Dashboard

The SSL Certificates panel on the right column of the dashboard shows all monitored domains sorted by days remaining:

```
SSL Certificates

 18d   itegasus.com          Apr 11
 64d   sonarr.itegasus.com   May 27
 89d   proxmox.itegasus.com  Jun 21
```

- **Red** — fewer than 10 days remaining
- **Yellow** — 10–30 days remaining  
- **Green** — more than 30 days remaining

Certs are populated from both Traefik-sourced discovery and standalone SSL checks.

---

## Priority Apps for SSL Monitoring

Some services break **immediately and completely** when their certificate expires. These should always be monitored:

| App | Why it matters |
|---|---|
| **Traefik / Nginx Proxy Manager** | Your entire reverse proxy stack goes down |
| **Authelia** | Auth proxy failure breaks every app behind it |
| **Vaultwarden** | Password manager with an expired cert is a very bad day |
| **Matrix (Synapse)** | Federation breaks instantly on cert expiry |
| **OPNsense** | Admin UI becomes inaccessible |
| **Any publicly-facing domain** | Browsers refuse to connect; users see scary warnings |

---

## Certificate Renewal

NORA monitors certificate expiry — it does not renew certificates. Renewal is handled by your ACME client (Traefik's built-in ACME, Certbot, Sewer, etc.). NORA's job is to tell you when renewal hasn't happened in time.

If you see a cert in the warning zone and renewal should have happened automatically, that's a signal your ACME client has a problem — not that you need to act manually.
