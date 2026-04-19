# Duplicati Webhook Configuration for NORA

Duplicati sends HTTP notifications via advanced options. You can set these
globally (all jobs) or per backup job.

---

## Setup — Global (all backup jobs)

1. In Duplicati go to **Settings → Default options**
2. Scroll to **Advanced options** and add the following:

```
--send-http-json-urls=http(s)://your-nora/ingest/{token}
--send-http-message={"action":"backup.%PARSEDRESULT%","backup_name":"%backup-name%","result":"%PARSEDRESULT%","operation":"%OPERATIONNAME%","source_path":"%LOCALPATH%"}
--send-http-level=Success,Warning,Error,Fatal
```

3. Save settings

---

## Setup — Per backup job

1. Open a backup job and go to **Options → Advanced options**
2. Add the same options as above
3. Save the job

Per-job settings override global settings.

---

## Setup — Via command line

Add these flags to your Duplicati command:

```bash
duplicati-cli backup <destination> <source> \
  --send-http-json-urls="http(s)://your-nora/ingest/{token}" \
  --send-http-message='{"action":"backup.%PARSEDRESULT%","backup_name":"%backup-name%","result":"%PARSEDRESULT%","operation":"%OPERATIONNAME%","source_path":"%LOCALPATH%"}' \
  --send-http-level=Success,Warning,Error,Fatal
```

---

## Result values

| %PARSEDRESULT% value | Meaning | NORA severity |
|---|---|---|
| `Success` | Backup completed cleanly | info |
| `Warning` | Backup completed with warnings | warn |
| `Error` | Backup failed | error |
| `Fatal` | Catastrophic failure | error |
