$body = @"
## Move API poller auth config from static files into app settings

### Context

API poller auth configuration currently lives in static per-app files in
``/internal/apipoller/`` as hardcoded fields:

``````
auth_type: apikey_header
auth_header: X-Api-Key
``````

This works for built-in app profiles but breaks down for custom apps where
the user needs to supply their own auth type and header. It also means
changing auth config requires editing files rather than using the UI.

Before starting: read claude.md. Read ``/internal/apipoller/`` thoroughly.
Read the apps DB schema and existing app settings API before touching anything.

---

### What to change

**Backend**

Add ``auth_type`` and ``auth_header`` fields to the app settings DB record
(not per polling endpoint — one auth config per app, at the top of the
polling config, not repeated for each API call).

Supported auth types to start:
- ``apikey_header`` — send API key in a named header (e.g. ``X-Api-Key: {key}``)
- ``bearer`` — send ``Authorization: Bearer {key}``
- ``basic`` — send ``Authorization: Basic {base64(user:pass)}``
- ``none`` — no auth

The API poller should read auth config from the app settings record, not
from static files. Static files in ``/internal/apipoller/`` should no longer
contain auth fields.

**Frontend**

Add an auth config section to the app settings UI. Position it at the top
of the API polling section — one block for the whole app, not repeated per
endpoint.

``````
┌─ API Polling Auth ─────────────────────────────┐
│  Auth Type    [ apikey_header ▼ ]              │
│  Header Name  [ X-Api-Key        ]             │
│  API Key      [ ******************  ] [show]   │
└────────────────────────────────────────────────┘
``````

- Auth type dropdown: apikey_header, bearer, basic, none
- Header Name field shown only when auth_type == apikey_header
- For built-in app profiles, pre-populate auth_type and auth_header from
  profile defaults — user just supplies the key value in app settings

**App profiles**

Built-in profiles that support API polling should declare their default
auth config at the top of the ``api_polling`` section:

``````yaml
api_polling:
  auth_type: apikey_header
  auth_header: X-Api-Key
  endpoints:
    - name: total_series
      ...
``````

This is a default only — user can override in app settings UI.

---

### Expected result

- Auth config is stored per app in the DB, not in static files
- API poller reads auth from DB at runtime
- Users can configure auth for custom apps through the UI
- Built-in profiles pre-populate sensible defaults
- Static auth fields removed from ``/internal/apipoller/`` files

---

### Acceptance criteria

- [ ] auth_type and auth_header fields added to apps DB record via migration
- [ ] API poller reads auth from DB, not static files
- [ ] App settings UI shows auth config section above polling endpoints
- [ ] Pre-population from profile defaults works for built-in apps
- [ ] Custom apps can set any auth_type and header name
- [ ] go test ./... passes
- [ ] npm run build passes with zero TypeScript errors
- [ ] golangci-lint passes
"@

$tmpFile = [System.IO.Path]::GetTempFileName() + ".md"
[System.IO.File]::WriteAllText($tmpFile, $body, [System.Text.Encoding]::UTF8)
gh issue create `
  --title "Move API poller auth config from static files into app settings" `
  --label "feat,frontend,backend" `
  --body-file $tmpFile
Remove-Item $tmpFile
Write-Host "Issue created."
