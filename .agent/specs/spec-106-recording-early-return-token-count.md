# 修复 recordingLLM 消费者提前 return 导致 token 计数漏记（enhance 调用不计入 llm_calls）

> **SPEC-106** | Status: 设计中

## 1. 目标

修复 `internal/adk/model/recording.go` 中 `recordingLLM.GenerateContent` 的 fallback 计量逻辑：当消费者（如 enhance 的 `enhanceViaADK`）在收到首个响应后提前 `return` 时，generator 的 fallback `Record` 被跳过，导致 **enhance 调用从不计入 `llm_calls` / `token_tokens` 统计**。目标：保证所有走 `recordingLLM` 的 LLM 调用无论消费者是否提前终止迭代，都被完整计量。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-048 | ✅ | Google ADK 引擎层（recordingLLM 属 ADK model 边界） |
| SPEC-051 | ✅ | LLM 全链路 Token 统计与 Redis 缓存（llmstats 计量体系） |
| SPEC-072 | ✅ | Dashboard 统计重构（stats_hourly 小时计数 + metrics.Counter） |
| SPEC-100 | ✅ | Token 用量埋点（session 维度双写） |

> 无阻塞项，可立即开始。

## 2. 背景

**现象**：`tests/ui/prompt.spec.ts` 的 `[UI-160] 增强调用计入 Token 统计` 断言失败——enhance 调用后 `/dashboard` 返回的 `llm_calls` 不递增（`before=119` 轮询 20s 仍为 119）。

**根因**（已通过 mockllm 日志确认 enhance LLM 调用确实发生）：

`recordingLLM.GenerateContent` 返回一个 generator 闭包，其结构如下：

```go
return func(yield func(*model.LLMResponse, error) bool) {
    var completion strings.Builder
    recorded := false
    for resp, err := range r.inner.GenerateContent(ctx, req, stream) {
        if resp != nil {
            if um := resp.UsageMetadata; um != nil && !recorded {
                _ = r.rec.Record(...) // 有 UsageMetadata 才记录
                recorded = true
            }
            // 累加 completion 文本
        }
        if !yield(resp, err) {
            return // ← 消费者提前终止 → 直接 return，跳过下方 fallback
        }
    }
    if !recorded {
        _ = r.rec.Record(...) // ← fallback（估算），被提前 return 跳过
    }
}
```

而 enhance 的消费者 `enhanceViaADK`（`internal/service/enhance/service.go`）在收到**首个**响应后即提前 `return`：

```go
for resp, err := range llm.GenerateContent(ctx, adkReq, false) {
    if err != nil { return prompt }
    if resp.Content != nil && len(resp.Content.Parts) > 0 {
        return resp.Content.Parts[0].Text // ← 提前 return
    }
}
```

当后端（mockllm / Ollama 等）**不返回 UsageMetadata**（即不触发上方 `Record`）时，消费者提前 `return` 会让 `yield` 返回 `false`，generator 在 `if !yield(...) { return }` 处提前退出，**fallback `Record` 永不执行** → enhance 调用的 `token_tokens` + `llm_calls` 均漏记。

**影响范围**：enhance 是唯一「流式 false + 消费者提前 return」的调用点。其余调用点（chat/task 的 run、KB chunking/image、intent/relevance、compaction）均迭代到完成，fallback 正常执行，计量不受影响。

## 3. 架构概述

不涉及架构变更。仅修正 `recordingLLM.GenerateContent` 内部计量时机，使其与消费者的迭代终止方式解耦。

| 模块 | 现状 | 变更 |
|------|------|------|
| `internal/adk/model/recording.go` | fallback Record 在 `for` 循环之后 | 改为 `defer` 闭包，保证正常退出与提前 return 均记录 |
| `internal/service/enhance/service.go` | 首响应提前 return | 不变 |

## 4. 详细设计

将 fallback `Record` 从「`for` 循环之后的普通语句」改为「generator 函数入口的 `defer` 闭包」。`defer` 在函数返回（无论正常走完循环、还是 `if !yield { return }` 提前退出）时都会执行；闭包内判断 `!recorded` 才补记，语义与现 fallback 完全一致。

```go
return func(yield func(*model.LLMResponse, error) bool) {
    var completion strings.Builder
    recorded := false
    defer func() {
        if !recorded {
            _ = r.rec.Record(ctx, llmstats.Record{
                CallPoint:        r.callPoint,
                PromptTokens:     estPrompt,
                CompletionTokens: llmstats.EstimateTokens(completion.String()),
                Multiplier:       1.0,
                Estimated:        true,
                SessionID:        sessionID,
            })
        }
    }()
    for resp, err := range r.inner.GenerateContent(ctx, req, stream) {
        if resp != nil {
            if um := resp.UsageMetadata; um != nil && !recorded {
                _ = r.rec.Record(ctx, llmstats.Record{ /* 真实 usage */ })
                recorded = true
            }
            // 累加 completion 文本
        }
        if !yield(resp, err) {
            return
        }
    }
}
```

要点：
- `defer` 闭包捕获 `recorded` / `completion` 变量（Go 闭包按引用捕获），在返回时读取最终值。
- 有 UsageMetadata 时 `recorded=true`，defer 不重复记录；无 UsageMetadata（mockllm/Ollama）时 defer 补记估算值。
- 提前 return 时 `completion` 已累加到「首响应文本」，估算的 CompletionTokens 正确。

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `stats_hourly` + `session_stats`） |
| 是否影响现有 API | No（纯内部计量时序修正，无接口变更） |
| 性能影响 | 无（`defer` 闭包零额外开销；计量仍是 buffered Counter O(1)） |
| 是否需要新增 Skill | No |
| 是否影响其它调用点 | 无副作用——chat/task/kb/intent/relevance 走完循环，defer 行为与原 fallback 等价；仅 enhance 从「漏记」变「正确记录」 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/model/recording.go` | recordingLLM fallback 计量改 `defer` | 小（约 10 行） |
| `internal/adk/model/recording_test.go` | 新增「消费者提前 return 仍计量」用例 | 中 |
| `tests/ui/prompt.spec.ts` | 恢复 `[UI-160] 增强调用计入 Token 统计`（当前已删除） | 中 |

## 7. 测试策略

1. **Unit tests**（Go）: `recording_test.go` 新增用例——构造一个「首响应后停止迭代」的消费者（`for range` 内 `return`），注入无 UsageMetadata 的 fake inner LLM + 记录用 Recorder，断言 `Record` 被调用一次且 `Estimated=true`、`PromptTokens` 正确。覆盖提前 return 与正常走完两种路径。
2. **Integration tests**: 不涉及（纯单测 + 现有 E2E）。
3. **E2E tests**: 恢复 `tests/ui/prompt.spec.ts` 的 `[UI-160]`，断言 enhance 后 `/dashboard` 的 `llm_calls` 递增。
4. **审计**: 使用 `.agent/skills/go-ut-audit` 审查新增 UT 质量。

## 8. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 恢复 `tests/ui/prompt.spec.ts` 的 `[UI-160]` 增强调用计入 Token 统计用例（当前 commit d6fd772 因后端 bug 删除）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | — |
| L2 | 依赖接口，可 mock | **100%** | `internal/adk/model`（recordingLLM 依赖 model.LLM 接口，可 mock） |

### 断言质量要求

- [ ] **必须** 提前-return 用例至少验证：`Record` 被调用 1 次、`Estimated=true`、`PromptTokens`/`CompletionTokens` 值正确（≥2 行为断言）
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证实际结果

### CI 门禁

- [ ] `go test -gcflags=all=-l -coverprofile=coverage.out ./internal/...` 通过（gomonkey 需 `-gcflags=all=-l`，不用 `-race`）
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）

参考:
- `.agent/specs/spec-045-go-service-ut.md` — Go UT 全覆盖 spec
- `.agent/skills/go-ut-audit/SKILL.md` — UT 审计 skill
- `.github/workflows/ut-workflow.yml` — CI UT workflow

## 10. 验证标准

1. enhance 调用后 `/dashboard` 的 `llm_calls` 与 `token_tokens` 均 +1（`[UI-160]` 恢复后通过）。
2. 新增 UT 通过，覆盖率不下降（`internal/adk/model` 包 ≥ 98%）。
3. 其余调用点（chat/task/kb/intent/relevance）计量值不受影响（回归 UT 通过）。
4. CI `sonar-check` + `ui-tests` + `ut-workflow` 全绿。
