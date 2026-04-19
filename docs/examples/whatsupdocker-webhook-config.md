# What's Up Docker (WUD) Webhook Configuration for NORA

WUD sends notifications via triggers configured as environment variables.
Use the HTTP trigger to POST container update events to NORA.

---

## docker-compose.yml configuration

Add these environment variables to your WUD service:

```yaml
services:
  wud:
    image: getwud/wud
    container_name: wud
    environment:
      # HTTP trigger — posts to NORA on every container update found
      - WUD_TRIGGER_HTTP_NORA_URL=http(s)://your-nora/ingest/{token}
      - WUD_TRIGGER_HTTP_NORA_METHOD=POST
      # Optional: only notify on new updates, not every check cycle
      - WUD_TRIGGER_HTTP_NORA_ONCE=true
      # Optional: threshold — all, major, minor, patch
      - WUD_TRIGGER_HTTP_NORA_THRESHOLD=all
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    ports:
      - 3000:3000
    restart: unless-stopped
```

Replace the URL with your NORA ingest URL from your app settings.

---

## How it works

WUD watches your running containers on a schedule (default: every hour).
When it detects a new image version is available, it fires the HTTP trigger
once per container with the update details. NORA receives one event per
container that has an available update.

---

## Threshold options

| Value | Behaviour |
|---|---|
| `all` | Notify on any update — tag or digest change |
| `major` | Only major version bumps (e.g. 1.x → 2.x) |
| `minor` | Major + minor version bumps |
| `patch` | Major + minor + patch version bumps |

---

## Tips

- Set `ONCE=true` so you only get notified once per update, not on every watch cycle
- WUD also has a built-in web UI at port 3000 showing all watched containers and their update status
- Label individual containers with `wud.watch=false` to exclude them from watching
