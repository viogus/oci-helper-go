# Feature Parity Design — Java → Go oci-helper

Date: 2026-06-30
Source: Gap analysis comparing `Yohann0617/oci-helper` (Java, Spring Boot 3 + Vue 2) vs `viogus/oci-helper-go` (Go 1.26, net/http + Vue 3)
Scope: 7 feature groups, ~25 new files, ~15 modified files
Tests: Backend-only, table-driven, `testing` stdlib

---

## Group 1: User Management (`/users`)

### Backend
Already complete. `/api/users` CRUD + `/api/users/{id}/{action}` (mfa, reset-password, delete).
No changes.

### Frontend — `Users.vue`
**Route:** `/users`
**Sidebar:** System submenu, after Tenants

**UI:**
- Table columns: Username, Email, Role (el-tag: primary=admin, info=user), Created At
- "Add User" button → dialog: username, password, email (optional), role select (admin/user)
- Row actions:
  - Reset Password: dialog with new-password input → `DELETE /api/users/{id}/reset-password` with body `{password}`
  - Clear MFA: confirm → `DELETE /api/users/{id}/mfa`
  - Delete: confirm → `DELETE /api/users/{id}` — prevent self-delete, prevent deleting last admin
- No edit dialog (admin resets, users change own password in Settings)

**Edge cases:**
- Can't delete own user (check against `auth.user.username`)
- Can't delete last admin user (fetch list, count admins, refuse if deleting only admin)
- Empty table: `el-empty` with "No users found"

### i18n
```
users.title, users.username, users.email, users.role, users.actions,
users.addUser, users.addTitle, users.password, users.resetPassword,
users.resetPasswordTitle, users.newPassword, users.clearMFA,
users.confirmClearMFA, users.confirmDelete, users.notFound,
users.cannotDeleteSelf, users.cannotDeleteLastAdmin
```

---

## Group 2: IP Pool (`/ip-pool`)

### Backend
Already complete. `/api/ip-data` CRUD + `load_oci` action + `/api/ip-data/{id}`.
No changes.

### Frontend — `IpPool.vue`
**Route:** `/ip-pool`
**Sidebar:** Network submenu

**UI:**
- Tenant selector (filters `GET /api/ip-data?tenant_id=X&type=Y`)
- Type tabs: Pool, Whitelist, Blacklist (maps to ipdata `type` field)
- Table: CIDR (monospace font), Label, Enabled (el-switch), Tenant ID
- "Add" button → dialog: CIDR (required), Label, Type (select), Enabled (switch)
- "Import from OCI" button → `POST /api/ip-data` with `{action: "load_oci", tenant_id}` → shows `ElMessage.success("Imported N IPs")`
- Row actions: Edit (PUT), Delete (confirm)
- Client-side pagination (page sizes 10/20/50)

**Edge cases:**
- Import from OCI with no instances → "No instances found with public IPs"
- Duplicate CIDR → server returns error, show in ElMessage
- Empty tenant → `el-empty` "Select a tenant to view IP pool"

### i18n
```
ipPool.title, ipPool.add, ipPool.importOci, ipPool.cidr, ipPool.label,
ipPool.type, ipPool.enabled, ipPool.pool, ipPool.whitelist, ipPool.blacklist,
ipPool.addTitle, ipPool.editTitle, ipPool.importSuccess, ipPool.noInstances,
ipPool.confirmDelete, ipPool.notFound
```

---

## Group 3: Instance Plans (`/instance-plans`)

### Backend
Already complete. `/api/instance-plans` CRUD + `/api/instance-plans/{id}`.
No changes.

### Frontend — `InstancePlans.vue`
**Route:** `/instance-plans`
**Sidebar:** Resources submenu

**UI:**
- Tenant selector (cascading — filters plans by tenant)
- Card grid layout (el-row + el-col, 3 cards per row)
- Each card displays:
  - Plan name (bold header)
  - Shape / OCPU / Memory GB / Boot GB
  - Image ID (truncated), Subnet ID (truncated), AD
  - "Use Plan" button → `router.push('/instances/create?plan_id=' + plan.id)`
  - "Edit" / "Delete" icon buttons
- "Add Plan" button → dialog with cascading selects:
  - Tenant → AD → Image → Shape → VCN → Subnet
  - Name input (required)
  - OCPU (number, default from shape), Memory GB (number), Boot Volume GB (number, min 50)
- Edit dialog: same form, pre-filled from plan

### Modified — `InstanceCreate.vue`
- On mount: check `route.query.plan_id`. If set:
  - Fetch `GET /api/instance-plans?tenant_id=X`, find matching plan
  - Pre-fill: tenantId, displayName, availabilityDomain, imageId, shape, subnetId, bootVolumeSizeGB, ocpus, memoryGB
  - Also pre-select VCN (need to find VCN containing the subnet) — or store VCN ID in plan
- "Load from Plan" el-select at top of form (optional convenience): fetches plans, on select → fills form

### i18n
```
instancePlans.title, instancePlans.add, instancePlans.usePlan,
instancePlans.name, instancePlans.shape, instancePlans.ocpu,
instancePlans.memoryGB, instancePlans.bootGB, instancePlans.image,
instancePlans.subnet, instancePlans.ad, instancePlans.actions,
instancePlans.addTitle, instancePlans.editTitle, instancePlans.confirmDelete,
instancePlans.notFound, instancePlans.loadFromPlan
```

---

## Group 4: Defense (`/defense`)

### Backend
Already complete. `/api/defense/enable`, `/api/defense/disable`, `/api/ip-blacklist`.
No changes.

### Frontend — `Defense.vue`
**Route:** `/defense`
**Sidebar:** Network submenu

**UI:**
- Tenant selector → VCN selector (cascading, `GET /api/vcns?tenant_id=X`)
- Status banner:
  - Defense active: green `el-alert` "Defense is ACTIVE — N CIDR(s) blocked"
  - Defense inactive: gray `el-alert` "Defense is INACTIVE"
- Enable section (shown when inactive):
  - CIDR textarea: one CIDR per line, placeholder "e.g. 1.2.3.4/32"
  - "Enable Defense" button → `POST /api/defense/enable` with `{tenant_id, vcn_id, blacklist: parsedCIDRs}`
- Disable section (shown when active):
  - "Disable Defense" button → confirm dialog → `POST /api/defense/disable` with `{tenant_id, vcn_id}`
- Blacklist table (shown when active):
  - Fetched from `GET /api/ip-blacklist?tenant_id=X`
  - Columns: CIDR, Created At
  - Read-only — modify via Enable/Disable cycle

**Edge cases:**
- No VCN selected → disable buttons
- Enable with empty CIDR list → ElMessage.warning
- Defense disabled but blacklist entries remain in DB (ORPHAN state) → show "Clean up orphaned blacklist entries" button

### i18n
```
defense.title, defense.status, defense.active, defense.inactive,
defense.enable, defense.disable, defense.cidrList, defense.cidrHint,
defense.confirmDisable, defense.confirmDisableTitle, defense.blacklist,
defense.blacklistEmpty, defense.noVcn, defense.orphaned, defense.cleanup
```

---

## Group 5: SSH Keys (`/ssh-keys`)

### Backend
Already complete. `/api/ssh/keys` CRUD + generate action + `/api/ssh/keys/{id}`.
No changes.

### Frontend — `SshKeys.vue`
**Route:** `/ssh-keys`
**Sidebar:** System submenu

**UI:**
- Table: Name, Fingerprint (truncated 24 chars + el-tooltip with full), Type (el-tag: RSA=primary, ED25519=success), Public Key (truncated, copy button), Created At
- "Generate Keypair" button → dialog:
  - Name input (required)
  - Type select: RSA 4096 / ED25519
  - On submit: `POST /api/ssh/keys?action=generate` → shows public key in readonly textarea
  - "Copy Public Key" button (uses `navigator.clipboard.writeText`)
  - Warning: "Save your private key now. It will not be shown again."
- "Upload PEM" button → file input → `POST /api/ssh/keys` with form-data
- Row actions: Delete (confirm dialog, warn if key may be in use)

**Edge cases:**
- Key name collision → server error, show ElMessage
- Delete key potentially referenced by tenant → warn "This key may be used by one or more tenants"

### i18n
```
sshKeys.title, sshKeys.generate, sshKeys.upload, sshKeys.name,
sshKeys.fingerprint, sshKeys.type, sshKeys.publicKey, sshKeys.actions,
sshKeys.generateTitle, sshKeys.copyKey, sshKeys.keyWarning,
sshKeys.confirmDelete, sshKeys.notFound
```

---

## Group 6: WebSocket Log Streaming

### Backend — NEW `handler_wslog.go`

**Route:** `GET /api/logs/ws` (exact path, withAuth)

**Handler:**
```go
func (s *Server) handleLogWS(w http.ResponseWriter, r *http.Request)
```

**Logic:**
1. Parse query: `tail` (int, default 100, max 2000)
2. `gorilla/websocket.Upgrader` upgrade (reuse same upgrader from shell handler or define new)
3. Open log file (`s.cfg.LogFile`), seek to end
4. Send initial batch: read last N bytes, split by newlines, send first N lines as JSON array:
   `{"type":"init","lines":["line1","line2",...]}`
5. Poll loop (every 500ms via `time.Ticker`):
   - `file.Stat()` check for truncation (size < lastPos → reset to 0)
   - `file.Seek(lastPos, io.SeekStart)` then `bufio.Scanner` for new lines
   - Each new line: `{"type":"line","data":"...","time":"2026-06-30T..."}`
   - Update `lastPos`
6. Exit on: WebSocket read error (client disconnected), context cancelled, ticker.Stop on defer
7. Pong handler for keepalive (30s read deadline)

**No new dependencies.** gorilla/websocket already in go.mod from Cloud Shell.

### Frontend — Modified `Logs.vue`

**Additions:**
- "Live" el-switch in header row (next to "Auto Refresh" button)
- When Live toggled on:
  - Open `new WebSocket("ws://" + location.host + "/api/logs/ws?tail=" + lineCount)`
  - `ws.onmessage`: parse JSON, push to `lines` array, cap at 5000 (shift oldest)
  - Auto-scroll: if user is within 50px of bottom, `nextTick(() => el.scrollTop = el.scrollHeight)`
  - Show green pulsing dot + "Live" indicator in header
- When Live toggled off: `ws.close()`, keep current lines
- `onBeforeUnmount`: close WS if open

**Edge cases:**
- WS connection failed → ElMessage.error, toggle off
- Log file rotated → server detects truncation, resets position, sends new `{"type":"reset"}` → frontend clears buffer
- Browser tab hidden → WS still receives, buffer fills; on tab visible, re-enable auto-scroll

### i18n
```
logs.live, logs.liveStarted, logs.liveStopped, logs.liveError, logs.fileRotated
```

---

## Group 7: Telegram Bot — Full Parity

### Backend — Modified `handler_tgmenu.go`

**New main keyboard layout (4 columns):**
```
Row 1: Instances | Tasks | Status | Help
Row 2: Defense | Blacklist | SSH Keys | Version
Row 3: Backup | Traffic | Volumes | Plans
Row 4: Logs | CheckAlive | Configs
```

**New callback routes parsed in `handleTGCallback`:**

| Callback Pattern | Handler | Description |
|-----------------|---------|-------------|
| `defense` | `tgDefenseMenu` | Show defense sub-keyboard: Enable / Disable / Status |
| `defense:enable` | `tgDefenseEnable` | Prompt for VCN select, then CIDR input. Call `/api/defense/enable` |
| `defense:disable` | `tgDefenseDisable` | Confirm message, call `/api/defense/disable` |
| `blacklist` | `tgBlacklistMenu` | Paginated blacklist list, keyboard: Add / Remove / Clear / Back |
| `blacklist:add` | `tgBlacklistAddPrompt` | Ask for CIDR, create IpData(type=deny) |
| `blacklist:remove:{id}` | `tgBlacklistRemove` | Remove IP data entry by ID |
| `blacklist:clear` | `tgBlacklistClear` | Confirm, then delete all deny-type IP data for tenants |
| `sshkeys` | `tgSSHKeysList` | Paginated SSH key list |
| `sshkeys:generate` | `tgSSHKeyGenerate` | Generate keypair, send public key as monospace message |
| `sshkeys:delete:{id}` | `tgSSHKeyDelete` | Confirm then delete |
| `backup` | `tgBackupTrigger` | Call backup service, send encrypted file? No — just trigger and report success/failure |
| `traffic` | `tgTrafficPrompt` | Show instance list, user picks instance → ask time range → query traffic → send summary |
| `traffic:{instId}` | `tgTrafficQuery` | Actually query traffic for instance with default 1h range |
| `volumes` | `tgVolumeList` | Paginated boot volume list across all tenants |
| `volumes:detail:{id}` | `tgVolumeDetail` | Volume details + action buttons: Resize |
| `plans` | `tgPlansList` | Paginated instance plans list |
| `plans:detail:{id}` | `tgPlanDetail` | Plan details + "Create Instance" button |
| `logs` | `tgLogTail` | Fetch last 20 lines from log file, send as monospace message |
| `version` | `tgVersionInfo` | Call update check API, send version + latest release info |
| `checkalive` | `tgCheckAlivePrompt` | Show instance list, user picks → TCP :22 check → report alive/dead |
| `checkalive:{id}` | `tgCheckAliveDo` | Execute check-alive for specific instance |
| `cfg:list` | `tgConfigList` | List tenant configs (paginated) |
| `cfg:select:{id}` | `tgConfigSelect` | Store selected config in TG session storage (future: per-user config context) |

**Implementation pattern (same as existing):**
```
func (s *Server) tgDefenseMenu(bot *tgbotapi.BotAPI, chatID int64, msgID int) {
    keyboard := buildDefenseKeyboard()  // 3 buttons: Enable, Disable, Back
    tgSend(bot, chatID, msgID, "🛡 Defense Mode\n\nBlock malicious IPs via security list rules.", keyboard)
}
```

**New helper:** `tgPaginatedList(items []T, page int, pageSize int, renderFn, backCallback string)` — generic pagination for any list type. Reduces boilerplate across all list handlers.

### i18n
Telegram messages are hardcoded English (matching existing pattern). No i18n needed for TG bot.

---

## Tests

### Strategy
- Table-driven Go tests using `testing` stdlib
- Each handler group gets a `*_test.go` file
- Patterns: `httptest.NewServer` + `:memory:` SQLite + seed data → HTTP request → assert

### Test Files

```
internal/handler/handler_users_test.go
  - TestHandleUsers_List
  - TestHandleUsers_Create
  - TestHandleUsers_Create_MissingUsername (error case)
  - TestHandleUsers_Create_Duplicate (error case)
  - TestHandleUserByID_Delete
  - TestHandleUserByID_ResetPassword
  - TestHandleUserByID_ClearMFA
  - TestHandleUserByID_InvalidID

internal/handler/handler_ipdata_test.go
  - TestHandleIpData_List
  - TestHandleIpData_List_ByType
  - TestHandleIpData_Create
  - TestHandleIpData_Create_MissingCIDR
  - TestHandleIpData_LoadOCI (seed instances with public IPs)
  - TestHandleIpDataByID_Update
  - TestHandleIpDataByID_Delete

internal/handler/handler_instanceplans_test.go
  - TestHandleInstancePlans_List
  - TestHandleInstancePlans_Create
  - TestHandleInstancePlans_List_ByTenant
  - TestHandleInstancePlanByID_Update
  - TestHandleInstancePlanByID_Delete

internal/handler/handler_defense_test.go
  - TestHandleDefense_Enable
  - TestHandleDefense_Disable
  - TestHandleDefense_AlreadyEnabled (error case)
  - TestHandleIPBlacklist_List
  - TestHandleIPBlacklist_ByTenant

internal/handler/handler_ssh_test.go
  - TestHandleSSHKeys_List
  - TestHandleSSHKeys_Generate
  - TestHandleSSHKeyByID_Delete
```

### Test Helpers (in `internal/handler/test_helpers_test.go`)

```go
func setupTestServer(t *testing.T) (*Server, *db.Store, func())
// Creates :memory: store, seeds schema, creates Server with test config
// Returns cleanup function (close DB)

func seedTenant(t *testing.T, store *db.Store) *db.Tenant
func seedInstance(t *testing.T, store *db.Store, tenantID int64) *db.Instance
func seedUser(t *testing.T, store *db.Store, username, password string) *db.User
func loginAndGetCookie(t *testing.T, server *Server) string
// Makes login request, returns session cookie
```

### Build Verification

```bash
# Frontend
cd frontend && npm run build && cd ..

# Backend binary
CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server

# Tests
go test -v -count=1 ./internal/handler/...

# Full verification
./oci-helper health
```

No CI config changes needed — GitHub Actions already builds on push to main.

---

## Implementation Order

Dependencies are minimal (each group is independent). Order by user impact:

| # | Group | New Files | Modified Files | Effort |
|---|-------|-----------|---------------|--------|
| 1 | IP Pool | 1 view | router, sidebar, i18n x2 | Small |
| 2 | User Management | 1 view | router, sidebar, i18n x2 | Small |
| 3 | Instance Plans | 1 view | router, sidebar, InstanceCreate.vue, i18n x2 | Medium |
| 4 | Defense | 1 view | router, sidebar, i18n x2 | Small |
| 5 | SSH Keys | 1 view | router, sidebar, i18n x2 | Small |
| 6 | WebSocket Logs | 1 handler | handler.go, Logs.vue, i18n x2 | Medium |
| 7 | Telegram Bot | 1 modified | handler_tgmenu.go (+ optional split) | Large |
| 8 | Tests | 7 test files | — | Medium |

---

## Non-Goals (explicitly excluded)

- OCI-native DNS management
- Load balancer management (NLB exists but just for 500Mbps)
- Block volumes (non-boot)
- Object storage
- IAM (compartments, groups, policies)
- Database services (ADB, MySQL, etc.)
- Kubernetes (OKE)
- Budget alerts
- Auto-update watcher container (Go has update check API, no auto-apply)
- VNC via embedded websockify (Java uses separate container; Go shows connection URL only)
- WebSocket metrics streaming (only logs in scope — metrics can follow same pattern later)
- Frontend unit tests (out of scope per decision B)
- TG session persistence across restarts
- Notification settings page (config already in Settings.vue; dedicated page unnecessary)
