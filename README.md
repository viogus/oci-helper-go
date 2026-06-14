# oci-helper-go

Oracle Cloud Infrastructure 管理面板。Go 重写版。

## 为什么重写

原版 [oci-helper](https://github.com/Yohann0617/oci-helper) 用 Java (Spring Boot) + Vue 2，功能成熟但架构重：

| 维度 | Java oci-helper | Go oci-helper-go | 改善 |
|------|:---:|:---:|---|
| **Docker 镜像** | ~80MB (JRE Alpine) | **~13MB** | 6× |
| **容器数** | 3 (app+watcher+websockify) | **1** | 3× |
| **内存占用** | 512MB 推荐 | **128MB** | 4× |
| **启动时间** | ~8s (JVM) | **<100ms** | 80× |
| **二进制大小** | 80MB JAR + 80MB JRE | **12MB 静态链接** | 13× |
| **部署** | 需 env + 挂载文件 + websockify | **docker run 一行** | — |
| **基础镜像** | eclipse-temurin:21-jre-alpine | **FROM scratch** | 零依赖 |

Go 单二进制、`FROM scratch`、`CGO_ENABLED=0`。Java 做的功能 Go 都能做，而且更轻、更快、更好部署。

## 架构

```
oci-helper-go/
├── cmd/server/main.go           # 入口：http server + healthcheck mode
├── internal/
│   ├── config/config.go         # 环境变量 → 配置
│   ├── db/
│   │   ├── models.go            # 数据模型 (Tenant/Instance/Task/Audit)
│   │   ├── sqlite.go            # SQLite 连接 + 自动迁移 (pure Go, no CGO)
│   │   └── queries.go           # CRUD 操作
│   ├── oci/client.go            # OCI Go SDK v65 封装
│   │                            #   compute / network / identity / blockstorage / monitoring
│   ├── auth/auth.go             # bcrypt + session + TOTP
│   ├── cloudflare/client.go     # Cloudflare DNS API
│   ├── telegram/bot.go          # Telegram Bot 客户端
│   ├── ai/assistant.go          # SiliconFlow AI 客户端
│   ├── i18n/i18n.go             # 多语言 (zh_CN/en)
│   └── handler/
│       ├── handler.go           # REST API + 路由
│       ├── worker.go            # 后台任务队列
│       ├── backup.go            # 加密备份/恢复 (AES-256-GCM)
│       └── dist/index.html      # SPA 全功能前端 (内嵌到二进制)
├── Dockerfile                   # golang:alpine → FROM scratch
├── docker-compose.yml           # 单容器 128MB 限制
└── .github/workflows/build.yml  # CI: amd64+arm64, ghcr.io
```

### 技术栈

| 层 | 技术 | 说明 |
|----|------|------|
| 后端 | Go 1.26 | 标准库 `net/http` |
| 数据库 | SQLite (`modernc.org/sqlite`) | 纯 Go，无 CGO，单文件 |
| OCI SDK | `oci-go-sdk/v65` | 官方 Go SDK |
| 认证 | `bcrypt` + session cookie | HttpOnly，24h TTL |
| 前端 | 内嵌 HTML/JS | 单文件 SPA，无构建工具 |
| 部署 | `FROM scratch` | 仅二进制 + ca-certificates |

## 功能规划

### 已完成

- [x] Web 面板登录（bcrypt + session + MFA）
- [x] 多租户配置管理（CRUD）
- [x] 实例同步（list instances + upsert to DB）
- [x] 实例操作（开/关/重启/终止）
- [x] 实例创建（选 image/shape/subnet/AD）
- [x] 公网 IP 管理（分配/释放）
- [x] 启动卷操作（扩容/挂载/卸载）
- [x] 批量开机（后台任务队列 + 断点续传）
- [x] 实时流量/CPU/内存统计（OCI Monitoring）
- [x] Cloudflare DNS 联动（IP 轮换自动更新记录）
- [x] MFA（TOTP）
- [x] Google OAuth 登录
- [x] Telegram Bot
- [x] AI 助手（SiliconFlow API）
- [x] Cloud Shell（Web 终端）
- [x] 加密备份/恢复（AES-256-GCM）
- [x] 多语言（zh_CN/en）
- [x] 审计日志
- [x] Docker 单容器部署（FROM scratch, 65534）
- [x] CI/CD（push 自动构建 amd64+arm64）
- [x] 健康检查（healthcheck mode）
- [x] 全功能 SPA 前端

## 部署

### Portainer Stack

```yaml
version: "3.8"
services:
  oci-helper:
    image: ghcr.io/viogus/oci-helper-go:latest
    restart: unless-stopped
    ports:
      - "8818:8818"
    volumes:
      - oci-helper-data:/app/oci-helper
    environment:
      - OCI_USERNAME=admin
      - OCI_PASSWORD=your-secret-password
    healthcheck:
      test: ["CMD", "/oci-helper", "health"]
      interval: 30s
    deploy:
      resources:
        limits:
          memory: 128M

volumes:
  oci-helper-data:
```

### docker run

```bash
docker run -d \
  --name oci-helper \
  -p 8818:8818 \
  -v oci-helper-data:/app/oci-helper \
  -e OCI_USERNAME=admin \
  -e OCI_PASSWORD=your-password \
  ghcr.io/viogus/oci-helper-go:latest
```

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `8818` | 监听端口 |
| `OCI_USERNAME` | `admin` | 面板登录账号 |
| `OCI_PASSWORD` | 随机生成 | 面板登录密码（留空则自动生成并打印到日志） |
| `OCI_DB_PATH` | `/app/oci-helper/oci-helper.db` | SQLite 数据库路径 |
| `OCI_KEYS_DIR` | `/app/oci-helper/keys` | OCI API 密钥目录 |
| `OCI_MFA` | `false` | 是否启用 MFA |
| `OCI_MFA_SECRET` | — | TOTP 密钥 |
| `GOOGLE_CLIENT_ID` | — | Google OAuth 客户端 ID |
| `GOOGLE_CLIENT_SECRET` | — | Google OAuth 密钥 |
| `GOOGLE_REDIRECT_URL` | — | Google OAuth 回调地址 |

以下配置在 Web 面板中设置，存入 SQLite：
- Cloudflare API Token
- SiliconFlow API Key
- Telegram Bot Token
- MFA Secret (TOTP)

## API

### 认证

| Method | Path | Auth | 说明 |
|--------|------|:---:|------|
| POST | `/api/login` | Basic | 登录（MFA: X-TOTP header） |
| POST | `/api/logout` | — | 登出 |
| GET | `/api/oauth/google/login` | — | Google OAuth 登录跳转 |
| GET | `/api/oauth/google/callback` | — | Google OAuth 回调 |

### 租户 & 实例

| Method | Path | Auth | 说明 |
|--------|------|:---:|------|
| GET | `/api/config` | ✓ | 获取配置 |
| GET/POST | `/api/tenants` | ✓ | 租户列表 / 添加 |
| GET/DELETE | `/api/tenants/:id` | ✓ | 租户详情 / 删除 |
| GET/POST | `/api/instances` | ✓ | 实例列表 / 创建实例 |
| POST | `/api/instances/:id/action` | ✓ | 实例操作 (start/stop/reboot/terminate) |
| POST | `/api/instances/batch-start` | ✓ | 批量开机 |
| POST | `/api/sync/:tenantId` | ✓ | 同步实例 |
| GET | `/api/metrics` | ✓ | 实例监控指标 (CPU/Mem/Network) |

### 参考数据

| Method | Path | Auth | 说明 |
|--------|------|:---:|------|
| GET | `/api/images` | ✓ | 镜像列表 (?tenant_id=X&os=) |
| GET | `/api/shapes` | ✓ | Shape 列表 (?tenant_id=X&image_id=Y) |
| GET | `/api/vcns` | ✓ | VCN 列表 (?tenant_id=X) |
| GET | `/api/subnets` | ✓ | Subnet 列表 (?tenant_id=X&vcn_id=Y) |
| GET | `/api/availability-domains` | ✓ | AD 列表 (?tenant_id=X) |

### 网络 & 存储

| Method | Path | Auth | 说明 |
|--------|------|:---:|------|
| GET/POST | `/api/public-ips` | ✓ | 公网 IP 列表 / 预留 |
| DELETE | `/api/public-ips/:id` | ✓ | 释放公网 IP |
| GET | `/api/boot-volumes` | ✓ | 启动卷列表 |
| POST | `/api/boot-volumes/:id/resize` | ✓ | 扩容启动卷 |
| POST | `/api/boot-volumes/:id/attach` | ✓ | 挂载到实例 |
| POST | `/api/boot-volumes/:id/detach` | ✓ | 卸载启动卷 |

### 任务 & 审计

| Method | Path | Auth | 说明 |
|--------|------|:---:|------|
| GET | `/api/tasks` | ✓ | 任务队列 |
| GET | `/api/audit` | ✓ | 审计日志 |

### MFA & 安全

| Method | Path | Auth | 说明 |
|--------|------|:---:|------|
| GET | `/api/mfa/setup` | ✓ | 生成 TOTP 密钥 |
| POST | `/api/mfa/verify` | ✓ | 验证 TOTP 并启用 |
| POST | `/api/mfa/disable` | ✓ | 禁用 MFA |
| POST | `/api/backup` | ✓ | 加密导出 (AES-256-GCM) |
| POST | `/api/restore` | ✓ | 加密导入恢复 |

### 集成

| Method | Path | Auth | 说明 |
|--------|------|:---:|------|
| GET/POST | `/api/cloudflare/zones` | ✓ | CF 区域列表 |
| GET/POST | `/api/cloudflare/:zoneId/records` | ✓ | DNS 记录 CRUD |
| POST | `/api/cloudflare/update-ip` | ✓ | 更新 DNS 记录 IP |
| POST | `/api/ai/chat` | ✓ | AI 对话 (SiliconFlow) |
| GET | `/api/shell/:instanceId` | ✓ | Cloud Shell 端点 |
| POST | `/api/telegram/webhook` | — | Telegram Bot Webhook |

## 构建

```bash
# 本地构建
CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server

# Docker 构建
docker build -t oci-helper .

# 多架构
docker buildx build --platform linux/amd64,linux/arm64 -t oci-helper .
```

## 镜像

| 镜像 | 说明 |
|------|------|
| `ghcr.io/viogus/oci-helper-go:latest` | 最新主分支构建 (amd64/arm64) |
| `ghcr.io/viogus/oci-helper-go:sha-xxxxx` | 精确 commit |

## 许可

与原版一致，Apache-2.0。
