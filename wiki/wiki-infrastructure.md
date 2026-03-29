# Infrastructure Components

Infrastructure components are the physical and virtual machines that run your homelab. They're first-class entities in NORA — not just parents for apps, but independently monitored and displayed.

---

## What Counts as Infrastructure

- **Physical hosts** — bare metal servers, Proxmox nodes, Synology NAS, OPNsense router, a Raspberry Pi sitting in a corner
- **Virtual hosts** — VMs and LXC containers running on a physical host
- **Docker engines** — Docker daemons, either local (socket) or remote (via socket proxy)

Infrastructure components are standalone. You don't need a full topology chain. Add a Synology NAS on its own — NORA will monitor it without needing to know what's running on it.

---

## Adding Infrastructure

Navigate to **Infrastructure** in the sidebar, then click **+ Add Component**.

### Physical Host

| Field | Description |
|---|---|
| Name | Display name — e.g., `proxmox-node1`, `synology-nas` |
| IP Address | LAN IP used for ping checks |
| Type | Proxmox Node · Synology DSM · Generic Linux · Generic Windows · Bare Metal |
| Notes | Optional — anything useful for future-you |

**Type matters.** Selecting Proxmox or Synology enables API-based resource collection (CPU, memory, disk). Generic types fall back to SNMP or ping-only.

### Virtual Host

| Field | Description |
|---|---|
| Name | Display name — e.g., `rocky-vm01` |
| IP Address | VM or container LAN IP |
| Type | VM · LXC · WSL |
| Parent Host | The physical host this runs on (dropdown, optional) |

### Docker Engine

| Field | Description |
|---|---|
| Name | Display name — e.g., `docker-engine-01` |
| Socket Type | Local (`/var/run/docker.sock`) or Remote Proxy |
| Socket Path / URL | Path for local socket; URL for remote proxy |
| Parent Host | The host this engine runs on (optional) |

---

## Collection Methods by Host Type

NORA supports three methods for pulling resource metrics from infrastructure components. The method used depends on host type.

### Proxmox REST API

Used for Proxmox VE nodes. Requires an API token.

**Setup:**
1. In Proxmox, go to **Datacenter → Permissions → API Tokens**
2. Create a token for user `root@pam` (or a dedicated monitoring user)
3. Copy the token ID and secret
4. In NORA, enter the Proxmox base URL (e.g., `https://192.168.1.10:8006`) and paste the token

**What NORA collects:**
- Node CPU, memory, and disk usage
- VM and LXC container running state
- Recent task history
- Storage pool health

**Note:** Proxmox uses a self-signed certificate by default. NORA skips TLS verification for Proxmox API connections — this is expected behavior, not a bug.

---

### Synology DSM API

Used for Synology NAS devices. Uses session-based authentication.

**Setup:**
1. In NORA, enter the Synology DSM base URL (e.g., `http://192.168.1.20:5000`)
2. Enter your DSM admin username and password
3. NORA establishes a session and polls on the configured interval

**What NORA collects:**
- CPU and memory usage
- Volume health and used/free space
- S.M.A.R.T. status (if available via API)

**Note:** NORA stores DSM credentials encrypted in the local database. Credentials never leave your network.

---

### SNMP

Used for generic Linux and Windows hosts that don't have a dedicated API. Requires SNMP to be enabled on the target host.

**Linux (Ubuntu/Debian) setup:**
```bash
sudo apt install snmpd
sudo nano /etc/snmp/snmpd.conf
# Change: agentaddress udp:161
# Add:    rocommunity public 192.168.1.0/24
sudo systemctl restart snmpd
```

**Linux (RHEL/Rocky) setup:**
```bash
sudo dnf install net-snmp net-snmp-utils
sudo systemctl enable --now snmpd
# Edit /etc/snmp/snmpd.conf similarly
```

**Windows setup:**
1. Open **Windows Features** → enable **Simple Network Management Protocol (SNMP)**
2. Open **Services** → SNMP Service → Properties → Security tab
3. Add an accepted community name (e.g., `public`) with Read Only access
4. Under **Traps**, add your NORA host IP

**In NORA:**

| Field | Description |
|---|---|
| Community | SNMP community string (default: `public`) |
| Port | UDP port (default: `161`) |
| Version | SNMP v2c (v3 not currently supported) |

**What NORA collects via SNMP:**
- CPU load (1-minute average)
- Memory used/total
- Disk used/total per mount point

---

## Resource Metrics on the Dashboard

Once a component is added and collection is configured, resource bars appear on the Infrastructure section of the dashboard:

```
proxmox-node1  ● Online
CPU  ████░░░░░░  34%
MEM  ██████░░░░  62%
DSK  ███░░░░░░░  28%
```

**Color thresholds:**

| Threshold | Color |
|---|---|
| < 70% | Blue (normal) |
| 70–90% | Yellow (warning) |
| > 90% | Red (critical) |

Resource readings are retained for 7 days raw, then rolled up to hourly (90 days) and daily (forever).

---

## Ping Monitoring for Infrastructure

Physical hosts with a configured IP address can have a ping check attached automatically. Ping checks are appropriate for:

- NAS devices
- Routers and firewalls (OPNsense)
- Proxmox nodes
- Any bare-metal host with a stable IP

Ping checks are **not** appropriate for Docker containers — those are covered by Docker socket health checks, which are more reliable than ICMP for containerized workloads.

To add a ping check for an infrastructure component, go to **Monitor Checks → + Add Check** and select **Ping**. See [Monitor Checks](wiki-monitor-checks) for details.

---

## Network Map

The Infrastructure page includes a network map view that shows relationships between components visually. This is separate from the topology tree and is intended for at-a-glance situational awareness — not configuration.

Relationships shown in the map are derived from the parent/child assignments you make when adding components. Components with no parent show as standalone nodes.
