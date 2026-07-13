# UI E2E 测试设计 — API 转换审核 (API)

> **SPEC-030** | Status: 已实现 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent API 转换审核模块的 E2E UI 测试用例规范。覆盖 API 转换审核页渲染、API 卡片渲染、上传 OpenAPI 文件、批准/驳回 API 转换、双重审核校验和批量上传。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §17
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-134 ~ UI-140（共 7 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-008 | ✅ | Skill 实现层（API→MCP 转换逻辑） |
| SPEC-013 | ✅ | 管理后台（API 审核页面定义） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |
| SPEC-018 | ✅ | LAYOUT |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| API 转换审核页渲染 | UI-134 | P0 |
| API 卡片渲染 | UI-135 | P0 |
| 上传 OpenAPI 文件 | UI-136 | P0 |
| 批准 API 转换 | UI-137 | P0 |
| 驳回 API 转换 | UI-138 | P0 |
| 双重审核校验（不可审核自己的提交） | UI-139 | P0 |
| 批量上传 OpenAPI 文件 | UI-140 | P1 |

## 4. 测试用例

### UI-134: API — API 转换审核页渲染

- **优先级**: P0
- **前置条件**: 以 system_admin 或知识管理员登录，点击「API 转换审核」
- **步骤**:
  1. 检查页面
- **预期结果**:
  - Header：「API 转换审核」
  - 「+ 上传 OpenAPI」按钮（蓝紫渐变）
  - 「📦 批量上传」按钮
  - API 卡片列表
  - `data-testid`: `api-page-header`, `api-upload-btn`, `api-batch-upload-btn`
- **相关设计**: UI原型设计文档 §3.12

### UI-135: API — API 卡片渲染

- **优先级**: P0
- **前置条件**: 有 API 转换记录
- **步骤**:
  1. 检查 API 卡片
- **预期结果**:
  - 玻璃卡片样式
  - 左侧：API 图标 + 信息区
  - 信息区内容：
    - API 名称（16px Bold）如「CRM 客户查询 API」
    - 描述：OpenAPI 3.0 · N 个端点 · 发起人 · 日期
    - 元数据：域名 · 频率限制（如 `crm.company.cn · 100/min`）
  - 右侧：
    - 审核状态 Pill：待审核（琥珀色）/ 已批准（绿色）/ 已驳回（粉色）
    - 操作按钮（待审核时）：批准 + 驳回
  - `data-testid`: `api-card-{apiId}`, `api-card-name`, `api-card-desc`, `api-card-meta`, `api-card-status`, `api-card-actions`

### UI-136: API — 上传 OpenAPI 文件

- **优先级**: P0
- **前置条件**: API 转换审核页
- **步骤**:
  1. 点击「+ 上传 OpenAPI」
  2. 选择 `crm-api.yaml`（OpenAPI 3.0 格式）
  3. 设置频率限制 100/min
  4. 确认
- **预期结果**:
  - 弹出上传模态窗口
  - 文件选择接受 .json / .yaml / .yml
  - 频率限制输入框
  - 上传成功后新卡片出现，状态「待审核」
  - `data-testid`: `api-upload-modal`, `api-upload-file`, `api-upload-rate-limit`, `api-upload-submit`

### UI-137: API — 批准 API 转换

- **优先级**: P0
- **前置条件**: 有待审核的 API，当前登录人与发起人不同（双重审核）
- **步骤**:
  1. 点击待审核 API 的「批准」按钮
  2. 确认批准
- **预期结果**:
  - 弹出确认弹窗
  - 确认后状态变为「已批准」（绿色 Pill）
  - 显示审核人和审核日期
  - API 变为 Agent 可调用的 MCP Tool
  - `data-testid`: `api-approve-btn-{apiId}`, `api-approve-confirm-modal`

### UI-138: API — 驳回 API 转换

- **优先级**: P0
- **前置条件**: 有待审核的 API
- **步骤**:
  1. 点击「驳回」按钮
  2. 输入驳回原因「域名不在白名单中」
  3. 确认
- **预期结果**:
  - 弹出驳回原因输入框（必填）
  - 确认后状态变为「已驳回」（粉色 Pill）
  - 驳回原因可查看
  - `data-testid`: `api-reject-btn-{apiId}`, `api-reject-reason`, `api-reject-confirm`

### UI-139: API — 双重审核校验（不可审核自己的提交）

- **优先级**: P0
- **前置条件**: 当前用户提交了一个 API 转换
- **步骤**:
  1. 查看自己提交的 API 卡片
  2. 检查操作按钮
- **预期结果**:
  - 「批准」和「驳回」按钮禁用或不可见
  - Tooltip 提示：「不可审核自己提交的转换」
  - `data-testid`: `api-card-actions-{apiId}`
- **相关 PRD**: PRD §F-10 双重审核

### UI-140: API — 批量上传 OpenAPI 文件

- **优先级**: P1
- **前置条件**: API 转换审核页
- **步骤**:
  1. 点击「📦 批量上传」
  2. 选择 2 个 OpenAPI 文件
- **预期结果**:
  - 多选支持
  - 每个文件独立上传进度
  - 上传后生成 2 张待审核卡片
  - `data-testid`: `api-batch-modal`, `api-batch-file-list`

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — API 转换审核 CRUD API、文件上传 API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — OpenAPI 文件 mock、审核状态 mock 数据 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | Medium — 需要 mock 双重审核逻辑和文件上传 |
| 是否有无法实现的用例 | **待确认** — `UI-139` 双重审核校验需要 mock 当前用户身份一致性检查 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/api-review.spec.ts` | API E2E 测试实现 | New |
| `tests/ui/fixtures/api.fixture.ts` | API mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-134 ~ UI-139）必须通过
- [ ] 所有 P1 用例（UI-140）必须通过
- [ ] API 审核 UI 100% 符合 UI 原型设计文档 §3.12
- [ ] 双重审核规则生效

## 8. UI Test / E2E 验收规则

- [ ] **必须** mock 后端 API Review API
- [ ] **必须** 验证 3 种审核状态的 Pill 渲染
- [ ] **必须** 验证批准/驳回的确认流程
- [ ] **必须** 测试双重审核保护（不可审核自己的提交）
- [ ] **严禁** 使用真实的 OpenAPI 文件处理

参考: `.agent/memory/E2E_TESTING.md`
