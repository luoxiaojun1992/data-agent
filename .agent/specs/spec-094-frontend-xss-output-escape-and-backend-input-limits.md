# 前端 XSS 输出转义组件 + 后端输入限制校验补齐

> **SPEC-094** | Status: 📐 立项（暂不深化，稍后调查完善，定稿后再实现）

> **术语红线**：**输入校验（validate）≠ 输出转义（escape）**。本 spec 的立场：
> 后端只补**结构性限制**（文本长度、图片数量/大小），XSS 校验后端**不做**；
> XSS 防护由**前端输出转义组件**统一承担（渲染任何后端/LLM 返回文本前转义）。

## 1. 目标

1. 前端新增 **XSS 转义组件**（如 `SafeText` / `escapeHtml` 工具函数），用于所有
   后端/LLM 返回内容的**输出转义**（暂不深化，待调查 Markdown 渲染管线后定稿）。
2. 调查并补齐后端输入限制缺口：**KB、chat、task** 三条链路的文本长度、图片
   数量/大小校验必须**后端兜底**（不能只靠前端限制）。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-077 | ✅ | chat 附件/PDF 限制（前端 100KB/5 图/2MB/PDF 20MB）已就绪，后端已有部分 |
| SPEC-081 | ✅ | `security.ValidateXSS` 与 KB 限制常量（MaxKBTitleRunes=200 / MaxKBTextBytes=5MB / MaxKBImageBytes=1MB / MaxKBImageCount=10）已就绪 |
| SPEC-084 | ✅ | task 创建校验（title/description ValidateXSS，e486197）已就绪 |
| — | — | 无阻塞项；立项阶段不实现 |

## 2. 背景（现状调查，2026-09-10 快查）

后端校验现状盘点：

| 模块 | 文本长度 | 图片数量/大小 | XSS | 备注 |
|------|---------|--------------|-----|------|
| KB | ✅ 标题 200 runes 截断 + 正文 5MB | ✅ 10 张 / 1MB/张 | ✅ 标题 ValidateXSS | `handler/knowledge.go` + `logic/webimport` |
| Chat | ✅ 100KB 合并（`validateChatTextSize`） | ✅ `ValidateImages`（5 张 / 2MB/张 / 总量 5MB / MIME 白名单） | ✅ 用户提示词 ValidateXSS | `domain/chat/image.go` + `service/chat` |
| Task | ❓ **文本长度待查** | ✅ 复用 `ValidateImages`（domain 共用） | ✅ title ValidateXSS | `handler/task.go:54`；message/params 长度上限待确认 |
| Chat PDF | ❓ **PDF 大小待查** | — | — | 前端限 20MB，后端是否有对应限制待确认 |

**待调查清单（定稿前必须逐项确认）**：
1. task 创建/运行的文本参数（message、params、title/description）后端长度上限
2. chat PDF 附件大小后端限制（前端 20MB 是否有后端兜底）
3. 前端输出渲染管线现状：Markdown 组件、session title、KB 文档内容、task 结果、
   notification 消息、human channel 消息等所有「后端/LLM 文本 → DOM」的出口
4. XSS 转义组件的落点：Markdown 渲染器内部转义 vs 独立 `SafeText` 包装组件；
   react-markdown 等依赖的默认转义行为
5. 现有 `ValidateXSS` 校验点的收编策略（是否保留后端校验作为纵深防御，还是
   按本 spec 立场统一收敛到输出转义）

## 3. 设计方向（暂不深化）

### 3.1 前端 XSS 输出转义组件

- 核心：一个出口转义函数/组件（`escapeHtml` + `<SafeText>`），所有动态文本渲染
  统一走该出口。
- 与 Markdown 渲染的关系（允许 `**bold**` 等语法 vs 转义 HTML 标签）待调查定稿。
- 定稿前暂不实现。

### 3.2 后端输入限制补齐

- 只补结构性限制：文本长度、图片数量/大小（对齐现有 domain 常量，单一事实源）。
- **不做**后端 XSS 校验（现有 ValidateXSS 校验点是否保留/移除，待调查定稿）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | 可能（task 长度限制等新增 400 错误映射） |
| 性能影响 | 忽略不计（O(n) 长度检查） |
| 是否需要新增 Skill | No |

## 7. 相关文件（预估，定稿时精化）

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/components/SafeText.tsx`（或 lib） | XSS 输出转义组件 | New |
| `frontend/app/**`（chat/kb/task/notification 等渲染出口） | 接入转义组件 | Medium |
| `internal/domain/task` + `internal/service/task` | task 文本长度限制 | Small |
| `internal/api/handler/chat.go` / `service/chat` | chat PDF 大小限制（如需） | Small |

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | `logic/sql`, `logic/openapi`, `logic/report`, `config` |
| L2 | 依赖接口，可 mock | **100%** | `queue/`, service interfaces |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `service/*`, `api/handler/*` |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc`（非 `ApplyMethodReturn`）验证 handler→service 参数传递正确性
- [ ] **必须** Service 测试的写操作（`UpdateOne`, `InsertOne` 等）验证写入内容的字段和值
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景（如确实不可行，需文档注释说明原因并记录到 spec 中）
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证操作的实际结果

### 测试模式

- Handler: `httptest.NewRecorder` + `gin.CreateTestContext` + real handler → mock service
- Service: 直接注入 mock repository / 使用 `gomonkey` 模拟 MongoDB collection
- Logic (L1): 纯 table-driven test，无 mock 依赖

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

参考:
- `.agent/specs/spec-045-go-service-ut.md` — Go UT 全覆盖 spec
- `.agent/skills/go-ut-audit/SKILL.md` — UT 审计 skill
- `.github/workflows/ut-workflow.yml` — CI UT workflow

## 10. 验证标准（立项级，定稿时扩展）

1. 待调查清单 5 项全部有明确结论并写入本 spec 后，方可定稿。
2. 定稿后：前端所有动态文本渲染出口走转义组件（`git grep` 可枚举）；后端
   KB/chat/task 三条链路的长度与图片限制均有 handler/service 层兜底与单测覆盖。
