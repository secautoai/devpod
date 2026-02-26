# DevPod Web – Feature Implementation Plan

## Overview

Three major features built in layered phases so each phase is independently
testable and shippable:

1. **SQLite Data Store** – replace the flat JSON file store with SQLite
2. **RBAC & User Management** – per-user provider allowlists, operation audit log
3. **Admin UI + Branding** – login page, admin panel, customisable logo / links

A **mock provider** (pure Go, no Docker/SSH needed) is built in Phase 6 to
enable full end-to-end testing without external infrastructure.

---

## Dependency to add

```
modernc.org/sqlite v1.x  (pure-Go SQLite, no CGO required)
```

Add with: `GOTOOLCHAIN=local go get modernc.org/sqlite` once Go 1.25.5 is
available, or vendor it manually. All SQLite code is isolated in
`cmd/web/db/` so it can be swapped for the JSON store via a build tag if
the download fails.

---

## Phase 1 – SQLite Database Layer  (`cmd/web/db/`)

### New files
| File | Purpose |
|------|---------|
| `cmd/web/db/db.go` | Open/migrate database, exported `DB` type |
| `cmd/web/db/schema.go` | `CREATE TABLE` SQL constants |
| `cmd/web/db/users.go` | `UserStore` backed by SQLite (same interface as current JSON store) |
| `cmd/web/db/permissions.go` | Provider-access RBAC queries |
| `cmd/web/db/oplog.go` | `OperationLog` write / query |
| `cmd/web/db/branding.go` | Key-value branding settings |
| `cmd/web/db/db_test.go` | Table-driven unit tests for every query |

### Schema

```sql
-- users
CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY,
    username    TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'user',   -- 'admin' | 'user'
    docker_host TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- which providers each user may use
-- empty table = all providers allowed for all users (backwards compat)
CREATE TABLE IF NOT EXISTS provider_permissions (
    username    TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    allowed     INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (username, provider_id)
);

-- audit log for workspace operations
CREATE TABLE IF NOT EXISTS operation_logs (
    id          TEXT PRIMARY KEY,
    username    TEXT NOT NULL,
    action      TEXT NOT NULL,   -- 'create' | 'delete' | 'start' | 'stop' | 'command'
    target      TEXT NOT NULL,   -- workspace id or provider id
    args        TEXT NOT NULL,   -- JSON array
    started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME,
    exit_code   INTEGER,
    error_msg   TEXT NOT NULL DEFAULT ''
);

-- admin-editable branding key-value store
CREATE TABLE IF NOT EXISTS branding (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);
-- Default rows inserted on first run:
--   app_name, logo_url, favicon_url, provider_download_url,
--   support_url, docs_url, primary_color
```

### Migration strategy
- `db.Open(path)` creates tables if missing (idempotent `CREATE TABLE IF NOT EXISTS`)
- On startup `web.go` calls `db.Open`, then `userStore := db.NewUserStore(conn)`
- Existing `users.json` is migrated once on first start → file renamed to `users.json.migrated`

### Tests (`db_test.go`)
- `TestUserCRUD` – create, authenticate, update role, delete, last-admin guard
- `TestProviderPermissions` – grant, revoke, list, default-allow behaviour
- `TestOperationLog` – write entry, query by user, query by time range
- `TestBranding` – set/get each key, list all

---

## Phase 2 – Auth Wired Into All API Handlers

### Changes to `cmd/web/web.go`
Currently `handleCommandRun`, `handleCommandStream`, etc. have **no auth**.
Wire `withAuth` around every handler that executes devpod commands.

The `Claims` from the JWT are used to:
1. Set `DEVPOD_HOME` to the caller's home directory (already in `user_isolation.go`)
2. Check provider RBAC before executing commands

```
POST /api/command          → withAuth(handleCommandRun)
WS   /api/command/stream   → token passed as ?token= query param (WS can't set headers)
GET  /api/providers        → withAuth(handleProviders)
GET  /api/workspaces       → withAuth(handleWorkspaces)
POST /api/signal           → withAuth(handleSignal)
```

Public (no auth needed): `/api/login`, `/api/health`, `/api/version`,
`/api/platform`, `/api/branding` (read-only public branding).

### Provider RBAC enforcement
`handleCommandRun` inspects `req.Args` to detect the provider being used:

```go
func extractProvider(args []string) string {
    // looks for --provider=<id> or -p <id> in args
}
```

If `provider_permissions` has rows for the user **and** the requested
provider is not in the allowed set → return HTTP 403 with JSON error.

### Tests (`web_test.go` additions)
- `TestAuthRequired` – unauthenticated command returns 401
- `TestProviderRBACEnforced` – user without permission gets 403
- `TestAdminBypasses` – admin role always allowed

---

## Phase 3 – Provider RBAC Admin API

### New endpoints
```
GET  /api/admin/users/{username}/providers
     → list allowed provider IDs for user, or "*" if unrestricted

PUT  /api/admin/users/{username}/providers
     body: { "providers": ["docker", "kubernetes"] }
     → replace allowed set; empty array = deny all; null / missing = unrestricted

DELETE /api/admin/users/{username}/providers/{providerID}
     → revoke single provider

GET  /api/user/providers
     → returns providers the calling user is allowed to use
        (filters the devpod provider list against the allowlist)
```

### `cmd/web/db/permissions.go` interface
```go
// SetAllowed replaces the entire allowlist for a user.
// providers == nil means "unrestricted" (delete all rows for user).
// providers == []string{} means "deny all".
func (p *PermissionStore) SetAllowed(username string, providers []string) error

// GetAllowed returns nil = unrestricted, []string = explicit list.
func (p *PermissionStore) GetAllowed(username string) ([]string, error)

// IsAllowed returns true if the user may use the given provider.
func (p *PermissionStore) IsAllowed(username, providerID string) (bool, error)
```

### Tests
- `TestSetAllowedReplaces` – second SetAllowed overwrites first
- `TestIsAllowedUnrestricted` – nil allowlist returns true for any provider
- `TestIsAllowedDenyAll` – empty allowlist returns false
- `TestRBACAPIHandler` – HTTP integration test for all three endpoints

---

## Phase 4 – Operation Audit Log

### Write side
Every `handleCommandRun` and `handleCommandStream` call writes an
`OperationLog` row **asynchronously** (fire-and-forget goroutine) so it
never blocks the command path.

```go
type OperationLog struct {
    ID         string
    Username   string
    Action     string     // inferred from Args[0:2]
    Target     string     // workspace/provider name
    Args       []string
    StartedAt  time.Time
    FinishedAt *time.Time
    ExitCode   *int
    ErrorMsg   string
}
```

### New read endpoints
```
GET /api/logs?limit=50&offset=0&action=create
    → paginated list for the calling user (own logs only)

GET /api/admin/logs?username=alice&limit=100
    → admin: all users, filterable by username / action / date range
```

### Tests
- `TestOpLogWritten` – after POST /api/command, a log row is created
- `TestOpLogPagination` – limit/offset work correctly
- `TestAdminLogsFilter` – filter by username and action
- `TestUserCannotSeeOtherLogs` – user querying /api/logs only sees own rows

---

## Phase 5 – Branding Settings API

### New endpoints
```
GET  /api/branding
     → public, no auth; returns current branding JSON
     { app_name, logo_url, favicon_url, provider_download_url,
       support_url, docs_url, primary_color }

PUT  /api/admin/branding
     → admin only; body: partial key-value map; merges with existing
```

### Default values
```json
{
  "app_name": "DevPod",
  "logo_url": "",
  "favicon_url": "",
  "provider_download_url": "https://github.com/loft-sh/devpod/releases",
  "support_url": "",
  "docs_url": "https://devpod.sh/docs",
  "primary_color": ""
}
```

### Tests
- `TestBrandingDefaults` – fresh DB returns defaults
- `TestBrandingUpdate` – PUT merges partial updates
- `TestBrandingPublic` – GET /api/branding needs no auth token

---

## Phase 6 – Mock Provider (`cmd/devpod-provider-mock/`)

A self-contained Go binary that implements the devpod provider protocol.
Workspaces are stored as JSON files under `~/.devpod-mock/workspaces/`.
No Docker, SSH, or network needed.

### Files
```
cmd/devpod-provider-mock/
  main.go          – cobra root + subcommands
  workspace.go     – file-backed workspace state
  provider.yaml    – provider manifest
```

### Commands implemented
| Command | Behaviour |
|---------|-----------|
| `init` | Creates `~/.devpod-mock/` directory |
| `create` | Creates workspace JSON file, echoes JSON log lines |
| `start` | Sets status = "Running" |
| `stop` | Sets status = "Stopped" |
| `delete` | Removes workspace file |
| `status` | Prints JSON `{ "id": "...", "state": "Running" }` |
| `get-name` | Prints provider name for `devpod helper get-provider-name` |

### `provider.yaml` (embedded in binary)
```yaml
name: mock
version: v0.0.1
description: Mock provider for testing – no external dependencies
icon: https://devpod.sh/favicon.ico
exec:
  init:
    - mock
    - init
  create:
    - mock
    - create
  start:
    - mock
    - start
  stop:
    - mock
    - stop
  delete:
    - mock
    - delete
  status:
    - mock
    - status
options: {}
```

### Tests
- `TestMockProviderInit`   – creates state directory
- `TestMockProviderCreate` – file written, JSON log output emitted
- `TestMockProviderCRUD`   – create → start → stop → delete lifecycle
- `TestMockProviderStatus` – returns correct JSON after each transition

---

## Phase 7 – Frontend: Authentication

### New files
```
desktop/src/contexts/AuthContext.tsx     – token, user, login(), logout()
desktop/src/views/Login/                 – Login page component
desktop/src/components/ProtectedRoute.tsx
desktop/src/client/authClient.ts         – login/me API calls
```

### Auth flow
1. App renders `<ProtectedRoute>` wrapper around all routes
2. On mount, reads `localStorage.getItem("devpod_token")`
3. Calls `GET /api/me` with the stored token to verify it is still valid
4. If 401 → clears token, redirects to `/login`
5. Login form POSTs to `/api/login`, stores token in localStorage

### All `fetch` calls updated
`WebCommand.run()` and `WebCommand.stream()` read
`AuthContext.token` and attach `Authorization: Bearer <token>` header.

The `vite-dev-api.ts` plugin validates the token in dev mode using the same
`/api/me` endpoint (passes through to Go backend if running, no-ops in pure
dev mode).

### Routes updated (`routes.tsx`)
```
/login        → <LoginPage>  (public)
/             → <ProtectedRoute> wrapping all existing routes
/admin/*      → <ProtectedRoute requiredRole="admin">
```

### `AuthContext` interface
```ts
type AuthContextValue = {
  token: string | null
  user: UserPublic | null
  isAdmin: boolean
  login(username: string, password: string): Promise<void>
  logout(): void
  changePassword(current: string, next: string): Promise<void>
}
```

### Tests
- `AuthContext.test.tsx` – login sets token, logout clears it, 401 triggers redirect
- `ProtectedRoute.test.tsx` – unauthenticated renders redirect, authenticated renders children
- `authClient.test.ts` – login/me fetch calls send correct headers

---

## Phase 8 – Frontend: Admin Panel (`/admin`)

### New views
```
desktop/src/views/Admin/
  AdminLayout.tsx          – shared sidebar (Users | Logs | Branding)
  Users/
    UserList.tsx           – table: username, role, provider count, actions
    UserCreateModal.tsx    – create user form
    UserEditModal.tsx      – change role, dockerHost, password reset
    UserProviders.tsx      – checkboxes for allowed providers
  Logs/
    OperationLogs.tsx      – filterable, paginated log table
  Branding/
    BrandingSettings.tsx   – form: app name, logo, colors, links
```

### Sidebar additions (`OSSApp.tsx`)
Add "Admin" link (shown only when `isAdmin === true`) between Settings and the
bottom status bar.

### Provider list in user edit
`UserProviders.tsx` fetches `GET /api/providers` (the full list) and
`GET /api/admin/users/{username}/providers` (the allowlist), renders a
checkbox list. On submit calls `PUT /api/admin/users/{username}/providers`.

### Tests
- `UserList.test.tsx`      – renders users, delete/edit buttons fire correct API calls
- `UserCreateModal.test.tsx` – form validation, submit creates user
- `UserProviders.test.tsx`  – checked/unchecked state, save PUT call
- `OperationLogs.test.tsx`  – pagination, filter UI
- `BrandingSettings.test.tsx` – fields pre-filled from API, save PUT call

---

## Phase 9 – Frontend: Branding Integration

### Changes
- `OSSApp.tsx` – loads `GET /api/branding` on startup via `useBranding()` hook
- Logo: if `logo_url` set, replace the DevPod icon in the sidebar header
- App name: `document.title` and sidebar label come from `app_name`
- Provider download link: used in the "Add Provider" flow where a binary
  download link is shown; falls back to the default GitHub releases URL
- Primary color: injected as a CSS custom property `--brand-primary` so
  Chakra UI tokens can reference it

### `useBranding()` hook
```ts
// fetched once on app start, cached in context
const { appName, logoUrl, faviconUrl, providerDownloadUrl, primaryColor } = useBranding()
```

### Tests
- `useBranding.test.ts` – fetches /api/branding, returns defaults when empty
- `BrandingLogo.test.tsx` – renders custom logo when logoUrl set, default icon when not

---

## Implementation Order & Milestones

| Milestone | Phases | Deliverable |
|-----------|--------|-------------|
| M1 – Data layer | 1 | SQLite DB, migrated from JSON, all unit tests pass |
| M2 – Secure backend | 2, 3 | All API handlers auth-gated, RBAC enforced, tests pass |
| M3 – Observability | 4, 5 | Audit log populated, branding API live |
| M4 – Mock provider | 6 | `devpod-provider-mock` binary, provider lifecycle tests |
| M5 – Secure frontend | 7 | Login page, token propagation, protected routes |
| M6 – Admin panel | 8 | Full admin UI with user/provider/log/branding management |
| M7 – Branding UI | 9 | Dynamic logo, app name, colors from admin settings |

Each milestone ends with `go test ./cmd/web/...` and `yarn test` both green.

---

## File Change Summary

### New Go files
```
cmd/web/db/db.go
cmd/web/db/schema.go
cmd/web/db/users.go
cmd/web/db/permissions.go
cmd/web/db/oplog.go
cmd/web/db/branding.go
cmd/web/db/db_test.go
cmd/web/rbac.go              – provider RBAC enforcement helpers
cmd/web/oplog_middleware.go  – write op-log rows around command handlers
cmd/web/branding_handlers.go – GET /api/branding, PUT /api/admin/branding
cmd/web/provider_perm_handlers.go
cmd/devpod-provider-mock/main.go
cmd/devpod-provider-mock/workspace.go
```

### Modified Go files
```
cmd/web/web.go     – wire auth, RBAC, op-log, branding endpoints
cmd/web/auth.go    – swap UserStore to DB-backed implementation
cmd/web/web_test.go – extend with auth / RBAC / log tests
```

### New TypeScript files
```
desktop/src/contexts/AuthContext.tsx
desktop/src/client/authClient.ts
desktop/src/views/Login/LoginPage.tsx
desktop/src/views/Login/index.ts
desktop/src/views/Admin/AdminLayout.tsx
desktop/src/views/Admin/Users/UserList.tsx
desktop/src/views/Admin/Users/UserCreateModal.tsx
desktop/src/views/Admin/Users/UserEditModal.tsx
desktop/src/views/Admin/Users/UserProviders.tsx
desktop/src/views/Admin/Logs/OperationLogs.tsx
desktop/src/views/Admin/Branding/BrandingSettings.tsx
desktop/src/contexts/BrandingContext.tsx
desktop/src/hooks/useBranding.ts
desktop/src/components/ProtectedRoute.tsx
```

### Modified TypeScript files
```
desktop/src/routes.tsx          – /login, /admin/*, ProtectedRoute
desktop/src/routes.constants.ts – Admin route constants
desktop/src/App/OSSApp.tsx      – Admin nav link, BrandingProvider
desktop/src/client/command.ts   – attach Authorization header
desktop/src/client/webClient/command.ts – same
desktop/vite-dev-api.ts         – pass auth header through to commands
```
