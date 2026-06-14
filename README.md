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
│   │                            #   compute / network / identity / blockstorage
│   ├── auth/auth.go             # bcrypt 密码 + HttpOnly session cookie
│   └── handler/
│       ├── handler.go           # REST API + 内嵌前端路由
│       └── dist/index.html      # SPA 前端 (内嵌到二进制)
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

- [x] Web 面板登录（bcrypt + session）
- [x] 多租户配置管理（CRUD）
- [x] 实例同步（list instances + upsert to DB）
- [x] 审计日志
- [x] Docker 单容器部署（FROM scratch, 65534）
- [x] CI/CD（push 自动构建 amd64+arm64）
- [x] 健康检查（healthcheck mode）

### 待实现

- [ ] 实例操作（开/关/重启/终止）
- [ ] 实例创建（选 image/shape/subnet/AD）
- [ ] 公网 IP 管理（分配/释放/轮换）
- [ ] 启动卷操作（扩容/缩容/救砖）
- [ ] 批量开机（后台任务队列）
- [ ] 任务队列（断点续传，progress tracking）
- [ ] 实时流量统计
- [ ] Cloudflare DNS 联动（IP 轮换自动更新记录）
- [ ] MFA（TOTP）
- [ ] Google OAuth 登录
- [ ] Telegram Bot
- [ ] AI 助手（SiliconFlow API）
- [ ] Cloud Shell（Web 终端）
- [ ] 加密备份/恢复
- [ ] 多语言（zh_CN/en）
- [ ] 前端优化（React/Svelte 重写）

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

## API

| Method | Path | Auth | 说明 |
|--------|------|:---:|------|
| POST | `/api/login` | — | Basic Auth 登录 |
| POST | `/api/logout` | — | 登出 |
| GET | `/api/config` | ✓ | 获取配置 |
| GET | `/api/tenants` | ✓ | 租户列表 |
| POST | `/api/tenants` | ✓ | 添加租户 |
| GET | `/api/tenants/:id` | ✓ | 租户详情 |
| DELETE | `/api/tenants/:id` | ✓ | 删除租户 |
| GET | `/api/instances` | ✓ | 实例列表 |
| GET | `/api/tasks` | ✓ | 任务列表 |
| GET | `/api/audit` | ✓ | 审计日志 |
| POST | `/api/sync/:tenantId` | ✓ | 同步实例 |

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
