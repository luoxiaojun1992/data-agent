# compaction 触发机制缺陷修复：token 估算补全 + tool 链配对保护

> **SPEC-069** | Status: 设计中（待拍板方案）

## 目标

记录并修复 compaction 在 SPEC-067 落地后的测试中暴露的两个缺陷：

1. `estimateEventTokens` 只统计文本，漏算 tool 调用内容，导致 token 阈值失真；
2. 压缩边界可能切在 tool 链配对中间，破坏 FunctionCall/FunctionResponse 关联。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-066 配置存储拆分 | ✅ 已实现 | compaction `MaxTokensFn` 依赖 model config 的 `context_len` |
| SPEC-067 意图识别 + 相关性检查 | ✅ 已实现 | `shouldCompact` 收敛为 user + FunctionResponse 触发，本 spec 的两个问题由此触发 |

## 背景 / 动机

### 问题 1：`estimateEventTokens` 只统计文本，漏算 tool 调用内容

- **位置**：`internal/adk/session/mongo.go` 的 `estimateEventTokens`
- **现状**：仅累加 `p.Text` 长度 `/3`，不统计 `FunctionCall.Name/Args`（SQL 查询、搜索关键词）和 `FunctionResponse.Response`（工具返回结果）。
- **实测证据**（2026-08-23）：一个纯 tool 链 session 共 23 条 events（几乎全是 FunctionCall/Response，`text` 长度均为 0），token 估算仅约 **255**（几乎全靠最终回复文本 729 字符），而实际上下文（含多轮 SQL / 知识库 / 记忆 / 外部 API 搜索）远大于此。
- **影响**：`MaxTokens` 阈值（`context_len/2`）在 tool 密集场景形同虚设，compaction 几乎只能靠 `MaxEvents` 触发，无法在 token 维度及时压缩。

### 问题 2：压缩边界可能切在 tool 链配对中间

- **位置**：`internal/adk/session/mongo.go` 的 `maybeCompact`（`cut = len(doc.Events) - KeepRecent`）
- **现状**：压缩把前 `len-KeepRecent` 条合并成 summary，可能把某个 `FunctionCall` 压掉而保留其后的 `FunctionResponse`（或反之），导致 ADK 报 `no function call event found for function responses ids`。
- **实测证据**（2026-08-23）：`KeepRecent=1` 时稳定复现该错误；正常 `KeepRecent=20` 概率低但非零。
- **影响**：`shouldCompact` 的「tool 输出（FunctionResponse）触发 compaction」与「tool 链配对完整性」存在边界耦合——触发时机恰在 tool 链进行中，压缩可能破坏尚未完成的配对。

## 详细设计（修复方向，待拍板）

### 问题 1：token 估算补全

`estimateEventTokens` 增加对非文本 part 的估算：

- `FunctionCall`：统计 `Name` + `Args`（`json.Marshal` 后长度 `/3`）；
- `FunctionResponse`：统计 `Response`（`json.Marshal` 后长度 `/3`，或统计 `Parts` 内联数据）；
- 保持 `len/3` 的轻量估算口径，不引入 tokenizer 依赖。

### 问题 2：压缩边界与 tool 链配对（三选一，待晓军拍板）

- **方案 A：压缩边界对齐配对** —— `cut` 前移到最近的 `FunctionCall` 之前，保证不把一对 Call/Response 拆开。
- **方案 B：触发时机避开 tool 链** —— 仅在「一轮完整回复结束」（最终 assistant 文本 / user 消息）后触发，而非每个 FunctionResponse 后。
- **方案 C：KeepRecent 动态下限** —— 保证 `KeepRecent ≥ 当前进行中 tool 链长度`，避免切在链中间。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | `estimateEventTokens` 增加 JSON 序列化开销，可接受（仅触发时计算） |
| 是否需要新增 Skill | No |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/session/mongo.go` | `estimateEventTokens` / `maybeCompact` | Modify |

## 测试策略

1. **Unit tests**（Go）: `estimateEventTokens` 补全后的行为断言（含 FunctionCall/Response 的 token 估算）；`maybeCompact` 边界对齐逻辑（如方案 A）的配对保护断言。
2. **Integration / E2E**：临时参数（`MaxEvents`/`KeepRecent` + model `context_len`）复现 tool 链场景，验证不再出现 `no function call event found`。

## UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 若影响前端展示（compaction 摘要/消息），同步更新对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试

## Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

- [ ] `estimateEventTokens` / `maybeCompact` 相关新增/修改逻辑的 UT 覆盖率达标（L1 纯逻辑 100%）
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景（如确实不可行，需文档注释说明原因）

## 验证标准

- [ ] `estimateEventTokens` 覆盖 `FunctionCall.Args` / `FunctionResponse.Response`
- [ ] tool 密集场景 token 估算接近真实（单测断言）
- [ ] `KeepRecent` 极小值下不复现 `no function call event found`
- [ ] 现有 compaction 端到端测试仍通过

## 提交约定

```bash
git add .agent/specs/spec-069-compaction-trigger-fixes.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-069 compaction trigger fixes (token 估算 + tool 链配对)"
```
