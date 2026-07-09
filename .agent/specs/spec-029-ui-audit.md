# UI E2E 测试设计 — 审计日志 (AUDIT)

> **SPEC-029** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 审计日志模块的 E2E UI 测试用例规范。覆盖审计日志页渲染、审计日志表格数据、按时间范围/操作类型/用户筛选、导出审计日志（弹窗设置和执行导出）、导出条数上限校验和审计日志分页。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §16
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-125 ~ UI-133（共 9 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-004 | ✅ | Agent 核心引擎（安全审计日志记录） |
| SPEC-013 | ✅ | 管理后台（审计日志页面定义） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |
| SPEC-018 | ✅ | LAYOUT |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 审计日志页渲染 | UI-125 | P0 |
| 审计日志表格数据 | UI-126 | P0 |
| 按时间范围筛选 | UI-127 | P0 |
| 按操作类型筛选 | UI-128 | P0 |
| 按用户筛选 | UI-129 | P1 |
| 导出审计日志 — 弹窗 | UI-130 | P0 |
| 执行导出 | UI-131 | P0 |
| 导出条数上限校验 | UI-132 | P1 |
| 审计日志分页 | UI-133 | P1 |

## 4. 测试用例

### UI-125: Audit — 审计日志页渲染

- **优先级**: P0
- **前置条件**: 以 system_admin 或审计员身份登录，点击「审计日志」
- **步骤**:
  1. 检查页面
- **预期结果**:
  - Header：「审计日志」
  - 「导出日志」按钮
  - 筛选区域：时间范围 / 操作人 / 操作类型
  - 审计表格：时间 / 操作人 / 操作类型 / 详情 / IP
  - `data-testid`: `audit-page-header`, `audit-export-btn`, `audit-filter-bar`, `audit-table`
- **相关设计**: UI原型设计文档 §3.11

### UI-126: Audit — 审计日志表格数据

- **优先级**: P0
- **前置条件**: 有审计数据
- **步骤**:
  1. 检查表格行数据
- **预期结果**:
  - 时间列：完整日期时间（如 `2026-07-09 10:32:15`）
  - 操作人：用户姓名
  - 操作类型：Pill 样式（如「Chat 查询」「知识库上传」「Agent 任务」「登录」「用户管理」等）
  - 详情：操作描述摘要
  - IP 列：来源 IP 地址
  - `data-testid`: `audit-row-{logId}`, `audit-row-time`, `audit-row-user`, `audit-row-type`, `audit-row-detail`, `audit-row-ip`

### UI-127: Audit — 按时间范围筛选

- **优先级**: P0
- **前置条件**: 审计日志页
- **步骤**:
  1. 选择日期范围：2026-07-01 ~ 2026-07-09
  2. 点击「筛选」
- **预期结果**:
  - 表格仅显示该时间段内的日志
  - 总条数更新
  - `data-testid`: `audit-date-start`, `audit-date-end`, `audit-filter-apply`

### UI-128: Audit — 按操作类型筛选

- **优先级**: P0
- **前置条件**: 审计日志页
- **步骤**:
  1. 选择操作类型：「Chat 查询」
  2. 点击筛选
- **预期结果**:
  - 仅显示 Chat 查询类型的日志
  - `data-testid`: `audit-type-select`, `audit-type-option-chat`

### UI-129: Audit — 按用户筛选

- **优先级**: P1
- **前置条件**: 审计日志页
- **步骤**:
  1. 选择操作人「张三」
  2. 点击筛选
- **预期结果**:
  - 仅显示张三的操作记录
  - `data-testid`: `audit-user-select`

### UI-130: Audit — 导出审计日志 — 弹窗

- **优先级**: P0
- **前置条件**: 审计日志页
- **步骤**:
  1. 点击「导出日志」按钮
- **预期结果**:
  - 弹出导出设置弹窗
  - 包含：
    - 日期范围选择（开始/结束日期选择器）
    - 导出条数上限（默认 50,000，可调整）
    - 导出格式选择：CSV / JSON / Excel（Radio 按钮组）
  - 「取消」和「确认导出」按钮
  - `data-testid`: `audit-export-modal`, `audit-export-date-start`, `audit-export-date-end`, `audit-export-limit`, `audit-export-format-csv`, `audit-export-format-json`, `audit-export-format-xlsx`
- **相关设计**: UI原型设计文档 §3.11

### UI-131: Audit — 执行导出

- **优先级**: P0
- **前置条件**: 导出弹窗已打开
- **步骤**:
  1. 选择导出格式「CSV」
  2. 点击「确认导出」
- **预期结果**:
  - 触发文件下载
  - 下载的文件名包含日期范围
  - 弹窗关闭
  - 成功 toast 提示
  - `data-testid`: `audit-export-submit`, `audit-export-success-toast`

### UI-132: Audit — 导出条数上限校验

- **优先级**: P1
- **前置条件**: 导出弹窗已打开
- **步骤**:
  1. 将导出条数设为 100,000（超过 50,000 上限）
  2. 点击确认
- **预期结果**:
  - 显示错误提示：「单次导出最多 50,000 条」
  - 导出不执行
  - `data-testid`: `audit-export-limit-error`

### UI-133: Audit — 审计日志分页

- **优先级**: P1
- **前置条件**: 审计日志总数 > 20
- **步骤**:
  1. 检查分页
- **预期结果**:
  - 底部显示「共 1,247 条」+ 页码
  - 支持 10/20/50/100 条每页
  - `data-testid`: `audit-pagination`

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 审计日志列表、筛选、导出 API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 多种操作类型的审计日志 mock 数据 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | Medium — 需要 mock 多维度筛选和文件下载 |
| 是否有无法实现的用例 | **待确认** — `UI-131` 文件下载验证：Playwright 可以验证下载触发但无法直接验证下载的 CSV 内容（可在 download 事件中读取）；`UI-127` 日期选择器交互可能需要特殊处理 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/audit.spec.ts` | AUDIT E2E 测试实现 | New |
| `tests/ui/fixtures/audit.fixture.ts` | Audit mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-125 ~ UI-128, UI-130, UI-131）必须通过
- [ ] 所有 P1 用例（UI-129, UI-132, UI-133）必须通过
- [ ] 审计日志 UI 100% 符合 UI 原型设计文档 §3.11
- [ ] 多条件筛选组合正确

## 8. UI Test / E2E 验收规则

- [ ] **必须** mock 后端 Audit API
- [ ] **必须** 验证筛选条件的组合应用
- [ ] **必须** 验证导出弹窗的字段完整性和格式选择
- [ ] **必须** mock 文件下载（使用 `page.waitForEvent('download')`）
- [ ] **严禁** 使用真实的审计数据
- [ ] **严禁** 跳过导出条数上限校验

参考: `.agent/memory/E2E_TESTING.md`
