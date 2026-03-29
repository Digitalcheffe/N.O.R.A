# Custom App Profiles

If your app isn't in the NORA library, the custom profile editor lets you map any webhook payload to NORA's event model. No files to upload, no YAML to write by hand — fill in a form and you're done.

---

## When to Use a Custom Profile

- Your app isn't in the built-in library
- You want a different field mapping or display format for a library app
- You've built an internal tool that fires webhooks and want NORA to understand them

---

## Creating a Custom Profile

Go to **Apps → Add App → Custom Profile**.

### Step 1: Basic Info

| Field | Description |
|---|---|
| App Name | Display name shown on dashboard and in events |
| Category | Media · Automation · Network · Infrastructure · Storage · Security |
| Base URL | The app's URL — used for URL checks and display |

---

### Step 2: Webhook Field Mappings

Field mappings extract values from the incoming JSON payload and tag them for display and filtering.

**Format:** `tag_name` → `$.json.path`

JSONPath examples:

| Payload | Path | Extracted Value |
|---|---|---|
| `{"title": "Breaking Bad"}` | `$.title` | `Breaking Bad` |
| `{"media": {"quality": "1080p"}}` | `$.media.quality` | `1080p` |
| `{"events": [{"type": "download"}]}` | `$.events[0].type` | `download` |

Add as many mappings as you need. Extracted fields are stored in the event's `fields` map and are available for display templates and severity mapping.

---

### Step 3: Display Template

The display template defines the one-line summary shown in the event feed and dashboard. Use `{field_name}` to reference extracted fields.

**Example:**
```
{title} — {quality} via {indexer}
```

Renders as:
```
Breaking Bad S03E04 — 1080p via NZBgeek
```

If no template is set, NORA uses the raw payload's first string field as a fallback.

---

### Step 4: Severity Mapping

Map a field value to a severity level so NORA can color-code and prioritize events correctly.

| Severity | Meaning | Color |
|---|---|---|
| `debug` | Verbose noise — usually filtered out | — |
| `info` | Normal operations | Blue |
| `warn` | Something worth noting | Yellow |
| `error` | Something broke | Red |
| `critical` | Immediate attention required | Red (persistent) |

**Example mapping:**

```
Match field:  event_type
  "download_complete"  → info
  "health_issue"       → warn
  "download_failed"    → error
```

If no severity mapping matches, the event defaults to `info`.

---

### Step 5: Monitor Check (Optional)

Optionally configure an active health check to run alongside webhook event collection:

| Field | Description |
|---|---|
| Check Type | URL · Ping · SSL |
| Target | URL or IP |
| Healthy Status | Expected HTTP status (URL checks) |
| Interval | 1m / 5m / 15m / 1h |

This is the same as adding a check manually from the Monitor Checks screen — it's just convenient to configure it here while you're setting up the app.

---

### Step 6: Digest Categories

Define how this app's events roll up into the monthly digest. Each category is a filter — events matching the filter are counted under that label.

**Example for a download app:**

| Label | Match Field | Match Value |
|---|---|---|
| Downloads | event_type | download_complete |
| Upgrades | event_type | upgrade |
| Errors | severity | error |

The labels you define here become columns in the monthly digest email and summary bar categories on the dashboard (when there's data for the current period).

---

## Profile YAML (Advanced)

Custom profiles are stored internally as YAML. If you want to contribute a profile to the NORA library, here's the schema:

```yaml
meta:
  name: string
  category: Media | Automation | Network | Infrastructure | Storage | Security
  logo: string              # filename in /profiles/logos/
  description: string
  capability: full | webhook_only | monitor_only | docker_only | limited

webhook:
  setup_instructions: string
  recommended_events: [string]
  field_mappings:
    tag_name: "$.json.path"
  display_template: string  # "{field_name} — {other_field}"
  severity_mapping:
    event_value: debug | info | warn | error | critical

monitor:
  check_type: url | ping | ssl
  check_url: string         # supports {base_url} variable
  auth_header: string       # supports {api_key} variable
  healthy_status: int       # default 200
  check_interval: string    # 1m | 5m | 15m | 1h

digest:
  categories:
    - label: string
      match_field: string
      match_value: string
      match_severity: string
```

To contribute a profile, open a GitHub issue with the YAML and we'll review and merge it into the library.
