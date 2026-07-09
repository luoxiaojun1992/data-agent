# UI E2E 测试设计 — 系统配置 (SYSCONFIG)

> **SPEC-026** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 系统配置模块的 E2E UI 测试用例规范。覆盖系统配置页渲染、修改并保存全局参数、仅 system_admin 可访问的权限控制、缓冲期上限校验和配置优先级验证。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §13
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-104 ~ UI-108（共 5 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施（全局配置管理） |
| SPEC-009 | ✅ | 任务队列（通知 TTL 配置） |
| SPEC-012 | ✅ | Hermes |
| SPEC-013 | ✅ | 管理后台（系统配置页面定义） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 系统配置页渲染 | UI-104 | P0 |
| 修改并保存全局参数 | UI-105 | P0 |
| 仅 system_admin 可访问 | UI-106 | P0 |
| 缓冲期上限校验 | UI-107 | P1 |
| 配置优先级验证 | UI-108 | P1 |

## 4. 测试用例

### UI-104: SysConfig — 系统配置页渲染

- **优先级**: P0
- **前置条件**: 以 system_admin 身份登录，点击「系统配置」
- **步骤**:
  1. 检查页面渲染
- **预期结果**:
  - Header：「系统配置」
  - 配置区域包含全局参数设置：
    - Session 恢复缓冲期（默认 24h，可配 1~168h）
    - 审计日志保留天数（默认 90 天）
    - 通知 TTL 天数（默认 90 天）
    - 邮件域名白名单（列表 + 添加/删除）
    - 报告格式校验重试次数（默认 3 次）
  - 每项配置有「保存」按钮
  - `data-testid`: `sysconfig-page-header`, `sysconfig-session-recovery`, `sysconfig-audit-retention`, `sysconfig-notif-ttl`, `sysconfig-email-whitelist`, `sysconfig-report-retry`
- **相关 Spec**: SPEC-013 §2, SPEC-012 (Hermes 配置独立), SPEC-009 (通知 TTL)

### UI-105: SysConfig — 修改并保存全局参数

- **优先级**: P0
- **前置条件**: 系统配置页已加载
- **步骤**:
  1. 修改 Session 恢复缓冲期为 48 小时
  2. 点击该配置项的「保存」按钮
- **预期结果**:
  - 保存成功 toast：「配置已更新」
  - 重新加载页面后配置值保持为 48 小时
  - 配置优先级：后台配置 > 环境变量 > 默认值
  - `data-testid`: `sysconfig-session-recovery-save`, `sysconfig-save-success-toast`

### UI-106: SysConfig — 仅 system_admin 可访问

- **优先级**: P0
- **前置条件**: 以 admin 或 user 身份登录
- **步骤**:
  1. 检查侧边栏是否显示「系统配置」导航项
  2. 尝试直接访问系统配置 URL
- **预期结果**:
  - 「系统配置」不出现在侧边栏中
  - 直接访问 URL 返回 403 或重定向
  - `data-testid`: `sidebar`
- **相关 Spec**: SPEC-013 §7

### UI-107: SysConfig — 缓冲期上限校验

- **优先级**: P1
- **前置条件**: 系统配置页已加载
- **步骤**:
  1. 在 Session 恢复缓冲期输入 200（超过 168 小时上限）
  2. 点击保存
- **预期结果**:
  - 显示错误提示：「缓冲期最长 1 周（168 小时）」
  - 配置未保存，恢复原值
  - `data-testid`: `sysconfig-session-recovery-error`

### UI-108: SysConfig — 配置优先级验证

- **优先级**: P1
- **前置条件**: 同时设置了环境变量 `SESSION_RECOVERY_HOURS=72` 和后台配置 48h
- **步骤**:
  1. 检查系统实际使用的缓冲期值
- **预期结果**:
  - 实际生效值为后台配置的 48h（后台 > 环境变量）
  - 若后台配置未设置（null），则使用环境变量值
  - 若均未设置，使用默认值 24h
  - `data-testid`: N/A (后端逻辑 + 配置页验证)

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 系统配置 CRUD API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 多种配置 mock 数据 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | Medium — 标准配置 CRUD mock |
| 是否有无法实现的用例 | **待确认** — `UI-108` 配置优先级验证主要为后端逻辑，UI 测试可通过 mock 不同的 API 响应验证前端展示；环境变量注入需要在测试启动时配置 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/sysconfig.spec.ts` | SYSCONFIG E2E 测试实现 | New |
| `tests/ui/fixtures/sysconfig.fixture.ts` | SysConfig mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-104 ~ UI-106）必须通过
- [ ] 所有 P1 用例（UI-107, UI-108）必须通过
- [ ] 系统配置 UI 100% 符合 SPEC-013 §2 规范
- [ ] 缓冲期上限校验生效

## 8. UI Test / E2E 验收规则

- [ ] **必须** mock 后端 SysConfig API
- [ ] **必须** 验证每项配置保存后重新加载一致性
- [ ] **必须** 验证仅 system_admin 可访问的权限控制
- [ ] **严禁** 使用真实数据库修改系统配置

参考: `.agent/memory/E2E_TESTING.md`
