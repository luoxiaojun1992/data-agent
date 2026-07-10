# Docker Compose 配置修复：移除 MinIO + 添加应用服务

> **SPEC-016** | Status: 设计中

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施定义，SeaweedFS 替代 MinIO |
| SPEC-005 | ✅ | Artifact 存储使用 SeaweedFS，MinIO 不再需要 |
| SPEC-015 | ✅ | 代码审核修复后一致性检查 |

## 1. 目标

修复 `docker-compose.yml` 和 `docker-compose.ui-test.yml` 中的两个一致性问题：
1. **移除 MinIO（含 etcd）**：根据架构设计（SPEC-003/SPEC-005），系统已全面使用 SeaweedFS 替代 MinIO 作为对象存储，且 Qdrant v2.5.9 standalone 模式支持本地嵌入式存储，不再依赖外部 MinIO 和 etcd。
2. **添加应用服务**：`docker-compose.yml` 缺少 `data-agent` 后端和 `frontend` 前端服务定义，开发环境需要一键启动完整技术栈。

## 2. 背景

### 2.1 MinIO 残留

- `docker-compose.yml` 第 61-73 行定义了 `minio` 服务（含健康检查、卷挂载）
- `docker-compose.ui-test.yml` 第 76-88 行同样定义了 `minio` 服务
- 两个文件中 `qdrant` 服务通过环境变量 `MINIO_ADDRESS=minio:9000` 和 `ETCD_ENDPOINTS=etcd:2379` 依赖外部存储
- 架构文档（`ARCHITECTURE.zh-CN.md`）明确对象存储为 **SeaweedFS**，技术栈列表中不包含 MinIO

### 2.2 应用服务缺失

- `docker-compose.yml` 仅包含基础设施服务（MongoDB, Redis, Qdrant, etcd, Minio, SeaweedFS, Vault），缺少 `data-agent` 后端和 `frontend` 前端
- `docker-compose.ui-test.yml` 已包含 `data-agent` 服务和 `ui-e2e` 测试服务，但缺少 `frontend` 前端服务
- 开发人员无法通过 `docker compose up` 一键启动完整开发环境

### 2.3 Qdrant standalone 模式

Qdrant v2.5.9 的 `qdrant run standalone` 默认使用本地嵌入式存储（Etcd + 本地磁盘），不需要外部 MinIO 和 etcd 服务。只需确保有持久化卷挂载即可。

## 3. 架构概述

### 修改前后对比

**MinIO 链路（旧）**：
```
qdrant ──depends_on──► etcd
       ──depends_on──► minio
```

**修改后**：
```
qdrant (standalone, 嵌入式存储)
       └── volumes: qdrant-data
```

**应用服务链路（新增）**：
```
docker-compose.yml:
  data-agent (Go backend, :8080)
  frontend (Next.js, :3000)
      └── depends_on: data-agent

docker-compose.ui-test.yml:
  data-agent (Go backend, :8080)  ← 已有，但需修复
  frontend (Next.js, :3000)       ← 新增
      └── depends_on: data-agent
  ui-e2e                          ← 已有
      └── depends_on: data-agent
```

## 4. 详细设计

### 4.1 移除 minio 服务

**两个文件均需修改**：

| 修改项 | docker-compose.yml | docker-compose.ui-test.yml |
|--------|:---:|:---:|
| 删除 `minio:` 服务块 | ✅ | ✅ |
| 删除 `minio-data:` 卷 | ✅ | ✅ |
| qdrant 环境变量移除 `MINIO_ADDRESS` | ✅ | ✅ |
| qdrant 环境变量移除 `ETCD_ENDPOINTS` | ✅ | ✅ |
| qdrant `depends_on` 移除 `etcd` | ✅ | ✅ |
| qdrant `depends_on` 移除 `minio` | ✅ | ✅ |
| qdrant 添加 `volumes: qdrant-data:/var/lib/qdrant` | ✅ | ✅ |
| 删除 `etcd:` 服务块 | ✅ | ✅ |
| 删除 `etcd-data:` 卷 | ✅ | ✅ |
| volumes 新增 `qdrant-data:` | ✅ | ✅ |

> **注意**：etcd 和 minio 一并移除，因为 etcd 的唯一消费者是 Qdrant，移除 Qdrant 的外部 etcd 依赖后 etcd 不再被任何服务使用。

### 4.2 添加 data-agent 后端服务

`data-agent` 是一个 Go 单二进制服务，暴露 `:8080` 端口：

```yaml
  data-agent:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    environment:
      - MONGO_URI=mongodb://mongodb:27017
      - REDIS_ADDR=redis:6379
      - QDRANT_URL=qdrant:6334
      - SEAWEEDFS_MASTER=http://seaweedfs:9333
      - SEAWEEDFS_FILER=http://seaweedfs:8080
    depends_on:
      mongodb:
        condition: service_healthy
      redis:
        condition: service_healthy
      qdrant:
        condition: service_healthy
      seaweedfs:
        condition: service_healthy
      vault:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 15
      start_period: 20s
```

### 4.3 添加 frontend 前端服务

`frontend` 是 Next.js 14 前端，暴露 `:3000` 端口。需要独立 Dockerfile：

```dockerfile
# frontend/Dockerfile
FROM node:22-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/public ./public
COPY --from=builder /app/package.json ./package.json
COPY --from=builder /app/node_modules ./node_modules
EXPOSE 3000
CMD ["npx", "next", "start"]
```

docker-compose 定义：

```yaml
  frontend:
    build:
      context: frontend
      dockerfile: Dockerfile
    ports:
      - "3000:3000"
    environment:
      - NEXT_PUBLIC_API_URL=http://data-agent:8080
    depends_on:
      data-agent:
        condition: service_healthy
```

### 4.4 docker-compose.ui-test.yml 的特殊处理

`docker-compose.ui-test.yml` 已有 `data-agent` 服务（第 105-136 行），但需要以下修改：
- 移除 qdrant 对 minio/etcd 的依赖（同 4.1）
- 移除 minio/etcd 服务块和卷
- 添加 `frontend` 前端服务
- `data-agent` 的环境变量中移除 `SONARQUBE_HOST/USER/PASSWORD`（不属于运行时依赖，是 CI 层配置）
- 添加 `secrets: data-agent-pat` 定义（用于 PR 自动创建/合并）

> **关于 secrets**：此处参考 `docker-compose.ui-test.yml` 的设计意图。当前文件中 data-agent 依赖 sonarqube，而 sonarqube 是 CI 级服务。需要解耦：sonarqube 保留（CI 需要的代码质量检查），但 data-agent 本身不需要感知 sonarqube 的内部凭证。

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 正面 — Qdrant 嵌入式存储减少网络层，MinIO 移除降低启动时间 ~15s |
| 是否需要新增 Skill | No |
| 是否需要新增 Dockerfile | Yes — `frontend/Dockerfile`（新增） |
| 是否存在向后不兼容 | No — 研发/测试环境变更，不影响生产 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `docker-compose.yml` | 移除 minio/etcd，添加 data-agent + frontend | **High** |
| `docker-compose.ui-test.yml` | 移除 minio/etcd，添加 frontend，清理 data-agent env | **High** |
| `frontend/Dockerfile` | Next.js 前端构建镜像 | **New** |

## 7. 测试策略

1. **Smoke test**: `docker compose up` 启动后，所有服务 healthcheck 通过
2. **API test**: `curl localhost:8080/health` 返回 200
3. **Frontend test**: `curl localhost:3000` 返回 Next.js 页面
4. **Qdrant test**: 验证 Qdrant standalone 不依赖 minio/etcd 仍正常运行

## 8. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **可选** 本次为基础设施配置变更，无前端 UI 交互新增，E2E 用例非必须
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9. 验证标准

- [ ] `docker compose up` 启动后，所有 7 个服务（mongodb, redis, qdrant, seaweedfs, vault, data-agent, frontend）healthcheck 均为 healthy
- [ ] `minio` 服务和 `etcd` 服务不再存在于两个 compose 文件中
- [ ] `qdrant` 不依赖外部 minio/etcd（无 `depends_on` 指向它们，无对应的环境变量）
- [ ] `frontend/Dockerfile` 存在且可构建
- [ ] `docker-compose.yml` 中 data-agent 挂载所有必要的 depends_on + healthcheck
- [ ] `docker-compose.ui-test.yml` 中 data-agent 环境变量不包含 CI 专属配置（sonarqube 凭证）
