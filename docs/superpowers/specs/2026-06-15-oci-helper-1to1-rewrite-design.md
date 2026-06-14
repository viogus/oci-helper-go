# oci-helper 1:1 Rewrite — Design Spec

**Reference:** [Yohann0617/oci-helper](https://github.com/Yohann0617/oci-helper) (Spring Boot + Vue.js)
**Target:** oci-helper-go (Go + Vue.js, single binary, FROM scratch)
**Date:** 2026-06-15

## 1. Approach

**Chosen: B — Vue.js SPA rewrite.** Match reference UI 1:1. Go backend extended with all missing APIs. Vite builds to `internal/handler/dist/`, `//go:embed` mechanism unchanged.

## 2. Architecture

```
oci-helper-go/
├── cmd/server/main.go                  # unchanged entry
├── internal/
│   ├── handler/
│   │   ├── handler.go                  # API routes (extended, ~2000 lines)
│   │   ├── handler_security.go         # security rule handlers
│   │   ├── handler_traffic.go          # traffic stats handlers
│   │   ├── handler_tasks.go            # batch-create + in-memory task handlers
│   │   ├── handler_instance.go         # instance mutation handlers
│   │   ├── backup.go                   # unchanged
│   │   ├── worker.go                   # extended: batch create background jobs
│   │   └── dist/                       # Vite build output (embedded, gitignored)
│   ├── db/                             # extended queries
│   ├── oci/
│   │   ├── client.go                   # extended: security rules, limits, traffic, load balancer
│   │   └── console.go                  # NEW: cloud shell / VNC helpers
│   └── ...
├── frontend/                           # NEW: Vue.js 3 + Vite SPA
│   ├── vite.config.js
│   ├── package.json
│   ├── index.html
│   ├── public/
│   └── src/
│       ├── main.js                     # Vue app entry, plugins
│       ├── App.vue                     # root layout (sidebar + topbar + router-view)
│       ├── router/index.js             # Vue Router (20 routes)
│       ├── api/                        # axios wrapper, per-resource modules
│       │   ├── index.js                # base axios instance + interceptors
│       │   ├── auth.js
│       │   ├── tenants.js
│       │   ├── instances.js
│       │   ├── securityRules.js
│       │   ├── traffic.js
│       │   ├── tasks.js
│       │   ├── cloudflare.js
│       │   └── settings.js
│       ├── stores/                     # Pinia stores
│       │   ├── auth.js
│       │   ├── tenants.js
│       │   └── app.js
│       ├── views/                      # 17 page components
│       │   ├── Login.vue
│       │   ├── Home.vue                # dashboard + server map
│       │   ├── Tenants.vue             # tenant CRUD + batch upload
│       │   ├── Instances.vue           # instance list (paginated, fuzzy search)
│       │   ├── InstanceCreate.vue      # single instance creation
│       │   ├── InstanceBatchCreate.vue # 抢机 batch config
│       │   ├── CreateTasks.vue         # batch task management
│       │   ├── SecurityRules.vue       # security rules + open all ports
│       │   ├── Traffic.vue             # traffic graphs
│       │   ├── Limits.vue              # quota / limits query
│       │   ├── PublicIPs.vue           # public IPs + reserve + replace
│       │   ├── BootVolumes.vue         # boot volumes + resize / shrink
│       │   ├── Cloudflare.vue          # DNS management
│       │   ├── AiChat.vue              # AI assistant
│       │   ├── Backup.vue              # encrypted backup / restore
│       │   ├── Logs.vue                # real-time log viewer
│       │   ├── InMemoryTasks.vue       # change IP / update cfg tasks
│       │   ├── Settings.vue            # system config + notifications
│       │   ├── VncConsole.vue          # Cloud Shell / VNC
│       │   └── IpInfo.vue              # public IP info page
│       └── components/                 # shared components
│           ├── AppLayout.vue           # sidebar + topbar shell
│           ├── Pagination.vue          # reusable paginator
│           ├── SearchFilter.vue        # fuzzy search bar
│           └── ...
├── Dockerfile                          # add Node build stage
└── Makefile                            # frontend build → Go build
```

### Build Flow

```
npm run build              # Vite: frontend/ → frontend/dist/
cp -r frontend/dist/* internal/handler/dist/
CGO_ENABLED=0 go build     # embeds dist/, single binary
```

### Docker Multi-Stage

```
Stage 1 (node:22-alpine): npm ci && npm run build
Stage 2 (golang:1.26-alpine): go build → static binary
Stage 3 (scratch): binary + ca-certs + /etc/passwd
```

## 3. Frontend Design

### Technology Stack

| Layer | Choice | Rationale |
|---|---|---|
| Framework | Vue 3 (Composition API) | Matches reference; `<script setup>` for terseness |
| Router | Vue Router 4 | Hash mode (no server-side routing needed) |
| State | Pinia | Lightweight, official Vue 3 store |
| UI Library | Element Plus | Matches reference's polished component set |
| Charts | ECharts (via vue-echarts) | Traffic graphs, cost pie charts |
| HTTP | axios | Interceptors for auth, error handling |
| Build | Vite 5 | Fast HMR, tree-shaking, code-splitting |
| Icons | @element-plus/icons-vue | Consistent with Element Plus |

### Page Mapping (Reference → Ours)

| Reference Page | JS Bundle | Our View | Description |
|---|---|---|---|
| UserLogin | `UserLogin.BxCGQCUq.js` | `Login.vue` | Username/password + MFA + Google OAuth |
| OciHome | `OciHome.DRDdMRhZ.js` | `Home.vue` | Global server map, stats cards |
| OciUser | `OciUser.BxCn5btE.js` | `Tenants.vue` | Tenant list + add/edit/delete |
| OciCfgTab | `OciCfgTab.BSkZt4_F.js` | N/A (merged into Tenants) | Tenant config form |
| OciCreateInstance | `OciCreateInstance.CQpubMSo.js` | `InstanceCreate.vue` | Single instance creation form |
| OciCreateInstanceBatch | `OciCreateInstanceBatch.wkdiCX2u.js` | `InstanceBatchCreate.vue` | Multi-tenant batch creation config |
| OciCreateTask | `OciCreateTask.CilBxvp9.js` | `CreateTasks.vue` | Batch task list + stop/pause/resume |
| SecurityRuleList | `SecurityRuleList.whLtWPjk.js` | `SecurityRules.vue` | Security rules + open-all-ports |
| UserDashboard | `UserDashboard.DKCu5xnV.js` | N/A (scattered) | Dashboard stats → merged into Home |
| OciUpdateInstanceCfg | `OciUpdateInstanceCfg.DTnG6kZN.js` | N/A (inline dialogs) | Change shape/volume → inline in Instances |
| OciLog | `OciLog.BXQ7Sdxi.js` | `Logs.vue` | Real-time server log viewer |
| OciSysCfg | `OciSysCfg.BnIFH7Ot.js` | `Settings.vue` | System configuration |
| IpDataView | `IpDataView.DY2pNqxB.js` | `IpInfo.vue` | Public IP info display |
| AiChat | `AiChat.Db4xCcQ9.js` | `AiChat.vue` | SiliconFlow AI chat |
| GoogleCallback | `GoogleCallback.D1nOsGwa.js` | N/A (inline in Login) | OAuth callback handler |
| N/A | — | `Traffic.vue` | Traffic graphs (new) |
| N/A | — | `Limits.vue` | Quota/limits query (new) |
| N/A | — | `InMemoryTasks.vue` | Change IP / update cfg recurring tasks |
| N/A | — | `VncConsole.vue` | Cloud Shell / VNC |

### Key UI Patterns (Matching Reference)

- **Sidebar navigation:** Dark themed, collapsible, icons + labels. Page groups: Home, Resources (Instances, IPs, Volumes), Network (Security Rules, Traffic, DNS), Tools (AI Chat, Backup, Logs), Tasks (Create Tasks, Memory Tasks), Settings
- **Pagination + Fuzzy Search:** Every list page. `el-table` + `el-pagination` + `el-input` search bar. Search matched against instance name, OCID, IP, region.
- **Dark mode:** `el-switch` toggle in topbar. Persisted to localStorage. Applied via `document.documentElement.classList`.
- **Batch selection:** Checkboxes in tables for batch operations (start, stop, terminate, check alive).
- **In-Memory task panels:** Dedicated page showing change-IP and update-CPU recurring tasks with pause/resume/delete actions, attempt counter, status badges.
- **Create task management:** Shows all batch-create tasks with status (running/paused/completed), instance count, progress. Stop/pause/resume/edit per task.

## 4. Backend API Surface

### New Endpoints (~25)

**Instance mutations:**
```
POST /api/instances/change-shape       {tenant_id, instance_id, shape?, ocpus?, memory_gb?}
POST /api/instances/change-boot-volume {tenant_id, instance_id, size_gb}
POST /api/instances/attach-ipv6        {tenant_id, instance_id}
POST /api/instances/update-name        {tenant_id, instance_id, name}
POST /api/instances/change-ip          {tenant_id, instance_id, cidr_list?}
POST /api/instances/check-alive        {tenant_id, instance_id}
POST /api/instances/check-alive-batch  {tenant_id, instance_ids[]}
POST /api/instances/auto-rescue        {tenant_id, instance_id}
POST /api/instances/one-click-500m     {tenant_id, instance_id}   # enable
POST /api/instances/one-click-close-500m {tenant_id, instance_id}
POST /api/instances/update-shape       {tenant_id, instance_id, shape}
```

**Security rules:**
```
POST /api/security-rules               {tenant_id, vcn_id, keyword?, page?, size?}
POST /api/security-rules/add-ingress   {tenant_id, vcn_id, protocol, port, source}
POST /api/security-rules/add-egress    {tenant_id, vcn_id, protocol, port, dest}
POST /api/security-rules/remove        {tenant_id, rule_ids[]}
POST /api/security-rules/release       {tenant_id, vcn_id}
```

**Traffic & monitoring:**
```
POST /api/traffic                      {tenant_id, instance_id, vnic_id?, start_time, end_time}
POST /api/limits                       {tenant_id, service_name?}
GET  /api/logs                         ?tail=100
GET  /api/ip-info                      # public IP info (no-auth)
```

**Batch create tasks (persisted in DB):**
```
POST /api/instances/batch-create       {tenant_ids[], instances_per_tenant, region, shape, image_id, ...}
GET  /api/create-tasks                 ?page=&size=&keyword=
POST /api/create-tasks/stop            {task_ids[]}
POST /api/create-tasks/pause           {task_ids[]}
POST /api/create-tasks/resume          {task_ids[]}
POST /api/create-tasks/delete          {task_ids[]}
POST /api/create-tasks/update          {task_id, new_params}
```

**In-memory recurring tasks:**
```
GET  /api/mem-tasks/change-ip          # list change-IP tasks
POST /api/mem-tasks/change-ip/add      # add recurring change-IP task
POST /api/mem-tasks/change-ip/pause    {task_ids[]}
POST /api/mem-tasks/change-ip/resume   {task_ids[]}
POST /api/mem-tasks/change-ip/delete   {task_ids[]}
GET  /api/mem-tasks/update-cfg         # list update-config tasks
POST /api/mem-tasks/update-cfg/add     # add recurring update-config task
POST /api/mem-tasks/update-cfg/pause   {task_ids[]}
POST /api/mem-tasks/update-cfg/resume  {task_ids[]}
POST /api/mem-tasks/update-cfg/delete  {task_ids[]}
```

### Modified Endpoints

- `GET /api/instances` — add `keyword` query param for fuzzy search
- `GET /api/tenants` — add `keyword` query param for fuzzy search
- `GET /api/tasks` — add pagination params `page`, `size`

### OCI Client Extensions (`internal/oci/client.go`)

New methods needed:
- `ListSecurityRules(ctx, compartmentID, vcnID) ([]core.SecurityRule, error)`
- `AddNetworkSecurityGroupRules(...)`
- `RemoveNetworkSecurityGroupRules(...)`
- `GetInstanceVNIC(ctx, instanceID) ([]core.Vnic, error)`
- `GetVNICTtraffic(ctx, vnicID, start, end) ([]monitoring.MetricData, error)`
- `GetLimits(ctx, serviceName) ([]limits.LimitValue, error)`
- `UpdateInstance(ctx, instanceID, shape, ocpus, memory) error`
- `UpdateBootVolume(ctx, volumeID, sizeGB) error`
- `CreateIPv6(ctx, vnicID) (string, error)`
- `GetPublicIPInfo(ctx) (string, error)`
- `CreateNetworkLoadBalancer(ctx, ...) error`  // for one-click 500M
- `DeleteNetworkLoadBalancer(ctx, ...) error`
- `GetInstanceConsoleConnection(ctx, instanceID) (string, error)`

## 5. Database Changes

No schema changes. Existing tables are sufficient:

| Feature | Storage |
|---|---|
| Batch create tasks | `tasks` table (type=`batch_create`, payload=config) |
| In-memory tasks | In-memory `sync.Map` (does not survive restart, matches reference behavior) |
| Notification settings | `config` table (notify_tg_token, notify_dingtalk_url, etc.) |
| Paginated search | SQL `LIKE` on existing columns + `LIMIT/OFFSET` |

New SQL queries needed in `queries.go`:
- `ListTasksPaginated(keyword, page, size) ([]Task, int64, error)` — paginated with count
- `ListInstancesPaginated(tenantID, keyword, page, size) ([]Instance, int64, error)`
- `ListTenantsPaginated(keyword, page, size) ([]Tenant, int64, error)`
- `UpdateTaskPayload(id, payload)` — for editing batch create task config

## 6. Implementation Phases

### Phase 1: Foundation (files: ~20)
- Scaffold Vue project with Vite, router, Pinia, Element Plus
- API client layer (axios interceptors for auth/error)
- Login page (username/password, MFA, Google OAuth callback)
- App layout shell (collapsible dark sidebar + topbar)
- Router guards (redirect to login if unauthenticated)
- Go: extend handler.go route registration for all new endpoints (stub implementations for phase 2+)

### Phase 2: Tenant & Instance Core (files: ~10)
- `Tenants.vue` — list (paginated, search), add/edit/delete, batch upload keys, OCI config paste
- `Instances.vue` — list (paginated, search, filter by tenant), actions (start/stop/reboot/terminate/soft)
- `InstanceCreate.vue` — create single instance form (AD, image, shape, VCN, subnet selectors)
- Instance detail drawer/modal: change shape, change boot volume, attach IPv6, update name
- Go: implement instance mutation handlers + OCI client methods

### Phase 3: Security & Traffic (files: ~8)
- `SecurityRules.vue` — paginated list, add ingress/egress, remove, one-click open all ports
- `Traffic.vue` — instance/VNIC selector, time range picker, ECharts line/area chart
- `Limits.vue` — service selector, quota table with usage bars
- One-click 500M: enable/disable buttons on AMD instances
- Go: implement security rule + traffic + limits handlers

### Phase 4: Batch Create (抢机) (files: ~6)
- `InstanceBatchCreate.vue` — multi-tenant selector, instance config form, submit
- `CreateTasks.vue` — task list (paginated, search), stop/pause/resume/delete/edit
- Go: extend worker.go for batch create background jobs, resumable task polling
- Go: implement batch create task CRUD handlers

### Phase 5: IP & Recurring Tasks (files: ~6)
- `PublicIPs.vue` — list (by tenant), reserve new, replace IP (CIDR list input)
- `BootVolumes.vue` — list, resize, shrink to 47GB
- `InMemoryTasks.vue` — change-IP tasks list, update-config tasks list, add/pause/resume/delete
- `Cloudflare.vue` — enrich with auto-update checkbox after IP change
- Go: implement change-IP retry loop, recurring task scheduler, boot volume handlers

### Phase 6: Monitoring & Settings (files: ~8)
- `Logs.vue` — log viewer with auto-scroll, tail dropdown
- `AiChat.vue` — chat box (preserve existing)
- `Backup.vue` — export/import with password (preserve existing)
- `Settings.vue` — notification tokens, system config, Google OAuth config
- Check alive: modal with per-instance TCP ping results
- Go: implement logs handler, notification config CRUD, check-alive

### Phase 7: Polish (files: ~5)
- `Home.vue` — dashboard cards (tenant count, instance count, task count), global server map (Leaflet.js)
- `VncConsole.vue` — iframe or WebSocket to Cloud Shell
- `IpInfo.vue` — simple page showing request IP info (no-auth endpoint)
- Dark mode toggle, mobile responsive adaptation
- Pagination + fuzzy search audit on all remaining list pages

## 7. Compatibility

- **Session cookie:** Unchanged. Existing sessions work after upgrade.
- **DB schema:** Unchanged. No migration needed.
- **Docker:** New image pull + restart. Bind mounts unchanged.
- **Environment variables:** No new required vars. Optional: `OCI_MAPBOX_TOKEN` for server map.

## 8. What Stays the Same

- Go backend architecture (`net/http`, no framework, single binary)
- SQLite WAL mode, single connection
- Auth (bcrypt + HMAC-signed session cookies, MFA TOTP)
- OCI SDK usage pattern (new client per call)
- Docker multi-stage with FROM scratch
- `//go:embed` for frontend embedding
- All existing API paths (no breaking changes)
- Backup/restore format
- Telegram bot webhook
