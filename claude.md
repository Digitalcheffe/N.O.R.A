@AGENTS.md
# Claude Code Instructions — NORA

## Before Starting Any Task

**Every new session starts here. No exceptions. Do not skip any step.**

### Step 1 — Establish ground truth from the remote
```
git fetch --all --prune
git checkout main
git pull origin main
git log --oneline -10
git branch -a
```
Read the output. Do not assume anything about what is merged or what branch you are on.
`git log` tells you what is actually on main right now.
`git branch -a` tells you what branches still exist locally and remotely.

### Step 2 — Confirm previous work is merged
Before starting anything new, verify the last known branch is no longer open.
If a branch still exists remotely, check its PR status with `gh pr list --state all`.
Do not declare something merged unless `git log` on main shows the commits.

### Step 3 — Clean up any stale local branches
```
git branch -d <any-branch-that-has-been-merged>
```
If `git branch -d` fails because the branch is not fully merged, stop and ask — do not force delete.

### Step 4 — Create a fresh branch for this session's work
```
git checkout -b <type>/<short-description>
```
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
- **Never assume git state — always verify with git fetch + git log before acting**
- **Never say a branch is merged or a feature is complete without confirming it in git log on main**

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
  apptemplate/      — YAML app template loader + field extraction engine
/frontend/          — React + Vite app
  src/
    components/     — shared UI components
    pages/          — one file per screen (Dashboard, AppDetail, Checks, etc.)
    api/            — typed API client (one function per endpoint)
    hooks/          — shared React hooks
/appprofiles/       — app YAML templates (embedded in binary at build)
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

### App Templates
- App templates are YAML files in `/appprofiles/*.yaml`
- They are embedded in the binary using `//go:embed *.yaml` in `/appprofiles/embed.go`
- The Go package lives in `/internal/apptemplate/` — main struct is `AppTemplate`
- JSONPath field extraction uses `github.com/PaesslerAG/jsonpath` or equivalent
- When writing a new app template, validate it loads correctly before committing

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
NORA_ADMIN_EMAIL       Bootstrap admin email — seeds first admin only when users table is empty
NORA_ADMIN_PASSWORD    Bootstrap admin password — used with NORA_ADMIN_EMAIL
```

---

## Before Writing Any New File
1. Check if the file already exists — don't create duplicates
2. Read `/docs/architecture.md` if your change touches data model or API surface
3. Check `/migrations/` before adding a new table — it may already be there
4. Check `/internal/repo/` before writing DB queries — the interface may exist
5. If adding a new app template, check `/appprofiles/` first

---

## Testing & Build Verification

Run all of the following before opening any PR. No exceptions.

### Backend
- `go test ./...` must pass
- Each handler must have at minimum a happy-path and an error-path test
- Use `net/http/httptest` for handler tests — no external test servers
- `golangci-lint run` — fix all warnings

### Frontend
- `cd frontend && npm run build` must succeed with zero TypeScript errors
- `npm run lint` — fix all warnings

### Docker Build
All development and testing runs in Docker Desktop. One Dockerfile. One `docker-compose.yml`. No exceptions.

```
docker compose up --build       — must complete with zero errors
curl http://localhost:8080/     — must return a response
docker images                   — nora image must be under 50MB
```

**Dockerfile is a 3-stage build — do not change this structure:**
- Stage 1 `frontend-build`: `node:20-alpine` → `npm ci` → `npm run build`
- Stage 2 `backend-build`: `golang:1.22-alpine` → `go mod download` → `go build`
- Stage 3 `final`: `alpine:3.19` → binary only

Final image contains ONLY the binary + ca-certificates + tzdata. No node_modules. No Go toolchain. No source code.

**Layer cache rule — always copy dependency manifests before source code:**
```
COPY package.json package-lock.json ./   ← before npm ci
COPY go.mod go.sum ./                    ← before go mod download
COPY . .                                 ← source always last
```
Violating this means every build downloads all dependencies from scratch.

**What Claude Code must NOT do:**
- Do NOT create a second compose file or any dev override
- Do NOT use concurrently, air, nodemon, or any process watcher
- Do NOT run backend and frontend as separate containers
- Do NOT write shell scripts that start multiple processes
- Do NOT use `--watch` flags on any docker command
- Do NOT copy node_modules into the final image
- Do NOT use the golang or node image as the runtime base — `alpine:3.19` only

**If the build fails, diagnose in this order:**
1. `cd frontend && npm run build`
2. `go build ./cmd/nora/`
3. `docker compose up --build`

Fix the earliest failing stage first. Use `docker build --progress=plain .` to see full output.

### PR Checklist
- [ ] `go test ./...` passes
- [ ] `npm run build` passes with zero TypeScript errors
- [ ] `docker compose up --build` completes with zero errors
- [ ] `curl http://localhost:8080/` returns a response
- [ ] `docker images` shows nora image under 50MB

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