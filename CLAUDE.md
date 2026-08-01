# AGENTS.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Local build (static binary, ~13MB)
CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server

# Run locally (needs port, credentials, data dir)
PORT=8818 OCI_USERNAME=admin OCI_PASSWORD=test OCI_DB_PATH=./data/oci.db go run ./cmd/server

# Docker build (multi-stage, FROM scratch)
docker build -t oci-helper .

# Multi-arch build
docker buildx build --platform linux/amd64,linux/arm64 -t oci-helper .

# Healthcheck (for Docker HEALTHCHECK)
./oci-helper health
```

Tests exist for `internal/handler` and `internal/system` (table-driven, `testing` stdlib):

```bash
go test ./...            # all packages
go test -race ./...      # with race detector
```

No lint config. Module: `github.com/viogus/oci-helper-go`, Go 1.26.

## Architecture

Single binary. `net/http` standard library, no framework. One `http.ServeMux` serves both REST API and embedded frontend SPA.

```
cmd/server/main.go            # Entry: config.Load → db.New → handler.New → ListenAndServe
                                Healthcheck mode: ./oci-helper health → GET /api/health
internal/
  config/config.go             # env → Config struct; auto-generates password if unset
  db/
    sqlite.go                  # modernc.org/sqlite (pure Go, WAL mode, busy_timeout=5s, max 1 conn)
    models.go                  # Tenant/Instance/Task/AuditLog/ConfigKV/CfCfg/IpData/SSHKey/User/... structs
    migrate.go                 # versioned schema migrations (v1..v9)
    queries.go                 # Raw SQL CRUD (no ORM)
  oci/client.go                # OCI Go SDK v65 wrapper: compute/vcn/identity/blockstorage + more
    client_create.go           # instance creation helpers
    client_domain.go           # Identity Domains SCIM fallback
    client_subscription.go     # OSP Gateway subscription
  auth/auth.go                 # bcrypt password + base64-encoded session cookie, HttpOnly, 24h TTL
  ai/assistant.go              # SiliconFlow OpenAI-compatible chat client + DuckDuckGo search
  system/metrics.go            # host CPU/mem/disk/network metrics (gopsutil, pure Go)
  cloudflare/client.go         # Cloudflare DNS API client
  dingtalk/bot.go              # DingTalk webhook notifications
  geoip/geoip.go               # IP geolocation via ip-api.com
  task/                        # background task definitions
  telegram/bot.go              # minimal native Telegram Bot API client
  middleware/                  # request logging / statusWriter helpers
  handler/
    handler.go                 # Server struct, route registration, auth, SSE, audit
    handler_*.go               # one file per feature area (instances, tenants, tgmenu, ...)
    dist/                      # embedded SPA frontend (//go:embed all:dist/*)
  handler/                     # tests: handler_*_test.go (16 files)
```

### Key design decisions

- **No CGO**: `CGO_ENABLED=0`, `modernc.org/sqlite` (pure Go SQLite), `FROM scratch`. Binary is fully static.
- **SQLite WAL mode, single connection**: `?_journal=WAL&_busy_timeout=5000` with `SetMaxOpenConns(1)`. WAL for concurrent reads, single writer avoids SQLite busy errors.
- **Auth middleware pattern**: `withAuth(f)` wraps handlers, checks session cookie. Login uses HTTP Basic Auth to set cookie. No JWT — just base64-encoded JSON session with TTL. CSRF token required on state-changing methods. Sessions invalidated server-side on logout via version bump.
- **MFA cache**: `mfa_enabled`/`mfa_secret` are cached in-memory (`s.mfaCache`); `refreshMFACache()` must be called after any direct config change (including backup restore).
- **OCI client per call**: `oci.NewClient(tenant)` creates fresh OCI SDK clients on each sync. Not pooled or cached. Tenant stores key file path in DB.
- **Frontend embedding**: `//go:embed all:dist/*` embeds SPA into binary. `fs.Sub` strips `dist/` prefix. Served as `/` catch-all behind API routes. Rebuilt in Docker from `frontend/`; `internal/handler/dist` is committed for local builds.
- **Instance IDs are composite**: `tenantID:ocid` format. Upserted on sync (INSERT ON CONFLICT DO UPDATE). Always strip via `bareOCID()` before OCI SDK calls; Telegram callbacks re-assemble via `tgInstID()` (OCIDs never contain `:`).
- **Host metrics**: `internal/system` uses gopsutil (pure Go on Linux; darwin static builds fall back to `top -l 1`). CPU% is tick-delta based; network rates are deltas between calls.
- **Telegram bot**: callback-data routing in `handler_tgmenu.go`/`handler_tgnew.go` splits on ALL `:` (not `SplitN`); case order matters — specific sub-actions must precede generic ones. Webhook guarded by `telegram_webhook_secret` AND optional `telegram_chat_id` allowlist. TG SSH uses TOFU host-key pinning shared with the web shell.
- **Audit**: every mutating web/TG action calls `s.audit(tenantID, action, detail, r)` (nil-safe `r` for TG-originated actions).

### Packages

| Package | Import | Purpose |
|---------|--------|---------|
| `github.com/oracle/oci-go-sdk/v65` | `oci/` | Oracle Cloud API (compute, network, identity, blockstorage, monitoring, limits, NLB, ospgateway, identitydomains) |
| `modernc.org/sqlite` | `db/sqlite.go` | Pure-Go SQLite driver |
| `golang.org/x/crypto` | `auth/auth.go` | bcrypt, ssh (shell/TG SSH) |
| `github.com/shirou/gopsutil/v3` | `system/metrics.go` | Host CPU/mem/disk/network metrics |
| `github.com/gorilla/websocket` | `handler/` (shell, logs WS) | WebSocket upgrade/bridge |

### Routes

API routes registered first in `routes()` (handler.go), then SPA catch-all. Notable groups (full list in README "API reference"):

| Method | Path | Auth | Handler |
|--------|------|:---:|---------|
| POST | `/api/login` / `/api/logout` | Basic / — | `handleLogin` / `handleLogout` |
| GET/POST | `/api/config` | Session | `handleConfig` |
| GET/POST | `/api/tenants` + `/api/tenants/` (by id) | Session | `handleTenants`, `handleTenantByID` |
| GET/POST | `/api/instances` + `/api/instances/` (actions) | Session | `handleInstances`, `handleInstanceAction` |
| POST | `/api/sync/{tenantId}` | Session | `handleSync` |
| GET | `/api/tasks`, `/api/audit`, `/api/system/metrics`, `/api/metrics` | Session | per-feature handlers |
| POST | `/api/ai/chat` (+ `/api/ai/chat/cache`) | Session | `handleAIChat` (SSE stream) |
| POST | `/api/telegram/webhook` | secret token | `handleTelegramWebhook` |
| POST | `/api/backup` / `/api/restore` | Session | `handleBackup` / `handleRestore` |
| GET | `/api/logs/ws`, `/api/shell/ws`, `/api/instances/vnc/proxy` | Session | WebSocket handlers |

### Docker

- **Build stage**: `node:22-alpine` builds frontend, then `golang:1.26-alpine` builds static binary with `CGO_ENABLED=0`
- **Run stage**: `FROM scratch`, copies binary + ca-certificates + `/etc/passwd`
- **User**: `nobody` (UID 65534)
- **Image**: ghcr.io/viogus/oci-helper-go (amd64 + arm64, built by GitHub Actions on push to main)
- **docker-compose.yml**: memory limit 128M, healthcheck via `oci-helper health` command
- **CI**: `.github/workflows/build.yml` — docker buildx multi-arch + push to ghcr.io (no test step; run `go test ./...` locally)

### Environment variables

See README.md for full table. Key: `PORT` (8818), `OCI_USERNAME` (admin), `OCI_PASSWORD` (auto-generated if unset, printed to stderr), `OCI_DB_PATH` (`/app/oci-helper/oci-helper.db`), `OCI_KEYS_DIR` (`/app/oci-helper/keys`), `OCI_SECURE_COOKIES` (default true; only effective behind TLS/HTTPS proxy).
