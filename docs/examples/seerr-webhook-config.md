# Seerr Webhook Configuration for NORA

In Seerr go to **Settings → Notifications → Webhook**.

- Webhook URL: `{base_url}/ingest/{token}`
- JSON Payload: paste the body below

Enable all notification types listed below.

---

## JSON Payload

Seerr uses a single webhook endpoint with one payload for all notification types.
Paste this into the JSON Payload field:

```json
{
  "notification_type": "{{notification_type}}",
  "subject": "{{subject}}",
  "message": "{{message}}",
  "media_type": "{{media_type}}",
  "media_status": "{{media_status}}",
  "requested_by": "{{requestedBy_username}}",
  "reported_by": "{{reportedBy_username}}",
  "commented_by": "{{commentedBy_username}}",
  "comment_message": "{{comment_message}}",
  "issue_id": "{{issue_id}}",
  "request_id": "{{request_id}}"
}
```

---

## Enable these notification types

- Request Pending Approval
- Request Automatically Approved
- Request Approved
- Request Declined
- Request Available
- Request Processing Failed
- Issue Reported
- Issue Comment
- Issue Resolved
- Issue Reopened
