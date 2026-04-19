# Home Assistant Webhook Configuration for NORA

NORA receives events from Home Assistant via automations using the
`rest_command` integration or the built-in `rest` action. You define what
triggers the notification and what it says — NORA just ingests it.

---

## Step 1 — Add the REST command to configuration.yaml

Add this to your `configuration.yaml` (or a split config file):

```yaml
rest_command:
  nora_event:
    url: "http(s)://your-nora/ingest/{token}"
    method: POST
    content_type: application/json
    payload: >
      {
        "action": "ha.event",
        "subject": "{{ subject }}",
        "message": "{{ message }}",
        "severity": "{{ severity }}",
        "extra": {}
      }
```

Replace the URL with your NORA ingest URL from your app settings.

Restart Home Assistant after saving.

---

## Step 2 — Call it from an automation

In your automation's action section, call the REST command and pass your values:

### Using the UI

1. Add an action → **Call service**
2. Service: `rest_command.nora_event`
3. Service data:
```yaml
subject: "Front Door Opened"
message: "Motion detected at front door"
severity: "warn"
```

### Using YAML directly

```yaml
action:
  - service: rest_command.nora_event
    data:
      subject: "Front Door Opened"
      message: "Motion detected at front door"
      severity: "warn"
```

---

## Step 3 — Using template variables

You can use Home Assistant templates to pull in dynamic values:

```yaml
action:
  - service: rest_command.nora_event
    data:
      subject: "{{ trigger.to_state.attributes.friendly_name }} changed"
      message: "State changed to {{ trigger.to_state.state }}"
      severity: "info"
```

---

## Severity guide

Use these values for `severity` to control how events appear in NORA:

| Value | When to use |
|---|---|
| `info` | Normal activity — automations fired, state changes |
| `warn` | Something needs attention — battery low, door left open |
| `error` | Something is wrong — sensor offline, automation failed |

---

## Example automations

### Battery low warning
```yaml
alias: Notify NORA - Battery Low
trigger:
  - platform: numeric_state
    entity_id: sensor.front_door_battery
    below: 20
action:
  - service: rest_command.nora_event
    data:
      subject: "Battery Low - Front Door Sensor"
      message: "Battery at {{ states('sensor.front_door_battery') }}%"
      severity: "warn"
```

### HA startup
```yaml
alias: Notify NORA - HA Started
trigger:
  - platform: homeassistant
    event: start
action:
  - service: rest_command.nora_event
    data:
      subject: "Home Assistant Started"
      message: "HA restarted successfully"
      severity: "info"
```

### Door left open
```yaml
alias: Notify NORA - Front Door Left Open
trigger:
  - platform: state
    entity_id: binary_sensor.front_door
    to: "on"
    for:
      minutes: 10
action:
  - service: rest_command.nora_event
    data:
      subject: "Front Door Left Open"
      message: "Front door has been open for 10 minutes"
      severity: "warn"
```
