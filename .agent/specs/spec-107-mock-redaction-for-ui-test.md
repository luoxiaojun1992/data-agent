# UI 测试用 mock-redaction 替换 presidio（类似 mock-llm）

> **SPEC-107** | Status: 📐 立项（设计已深化，待排期实现）

## 1. 目标

新增一个轻量级 `mock-redaction` 服务（类似 `mock-llm`），在 **UI 测试环境**（`docker-compose.ui-test.yml`）中替换真实 presidio（analyzer + anonymizer）容器，消除 presidio 容器「gunicorn sync worker 偶发卡 boot」导致的 UI Tests 反复失败、重跑不可靠的问题。生产环境（`docker-compose.yml`）继续使用真实 presidio，不受影响。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-043 mock 模型服务 | ✅ | mock-llm 的既有模式（独立 Go 服务 + Dockerfile + compose 服务），本 spec 复用该模式 |
| SPEC-068 KB PII 脱敏 | ✅ | 后端 `PIIRedactor` 的调用契约（/analyze + /anonymize）是本 spec 兼容目标 |
| SPEC-093 Chat 输入框脱敏 | ✅ | 同上，输入脱敏走同一 `PIIRedactor` |
| SPEC-079 在线指示灯 | ✅ | `Health` 探针依赖 presidio /health，替换后需同步 mock-redaction /health |

> 无阻塞项。后端代码无需改动（mock-redaction 完全兼容 presidio 接口契约）。

## 2. 背景

- **现状**：UI 测试环境启动真实 presidio（`mcr.microsoft.com/presidio-analyzer:2.2.362` + `presidio-anonymizer:2.2.362`），提供 PII 脱敏能力（检测 + `<PII>` 替换）。
- **问题**：presidio 镜像（gunicorn sync worker，non-root UID 1001）在 CI runner 上**偶发 worker 卡 boot**（主进程 Listening 但 worker 永不 boot）。2026-11-20~21 实测卡 boot 概率约 80%（4 卡 / 1 成），导致 `depends_on: service_healthy` 永远等不到，UI Tests 超时失败。
- **已尝试的缓解**（均未根治）：pin 镜像 2.2.362、restart: unless-stopped、延长健康检查到 10min、healthcheck 失败计数 + `kill -9 1`（脚本正确但容器内未生效，根因需 docker 本地环境定位）。
- **根本矛盾**：UI 测试**不需要**真实的 spaCy NER 脱敏能力，只需要「确定性」的 PII 检测 + `<PII>` 替换来验证后端 `PIIRedactor` 链路。真实 presidio 的「重模型 + 偶发卡死」是纯负担。

## 3. 架构概述

```
UI 测试环境（现状）:
  data-agent --PRESIDIO_ANALYZER_URL--> presidio-analyzer (spaCy NER, 偶发卡 boot)
  data-agent --PRESIDIO_ANONYMIZER_URL--> presidio-anonymizer (gunicorn)

UI 测试环境（目标）:
  data-agent --PRESIDIO_ANALYZER_URL--> mock-redaction /analyze  (纯正则, 确定性)
  data-agent --PRESIDIO_ANONYMIZER_URL--> mock-redaction /anonymize (纯正则替换)
```

- 后端 `internal/service/pii/redactor.go` 的 `PIIRedactor` **完全不改**：它只依赖 HTTP 接口（POST /analyze、POST /anonymize、GET /health），mock-redaction 实现相同契约即可无缝替换。
- 生产环境 `docker-compose.yml` 保留真实 presidio，两条配置互不影响。

## 4. API 设计（mock-redaction 对外接口，兼容 presidio）

| Method | Path | 说明 |
|--------|------|------|
| POST | `/analyze` | 请求 `{"text":"...","language":"en"}` → 响应 `[{"entity_type":"...","start":N,"end":M,"score":0.9}]` |
| POST | `/anonymize` | 请求 `{"text":"...","analyzer_results":[...],"anonymizers":{"DEFAULT":{"type":"replace","new_value":"<PII>"}}}` → 响应 `{"text":"脱敏后","items":[]}` |
| GET | `/health` | 返回 200 |

> 契约与 `redactor.go` 的 `analyze`/`anonymize`/`healthEndpoint` 逐字段对齐（`analyzeResult`、`anonymizeResponse` 结构）。

## 5. 详细设计

### 5.1 服务结构（复用 mock-llm 模式）

```
mockredaction/
  go.mod
  main.go        # Go net/http 服务，实现 /analyze /anonymize /health
  Dockerfile     # 多阶段构建，alpine 运行，EXPOSE 8083
```

### 5.2 检测规则（纯正则，确定性，复用 docker/presidio/custom_recognizers.yaml 的规则）

| entity_type | 正则 | 来源 |
|-------------|------|------|
| PHONE_NUMBER | `(?<!\d)1[3-9]\d{9}(?!\d)` | CnPhoneRecognizer |
| CN_ID_CARD | `(?<!\d)\d{17}[\dXx](?!\d)` | CnIdCardRecognizer |
| CN_BANK_CARD | `(?<!\d)\d{16,19}(?!\d)` | CnBankCardRecognizer |
| CN_USCC | `(?<![0-9A-HJ-NPQRTUWXY])[0-9A-HJ-NPQRTUWXY]{18}(?![0-9A-HJ-NPQRTUWXY])` | CnUsccRecognizer |
| EMAIL_ADDRESS | 标准邮箱正则 | 内置 |
| IP_ADDRESS / URL / CREDIT_CARD / IBAN | 内置简单规则（可选，覆盖内置 recognizer） | 内置 |

- `/analyze`：对 text 逐规则匹配，返回 `entity_type` + `start`/`end`（按 rune 索引）+ `score`（固定 0.9）。
- `/anonymize`：按 `analyzer_results` 的区间，用 `DEFAULT.new_value`（默认 `<PII>`）替换，返回 `text`。
- 边界：用 `(?<!\d)...(?!\d)` lookaround（与 custom_recognizers.yaml 一致，因 Python `\b` 对 CJK 不生效）。

### 5.3 部署（docker-compose.ui-test.yml）

- 删除 `presidio-analyzer` / `presidio-anonymizer` 两个服务。
- 新增 `mockredaction` 服务（单容器，EXPOSE 8083，提供 /analyze + /anonymize + /health）。
- `data-agent` 环境变量改为 `PRESIDIO_ANALYZER_URL=http://mockredaction:8083` + `PRESIDIO_ANONYMIZER_URL=http://mockredaction:8083`（同一服务暴露两个「逻辑」端点，或拆 `/analyze`/`/anonymize` 路径前缀，后端 URL 前缀适配）。
- `data-agent` 的 `depends_on` 去掉 `presidio-analyzer`/`presidio-anonymizer`，改为 `mockredaction: service_healthy`。
- `docker/presidio/` 目录与 `custom_recognizers.yaml` 保留（生产仍在用）。

> 关键：`PRESIDIO_ANALYZER_URL` 与 `PRESIDIO_ANONYMIZER_URL` 后端拼的是 `{URL}/analyze` 和 `{URL}/anonymize`，若 mock-redaction 是单服务，两个 URL 需指向同 host 但不同 path 前缀（如 `http://mockredaction:8083/analyzer` 与 `.../anonymizer`，服务端拆前缀），或直接用两个虚拟端点。实现时二选一并写进 spec 细节。

### 5.4 是否保留 presidio 生产镜像

- `docker-compose.yml`（生产）**不改**，继续用 `mcr.microsoft.com/presidio-*:2.2.362`。
- 本 spec 只改 UI 测试配置，不触碰生产脱敏能力。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（mock-redaction 纯内存/正则，无持久化） |
| 是否影响现有 API | No（后端 `PIIRedactor` 与 presidio HTTP 契约不变，仅换 URL 指向） |
| 性能影响 | 正向（去掉 spaCy 模型加载，启动从「分钟级 + 偶发卡死」降到「毫秒级」） |
| 是否需要新增 Skill | No |
| 是否影响生产 | No（只改 docker-compose.ui-test.yml，生产保留真实 presidio） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `mockredaction/main.go` | 新 Go 服务，实现 /analyze + /anonymize + /health | New |
| `mockredaction/Dockerfile` | 多阶段构建，alpine 运行 | New |
| `mockredaction/go.mod` | 模块定义 | New |
| `docker-compose.ui-test.yml` | 删除 presidio 服务，新增 mockredaction，改 data-agent 环境变量 + depends_on | 中 |
| `internal/service/pii/redactor.go` | **不改**（mock-redaction 兼容契约） | 无 |
| `docker-compose.yml` | 不改（生产保留 presidio） | 无 |

## 8. 测试策略

1. **Unit tests**（Go）: `mockredaction/main.go` 的 analyze/anonymize 逻辑——正则检测（手机号/身份证/银行卡/USCC 命中、`(?<!\d)` 边界）、`<PII>` 替换（区间正确、多 PII 场景）。L1 纯逻辑 100%。
2. **Integration tests**: 不涉及（mock-redaction 是独立服务，靠 E2E 覆盖）。
3. **E2E tests**: 现有 `[UI-185]`（输出脱敏）与 chat-redact 套件**不改断言**，仅验证换 mock-redaction 后链路仍绿（手机号/身份证 → `<PII>`）。`online-indicator.spec.ts` 的 presidio 探活项需同步（若该 spec 有 presidio status 断言）。
4. **审计**: 使用 `.agent/skills/go-ut-audit` 审查 mock-redaction 的 UT 质量。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 保持 `[UI-185] 输出敏感信息脱敏` 通过（验证 mock-redaction 替代 presidio 后，手机号/身份证仍被 `<PII>` 替换）
- [ ] **必须** chat-redact 套件（UI-219~223）不回归
- [ ] **必须** CI Pipeline 中 ui-tests 通过，且**不再出现 presidio worker 卡 boot 导致的失败**
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/正则检测/替换逻辑 | **100%** | `mockredaction/main.go` 的 detect/anonymize |

### 断言质量要求

- [ ] **必须** 每个检测规则至少 1 个命中 + 1 个不命中用例（含 `(?<!\d)` 边界：`x13812345678` 不应命中手机号）
- [ ] **必须** 替换验证 `start/end` 区间正确、多 PII 场景全替换
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证实际结果

### CI 门禁

- [ ] `go test -gcflags=all=-l -coverprofile=coverage.out ./mockredaction/...` 通过，覆盖率 ≥ 98%
- [ ] `go vet` 无警告

参考: `.agent/specs/spec-045-go-service-ut.md`、`.agent/skills/go-ut-audit/SKILL.md`

## 10. 验证标准

1. UI 测试环境启动后，`mockredaction` 健康检查快速 Healthy（秒级，无 spaCy 加载）。
2. `[UI-185]` 通过，手机号 `13812345678`、身份证 `320123199001011234` 均被替换为 `<PII>`。
3. UI Tests 连续 N 次运行（N≥3）**零 presidio 卡 boot 失败**（根因消除）。
4. 生产 `docker-compose.yml` 不受影响，生产脱敏能力不变。
5. `go test ./mockredaction/...` 覆盖率 ≥ 98%，CI 全绿（Lint / Go UT / UI Tests / License Scan）。
