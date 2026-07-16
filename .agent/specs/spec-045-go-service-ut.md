# Phase 10 — Go Service 单元测试全覆盖

> **SPEC-045** | Status: 📐 设计中 | Date: 2026-07-16 | Phase: P10 | 依赖: SPEC-002, SPEC-014, SPEC-003 ~ SPEC-013

## 1. 目标

为 `data-agent/` 后端所有 Go Service 包编写单元测试，**覆盖率底线 98%，目标 100%**，并在 GitHub Actions 中增加独立的 `ut-workflow.yml` 强制执行覆盖率门禁。

**现状**: 后方 ~50 个 Go 包中仅 `internal/logic/sql/validator_test.go` 存在 6 个测试函数，其余 49 个包零 UT 覆盖。SPEC-014 原定 >70% 覆盖率目标也远未达到。

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-002 | ✅ | CI Pipeline 就绪，Go 工具链可用 |
| SPEC-003 | ✅ | 基础设施接口已定义（可 mock） |
| SPEC-004 | ✅ | Agent 核心引擎接口已定义 |
| SPEC-005 | ✅ | Artifact 存储接口已定义 |
| SPEC-006 | ✅ | 知识库接口已定义 |
| SPEC-007 | ✅ | 数据分析 Logic 层 |
| SPEC-008 | ✅ | Skill 实现层 |
| SPEC-009 | ✅ | 任务队列接口（Redis Stream） |
| SPEC-010 | ✅ | 统计监控 |
| SPEC-011 | ✅ | IM 集成 |
| SPEC-012 | ✅ | Hermes（可能跳过深层 mock） |
| SPEC-013 | ✅ | 管理后台 |
| SPEC-014 | ✅ | 原测试体系 spec（>70% 目标未达成，本 spec 升级替代） |

## 3. 背景

SPEC-014（Phase 6）定义了 >70% UT 覆盖率目标，但项目快速迭代期间未强制执行，导致除 `internal/logic/sql/` 外所有包零测试。当前 Makefile 虽有 `make test` 目标，但 CI 中无 `go test` 步骤。这带来以下风险：

- **回归风险**：修改任意 service/logic 代码无 UT 验证
- **重构恐惧**：无安全网，不敢大改
- **合入门槛过低**：lint + UI test 通过即可合并，缺乏代码级正确性保障

本 spec 是 SPEC-014 的升级替代：大幅提高覆盖率基线（70% → 98%），并新增 CI 门禁强制执行。

## 4. 测试分层策略

### 4.1 测试难度分级

按 mock 依赖复杂度将 ~50 个包分为三个等级：

| 等级 | 特征 | 包数 | 目标覆盖率 | mock 成本 |
|:---:|------|:---:|:--------:|:--------:|
| **L1 - 纯逻辑** | 无外部依赖，纯函数/纯结构体 | ~15 | **100%** | 零成本，table-driven |
| **L2 - 接口依赖** | 依赖接口（可 mock），无状态 | ~20 | **100%** | 需定义 mock interface |
| **L3 - 集成依赖** | 依赖具体实现（MongoDB/Redis/HTTP） | ~15 | **98%** | 需 `httptest` / 集成容器 |

### 4.2 分包包级测试计划

#### L1 — 纯逻辑（100% 覆盖率，table-driven tests）

| 包 | 文件 | 测试重点 |
|---|------|---------|
| `internal/logic/sql` | `validator.go` | 已有 6 tests，补充边缘 SQL AST case（CTE、UNION、子查询变体） |
| `internal/logic/openapi` | `parser.go` | OpenAPI 3.0 JSON 解析；非法/残缺/边界 schema 用例 |
| `internal/logic/report` | `report.go` | Markdown AST 校验；合法/非法头部、表格、图表 |
| `internal/logic/stats` | `stats.go` | 回归/PCA/聚类算法；空输入、单点数据、零方差异常 |
| `internal/domain/model` | `model.go` | User/Role/Permission 结构体序列化/反序列化 |
| `internal/domain/task` | `task.go` | Task 状态机（枚举转换、状态流转合法性） |
| `internal/domain/apireview` | `apireview.go` | API 审核状态机 |
| `internal/domain/artifact` | `artifact.go` | Artifact 领域模型 |
| `internal/domain/knowledge` | `knowledge.go` | Knowledge Doc 模型 |
| `internal/domain/skill` | `registry.go`, `context.go` | Skill 注册/查找/Conext 注入（纯 map + struct） |
| `internal/domain/security` | `auditor.go`, `circuit_breaker.go` | 安全规则匹配、熔断器状态机 |
| `internal/config` | `config.go` | Viper 配置加载（env + yaml），默认值校验 |
| `skills/sql_executor` | `executor.go` | SQL 执行逻辑（mock DB 接口） |
| `skills/stats_engine` | `engine.go` | 统计算法调用链 |

#### L2 — 接口依赖（100% 覆盖率，需 mock interface）

| 包 | 文件 | 依赖接口 | 测试策略 |
|---|------|---------|---------|
| `internal/api/middleware` | `jwt.go`, `rbac.go`, `cors.go`, `audit.go` | `*gin.Context`, JWT parser | `gin.CreateTestContext()` + `httptest.NewRecorder()` |
| `internal/api/handler` | 7 files | Service 接口 | Mock service 层，测试 HTTP 序列化/反序列化、错误码映射 |
| `internal/service/auth` | `auth_service.go` | `mongo.UserRepository` 接口 | Mock UserRepository |
| `internal/service/apireview` | `apireview_service.go` | `mongo` 接口 | Mock Mongo |
| `internal/service/audit` | `audit_service.go` | `mongo.AuditLogRepository` 接口 | Mock AuditLogRepository |
| `internal/service/notification` | `notification_service.go` | `mongo` 接口 | Mock Notification repo |
| `internal/service/task` | `task_service.go` | `mongo.TaskRepository` + Redis | Mock repo + redis |
| `internal/service/knowledge` | `knowledge_service.go` | `mongo` + `qdrant` 接口 | Mock repo + vector client |
| `internal/service/artifact` | `artifact_service.go` | `mongo` + `seaweedfs` 接口 | Mock repo + filer client |
| `internal/service/monitor` | 2 files | `mongo` 接口 | Mock repo |
| `internal/service/im` | `im_service.go` | `mongo`/飞书 HTTP | Mock repo + `httptest.Server` 模拟飞书 |
| `internal/service/agent` | `agent_service.go` | ADK Engine + Redis Stream + Mongo | Mock engine (LLM router)、mock queue |
| `internal/service/chat` | 3 files | ADK + SSE + Mongo + Redis | Mock engine；`httptest` 捕获 SSE 流 |
| `internal/service/hermes` | `hermes_service.go` | Hermes HTTP API | `httptest.Server` 模拟上游 |
| `internal/domain/agent` | 4 files | LLM Router、ADK engine | Mock LLMProvider、ToolExecutor |
| `skills/knowledge_search` | `search.go` | Qdrant client 接口 | Mock vector search client |
| `skills/save_report` | `save.go` | SeaweedFS + Mongo | Mock filer + repo |
| `internal/logic/workspace` | `workspace.go` | `seaweedfs.Client` 接口 | Mock filer client |

#### L3 — 集成依赖（98% 覆盖率）

| 包 | 文件 | 依赖 | 测试策略 |
|---|------|------|---------|
| `internal/infra/mongo` | 4 files | MongoDB 连接 | Docker 集成测试（`docker-compose.ui-test.yml` 已有 MongoDB） |
| `internal/infra/redis` | `redis.go` | Redis 连接 | Docker 集成测试 |
| `internal/infra/qdrant` | `qdrant.go` | Qdrant 连接 | Docker 集成测试 |
| `internal/infra/seaweedfs` | `seaweedfs.go` | SeaweedFS 连接 | Docker 集成测试 |
| `internal/infra/vault` | `vault.go` | Vault 连接 | Docker 集成测试 |
| `internal/queue` | `queue.go` | Redis Stream | 集成测试或 mock `redis.Client` 接口 |
| `internal/worker` | `pool.go` | Redis + Agent Service | Mock 接口 |
| `internal/scheduler` | 2 files | `robfig/cron` + Redis | Mock 接口；纯 cron 逻辑可 table-test |
| `cmd/server` | `main.go` | 全量启动 | Skip — `main.go` 不计入覆盖率统计 |

### 4.3 覆盖率排除项（不计入 100% 目标）

| 排除文件 | 理由 |
|---------|------|
| `cmd/server/main.go` | 入口函数，由集成测试验证，UT 可不计 |
| `*_gen.go` / `*.pb.go` | 自动生成代码 |
| mock 文件 | 测试辅助代码 |

> **总覆盖目标**: 排除上述文件后，所有包的 `go test -race -coverprofile` 合并覆盖率 ≥ 98%。

## 5. 开源工具选型

| 工具 | 用途 | 适用层级 | 理由 |
|------|------|:---:|------|
| **Mockery** (vektra/mockery v3) | 自动生成 mock 实现 | L2、L3 | Go 社区首选 mock 生成器（7k+ stars），生成 testify 兼容 mock，支持泛型、自定义模板，`//go:generate` 驱动 |
| **Ginkgo + Gomega** (v2.28+) | BDD 测试框架 + 断言库 | L2 Service 层 | 分层 `Describe/Context/It` 结构非常适合 Service 层复杂测试的组织；`Eventually`/`Consistently` 支持异步断言；Gomega 提供 100+ 内置 matcher |
| **gomonkey** (agiledragon/gomonkey v2) | 运行时函数替换（monkey patching） | 预留 L2/L3 边缘 case | 用于无法提取 interface 的场景（如 `time.Now()`、`os.Getenv()`、未导出函数）；**限制使用**，仅作为最后手段 |
| `testing` (标准库) | 表驱动测试 | L1 纯逻辑 | Go 原生，零依赖，最轻量 |

### 5.1 分层工具映射

```
L1 (纯逻辑/Domain)
  ├── 框架: testing (table-driven)
  └── 断言: testing.T (Errorf/Fatalf) — 无外部依赖

L2 (Handler/Middleware/Service)
  ├── 框架: Ginkgo + Gomega (Describe/Context/It)
  ├── Mock:  Mockery (testify/mock 兼容)
  ├── HTTP:  httptest.NewRecorder + gin.CreateTestContext
  └── 猴子补丁: gomonkey (仅 time.Now/os.Getenv 等必要场景)

L3 (Infra/Queue/Worker)
  ├── 框架: testing (集成 Docker 容器)
  ├── Mock:  Mockery
  └── 猴子补丁: gomonkey (预留)
```

### 5.2 Mockery — 主要 Mock 生成器

**安装 & 配置**:

```bash
go install github.com/vektra/mockery/v3@latest
```

项目根目录 `.mockery.yaml`:

```yaml
with-expecter: true          # 生成带 EXPECT() 的 expecter API
inpackage: true               # mock 生成到被测包内（避免循环导入）
filename: "mocks_test.go"     # mock 文件统一命名为 mocks_test.go（自动排除覆盖率）
mockname: "Mock{{.InterfaceName}}"
packages:
  github.com/luoxiaojun1992/data-agent/internal/infra/mongo:
    interfaces:
      UserRepository:
      AuditLogRepository:
      TaskRepository:
      SessionRepository:
      KnowledgeRepository:
      ArtifactRepository:
  github.com/luoxiaojun1992/data-agent/internal/infra/redis:
    interfaces:
      RedisClient:
  github.com/luoxiaojun1992/data-agent/internal/infra/qdrant:
    interfaces:
      QdrantClient:
  github.com/luoxiaojun1992/data-agent/internal/infra/seaweedfs:
    interfaces:
      SeaweedFSClient:
  github.com/luoxiaojun1992/data-agent/internal/infra/vault:
    interfaces:
      VaultClient:
  github.com/luoxiaojun1992/data-agent/internal/domain/agent:
    interfaces:
      LLMProvider:
```

**Makefile target**:

```makefile
gen-mocks:
	mockery
```

### 5.3 Ginkgo + Gomega — L2 Service 层测试框架

**安装**:

```bash
go install github.com/onsi/ginkgo/v2/ginkgo@latest
go get github.com/onsi/ginkgo/v2 github.com/onsi/gomega
```

**适用场景**: Handler、Middleware、Service 层（~32 个包），这些包的特点是：
- 调用链复杂（handler → service → repository）
- 需要分层组织测试（正常路径 / 错误路径 / 边界条件）
- 涉及异步操作（SSE 流、Redis Stream）

**典型 Ginkgo 测试结构** (以 `auth_service` 为例):

```go
package auth_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("AuthService", func() {
    var (
        svc  *auth.Service
        repo *mongo.MockUserRepository
    )

    BeforeEach(func() {
        repo = mongo.NewMockUserRepository(GinkgoT())
        svc = auth.NewService(repo)
    })

    Describe("Login", func() {
        Context("with valid credentials", func() {
            It("returns a JWT token", func() {
                repo.EXPECT().FindByUsername("admin").Return(validUser, nil)
                token, err := svc.Login(ctx, "admin", "correct-password")
                Expect(err).NotTo(HaveOccurred())
                Expect(token).NotTo(BeEmpty())
            })
        })

        Context("with incorrect password", func() {
            It("returns ErrInvalidCredentials", func() {
                repo.EXPECT().FindByUsername("admin").Return(validUser, nil)
                _, err := svc.Login(ctx, "admin", "wrong-password")
                Expect(err).To(MatchError(auth.ErrInvalidCredentials))
            })
        })
    })
})
```

**L1 层不强制 Ginkgo** — 纯逻辑/Domain 层继续使用 `testing` + table-driven，保持简洁。

### 5.4 gomonkey — 运行时函数替换（严格限制使用）

**安装**:

```bash
go get github.com/agiledragon/gomonkey/v2
```

**允许使用的场景**（且仅在无法通过接口 mock 解决时）：

| 场景 | 示例 | 备选方案 |
|------|------|---------|
| 时间依赖 | `gomonkey.ApplyFunc(time.Now, func() time.Time { return fixedTime })` | 接口注入 `Clock` interface（优先） |
| 环境变量 | `gomonkey.ApplyFunc(os.Getenv, func(string) string { return "test" })` | `t.Setenv()`（优先） |
| 包级私有函数 | `gomonkey.ApplyPrivateMethod(svc, "internalHelper", mockFunc)` | 重构为接口方法（优先） |
| 标准库函数无接口 | `gomonkey.ApplyFunc(json.Marshal, ...)` | 避免 — 极少数场景 |

**硬性约束**:

- ❌ 禁止用 gomonkey 绕过设计问题（凡是能通过 interface mock 的，绝不用 monkey patch）
- ❌ 禁止在并行测试中使用 gomonkey（gomonkey 不是线程安全的）
- ❌ 禁止在同一测试中同时 patch 超过 3 个函数
- ✅ 每次使用 `defer patches.Reset()` 确保清理
- ✅ 运行 gomonkey 测试时需加 `-gcflags=all=-l`（禁用内联优化）

**Makefile 中区分对待**:

```makefile
# 常规 UT（不含 gomonkey）
test-cover:
	go test -race -coverprofile=coverage.out ./internal/... ./skills/...

# 含 gomonkey 的 UT（禁用内联）
test-cover-monkey:
	go test -race -gcflags=all=-l -coverprofile=coverage-monkey.out ./internal/... ./skills/...
```

### 5.5 需要定义的 Mock Interface

Mockery 通过扫描源码中的 interface 自动生成 mock。以下为需要定义的核心接口：

| 接口名 | 定义位置 | 用途 |
|--------|---------|------|
| `UserRepository` | `internal/infra/mongo/` | Auth/User 相关 service 测试 |
| `AuditLogRepository` | `internal/infra/mongo/` | Audit service 测试 |
| `TaskRepository` | `internal/infra/mongo/` | Task service 测试 |
| `SessionRepository` | `internal/infra/mongo/` | Chat service 测试 |
| `KnowledgeRepository` | `internal/infra/mongo/` | Knowledge service 测试 |
| `ArtifactRepository` | `internal/infra/mongo/` | Artifact service 测试 |
| `RedisClient` | `internal/infra/redis/` | Queue/Worker/Session 测试 |
| `QdrantClient` | `internal/infra/qdrant/` | KB search 测试 |
| `SeaweedFSClient` | `internal/infra/seaweedfs/` | Artifact/Workspace 测试 |
| `VaultClient` | `internal/infra/vault/` | Agent engine 测试 |
| `LLMProvider` | `internal/domain/agent/` | Agent engine / Chat service 测试 |

### 5.6 测试辅助工具

- **`internal/testutil/`** — 通用测试 helper
  - `testutil/gin.go` — `gin.CreateTestContext()` 快捷工厂
  - `testutil/mock.go` — mock 对象组装 helper
  - `testutil/fixture.go` — 常用测试数据工厂（validUser、validTask 等）
- **Gomega matcher 扩展**（可选）：自定义 `HaveStatus(http.StatusOK)` 等业务 matcher

### 5.7 本地覆盖率检查

```makefile
# Makefile 新增 targets
.PHONY: test test-cover test-cover-html gen-mocks test-cover-check

gen-mocks:
	mockery

test-cover:
	go test -race -coverprofile=coverage.out ./internal/... ./skills/...
	go tool cover -func=coverage.out | grep total

test-cover-html:
	go test -race -coverprofile=coverage.out ./internal/... ./skills/...
	go tool cover -html=coverage.out -o coverage.html

test-cover-check:
	@go test -race -coverprofile=coverage.out ./internal/... ./skills/... || exit 1
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//') ; \
	echo "Coverage: $$COVERAGE%" ; \
	if [ $$(echo "$$COVERAGE < 98" | bc -l) -eq 1 ]; then \
		echo "ERROR: Coverage $$COVERAGE% below 98% threshold" ; \
		exit 1 ; \
	fi
```

## 6. CI Workflow — `.github/workflows/ut-workflow.yml`

### 6.1 设计

```
ut-workflow.yml:
  on: [push, pull_request] branches: [main]
  jobs:
    go-ut:
      - checkout
      - setup-go 1.25
      - gen-mocks (生成 interface mock)
      - go test -race -coverprofile=coverage.out ./internal/... ./skills/...
      - coverage check: 总覆盖率 < 98% → fail
      - 上传 coverage.out artifact
```

> **关键约束**: UT workflow 独立运行，不依赖 Docker（L1+L2 级别通过 mock 运行，L3 级别依赖 `docker-compose.ui-test.yml` 启动的容器）。优先保障 L1+L2 在无 Docker 环境下全通过。

### 6.2 Step-by-step

```yaml
name: Go Unit Tests

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  go-ut:
    runs-on: ubuntu-latest
    timeout-minutes: 30

    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Install Mockery
        run: go install github.com/vektra/mockery/v3@latest

      - name: Generate Mocks
        run: mockery

      # L1 + L2: 无外部依赖，纯逻辑 + mock 接口
      # -gcflags=all=-l 用于兼容 gomonkey（禁用内联优化）
      - name: Run L1+L2 Unit Tests (no external deps)
        run: |
          go test -race -gcflags=all=-l -coverprofile=coverage-l1l2.out \
            -coverpkg=./internal/logic/...,./internal/domain/...,./internal/config/...,./internal/api/...,./internal/service/...,./skills/... \
            ./internal/logic/... ./internal/domain/... ./internal/config/... ./internal/api/... ./internal/service/... ./skills/...

      - name: Check L1+L2 Coverage Threshold
        run: |
          COVERAGE=$(go tool cover -func=coverage-l1l2.out | grep total | awk '{print $3}' | sed 's/%//')
          echo "L1+L2 Coverage: ${COVERAGE}%"
          if (( $(echo "$COVERAGE < 98" | bc -l) )); then
            echo "ERROR: Coverage ${COVERAGE}% below 98% threshold for L1+L2 packages"
            exit 1
          fi
          echo "L1+L2 Coverage check passed"

      # L3: 需要 Docker 容器（MongoDB/Redis/Qdrant/SeaweedFS/Vault）
      - name: Start Docker services for L3 tests
        run: |
          docker compose -f docker-compose.ui-test.yml up -d --wait mongodb redis qdrant seaweedfs vault 2>&1

      - name: Run L3 Integration Tests
        run: |
          go test -race -coverprofile=coverage-l3.out \
            -coverpkg=./internal/infra/...,./internal/queue/...,./internal/worker/...,./internal/scheduler/... \
            ./internal/infra/... ./internal/queue/... ./internal/worker/... ./internal/scheduler/...

      - name: Check L3 Coverage Threshold
        run: |
          COVERAGE=$(go tool cover -func=coverage-l3.out | grep total | awk '{print $3}' | sed 's/%//')
          echo "L3 Coverage: ${COVERAGE}%"
          if (( $(echo "$COVERAGE < 98" | bc -l) )); then
            echo "ERROR: Coverage ${COVERAGE}% below 98% threshold for L3 packages"
            exit 1
          fi
          echo "L3 Coverage check passed"

      - name: Merge Coverage Profiles
        run: |
          go run golang.org/x/tools/cmd/cover -merge coverage-l1l2.out coverage-l3.out -o coverage-merged.txt || true
          # fallback: 分别检查 L1+L2 和 L3 各自达标即可
          echo "Coverage merge complete (L1+L2 and L3 checked independently)"

      - name: Teardown Docker services
        if: always()
        run: |
          docker compose -f docker-compose.ui-test.yml down --remove-orphans 2>/dev/null || true

      - name: Upload Coverage Report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: go-coverage-report
          path: |
            coverage-l1l2.out
            coverage-l3.out
          retention-days: 7
          if-no-files-found: warn
```

### 6.3 为什么拆 L1+L2 / L3

| 考量 | 分级方案 |
|------|---------|
| **快速反馈** | L1+L2 无 Docker，< 2min 跑完，覆盖最核心业务逻辑 |
| **隔离失败域** | L3 容器故障不影响 L1+L2 覆盖率判定 |
| **本地对齐** | 开发者本地跑 `make test-cover-check` 即对应 L1+L2 路径 |

### 6.4 CI Pipeline 合并后的完整门禁矩阵

| Workflow | 检查内容 | 耗时 | 依赖 |
|----------|---------|:---:|------|
| `lint-check.yml` | golangci-lint + govulncheck + SonarQube | ~15min | Go + SonarQube 容器 |
| `ut-workflow.yml` | Go UT 覆盖率 ≥ 98% | ~10min | Go (L3 需容器) |
| `ui-tests.yml` | Playwright E2E + Allure | ~30min | Docker Compose 全栈 |

> **所有三个 workflow 必须全部通过才允许合并 PR。**

## 7. 实施顺序

推荐按可测性从高到低推进，尽早看到覆盖率提升：

| 阶段 | 包范围 | 预计文件数 | 覆盖目标 |
|:---:|------|:---:|:---:|
| **S1** | Logic 层全部（sql/openapi/report/stats/workspace） | 5 包 | **L1 100%** |
| **S2** | Domain 层全部（model/task/skill/security等） | 8 包 | **L1 100%** |
| **S3** | Config + Middleware | 2 包 | **L1+L2 100%** |
| **S4** | Handler 层全部（7 handlers） | 7 包 | **L2 100%** |
| **S5** | Service 层（auth→audit→notification→task→knowledge→artifact→monitor→im→agent→chat→hermes→apireview） | 12 包 | **L2 100%** |
| **S6** | Skills 层（sql_executor/stats_engine/knowledge_search/save_report） | 4 包 | **L1+L2 100%** |
| **S7** | Infra 层（mongo/redis/qdrant/seaweedfs/vault） | 5 包 | **L3 98%** |
| **S8** | Queue + Worker + Scheduler | 3 包 | **L2+L3 98%** |
| **S9** | CI Workflow + Makefile targets | — | 覆盖率门禁可用 |

每个阶段完成后跑 `make test-cover-check` 验证覆盖率增量。

## 8. 注意事项

### 8.1 覆盖率的正确计量

- **`-coverpkg` 精确指定**: 避免将无关包计入总覆盖率分母，造成虚假指标
- **排除 `main.go`**: `cmd/server/main.go` 不计入，由集成/E2E 验证
- **排除 mock 文件**: `*_mock.go` / `mock_*.go` 文件不参与覆盖率计算

### 8.2 禁止形式化测试

以下测试视为无效，code review 必须拒绝：

- ❌ 仅调用函数但不验证返回值/副作用
- ❌ 空表驱动的 table-driven tests（无 case 的循环）
- ❌ `assert.True(t, true)` 凑覆盖率
- ❌ 依赖真实网络/数据库但无容器环境的测试（会 flaky）

### 8.3 与现有 CI 的兼容性

- `ut-workflow.yml` 与 `lint-check.yml`、`ui-tests.yml` **并行运行**，互不阻塞
- PR 合并门禁要求三者全部 ✅
- 本地开发：`make test-cover-check` 跑 L1+L2，快速反馈 < 2min

### 8.4 Package 修改限制

- Infra 层（L3 包）添加测试可能需要提取 interface 以便 mock（小范围重构）
- 重构必须以接口抽取为主，**不允许改变业务行为**
- 每个 L3 包的 interface 抽取需先写测试验证原始行为不变，再替换为接口

## 9. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No（测试代码仅在 `_test.go`） |
| 是否需要新外部服务 | No（L3 复用已有 Docker Compose 容器） |
| 是否需要新增 Skill | No |
| Go 工具链就绪 | ✅ Go 1.25 + `go test -race -coverprofile` 原生支持 |
| Mock 工具 | ✅ **Mockery v3** — Go 社区首选（7k+ stars），testify 兼容，泛型支持 |
| 测试框架 L1 | ✅ Go 原生 `testing` + table-driven，零依赖 |
| 测试框架 L2 | ✅ **Ginkgo v2.28+ + Gomega** — BDD 风格，适合复杂 Service 层 |
| Monkey 补丁 | ✅ **gomonkey v2** — 严格限制使用，仅 edge case |
| L1+L2 是否无需 Docker | ✅ 通过 Mockery mock 接口，纯 Go 运行（含 gomonkey 需 `-gcflags=all=-l`） |
| L3 是否可借助已有容器 | ✅ `docker-compose.ui-test.yml` 已含 MongoDB/Redis/Qdrant/SeaweedFS/Vault |

## 10. 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|:---:|
| `internal/**/_test.go` | UT 测试文件（~50+ 包） | **新建** |
| `internal/testutil/` | 通用测试 helper | **新建** |
| `.mockery.yaml` | Mockery 配置文件 | **新建** |
| `internal/**/mocks_test.go` | Mockery 生成的 mock 文件 | **新建（自动生成）** |
| `internal/**/interface.go` | Mock interface 定义（L2/L3 包） | **新增/小幅** |
| `.github/workflows/ut-workflow.yml` | CI UT workflow | **新建** |
| `Makefile` | 新增 `test-cover`/`test-cover-html`/`test-cover-check`/`gen-mocks` targets | **小幅** |
| `go.mod` | 新增依赖: mockery, ginkgo, gomega, gomonkey | **小幅** |

## 11. 验证标准

1. ✅ `go test -race -coverprofile=coverage.out ./internal/... ./skills/...` 总覆盖率 ≥ 98%
2. ✅ L1+L2 层覆盖率 = 100%（纯逻辑 + mock 接口）
3. ✅ L3 层覆盖率 ≥ 98%（集成 + 容器）
4. ✅ `.github/workflows/ut-workflow.yml` 在 PR 时自动触发并通过
5. ✅ `make test-cover-check` 本地覆盖率检查可用
6. ✅ 无形式化凑覆盖率测试（code review 把关）
7. ✅ 所有 mock interface 通过 Mockery 统一生成（`mockery` + `.mockery.yaml`），不手写
8. ✅ gomonkey 仅在文档标注的许可场景中使用，code review 严格把关
