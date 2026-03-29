# Webhooks

Webhooks are how apps push events to NORA in real time. When something happens in Sonarr, n8n, Home Assistant, or any other configured app, it fires an HTTP POST to NORA's ingest endpoint. NORA extracts fields, applies severity, stores the event, and updates the dashboard.

---

## Ingest Endpoint

Each app gets a unique ingest URL:

```
POST http://your-nora-host:8080/api/v1/ingest/{app-id}
```

The app ID is shown on the app's detail page. You'll copy this URL into your app's webhook configuration.

**No authentication is required** on the ingest endpoint. Secure it at the network level — run NORA on your internal network and don't expose the ingest endpoint to the internet.

---

## Setting Up Webhooks by App

Profile-based apps show step-by-step setup instructions inside NORA after you add the app. The instructions are specific to each app's webhook configuration UI. Here's the general pattern for common apps:

### Sonarr / Radarr / Lidarr

1. Open the app → **Settings → Connect → + Add Connection → Webhook**
2. Name: `NORA`
3. Notification Triggers: enable the events you want (Grab, Download, Upgrade, Health Issue)
4. URL: paste your NORA ingest URL
5. Method: `POST`
6. Save and test

**Recommended events:** Download, Upgrade, Health Issue  
**Skip:** Rename (noisy, low signal)

---

### n8n

1. In n8n, add a **HTTP Request** node at the end of any workflow you want to track
2. Method: `POST`
3. URL: your NORA ingest URL
4. Body: JSON with fields you want to capture — e.g., `{"workflow": "{{$workflow.name}}", "status": "success"}`

Or use n8n's built-in **Error Workflow** trigger to automatically POST to NORA whenever any workflow fails.

---

### Home Assistant

Add an automation that fires on any trigger you care about:

```yaml
automation:
  - alias: "NORA — Notify on alarm state change"
    trigger:
      platform: state
      entity_id: alarm_control_panel.home
    action:
      service: rest_command.nora_ingest
      data:
        payload:
          event: "alarm_state_change"
          state: "{{ trigger.to_state.state }}"
          entity: "{{ trigger.entity_id }}"
```

Add the `rest_command` to your `configuration.yaml`:

```yaml
rest_command:
  nora_ingest:
    url: "http://your-nora-host:8080/api/v1/ingest/{app-id}"
    method: POST
    content_type: "application/json"
    payload: "{{ payload | to_json }}"
```

---

### Watchtower

Add these environment variables to your Watchtower container:

```yaml
environment:
  - WATCHTOWER_NOTIFICATIONS=http
  - WATCHTOWER_NOTIFICATION_URL=http://your-nora-host:8080/api/v1/ingest/{app-id}
```

Watchtower will POST a notification every time it updates a container image.

---

### DIUN (Docker Image Update Notifier)

In your DIUN config (`diun.yml`):

```yaml
notif:
  webhook:
    endpoint: "http://your-nora-host:8080/api/v1/ingest/{app-id}"
    method: POST
    headers:
      Content-Type: "application/json"
```

---

### Duplicati

1. Open Duplicati → **Settings → Default options**
2. Add option: `send-http-url` = your NORA ingest URL
3. Add option: `send-http-verb` = `POST`
4. Add option: `send-http-result-output-format` = `Json`

---

### Gotify / Any Generic App

NORA accepts any valid JSON payload. If an app can fire a webhook with a JSON body, NORA can ingest it. Use a custom profile to define how the fields map to NORA's event model.

---

## Payload Format

NORA accepts any JSON. Profile field mappings extract values using JSONPath expressions. If no profile field mappings match, the raw payload is still stored and the event appears in the feed with a generic display.

**Minimum useful payload:**
```json
{
  "event": "download_complete",
  "title": "Breaking Bad S03E04",
  "quality": "1080p"
}
```

**NORA stores:**
- The full raw payload (never modified)
- Extracted fields from profile mappings
- A computed display string from the profile's display template
- Severity (derived from profile severity mapping, or `info` by default)
- Timestamp of receipt

---

## Rate Limiting

Each app has a configurable rate limit (default: 100 events/minute). Events exceeding the limit are dropped and a warning is logged. This protects NORA from runaway webhook loops — a misconfigured Home Assistant automation that fires 1,000 times a minute shouldn't take NORA down.

Adjust the rate limit per app in **Apps → [App Name] → Settings**.

---

## Testing Your Webhook

Use the **Test** button on the app's detail page to send a test event and verify NORA receives and parses it correctly. The test event appears in the event feed with a `[test]` label and is not counted in digest summaries.

You can also use curl:

```bash
curl -X POST http://your-nora-host:8080/api/v1/ingest/{app-id} \
  -H "Content-Type: application/json" \
  -d '{"event": "test", "message": "hello from curl"}'
```

A `200 OK` response means NORA received and stored the event.
