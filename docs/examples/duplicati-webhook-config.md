# Duplicati Webhook Configuration for NORA

Duplicati sends its native JSON result object via HTTP. NORA's `duplicati`
profile parses that payload directly — no custom message template needed.

---

## Setup — Per backup job

1. Open the backup job and go to **Edit → Options → Advanced options**
2. Add:

```
send-http-url                  = http(s)://your-nora/ingest/{token}
send-http-result-output-format = Json
send-http-level                = All
```

3. Save the job.

`send-http-level=All` covers Success, Warning, Error, and Fatal. Narrow it
if you only want failures.

---

## Setup — Via command line

```bash
duplicati-cli backup <destination> <source> \
  --send-http-url="http(s)://your-nora/ingest/{token}" \
  --send-http-result-output-format=Json \
  --send-http-level=All
```

---

## Payload shape

Duplicati posts `Content-Type: application/json` with a body like:

```json
{
  "Data": {
    "MainOperation": "Backup",
    "ParsedResult": "Success",
    "BeginTime": "2026-04-24T02:00:00Z",
    "EndTime":   "2026-04-24T02:04:11Z",
    "Duration":  "00:04:11",
    "AddedFiles": 12,
    "ModifiedFiles": 3,
    "DeletedFiles": 0,
    "SizeOfAddedFiles": 4823191,
    "Warnings": [],
    "Errors": []
  },
  "Extra": {
    "OperationName": "Backup",
    "backup-name":   "nightly-photos",
    "machine-name":  "nas01"
  }
}
```

NORA's profile maps these fields:

| Field          | JSONPath                  |
|----------------|---------------------------|
| operation      | `$.Extra.OperationName`   |
| backup_name    | `$.Extra.backup-name`     |
| machine_name   | `$.Extra.machine-name`    |
| action         | `$.Data.MainOperation`    |
| result         | `$.Data.ParsedResult`     |
| begin_time     | `$.Data.BeginTime`        |
| end_time       | `$.Data.EndTime`          |
| duration       | `$.Data.Duration`         |
| warnings       | `$.Data.Warnings`         |
| errors         | `$.Data.Errors`           |
| files_added    | `$.Data.AddedFiles`       |
| files_modified | `$.Data.ModifiedFiles`    |
| files_deleted  | `$.Data.DeletedFiles`     |
| bytes_added    | `$.Data.SizeOfAddedFiles` |

---

## Result values

| `$.Data.ParsedResult` | Meaning                        | NORA severity |
|-----------------------|--------------------------------|---------------|
| `Success`             | Backup completed cleanly       | info          |
| `Warning`             | Backup completed with warnings | warn          |
| `Error`               | Backup failed                  | error         |
| `Fatal`               | Catastrophic failure           | error         |
