# oci-helper-go

OCI management panel. Go rewrite of [oci-helper](https://github.com/Yohann0617/oci-helper).

## Why the rewrite

The original Java (Spring Boot) + Vue 2 version was mature but heavy. Go makes it dramatically lighter:

| Metric | Java | Go | Improvement |
|--------|:---:|:--:|:-----------:|
| Docker image | ~80MB (JRE Alpine) | **~13MB** | 6× |
| Containers | 3 (app+watcher+websockify) | **1** | 3× |
| Memory | 512MB recommended | **128MB** | 4× |
| Startup | ~8s (JVM) | **<100ms** | 80× |
| Binary size | 80MB JAR + 80MB JRE | **12MB static** | 13× |
| Base image | eclipse-temurin:21-jre-alpine | **FROM scratch** | Zero deps |

Single Go binary, `FROM scratch`, `CGO_ENABLED=0`. No JVM, no middleware, no extra containers.

## Architecture

```
oci-helper-go/
├── cmd/server/main.go             # Entry point: HTTP server + healthcheck mode
├── internal/
│   ├── config/config.go           # Environment → Config struct
│   ├── db/
│   │   ├── models.go              # Data models (Tenant, Instance, Task, User, ...)
│   │   ├── sqlite.go              # SQLite connection + auto-migration (pure Go, no CGO)
│   │   ├── queries.go             # Full CRUD operations
│   │   └── migrate.go             # Schema versioning (v1 → v2)
│   ├── oci/client.go              # OCI Go SDK v65 wrapper (compute, VCN, identity,
│   │                              #   block storage, monitoring, limits, NLB)
│   ├── auth/auth.go               # bcrypt + AES-GCM session cookie + TOTP MFA
│   ├── cloudflare/client.go       # Cloudflare DNS API v4 client
│   ├── telegram/bot.go            # Telegram Bot API client
│   ├── dingtalk/bot.go            # DingTalk webhook bot client
│   ├── ai/assistant.go            # SiliconFlow AI chat client (OpenAI-compatible)
│   ├── i18n/i18n.go               # Chinese/English locale messages
│   ├── middleware/                 # HTTP middleware
│   └── handler/
│       ├── handler.go             # REST API routes + embedded SPA frontend
│       ├── worker.go              # Background task queue
│       ├── backup.go              # AES-256-GCM encrypted backup/restore
│       ├── handler_tenants.go     # Tenant CRUD
│       ├── handler_instances.go   # Instance operations
│       ├── handler_tasks.go       # Batch operations
│       ├── handler_bootvolumes.go # Boot volume operations
│       ├── handler_publicips.go   # Public IP management
│       ├── handler_security.go    # Security rules
│       ├── handler_traffic.go     # VNIC traffic monitoring
│       ├── handler_vcn.go         # VCN operations
│       ├── handler_cloudflare.go  # Cloudflare DNS integration
│       ├── handler_ssh.go         # SSH key management
│       ├── handler_ipdata.go      # IP data / CIDR management
│       ├── handler_instanceplans.go # Launch templates
│       ├── handler_memtasks.go    # In-memory recurring tasks
│       ├── handler_defense.go     # Security defense rules
│       ├── handler_misc.go        # Tasks, audit, updates, notifications
│       ├── handler_tgmenu.go      # Telegram bot inline menus
│       ├── handler_keys.go        # PEM key file management
│       ├── handler_users.go       # Multi-user management
│       └── dist/index.html        # Full SPA frontend (embedded in binary)
├── frontend/                      # Vue 3 + Element Plus SPA source
│   ├── index.html
│   ├── src/
│   │   ├── main.js
│   │   ├── App.vue
│   │   ├── api/                   # API client modules
│   │   ├── components/            # Reusable UI components
│   │   ├── views/                 # Page-level views
│   │   ├── stores/                # Pinia state stores
│   │   ├── router/                # Vue Router config
│   │   └── styles/                # Global styles
│   └── package.json
├── .github/workflows/build.yml    # CI: amd64 + arm64 → ghcr.io
├── Dockerfile                     # Multi-stage: Node → Go → FROM scratch
└── docker-compose.yml             # Single container, 128MB memory limit
```

### Tech stack

| Layer | Technology | Notes |
|-------|-----------|-------|
| Backend | Go 1.26 (stdlib `net/http`) | No framework |
| Database | SQLite (`modernc.org/sqlite`) | Pure Go, no CGO, single file |
| OCI SDK | `oci-go-sdk/v65` | Compute, VCN, Identity, Block Storage, Monitoring, Limits, NLB |
| Auth | bcrypt + AES-256-GCM session cookie | HttpOnly, 24h TTL, server-side invalidation |
| Frontend | Vue 3 + Element Plus + Vue Router + Pinia + ECharts | Built with Vite, embedded in binary |
| Deployment | `FROM scratch` | Binary + ca-certificates only, ~13MB |

## Features

- [x] Web panel login (bcrypt + session cookie + MFA TOTP)
- [x] Google OAuth login
- [x] Multi-user management (create, set password, roles)
- [x] Multi-tenant OCI configuration management (CRUD)
- [x] Instance sync: pull OCI instances into local DB
- [x] Instance lifecycle: start, stop, reboot, terminate
- [x] Instance creation with image/shape/subnet/AD selection
- [x] Instance shape modification (resize OCPUs/memory)
- [x] Instance rename and IP change (ephemeral IP replacement)
- [x] Batch instance start (background task queue with progress)
- [x] Batch instance creation from saved plans
- [x] Public IP management (allocate, release, list)
- [x] Boot volume operations (resize, attach, detach, delete, VPU tuning)
- [x] VCN operations (list, delete, manage security lists)
- [x] Security rules: view, add, remove, batch update, defense blacklisting
- [x] Real-time CPU/memory/network metrics (OCI Monitoring)
- [x] VNIC traffic monitoring over time range
- [x] OCI Limits viewer
- [x] IPv6 attachment
- [x] One-click 500Mbps (NLB-based bandwidth boost)
- [x] Console connection management (VNC/SSH)
- [x] Auto-rescue mode
- [x] Instance aliveness check (TCP port probing)
- [x] Cloudflare DNS integration (sync DNS records with instance IPs)
- [x] CIDR-based IP data management
- [x] SSH key generation and management (RSA-4096)
- [x] Saved instance plans (launch templates)
- [x] In-memory recurring tasks (automatic IP rotation, config updates)
- [x] Telegram Bot with inline keyboard menus
- [x] DingTalk webhook notifications
- [x] AI assistant (SiliconFlow API, OpenAI-compatible, streaming)
- [x] Encrypted backup/restore (AES-256-GCM + PBKDF2)
- [x] Audit log with pagination and keyword search
- [x] Dual logging (stderr + file)
- [x] i18n (zh_CN / en), auto-detected from Accept-Language
- [x] Auto-update check and trigger
- [x] Docker single-container deploy (FROM scratch, user 65534)
- [x] CI/CD: push to main → build amd64+arm64 → push to ghcr.io
- [x] Docker HEALTHCHECK support (`./oci-helper health`)
- [x] Full SPA frontend (Vue 3 + Element Plus)

## Quick start

```bash
# Run locally (requires OCI API key and credentials)
CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server
OCI_DB_PATH=./data/oci.db OCI_KEYS_DIR=./data/keys ./oci-helper

# Or with Docker
docker run -d \
  --name oci-helper \
  -p 8818:8818 \
  -v oci-helper-data:/app/oci-helper \
  -e OCI_USERNAME=admin \
  -e OCI_PASSWORD=your-password \
  ghcr.io/viogus/oci-helper-go:latest
```

Open http://localhost:8818 and log in.

For local development, the frontend is at `frontend/` and can be served by Vite dev server:

```bash
cd frontend && npm run dev
```

Then run the Go backend separately. The Vite dev server proxies `/api/*` to the Go backend.

## Deployment

### docker-compose

```yaml
services:
  oci-helper:
    image: ghcr.io/viogus/oci-helper-go:latest
    container_name: oci-helper
    restart: unless-stopped
    ports:
      - "8818:8818"
    volumes:
      - oci-helper-data:/app/oci-helper
    environment:
      - OCI_USERNAME=admin
      - OCI_PASSWORD=change-me
    healthcheck:
      test: ["CMD", "/oci-helper", "health"]
      interval: 30s
      timeout: 5s
      retries: 3
    deploy:
      resources:
        limits:
          memory: 128M
volumes:
  oci-helper-data:
```

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8818` | HTTP listen port |
| `OCI_USERNAME` | `admin` | Panel login username |
| `OCI_PASSWORD` | random | Panel login password (auto-generated if unset, printed to stderr) |
| `OCI_DB_PATH` | `/app/oci-helper/oci-helper.db` | SQLite database path |
| `OCI_KEYS_DIR` | `/app/oci-helper/keys` | OCI API key PEM upload directory |
| `OCI_LOG_FILE` | `/app/oci-helper/oci-helper.log` | Log file path (also logged to stderr) |
| `OCI_LOG_LEVEL` | `info` | Log level |
| `OCI_MFA` | `false` | Enable TOTP MFA (`true` / `false`) |
| `OCI_MFA_SECRET` | — | Pre-configured TOTP secret |
| `GOOGLE_CLIENT_ID` | — | Google OAuth 2.0 client ID |
| `GOOGLE_CLIENT_SECRET` | — | Google OAuth 2.0 client secret |
| `GOOGLE_REDIRECT_URL` | — | Google OAuth redirect URI |

The following are configured through the web panel (stored in SQLite `config` table):
- Cloudflare API token (`cloudflare_token`)
- SiliconFlow API key (`siliconflow_key`) and model (`siliconflow_model`)
- Telegram Bot token (`telegram_token`)
- MFA TOTP secret (set up via `/api/mfa/setup`)
- AI search toggle (`ai_search_enabled`)

## API reference

All API routes return JSON. Most require a valid session cookie obtained via `/api/login`.

### Authentication

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| POST | `/api/login` | Basic | Login. Set `X-TOTP` header if MFA is enabled |
| POST | `/api/logout` | — | Logout (invalidates session) |
| GET | `/api/oauth/google/login` | — | Redirect to Google OAuth |
| GET | `/api/oauth/google/callback` | — | Google OAuth callback handler |

### Configuration

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET/POST | `/api/config` | ✓ | Get config / set a config key-value |

### MFA

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET/POST | `/api/mfa/setup` | ✓ | Generate TOTP secret and otpauth:// URI |
| POST | `/api/mfa/verify` | ✓ | Verify TOTP code and enable MFA |
| POST | `/api/mfa/disable` | ✓ | Disable MFA (requires valid TOTP code) |

### Tenants

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/tenants` | ✓ | List tenants (?keyword=&page=&size=) |
| POST | `/api/tenants` | ✓ | Create tenant |
| GET | `/api/tenants/{id}` | ✓ | Get tenant details |
| DELETE | `/api/tenants/{id}` | ✓ | Delete tenant |

### Instances

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/instances` | ✓ | List instances (?tenant_id=&keyword=&page=&size=) |
| POST | `/api/instances` | ✓ | Create a new instance |
| POST | `/api/instances/{id}/action` | ✓ | Instance action: start/stop/reboot/softstop/softreboot/terminate |
| POST | `/api/instances/batch-start` | ✓ | Batch start → creates background task |
| POST | `/api/instances/batch-create` | ✓ | Batch create instances across tenants |
| POST | `/api/instances/change-shape` | ✓ | Change instance shape/OCPUs/memory |
| POST | `/api/instances/change-boot-volume` | ✓ | Change boot volume (detach old, attach new) |
| POST | `/api/instances/attach-ipv6` | ✓ | Attach an IPv6 address |
| POST | `/api/instances/update-name` | ✓ | Rename instance |
| POST | `/api/instances/change-ip` | ✓ | Change ephemeral public IP (optional CIDR filter) |
| POST | `/api/instances/check-alive` | ✓ | TCP port aliveness check |
| POST | `/api/instances/one-click-500m` | ✓ | Create NLB for 500Mbps bandwidth boost |
| POST | `/api/instances/one-click-close-500m` | ✓ | Delete the NLB |
| POST | `/api/instances/auto-rescue` | ✓ | Auto-rescue mode operations |
| POST | `/api/instances/update-shape` | ✓ | Update shape (alias for change-shape) |
| GET | `/api/instances/vnc` | ✓ | Start VNC console connection |
| GET | `/api/instances/config-info` | ✓ | Get instance configuration info |
| POST | `/api/instances/update-password` | ✓ | Update instance OS password |

### Reference data

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/images` | ✓ | List images (?tenant_id=X&os=Oracle%20Linux) |
| GET | `/api/shapes` | ✓ | List shapes (?tenant_id=X&image_id=Y) |
| GET | `/api/vcns` | ✓ | List VCNs (?tenant_id=X&page=&size=) |
| DELETE | `/api/vcns/{id}` | ✓ | Delete a VCN |
| GET | `/api/subnets` | ✓ | List subnets (?tenant_id=X&vcn_id=Y) |
| GET | `/api/availability-domains` | ✓ | List ADs (?tenant_id=X) |

### Public IPs

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/public-ips` | ✓ | List public IPs (?tenant_id=X) |
| POST | `/api/public-ips` | ✓ | Create a reserved public IP |
| DELETE | `/api/public-ips/{id}` | ✓ | Release a public IP |

### Boot volumes

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/boot-volumes` | ✓ | List boot volumes (?tenant_id=X) |
| GET | `/api/boot-volumes/{id}` | ✓ | Get boot volume details |
| POST | `/api/boot-volumes/{id}/resize` | ✓ | Resize boot volume |
| POST | `/api/boot-volumes/{id}/attach` | ✓ | Attach boot volume to instance |
| POST | `/api/boot-volumes/{id}/detach` | ✓ | Detach boot volume |
| POST | `/api/boot-volumes/{id}/delete` | ✓ | Delete boot volume |

### Security rules

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| POST | `/api/security-rules` | ✓ | List/add/remove/batch-update security rules |
| POST | `/api/defense/enable` | ✓ | Enable defense rules (add blacklist to security list) |
| POST | `/api/defense/disable` | ✓ | Disable defense rules (release blocked ports) |
| GET/POST | `/api/ip-blacklist` | ✓ | IP blacklist management |

### Metrics & traffic

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/metrics` | ✓ | Real-time CPU/memory/network metrics |
| POST | `/api/traffic` | ✓ | VNIC traffic over time range |
| GET | `/api/limits` | ✓ | OCI service limits |
| GET | `/api/logs` | ✓ | System logs |

### Tasks & audit

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/tasks` | ✓ | List background tasks (?keyword=&page=&size=) |
| POST | `/api/create-tasks` | ✓ | Create custom tasks |
| GET | `/api/audit` | ✓ | Audit log (?keyword=&page=&size=) |

### Sync

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| POST | `/api/sync/{tenantId}` | ✓ | Sync instances from OCI to local DB |

### Cloudflare DNS

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/cloudflare/zones` | ✓ | List Cloudflare zones |
| GET | `/api/cloudflare/{zoneId}/records` | ✓ | List DNS records |
| POST | `/api/cloudflare/{zoneId}/records` | ✓ | Create DNS record |
| PATCH | `/api/cloudflare/{zoneId}/records/{recordId}` | ✓ | Update DNS record |
| DELETE | `/api/cloudflare/{zoneId}/records/{recordId}` | ✓ | Delete DNS record |
| POST | `/api/cloudflare/update-ip` | ✓ | Update DNS record IP for an instance |
| POST | `/api/cloudflare/oci-sync` | ✓ | Sync OCI instance IPs to Cloudflare DNS |

### Cloudflare configs

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET/POST | `/api/cloudflare/cfgs` | ✓ | List/create CF configurations |
| GET/PUT/DELETE | `/api/cloudflare/cfgs/{id}` | ✓ | Get/update/delete a CF config |

### SSH keys

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/ssh/keys` | ✓ | List SSH keys (?tenant_id=X) |
| POST | `/api/ssh/keys` | ✓ | Add SSH key or generate new keypair |
| DELETE | `/api/ssh/keys/{id}` | ✓ | Delete SSH key |

### Instance plans

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET/POST | `/api/instance-plans` | ✓ | List / create launch templates |
| PUT | `/api/instance-plans/{id}` | ✓ | Update instance plan |
| DELETE | `/api/instance-plans/{id}` | ✓ | Delete instance plan |

### IP data / CIDR management

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET/POST | `/api/ip-data` | ✓ | List / create IP data entries |
| PUT | `/api/ip-data/{id}` | ✓ | Update IP data |
| DELETE | `/api/ip-data/{id}` | ✓ | Delete IP data |

### Users (multi-user)

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/users` | ✓ | List all users |
| POST | `/api/users` | ✓ | Create user |
| PUT | `/api/users/{id}` | ✓ | Update user |
| DELETE | `/api/users/{id}` | ✓ | Delete user |

### PEM key management

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/keys` | ✓ | List uploaded PEM files |
| POST | `/api/keys` | ✓ | Upload PEM file(s) (multipart) |
| DELETE | `/api/keys/{name}` | ✓ | Delete PEM file |

### Backup & restore

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| POST | `/api/backup` | ✓ | Encrypted export (AES-256-GCM + PBKDF2) |
| POST | `/api/restore` | ✓ | Encrypted import and restore |

### AI assistant

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| POST | `/api/ai/chat` | ✓ | Chat with AI (?stream=true for SSE streaming) |

### Notifications

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| POST | `/api/dingtalk/notify` | ✓ | Send DingTalk notification |
| POST | `/api/dingtalk/test` | ✓ | Test DingTalk webhook |
| POST | `/api/notify/test` | ✓ | Test notification channel |
| POST | `/api/telegram/webhook` | — | Telegram Bot webhook endpoint |

### In-memory tasks

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET/POST | `/api/mem-tasks/change-ip` | ✓ | List/create IP rotation tasks |
| GET/POST | `/api/mem-tasks/update-cfg` | ✓ | List/create config update tasks |

### Shell / console

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/shell/{instanceId}` | ✓ | Get console connection info |

### System

| Method | Path | Auth | Description |
|--------|------|:---:|-------------|
| GET | `/api/update/check` | ✓ | Check for updates |
| POST | `/api/update/now` | ✓ | Trigger update |
| GET | `/api/ip-info` | — | Public IP info (no auth) |

### Error format

Errors return `{"error": "message"}` with an appropriate HTTP status code (400 for bad requests, 401 for auth failures, etc.).

## Build

```bash
# Local build (static binary, ~12MB)
CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server

# Build with frontend
cd frontend && npm run build && cd ..
cp -r frontend/dist internal/handler/dist/
CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server

# Docker build (includes frontend build in multi-stage)
docker build -t oci-helper .

# Multi-arch
docker buildx build --platform linux/amd64,linux/arm64 -t oci-helper .

# Healthcheck mode (used by Docker HEALTHCHECK)
./oci-helper health
```

## Development

```bash
# Terminal 1: Go backend
PORT=8818 OCI_DB_PATH=./data/oci.db OCI_KEYS_DIR=./data/keys go run ./cmd/server

# Terminal 2: Vue dev server (hot-reload)
cd frontend && npm run dev
```

The Vite dev server proxies `/api/*` to the Go backend on port 8818.

## Docker images

| Image | Description |
|-------|-------------|
| `ghcr.io/viogus/oci-helper-go:latest` | Latest main branch (amd64 + arm64) |
| `ghcr.io/viogus/oci-helper-go:sha-xxxxx` | Specific commit |

CI builds on push to `main`, on weekly schedule, and on tag pushes (`v*`).

## License

Apache-2.0 (same as original oci-helper).
