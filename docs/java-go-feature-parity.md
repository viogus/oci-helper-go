# Java (Y探长) vs Go (oci-helper-go) — Feature Parity Analysis

> Source: `Yohann0617/oci-helper` (Spring Boot + MyBatis-Plus + Vue 3)
> Target: `viogus/oci-helper-go` (Go net/http + SQLite + Vue 3)
> **Status: 100% parity** — all Java features implemented. Go adds 20+ extras.
> Date: 2026-07-01

---

## 1. Controllers & Endpoints Comparison

### 1.1 SysCfgController (12/12) → Go handler.go + handler_misc.go

| # | Java Endpoint | Go Equivalent |
|---|---------------|---------------|
| 1 | POST `/api/sys/login` | POST `/api/login` |
| 2 | POST `/api/sys/updateVersion` | GET `/api/update/check` + POST `/api/update/now` |
| 3 | POST `/api/sys/getEnableMfa` | GET `/api/config` (mfa_enabled) |
| 4 | POST `/api/sys/getSysCfg` | GET `/api/config` |
| 5 | POST `/api/sys/updateSysCfg` | POST `/api/config` |
| 6 | POST `/api/sys/sendMsg` | POST `/api/notify/test` |
| 7 | POST `/api/sys/checkMfaCode` | POST `/api/mfa/verify` |
| 8 | POST `/api/sys/backup` | POST `/api/backup` |
| 9 | POST `/api/sys/recover` | POST `/api/restore` |
| 10 | GET `/api/sys/glance` | GET `/api/glance` (+ cities IP geolocation map data) |
| 11 | POST `/api/sys/googleLogin` | GET `/api/oauth/google/login` + `/callback` |
| 12 | POST `/api/sys/getGoogleClientId` | GET `/api/config` (google_client_id) |

### 1.2 OciController (37/37) → Go handler_instances.go + handler_tasks.go + handler_memtasks.go + handler_tenants.go

| # | Java Endpoint | Go Equivalent |
|---|---------------|---------------|
| 1 | `/api/oci/userPage` | GET `/api/tenants` |
| 2 | `/api/oci/addCfg` | POST `/api/tenants` |
| 3 | `/api/oci/updateCfgName` | PUT `/api/tenants/{id}` |
| 4 | `/api/oci/updateCfgProxy` | POST `/api/tenants/{id}/proxy` |
| 5 | `/api/oci/uploadCfg` | POST `/api/tenants/upload` |
| 6 | `/api/oci/removeCfg` | DELETE `/api/tenants/{id}` |
| 7 | `/api/oci/createInstance` | POST `/api/instances` |
| 8 | `/api/oci/details` | GET `/api/tenants/{id}/info` |
| 9 | `/api/oci/changeIp` | POST `/api/instances/change-ip` |
| 10 | `/api/oci/stopCreate` | POST `/api/create-tasks/{id}/stop` |
| 11 | `/api/oci/stopChangeIp` | POST `/api/mem-tasks/change-ip` (action:delete) |
| 12 | `/api/oci/createTaskPage` | GET `/api/instance-plans` |
| 13 | `/api/oci/stopCreateBatch` | POST `/api/create-tasks` (action:stop) |
| 14 | `/api/oci/updateCreateTask` | PUT `/api/create-tasks/{id}` |
| 15 | `/api/oci/updateCreateTaskBatch` | POST `/api/create-tasks` (action:update batch) |
| 16 | `/api/oci/pauseCreateBatch` | POST `/api/create-tasks` (action:pause) |
| 17 | `/api/oci/resumeCreateBatch` | POST `/api/create-tasks` (action:resume) |
| 18 | `/api/oci/createInstanceBatch` | POST `/api/instances/batch-create` |
| 19 | `/api/oci/updateInstanceState` | POST `/api/instances/{id}` (start/stop/reboot/softstop/softreset) |
| 20 | `/api/oci/sendCaptcha` | POST `/api/captcha/send` |
| 21 | `/api/oci/terminateInstance` | POST `/api/instances/{id}` (terminate) |
| 22 | `/api/oci/releaseSecurityRule` | POST `/api/security-rules` (release) |
| 23 | `/api/oci/getInstanceCfgInfo` | POST `/api/instances/config-info` |
| 24 | `/api/oci/createIpv6` | POST `/api/instances/attach-ipv6` |
| 25 | `/api/oci/updateInstanceName` | POST `/api/instances/update-name` |
| 26 | `/api/oci/updateInstanceRootPassword` | POST `/api/instances/update-password` |
| 27 | `/api/oci/updateInstanceCfg` | POST `/api/instances/config-update` (direct) + `/api/mem-tasks/update-cfg` (retry loop) |
| 28 | `/api/oci/updateBootVolumeCfg` | PUT `/api/boot-volumes/{id}` |
| 29 | `/api/oci/checkAlive` | POST `/api/instances/check-alive` |
| 30 | `/api/oci/checkAliveBatch` | POST `/api/instances/check-alive-batch` |
| 31 | `/api/oci/refreshPlanTypeBatch` | POST `/api/tenants/{id}/refresh-plan-type` |
| 32 | `/api/oci/refreshCfgBatch` | POST `/api/sync/{tenantId}` |
| 33 | `/api/oci/startVnc` | POST `/api/instances/vnc` |
| 34 | `/api/oci/autoRescue` | POST `/api/instances/auto-rescue` |
| 35 | `/api/oci/oneClick500M` | POST `/api/instances/one-click-500m` |
| 36 | `/api/oci/oneClickClose500M` | POST `/api/instances/one-click-close-500m` |
| 37 | `/api/oci/updateInstanceShape` | POST `/api/instances/update-shape` |

### 1.3 TenantController (8/8) → Go handler_tenants.go

| # | Java | Go |
|---|------|----|
| 1 | `/api/tenant/tenantInfo` | GET `/api/tenants/{id}/info` |
| 2 | `/api/tenant/deleteUser` | POST `/api/tenants/{id}/users/delete` |
| 3 | `/api/tenant/deleteMfaDevice` | POST `/api/tenants/{id}/mfa/clear` |
| 4 | `/api/tenant/deleteApiKey` | POST `/api/tenants/{id}/api-keys/clear` |
| 5 | `/api/tenant/resetPassword` | POST `/api/tenants/{id}/users/reset-password` (+ Identity Domains SCIM fallback) |
| 6 | `/api/tenant/updateUserInfo` | POST `/api/tenants/{id}/users/update` |
| 7 | `/api/tenant/updatePwdEx` | POST `/api/tenants/{id}/password-policy` |
| 8 | `/api/tenant/updateNotificationRecipients` | PATCH `/api/tenants/{id}` (notify_tg/notify_dingtalk) |

### 1.4 — 1.13 All remaining controllers (100%)

| Controller | Endpoints | Status |
|------------|-----------|--------|
| TrafficStatisticsController | 4/4 | ✅ All match |
| CostAnalysisController | 1/1 | ✅ Match |
| LimitsController | 2/2 | ✅ All match |
| BootVolumeController | 3/3 | ✅ All match |
| SecurityRuleController | 5/5 | ✅ All match (incl. release, release_by_vcn, update_batch) |
| VcnController | 2/2 | ✅ All match |
| CloudflareController | 9/9 | ✅ All match |
| InMemoryTaskController | 10/10 | ✅ All match (add/page/pause/resume/delete × 2 types) |
| IpDataController | 5/5 | ✅ All match (incl. loadOciIpData) |
| AiChatController | 2/2 | ✅ All match (removeCache + stream) |

**Total: ~80 Java endpoints → ~80 Go equivalents = 100%**

---

## 2026-08-01 Gap Closure Update

The July 2026 "100%" claim predates a fresh audit against the Java source.
The following gaps found in that audit are now implemented:

- Recurring create-task subsystem (`create_tasks` table, scheduler,
  pause/resume/stop, restart-safe; auto-select shape/image/public subnet).
- Tenant API aliveness check + `account_status` (ACTIVE/INACTIVE),
  including `refreshCfgBatch` parity and daily broadcast.
- Instance root-password freeform tag update endpoint.
- 9-step auto-rescue flow (backup, detach/delete, AMD temp instance,
  boot-volume clone, reattach, notify).
- Change-IP Cloudflare DNS update (manual + in-memory recurring tasks).
- 500Mbps `ssh_port`/AMD validation and NAT cleanup on disable.
- AI chat server-side multi-turn sessions and Telegram AI chat.
- Full-table encrypted backup/restore including key files.
- Bulk `.ini/.txt` OCI config import.
- Extended security-rule editor (ICMP, stateless, description, port ranges).
- Global async OCI IP-data sync (type `oracle`).
- Actual update trigger flag instead of an instructions-only stub.
- Login IP blacklist persistence, startup/daily/version notifications,
  periodic log cleanup, and tenant detail NLB/CF lists.

Telegram bot: AI chat, MFA management, SSH key generation, recurring
create-task wizard, VNC instance selection (with `panel_url` web-console
links), boot-volume terminate/update, defense, blacklist, traffic, plans,
logs, and config lists are all covered. The Go rewrite intentionally uses the
embedded Web VNC/console proxy instead of spawning a host-side SSH tunnel like
the Java process did.

---

## 2026-08-02 Gap Closure Update (fresh audit vs Yohann0617/oci-helper @HEAD)

A fresh comparison against the Java source found and closed these gaps:

- **Host system metrics (资源监控)**: Java's `/metrics/{token}` WebSocket +
  `UserDashboard` page + TG `system_metrics` menu streamed host CPU/memory/disk/
  network. Go now has `GET /api/system/metrics` (pure-Go gopsutil; darwin
  static-build fallback via `top -l 1`), a new **System Metrics** frontend page
  (`/system-metrics`, sidebar System submenu, i18n en/zh-CN), and the TG
  **SysMetrics** menu item with a refresh button.
- **TG AI model selection**: `🧠 AI Model` menu mirrors Java's
  `ai_select_model` / `ai_set_model_*` (DeepSeek-R1 / DeepSeek-V3 / Qwen-2.5);
  per-chat model overrides the global `siliconflow_model` config.
- **TG instance terminate**: `instances:terminate` confirm flow with the
  Java `terminate_instances:true/false` preserve-boot-volume choice.
- **TG backup restore**: `💾 Backup` menu gained a restore flow (password →
  base64 payload) reusing the web restore pipeline (`restoreData`).
- **TG SSH management**: `/ssh_config host port user pwd` + `/ssh <cmd>`
  one-shot execution (30s timeout, interactive-command blocklist) and a
  `🖥 SSH` menu showing the saved connection with remove button. 30-min idle
  expiry like Java's `SshConnectionStorage`/`TelegramCleanupTask`.
- **TG defense full flow**: enable (tenant → VCN → CIDRs → confirm) and
  disable (tenant → VCN → confirm) now run inside Telegram instead of pointing
  at the web UI; shared `enableDefense`/`disableDefense` helpers back both the
  HTTP handlers and the bot.
- **TG `cancel` (关闭窗口)**: clears per-chat state and deletes the message.
- **TG callback parsing fix**: instance IDs are composite `tenantID:ocid`;
  callbacks are now split on all `:` and re-assembled (`tgInstID`), fixing
  broken instance detail/action/traffic/checkalive/vnc/volume flows that used
  `SplitN(data, ":", 4)`.
- New tests: `internal/system` (CPU delta math, top-regex, rounding) and
  `internal/handler/handler_tg_test.go` (composite-ID parsing, CIDR list,
  model mapping).

---

## 2026-08-02 Code Audit & Hardening Update

Two independent code reviews (concurrency/panic/error-handling audit + a
follow-up review of that commit) produced 16 + 5 fixes. Behavior-neutral,
robustness & security hardening:

**Concurrency**
- `memTasks` GET no longer marshals live task pointers outside the lock —
  values are copied under `memTasksMu` (was a data race with worker writes).
- `audit()` is nil-`*http.Request` safe (TG-originated actions pass `nil`).

**Correctness**
- Restore now calls `refreshMFACache()` after commit — previously a restored
  DB enabling MFA was ignored by login until restart (MFA bypass window).
- Backup aborts if any table read fails instead of silently exporting empty
  tables (tenants/instances/config/users/plans/keys/tasks/cfgs/ip-data...).
- OCI SDK nil-pointer guards on 7 dereferences (console connection id,
  subscription id, launched-instance id, boot-volume attachment id, SCIM
  user id, VCN id in TG menu) — panic risk on malformed API responses.
- `statusWriter.Unwrap()` lets `http.ResponseController` clear the 60s
  `WriteTimeout` for the AI SSE stream and the 10-minute netboot-rescue flow
  (previously hard-cut mid-stream).

**Telegram bot security**
- TG SSH now uses the shared TOFU host-key pinning (was: accept-any-key) and
  rejects command chaining/injection (`;`, `&&`, `||`, backticks, `$(`, `\n`,
  `\r`) plus shells smuggled through pipes (`echo x | bash`).
- `telegram_chat_id` (when set) now also acts as a webhook chat allowlist —
  only that chat may drive the bot (protects `/ssh`, terminate, restore).
- Backup-restore key file names validated against path traversal.
- TG restore/terminate actions write audit entries.

**HTTP hardening**
- `OCI_SECURE_COOKIES` now defaults to `true` (Secure attribute applied only
  behind TLS / HTTPS proxy, so plain-HTTP deployments are unaffected).
- Login blacklist, tenant account-status, instance-state and worker
  checkpoint DB errors are now logged instead of silently dropped.
- Aliveness check rejects malformed bodies (previously checked ALL tenants);
  config upload uses `io.ReadFull` to avoid truncated key files.

**Dependencies**: oci-go-sdk `v65.122.0`, x/crypto `v0.54.0`,
modernc.org/sqlite `v1.55.0`.

**Tests added**: `handler_audit_test.go` — memTasks race under `-race`,
SSH command filter (interactive/chaining/pipe/newline bypass cases), MFA
cache refresh after restore, key-file traversal rejection.



## 2. Architecture Parity — All Java Patterns Have Go Equivalents

| Java Pattern | Go Equivalent | File |
|--------------|---------------|------|
| Virtual threads (`virtualExecutor`) | Goroutines (`go func()`) | worker.go, handler_memtasks.go, handler_instances.go |
| MyBatis-Plus pagination | Raw SQL + in-memory pagination | queries.go |
| Guava cache (TTL) | sync.Map with expiry + goroutine cleanup | handler.go (conversationCache), handler_misc.go (captchaStore) |
| Spring reactive SSE (Flux) | Channel-based `text/event-stream` | handler.go (handleAIChat) |
| Telegram bot (Spring-based) | Native Go Telegram client | handler_tgmenu.go (29KB), telegram/ |
| Identity Domains SCIM fallback | `identitydomains` SDK + `PutUserPasswordChanger` | client_domain.go |
| OSP Gateway subscription | `ospgateway.SubscriptionServiceClient` | client.go:94-97 |
| IP Geolocation (MaxMind) | ip-api.com free API | geoip/geoip.go |

---

## 3. Remaining Differences (cosmetic only)

| Item | Note |
|------|------|
| AI chat session cache clear granularity | Java: per-session. Go: per-session (when `?session_id=` set) or all (no param) |
| Glance cities data source | Java: ipData table with geolocation. Go: same — ip_data with lat/lng/country/area/city/org/asn |
| Frontend framework | Java: Vue 3 + Element Plus. Go: Vue 3 + Element Plus (same) |

---

## 4. Go-Exclusive Features (not in Java original)

| # | Feature | File(s) |
|---|---------|---------|
| 1 | Multi-user RBAC system (admin/user roles) | handler_users.go, Users.vue, db models |
| 2 | WebSocket real-time log streaming | handler_wslog.go, Logs.vue |
| 3 | Web VNC Console | handler_instances.go:930, VncConsole.vue |
| 4 | Web SSH Terminal (xterm.js) | handler_shell.go, ShellConsole.vue |
| 5 | Instance Batch Create page | InstanceBatchCreate.vue |
| 6 | Audit Trail with search + pagination | Audit.vue |
| 7 | Backup/Restore (SQLite dump) | Backup.vue, handler.go:96-97 |
| 8 | SSH Keys management (generate/upload) | handler_ssh.go, SshKeys.vue |
| 9 | Persistent Instance Plans | handler_instanceplans.go, InstancePlans.vue |
| 10 | IP Pool management | IpPool.vue |
| 11 | Defense mode (IP blacklisting) | handler_defense.go, Defense.vue |
| 12 | DingTalk bot notification channel | dingtalk/ package, handler_misc.go |
| 13 | Telegram inline keyboard bot (full menu) | handler_tgmenu.go (29KB) |
| 14 | Public IP Info page (no auth required) | IpInfo.vue |
| 15 | Cloudflare OCI DNS auto-sync | handler_cloudflare.go:204 |
| 16 | Dedicated Instance Detail page | InstanceDetail.vue |
| 17 | Boot Volumes management page | BootVolumes.vue |
| 18 | VCN Management page | VcnManagement.vue |
| 19 | Service Limits query page | Limits.vue |
| 20 | Cost Analysis page | CostAnalysis.vue |
| 21 | Public IPs management page | PublicIPs.vue |
| 22 | Login rate limiter | handler.go:ratelimit |
| 23 | IP geolocation on IP data create | geoip/geoip.go, handler_ipdata.go |
