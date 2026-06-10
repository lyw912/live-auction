# Setup Guide

> 本文只保留本地复现需要的最小步骤。架构与评测说明见 [README](README.md)。

## 环境要求

| 工具 | 用途 |
|---|---|
| Docker / Docker Compose | PostgreSQL、Redis、Kafka、MinIO、Prometheus、Grafana |
| Go | 后端服务、迁移、测试 |
| Node.js + pnpm | H5/PC 前端 |
| goose | SQL migration |

## 本地启动

```bash
# 1. 基础设施
cd infra
docker compose up -d

# 2. 数据库迁移
cd ../backend
make migrate-up

# 3. 后端
cp ../.env.example ../.env
make run

# 4. 前端
cd ..
pnpm dev:h5
pnpm dev:pc
```

默认端口：

| 服务 | 地址 |
|---|---|
| Backend | `http://127.0.0.1:18080` |
| H5 | `http://127.0.0.1:5276` |
| PC Console | `http://127.0.0.1:5277` |
| PostgreSQL | `127.0.0.1:5432` |
| Redis | `127.0.0.1:6380` |
| Kafka | `127.0.0.1:9092` |

## 验证入口

```bash
cd backend
go test ./...
```

S1-S5 压测与故障验证不要手工拼环境变量，按 [s1-s5/12-readiness-checklist.md](s1-s5/12-readiness-checklist.md) 和 `tests/pts/MANIFEST.md` 执行 reset、preflight、run、verify。

## 文档入口

- [design/01-architecture.md](design/01-architecture.md)
- [design/02-performance-correctness-contract.md](design/02-performance-correctness-contract.md)
- [s1-s5/00-overview.md](s1-s5/00-overview.md)
- [judge/01-final-review.md](judge/01-final-review.md)
