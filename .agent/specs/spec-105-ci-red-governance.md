# CI 红灯治理专项（Lint / Go Unit Tests / UI Tests 长期失败修复）

> **SPEC-105** | Status: 📐 立项（根因已定位，待展开实现）
> 日期：2026-09-28 立项

## 0. 背景 / 基线证据

`main` 分支 CI 共 4 个 job，其中 **3 个长期红灯**，自 2026-09-15（`fc41291`）起持续至今，仅 License Scan 一直绿：

| Job | 状态 | 首次红灯 | 根因 |
|---|---|---|---|
| License Scan (ScanCode) | 🟢 绿 | — | — |
| Lint Check | 🔴 红 | 09-15 起 | golangci-lint 报既有文件问题（30 条） |
| Go Unit Tests | 🔴 红 | 09-15 起 | 覆盖率 71.3% < 98% 门槛（4 个 0% 包） |
| UI Tests | 🔴 红 | 09-15 起 | `online-indicator.spec.ts` 缺分号，Playwright 整套件编译失败 |

> 关键结论：**这三个红灯均为既有负债，与 SPEC-104（Task Run 删除与会话并发治理，commit `d119c75`）无关**。SPEC-104 自身代码/测试全绿（`go build` / `go vet` / `go test -gcflags=all=-l` 通过，新增/改动代码 100% 函数覆盖），且合并前后红灯集合完全一致。

## 1. 三项负债根因明细

### 1.1 Lint Check（golangci-lint，30 条错误，全为既有文件）

按文件分布：

| 文件 | 条数 | 类别 |
|---|---|---|
| `internal/adk/modelcfg/provider.go` | 6 | unused func / deadcode |
| `cmd/server/migration/rbac_seed.go` | 4 | 待定（需展开时确认） |
| `internal/infra/metrics/metrics_test.go` | 3 | SA1012 nil Context |
| `internal/api/handler/rbac.go` | 3 | SA5008 duplicate struct tag `json` |
| `internal/adk/tools/tools.go` | 3 | S1016 struct literal 可转类型转换 |
| `internal/infra/mongo/rbac_repository.go` | 2 | 待定 |
| `internal/infra/mongo/converter.go` | 2 | unused func（taskToDoc/docToTask） |
| `internal/api/handler/voice_integration_test.go` | 2 | ineffassign |
| `internal/service/apicollection/service.go` | 1 | 待定 |
| `internal/scheduler/scheduler.go` | 1 | unused func（nextCronTime） |
| `internal/logic/webfetch/fetch_test.go` | 1 | 待定 |
| `internal/api/handler/api_collection.go` | 1 | 待定 |
| `internal/adk/modelcfg/provider_test.go` | 1 | unused func（newProviderWithRepo） |

> 所有错误均落在**既有文件**，SPEC-104 新增/改动的 8 个文件（`infra/redis/lock.go`、`service/task/service.go`、`logic/agent/executor.go`、`service/chat/session.go`、`service/chat/chat_service.go`、`api/handler/task.go`、`api/handler/session.go`、`infra/mongo/session_repository.go`）**零条 lint 错误**。

### 1.2 Go Unit Tests（覆盖率 71.3% < 98%）

CI `ut-workflow.yml` 的 `-coverpkg` 覆盖 `internal/api|config|domain|logic|service` 全部包，98% 门槛按 **total** 计算。被 4 个**完全没有测试文件**的历史包拉低：

| 包 | 覆盖率 | 说明 |
|---|---|---|
| `internal/domain/modelconfig` | 0% | 无 `_test.go` |
| `internal/service/apicollection` | 0% | 无 `_test.go` |
| `internal/service/feishu` | 0% | 无 `_test.go` |
| `internal/service/pii` | 0% | 无 `_test.go` |

> SPEC-104 自身所有新增/改动代码（`lock.go`、`DeleteRun`、`acquireRunLock`、`acquireSessionLock`、`chatSourceFilter`、写挡读放、`Messages` 前置）均已 100% 函数覆盖；total 71.3% 的缺口主要来自上述 4 个 0% 包，为历史覆盖率负债。

### 1.3 UI Tests（Playwright 套件编译失败）

```
ui-e2e-1 | SyntaxError: /workspace/online-indicator.spec.ts: Missing semicolon. (18:23)
```

- 文件 `tests/ui/online-indicator.spec.ts` 第 18 行缺分号，导致整套 Playwright 用例无法编译 → 全量 UI 测试失败。
- 该文件属 **SPEC-079（全局在线指示灯）**，最近提交 `46fe083`（UI-249 回归用例）；非本 SPEC-104 变更范围。
- 连带债务：SPEC-103 已记录的「存量 E2E 套件仍调 SPEC-084 已删除的 `/auth/register`（404）」，需在本次治理中一并迁移到预置用户模式。

## 2. 目标

让 `main` 分支 CI 三个红灯 job 全绿，恢复「CI 通过 = 可合并」的基线（对齐最高红线：禁止绕过 CI）。

## 3. 验收标准

| Job | 验收 |
|---|---|
| Lint Check | golangci-lint 0 错误（30 条既有问题清零，或经晓军确认将死代码类合理移除/注释） |
| Go Unit Tests | `-coverpkg` total ≥ 98%（4 个 0% 包补测试，或经晓军确认调整 coverpkg 排除列表） |
| UI Tests | Playwright 套件可编译并全量通过（修 `online-indicator.spec.ts` 语法错误 + 迁移 `/auth/register` 存量用例） |

## 4. 范围与排期说明（立项不展开）

> 本 spec 仅**立项**，暂不展开详细设计。待晓军排期后，再按以下待展开项逐项评估：

- 待展开项（后续）：
  1. Lint：30 条错误的逐条修复方案（死代码删除 vs 合理保留 + `//nolint` 标注），以及是否收紧/放宽 `.golangci.yml` 规则集。
  2. 覆盖率：4 个 0% 包的测试补齐范围与工作量评估，或与晓军确认 `-coverpkg` 是否排除这些历史包（需权衡「假性提高覆盖率」与「合理缩小 gate 范围」）。
  3. UI Tests：`online-indicator.spec.ts` 语法修复 + 存量 E2E 的 `/auth/register` → 预置用户模式迁移（承接 SPEC-103 遗留债务）。
  4. 修复后回归验证：三个 job 在 CI 全绿，且不回归 SPEC-104 等已实现功能。

## 5. 提交约定

```bash
git add .agent/specs/spec-105-ci-red-governance.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-105 CI red-light governance (立项)"
```
