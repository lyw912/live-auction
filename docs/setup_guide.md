# Setup Guide

本文档记录开工前需要安装、配置和申请的内容。当前仓库已经创建基础目录、`.env.example` 和本地 `infra/docker-compose.yml`；业务代码尚未初始化。

## 1. 已在仓库内准备好的内容

- `README.md`：项目目标、技术路线和目录说明。
- `CLAUDE.md`：给编码助手/协作者的工程规则。
- `.env.example`：本地开发环境变量样例。
- `infra/docker-compose.yml`：本地 PostgreSQL、Redis、MinIO。
- `backend/`、`frontend/`、`tests/`、`docs/evidence/`、`docs/perf/`、`docs/demo/`、`docs/adr/`：开工目录骨架。
- `.github/workflows/ci.yml`：最小 CI 占位，后续补 Go/前端测试命令。

## 2. 需要你本机安装的工具

### 必装

| 工具 | 用途 | 建议 |
|---|---|---|
| Git | 版本控制 | 已有即可，建议确认 `git --version` |
| Go | 后端服务、工具链、测试 | 安装官方 Windows MSI，使用官方当前稳定版 |
| Node.js | React/Vite/Playwright | 安装当前 LTS；Node 官方下载页当前显示 v24 LTS |
| pnpm | 前端包管理 | 用 Corepack 或 npm 安装，要求 Node 22+ 才能跑 pnpm 11 |
| Docker Desktop | 本地 PostgreSQL、Redis、MinIO | Windows 推荐 WSL 2 backend |
| goose | SQL migration 工具 | `go install github.com/pressly/goose/v3/cmd/goose@latest` |

### 后续里程碑需要

| 工具 | 用途 | 何时需要 |
|---|---|---|
| Playwright browsers | E2E 测试 | 前端 P0 开始前 |
| k6 | HTTP/WS 压测 | P1 性能证据阶段，或提前写脚本 |
| Toxiproxy | Redis/DB/网络故障注入 | chaos gate 开始前 |
| PostgreSQL client / DBeaver / DataGrip | 本地查库 | 可选，但调试方便 |
| MinIO Client `mc` | 创建 bucket、查对象 | 可选；也可用 MinIO Console |

## 3. Windows 安装建议

打开 PowerShell，先确认已有工具：

```powershell
git --version
go version
node --version
npm --version
docker --version
docker compose version
```

安装 Go：

1. 打开官方 Go 安装页：https://go.dev/doc/install
2. 下载 Windows MSI。
3. 安装后关闭并重新打开 PowerShell。
4. 验证：

```powershell
go version
```

安装 Node.js：

1. 打开官方 Node 下载页：https://nodejs.org/en/download/
2. 选择 LTS Windows installer。
3. 安装后验证：

```powershell
node --version
npm --version
```

安装 pnpm，推荐 Corepack：

```powershell
npm install --global corepack@latest
corepack enable pnpm
corepack use pnpm@latest-11
pnpm --version
```

如果 Corepack 被公司环境限制，可以改用：

```powershell
npm install -g pnpm@latest-11
```

安装 Docker Desktop：

1. 打开官方 Docker Windows 安装页：https://docs.docker.com/desktop/setup/install/windows-install/
2. 选择 WSL 2 backend。
3. 启动 Docker Desktop 并接受条款。
4. 验证：

```powershell
docker run --rm hello-world
docker compose version
```

注意：Docker Desktop 商业使用可能需要订阅许可。若这是公司项目，请确认公司政策。

安装 goose：

```powershell
go install github.com/pressly/goose/v3/cmd/goose@latest
goose -version
```

如果 `goose` 不在 PATH，把 Go bin 目录加入用户 PATH。常见路径：

```text
%USERPROFILE%\go\bin
```

## 4. 本地环境变量

从仓库根目录创建本地 `.env`：

```powershell
Copy-Item .env.example .env
```

默认配置：

| 服务 | 地址 | 账号 |
|---|---|---|
| PostgreSQL | `localhost:5432` | `live_auction` / `live_auction` |
| Redis | `localhost:6379` | 无密码 |
| MinIO API | `localhost:9000` | `liveauction` / `liveauction123` |
| MinIO Console | `http://localhost:9001` | `liveauction` / `liveauction123` |

P0 使用 mock auth 和 mock payment，不需要真实登录、短信、支付或直播推流配置。

## 5. 启动本地基础设施

```powershell
docker compose -f infra\docker-compose.yml up -d
docker compose -f infra\docker-compose.yml ps
```

健康检查：

```powershell
docker exec live-auction-postgres pg_isready -U live_auction -d live_auction
docker exec live-auction-redis redis-cli ping
```

打开 MinIO Console：

```text
http://localhost:9001
```

创建 bucket：

```text
live-auction-items
```

后续可以把 bucket 初始化放进 seed/migration 辅助命令；当前阶段先手动创建即可。

停止本地服务：

```powershell
docker compose -f infra\docker-compose.yml down
```

清空本地数据时才使用：

```powershell
docker compose -f infra\docker-compose.yml down -v
```

## 6. 数据库迁移约定

迁移文件放在：

```text
backend/migrations/
```

创建迁移：

```powershell
goose -dir backend\migrations create init_schema sql
```

执行迁移：

```powershell
$env:DATABASE_URL="postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"
goose -dir backend\migrations postgres $env:DATABASE_URL up
```

查看状态：

```powershell
$env:DATABASE_URL="postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"
goose -dir backend\migrations postgres $env:DATABASE_URL status
```

后续实现要求：

- money 字段全部使用 integer cents。
- enum-like 字段在应用层有常量和测试。
- partial unique index 必须有命名和错误码映射。
- 不用 DB trigger 做隐藏业务状态变更。

## 7. 前端初始化约定

PC 控制台：

```text
frontend/pc-console/
```

Mobile H5：

```text
frontend/mobile-h5/
```

后续初始化时使用 Vite React TypeScript，并用 pnpm workspace 管理：

```powershell
pnpm create vite frontend/pc-console --template react-ts
pnpm create vite frontend/mobile-h5 --template react-ts
```

PC UI 库选 Arco Design；H5 使用自定义移动端竞拍 UI，不套 admin 组件。

Playwright 后续安装：

```powershell
pnpm create playwright
pnpm exec playwright install
```

## 8. 后端初始化约定

后端保持 Go modular monolith：

```text
backend/cmd/server/
backend/internal/gateway/
backend/internal/auction/
backend/internal/order/
backend/internal/realtime/
backend/internal/outbox/
backend/internal/scheduler/
backend/internal/observability/
backend/internal/storage/
```

技术选择：

- HTTP router：`chi`
- PostgreSQL driver/pool：优先 `pgx`
- Redis client：后续实现时选成熟 Go client
- MinIO/S3：S3-compatible SDK
- logger：结构化日志，所有请求带 `trace_id`

第一批后端代码应先做：

1. config/env loader
2. health endpoint
3. PostgreSQL/Redis/MinIO client 初始化
4. migration runner 或迁移命令约定
5. seed users/room/items

## 9. 外部资源与 API 申请

P0 本地开发不需要申请外部 API。

明确不需要：

- 真实支付 API
- 真实直播推流服务
- 短信/邮箱服务
- AI API
- 云对象存储
- 云 PostgreSQL/Redis

后续如果要线上演示，才需要准备：

- 一台 Linux 主机或云容器环境。
- 域名和 HTTPS 证书。
- 对象存储 bucket，或继续自托管 MinIO。
- 可观测性平台，或自托管 Prometheus/Grafana。
- Docker Desktop 商业许可确认，若在公司环境使用。

## 10. 开工前检查清单

```powershell
git status --short
go version
node --version
pnpm --version
docker compose version
goose -version
docker compose -f infra\docker-compose.yml up -d
docker compose -f infra\docker-compose.yml ps
```

确认：

- `.env` 已由 `.env.example` 创建。
- PostgreSQL、Redis、MinIO 都是 healthy 或可连接。
- MinIO bucket `live-auction-items` 已创建。
- `docs/design-v2-industrial/12-engineering-rules.md` 已读。
- 写 bid/cancel/end/realtime 前，先补 migration、错误码、规则测试和 idempotency 常量。

## 11. 参考来源

- Go Windows 安装：https://go.dev/doc/install
- Node.js LTS 下载：https://nodejs.org/en/download/
- pnpm 安装与 Corepack：https://pnpm.io/installation
- Docker Desktop Windows 安装：https://docs.docker.com/desktop/setup/install/windows-install/
- goose migration tool：https://github.com/pressly/goose
- Playwright 安装：https://playwright.dev/docs/intro
- k6 安装：https://grafana.com/docs/learning-paths/run-first-k6-test/install-k6/
- Toxiproxy：https://github.com/Shopify/toxiproxy
