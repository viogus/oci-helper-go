# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Local build (static binary, ~12MB)
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

No test suite yet. No lint config. Module: `github.com/viogus/oci-helper-go`, Go 1.26.

## Architecture

Single binary. `net/http` standard library, no framework. One `http.ServeMux` serves both REST API and embedded frontend SPA.

```
cmd/server/main.go            # Entry: config.Load → db.New → handler.New → ListenAndServe
                                Healthcheck mode: ./oci-helper health → GET /api/config
internal/
  config/config.go             # env → Config struct; auto-generates password if unset
  db/
    sqlite.go                  # modernc.org/sqlite (pure Go, WAL mode, busy_timeout=5s, max 1 conn)
    models.go                  # Tenant/Instance/Task/AuditLog/ConfigKV structs
    queries.go                 # Raw SQL CRUD (no ORM)
  oci/client.go                # OCI Go SDK v65 wrapper: compute/vcn/identity/blockstorage clients
  auth/auth.go                 # bcrypt password + base64-encoded session cookie, HttpOnly, 24h TTL
  handler/
    handler.go                 # REST handlers + //go:embed dist/* for SPA frontend
    dist/index.html            # Single-file SPA (embedded into binary)
```

### Key design decisions

- **No CGO**: `CGO_ENABLED=0`, `modernc.org/sqlite` (pure Go SQLite), `FROM scratch`. Binary is fully static.
- **SQLite WAL mode, single connection**: `?_journal=WAL&_busy_timeout=5000` with `SetMaxOpenConns(1)`. WAL for concurrent reads, single writer avoids SQLite busy errors.
- **Auth middleware pattern**: `withAuth(f)` wraps handlers, checks session cookie. Login uses HTTP Basic Auth to set cookie. No JWT — just base64-encoded JSON session with TTL.
- **OCI client per call**: `oci.NewClient(tenant)` creates fresh OCI SDK clients on each sync. Not pooled or cached. Tenant stores key file path in DB.
- **Frontend embedding**: `//go:embed all:dist/*` embeds SPA into binary. `fs.Sub` strips `dist/` prefix. Served as `/` catch-all behind API routes.
- **Instance IDs are composite**: `tenantID:ocid` format. Upserted on sync (INSERT ON CONFLICT DO UPDATE).

### Packages

| Package | Import | Purpose |
|---------|--------|---------|
| `github.com/oracle/oci-go-sdk/v65` | `oci/client.go` | Oracle Cloud API (compute, network, identity, blockstorage) |
| `modernc.org/sqlite` | `db/sqlite.go` | Pure-Go SQLite driver |
| `golang.org/x/crypto` | `auth/auth.go` | bcrypt |

### Routes

API routes registered first in `routes()`, then `http.FileServer` as catch-all for SPA:

| Method | Path | Auth | Handler |
|--------|------|:---:|---------|
| POST | `/api/login` | Basic | `handleLogin` |
| POST | `/api/logout` | — | `handleLogout` |
| GET | `/api/config` | Session | `handleConfig` |
| GET/POST | `/api/tenants` | Session | `handleTenants` |
| GET/DELETE | `/api/tenants/:id` | Session | `handleTenantByID` |
| GET | `/api/instances` | Session | `handleInstances` (query: `tenant_id`) |
| GET | `/api/tasks` | Session | `handleTasks` |
| GET | `/api/audit` | Session | `handleAudit` |
| POST | `/api/sync/:tenantId` | Session | `handleSync` |

### Docker

- **Build stage**: `golang:1.26-alpine`, downloads modules, builds static binary with `CGO_ENABLED=0`
- **Run stage**: `FROM scratch`, copies binary + ca-certificates + `/etc/passwd`
- **User**: `nobody` (UID 65534)
- **Image**: ghcr.io/viogus/oci-helper-go (amd64 + arm64, built by GitHub Actions on push to main)
- **docker-compose.yml**: memory limit 128M, healthcheck via `oci-helper health` command

### Environment variables

See README.md for full table. Key: `PORT` (8818), `OCI_USERNAME` (admin), `OCI_PASSWORD` (auto-generated if unset, printed to stderr), `OCI_DB_PATH` (`/app/oci-helper/oci-helper.db`), `OCI_KEYS_DIR` (`/app/oci-helper/keys`).
