@AGENTS.md
# Claude Code Instructions — NORA

## Before Starting Any Task
- Checkout main: `git checkout main`
- Pull latest: `git pull origin main`
- Create a fresh branch: `git checkout -b <type>/<short-description>`
- Branch naming: feat/, fix/, chore/, docs/
- Never commit directly to main or master
- Never start work on a stale branch

## During Task
- Commit after every meaningful change immediately
- Stage changes: `git add -A`
- Commit with a clear message: `git commit -m "<type>: <description>"`
- Push commits right away: `git push -u origin <branch-name>`
- Never leave work uncommitted

## After Task is Complete
- Run tests before creating PR
- Open a pull request: `gh pr create --title "<type>: <description>" --body "<what changed and why>"`
- Confirm the PR URL was created before finishing

## After PR is Merged
- Switch to main: `git checkout main`
- Pull latest: `git pull origin main`
- Delete stale branch: `git branch -d <branch-name>`
- Confirm clean state with `git status` before doing anything else

## Autonomy
- Do not ask for confirmation on routine git operations
- Do not pause for approval on branch creation, commits, pushes, or PRs
- Only stop and ask if something is ambiguous or destructive

---

## Project: NORA
**Full name:** Nexus Operations Recon & Alerts  
**Stack:** Go backend · SQLite · React + Vite frontend · Single Docker image  
**Spec:** /docs/architecture.md — read this before touching anything structural  
**Dashboard mockup:** /docs/dashboard.html — pixel reference for all UI work

---

## Directory Layout
```
/cmd/nora/          — main.go, entry point
/internal/
  auth/             — session middleware, JWT/cookie logic
  api/              — HTTP handlers (one file per resource group)
  ingest/           — webhook ingest pipeline
  monitor/          — ping / URL / SSL check runners + scheduler
  docker/           — Docker socket watcher + resource poller
  jobs/             — scheduled background jobs (rollups, retention, metrics, digest)
  models/           — Go structs for all DB entities
  repo/             — repository interfaces + SQLite implementations
  profile/          — YAML profile loader + field extraction engine
/frontend/          — React + Vite app
  src/
    components/     — shared UI components
    pages/          — one file per screen (Dashboard, AppDetail, Checks, etc.)
    api/            — typed API client (one function per endpoint)
    hooks/          — shared React hooks
/profiles/          — app YAML profiles (embedded in binary at build)
/migrations/        — SQL migration files (embedded, auto-run on startup)
/docs/              — architecture.md, dashboard.html
```

---

## Key Conventions

### Backend
- Router: chi (`github.com/go-chi/chi/v5`)
- All handlers live in `/internal/api/` — one file per resource group (apps.go, events.go, checks.go, etc.)
- No raw SQL in handlers — always go through a repository
- Config is loaded from environment variables at startup via a `Config` struct in `/internal/config/`
- All background jobs (scheduler, rollups, retention) start as goroutines in main.go and receive a context for clean shutdown
- Error responses are JSON: `{"error": "message"}`
- Successful list responses wrap in `{"data": [...], "total": N}`

### Dev Mode
- `NORA_DEV_MODE=true` injects a hardcoded admin session — no login required
- The dev bypass lives in `/internal/auth/middleware.go` — a clear comment marks it for removal in T-30
- The frontend API client reads `VITE_DEV_MODE=true` and skips auth headers
- **Never ship NORA_DEV_MODE=true in a Docker image tag meant for production**

### Frontend
- All API calls go through `/frontend/src/api/client.ts` — never fetch() directly in a component
- One page component per screen, colocated with its types
- Styling matches `/docs/dashboard.html` — use the same CSS variables, color palette, and font choices
- CSS variables from the dashboard mockup are defined in `/frontend/src/styles/variables.css`
- Sparklines are inline SVG — no chart library for sparklines specifically
- No `<form>` elements — use controlled inputs with onClick handlers

### Database / Migrations
- Migration files live in `/migrations/` named `001_init.sql`, `002_...sql` etc.
- All tables are created in migrations — no `CREATE TABLE IF NOT EXISTS` in application code
- The `alert_rules` table is created in the initial migration (T-03) even though the feature is v2
- The `web_push_subscriptions` table is created in the initial migration

### App Profiles
- Profiles are YAML files in `/profiles/*.yaml`
- They are embedded in the binary using `//go:embed profiles`
- The Go struct for a profile lives in `/internal/profile/profile.go`
- JSONPath field extraction uses `github.com/PaesslerAG/jsonpath` or equivalent
- When writing a new profile, validate it loads correctly before committing

### Scheduler / Background Jobs
- All scheduled jobs use a simple ticker-based runner in `/internal/jobs/scheduler.go`
- Each job is a function that accepts a context and a repo handle
- Jobs log what they do at info level, log errors but do not crash the process

---

## Environment Variables
```
NORA_DEV_MODE          true | false (default false)
NORA_SECRET            required — used for JWT signing / session encryption
NORA_DB_PATH           path to SQLite file (default /data/nora.db)
NORA_PORT              HTTP port (default 8080)
NORA_SMTP_HOST         SMTP server hostname
NORA_SMTP_PORT         SMTP port (default 587)
NORA_SMTP_USER         SMTP username
NORA_SMTP_PASS         SMTP password
NORA_SMTP_FROM         From address for digest emails
NORA_DIGEST_SCHEDULE   Cron expression for monthly digest (default: 0 8 1 * *)
NORA_VAPID_PUBLIC      VAPID public key (auto-generated on first run if absent)
NORA_VAPID_PRIVATE     VAPID private key (auto-generated on first run if absent)
```

---

## Before Writing Any New File
1. Check if the file already exists — don't create duplicates
2. Read `/docs/architecture.md` if your change touches data model or API surface
3. Check `/migrations/` before adding a new table — it may already be there
4. Check `/internal/repo/` before writing DB queries — the interface may exist
5. If adding a new profile, check `/profiles/` first

---

## Testing Expectations
- Backend: `go test ./...` must pass before opening a PR
- Each handler should have at minimum a happy-path and an error-path test
- Use `net/http/httptest` for handler tests — no external test servers
- Frontend: `npm run build` must succeed with zero TypeScript errors
- Lint: `golangci-lint run` for Go, `npm run lint` for frontend — fix all warnings

---

## PR Description Template
```
## What
Brief description of what this PR does.

## Why
Which task (T-XX) this closes. Link to the GH issue.

## How
Key implementation decisions made. Anything non-obvious.

## Test coverage
What was tested and how.

## Closes
Closes #<issue-number>
```
