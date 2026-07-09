# Design: 500M + IPv6 行内开关 + 实现修复

**Date:** 2026-07-09
**Status:** Approved (pending spec review)

## 背景

原版 oci-helper 有两个功能："一键开启免费 AMD 实例下行 500Mbps" 与 "附加 IPv6"。用户以为 Go 版没实现。

实际排查结果：两个功能的代码**已经存在并提交在 HEAD**（前端下拉菜单项、后端路由/handler、OCI client 方法都在），当前构建的 `dist` 资产也包含。用户"没看到"是因为入口埋在实例行的"更多操作"下拉菜单里，不够显眼；且实现存在两个 bug 导致真实调用时可能失败。

本设计做两件事：
1. **入口**：把两个功能提升为实例表格里的行内开关列，显眼且反映当前状态。
2. **修 bug**：补全 IPv6 前置流程、修正 500M NLB 全端口转发，并加幂等。

## 现状（代码位置）

- 前端 `frontend/src/views/Instances.vue`：下拉项 `attachIPv6` / `enable500M` / `disable500M`，`handle500M()`，IPv6 弹窗。
- 路由 `internal/handler/handler.go:179,183,184`：`/api/instances/attach-ipv6`、`/one-click-500m`、`/one-click-close-500m`。
- Handler `internal/handler/handler_instances.go:477` (`handleAttachIPv6`)、`:699` (`handleOneClick500M`)、`:724` (`handleOneClickClose500M`)。
- OCI client `internal/oci/client.go`：`AssignIPv6:685`、`Enable500Mbps:1540`、`Disable500Mbps:1637`、`GetInstanceVNICs:665`。

## 已知 Bug

0. **下拉菜单被 row-click 吞掉（根因）** — `Instances.vue:46` 整行 `@row-click` 跳实例详情。点 `Actions ▾` 按钮时点击冒泡到 row-click，先跳详情，下拉根本弹不出来。**现有所有下拉操作（含 500M/IPv6）当前全部不可达**，这才是"没看到功能"的真正原因。
1. **IPv6 不完整** — `AssignIPv6` 只调 `CreateIpv6`。缺前置：VCN 未启 IPv6 CIDR、子网无 IPv6 CIDR、路由无 `::/0`、安全列表无 IPv6 入站。VCN 没 IPv6 段时直接失败。
2. **500M NLB 端口** — backend 固定 `Port: 22`，非 22 端口流量不转发。应全端口透传。
3. **无幂等** — `Enable500Mbps` 每次都新建 NLB，重复点会建多个。

## 范围

### In scope
- 实例表新增 `500M`、`IPv6` 两列开关。
- IPv6 全流程开启 + 关闭（删 VNIC 的 IPv6）。
- 500M 全端口转发 + 幂等 + 返回 NLB 公网 IP。
- 批量网络状态接口，进页面自动查。
- i18n（zh-CN + en），替换现有硬编码英文。
- 移除下拉里 3 个旧菜单项（被开关列取代）。

### Non-goals
BYOIP、双栈复杂路由、非主 VNIC 的 IPv6、NLB 自定义端口/多监听器。

## 后端设计

### 3.1 500M 修复 — `internal/oci/client.go`

`Enable500Mbps(ctx, instanceID) (nlbPublicIP string, err error)`：
- **幂等**：先 `ListNetworkLoadBalancers`，按 freeform tag `oci-helper-instance-id == instanceID` 查。已存在 → 直接返回其公网 IP，不新建。
- **端口修复**：BackendSet 的 backend `Port: 0`（全端口透传）；listener 保持 `Port: 0` / `ListenerProtocolsAny`；healthcheck 保持 TCP:22 做存活探测。
- 建成后轮询 work request（现有逻辑），成功后取 NLB 公网 IP 返回。

`Disable500Mbps(ctx, instanceID) error`：
- 现有按 tag 查删逻辑保留。
- 未找到 NLB → 不报错，当作已关（幂等）。

### 3.2 IPv6 全流程 — `internal/oci/client.go`

新增 `EnableIPv6(ctx, instanceID) (ipv6Addr string, err error)`，每步先查后建（幂等）：

1. `GetInstanceVNICs` → 主 VNIC（`vnics[0]`），取 `SubnetId`。
2. `GetSubnet` → 取 `VcnId`、`RouteTableId`、`Ipv6CidrBlock`、`SecurityListIds`。
3. **VCN**：`GetVcn`，若 `Ipv6CidrBlocks` 为空 → `AddIpv6VcnCidr{IsOracleGuaAllocationEnabled: true}`，轮询/重取直到 /56 出现。
4. **子网**：若 `Ipv6CidrBlock` 为空 → 由 VCN /56 推导 /64（同前缀，掩码改 64，子网号取 `00`），`AddIpv6SubnetCidr{Ipv6CidrBlock: <derived/64>}`。
5. **路由**：`ListInternetGateways` 找 VCN 的 IGW；取子网路由表，若无 `Destination: ::/0` 规则 → `UpdateRouteTable` 追加 `{Destination:"::/0", DestinationType:CIDR_BLOCK, NetworkEntityId: igwID}`（保留原有规则）。
6. **安全列表**：镜像 IPv4 入站到 IPv6 —— 遍历子网各 SecurityList 的 `IngressSecurityRules`，对每条 `Source: 0.0.0.0/0` 的规则，若不存在等价的 `Source: ::/0`（同 protocol + 端口范围）规则 → 追加一条 IPv6 版，`UpdateSecurityList`。不无脑全开，只镜像已有放行。
7. `CreateIpv6{VnicId}` → 重取 VNIC，返回 `Ipv6Addresses[0]`。

新增 `DisableIPv6(ctx, instanceID) error`：
- `ListIpv6s`（该 VNIC）→ 逐个 `DeleteIpv6`。
- **只删 VNIC 的 IPv6 地址**，不动 VCN/子网/路由/安全列表（共享资源，删了伤同 VCN 其他实例；保留即可反复开关）。
- 无 IPv6 → 不报错。

现有 `AssignIPv6` 由 `EnableIPv6` 取代。

### 3.3 Handler 层 — `internal/handler/handler_instances.go`

- `handleOneClick500M`：改为返回 `{status:"ok", nlb_ip: <ip>}`。
- `handleOneClickClose500M`：不变（幂等已在 client）。
- `handleAttachIPv6` → 语义扩为"启用 IPv6"，调 `EnableIPv6`，返回 `{status:"ok", ipv6: <addr>}`。
- 新增 `handleDisableIPv6`，调 `DisableIPv6`。路由 `POST /api/instances/disable-ipv6`。
- 审计 action 名沿用现有风格（`instance:500m-enable` 等），新增 `instance:ipv6-disable`。

### 3.4 状态接口 — `internal/handler/handler_instances.go`

新增 `POST /api/instances/network-status` `{tenant_id, instance_ids: []string}` → 
```json
{ "<instance_id>": { "ipv6_enabled": bool, "ipv6_addr": "", "nlb_enabled": bool, "nlb_ip": "" }, ... }
```
- 500M：一次 `ListNetworkLoadBalancers`，按 tag 建 map（O(1) 查每实例）。
- IPv6：并发 `GetInstanceVNICs` 各实例，读 `Ipv6Addresses`（复用现有并发上限模式，如 metrics 的 errgroup / sync.WaitGroup）。
- 单实例失败不影响整体，该行状态返回 `enabled:false` + 记日志。

## 前端设计 — `frontend/src/views/Instances.vue`

- **修 row-click 吞点击（Bug 0）**：所有交互单元格（新开关列、操作列的 `Actions ▾` 按钮及其下拉、Metrics 按钮）加 `@click.stop`，阻止冒泡到行级 `@row-click`（`:46`）。否则点开关/按钮会跳实例详情而非触发操作。行其余空白区仍可点击进详情。
- 表格新增两列 `500M`、`IPv6`，各一个 `el-switch`（`@click.stop`）：
  - 值来自 `network-status`，`onMounted` 与换页 (`loadInstances`) 后自动批量查。
  - 查询期间列显示 loading 占位；切换期间该开关 `:loading` + 禁用。
- **切换 on**：
  - 500M → `ElMessageBox.confirm`："将为该实例创建免费 NLB 以获得下行 500M，确认？" → `POST /one-click-500m` → 成功 toast 显示 NLB 公网 IP → 刷新该行状态。
  - IPv6 → `ElMessageBox.confirm`：**提示会修改 VCN/子网/路由，并按现有 IPv4 入站规则镜像开放 IPv6 入站** → `POST /attach-ipv6` → 成功 toast 显示分配到的 IPv6 → 刷新该行。
- **切换 off**：对应 confirm → `/one-click-close-500m` 或 `/disable-ipv6` → 刷新该行。
- 失败：开关回弹到原状态，`ElMessage.error` 显示后端 error。
- 移除下拉里 `attachIPv6` / `enable500M` / `disable500M` 三项及其 case 分支、旧 IPv6 弹窗。
- i18n：`frontend/src/locales/{zh-CN,en}.json` 新增键（列头、开关提示、确认文案、成功/失败），替换现有硬编码 `One-Click 500M` / `Close 500M` / `Attach IPv6`。

## 错误处理与安全

- 所有 client 方法每步先查后建，可重复调用不产生副作用（幂等）。
- IPv6 关闭不拆共享网络资源，避免误伤同 VCN 其他实例。
- 安全列表只镜像已有 IPv4 放行到 IPv6，不新增开放面。
- 长操作（NLB 建/删、VCN IPv6 轮询）沿用现有 context timeout（120s–5min）。

## 影响文件

| 文件 | 改动 |
|------|------|
| `internal/oci/client.go` | `Enable500Mbps` 改签名+幂等+端口；`Disable500Mbps` 幂等；新增 `EnableIPv6`/`DisableIPv6`；删 `AssignIPv6` |
| `internal/handler/handler_instances.go` | 500M 返回 nlb_ip；IPv6 启用返 addr；新增 `handleDisableIPv6`、`handleNetworkStatus` |
| `internal/handler/handler.go` | 注册 `/disable-ipv6`、`/network-status` |
| `frontend/src/views/Instances.vue` | 两列开关、状态查询、确认弹窗、移除旧下拉项 |
| `frontend/src/api/instances.js` | 加 `disableIPv6`、`networkStatus`；`attachIPv6` 保留 |
| `frontend/src/locales/{zh-CN,en}.json` | 新增 i18n 键 |
| `internal/handler/dist/*` | 重新构建前端后更新嵌入资产 |

## 验收

- 旧构建/容器"没看到功能" → 重新构建后，实例表出现两列开关。
- 点开关/操作按钮**只触发对应操作，不再误跳实例详情**（Bug 0 修复）；点行空白仍进详情。
- 全新 VCN（无 IPv6）点 IPv6 开关 → 自动建 VCN/子网 IPv6、路由、镜像安全规则、分配地址，实例可 IPv6 出网。
- 点 500M 开关 → 建 NLB，全端口经 NLB 转发，toast 显示公网 IP；重复点不重复建。
- 关开关 → 对应资源清理（IPv6 仅删 VNIC 地址；NLB 删除）。
