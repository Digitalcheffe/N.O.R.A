# DIUN Webhook Configuration for NORA

Configure DIUN via environment variables in docker-compose.

---

## docker-compose.yml configuration

```yaml
services:
  diun:
    image: crazymax/diun:latest
    container_name: diun
    command: serve
    volumes:
      - ./data:/data
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=America/New_York
      - DIUN_WATCH_SCHEDULE=0 */6 * * *
      - DIUN_PROVIDERS_DOCKER=true
      - DIUN_PROVIDERS_DOCKER_WATCHBYDEFAULT=true
      # NORA webhook
      - DIUN_NOTIF_WEBHOOK_ENDPOINT=http(s)://your-nora/ingest/{token}
      - DIUN_NOTIF_WEBHOOK_METHOD=POST
      - DIUN_NOTIF_WEBHOOK_HEADERS_CONTENT-TYPE=application/json
      - DIUN_NOTIF_WEBHOOK_TIMEOUT=10s
    restart: unless-stopped
```

Replace the URL with your NORA ingest URL from your app settings.

---

## Tips

- `WATCHBYDEFAULT=true` watches all running containers automatically
- Label individual containers with `diun.enable=false` to exclude them
- `DIUN_WATCH_FIRSTCHECKNOTIF=true` sends a notification on first run for all watched images — useful to confirm it's working but can be noisy
- Schedule uses cron syntax — `0 */6 * * *` checks every 6 hours
