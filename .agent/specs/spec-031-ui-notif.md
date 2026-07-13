# UI E2E 测试设计 — 站内信系统 (NOTIF)

> **SPEC-031** | Status: 已实现 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 站内信系统的 E2E UI 测试用例规范。覆盖铃铛图标与未读数红点、点击展开通知列表、标记已读、一键全部已读、发送站内信（点对点）、群发站内信、全站发送（仅 system_admin）和通知 TTL 90 天自动清理。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §18
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-141 ~ UI-148（共 8 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-009 | ✅ | 任务队列（通知 TTL 配置） |
| SPEC-013 | ✅ | 管理后台（站内信系统定义） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 铃铛图标与未读数红点 | UI-141 | P0 |
| 点击展开通知列表 | UI-142 | P0 |
| 标记已读 | UI-143 | P0 |
| 一键全部已读 | UI-144 | P1 |
| 发送站内信（点对点） | UI-145 | P0 |
| 群发站内信 | UI-146 | P1 |
| 全站发送（仅 system_admin） | UI-147 | P1 |
| 通知 TTL 90 天自动清理 | UI-148 | P2 |

## 4. 测试用例

### UI-141: Notif — 铃铛图标与未读数红点

- **优先级**: P0
- **前置条件**: 已登录，有未读通知
- **步骤**:
  1. 检查后台 Header 区域
- **预期结果**:
  - 右侧显示铃铛图标按钮
  - 有未读通知时图标右上角显示红色圆点 + 未读数字
  - 无未读时红点不可见
  - `data-testid`: `notif-bell-icon`, `notif-unread-badge`, `notif-unread-count`

### UI-142: Notif — 点击展开通知列表

- **优先级**: P0
- **前置条件**: 有通知数据
- **步骤**:
  1. 点击铃铛图标
- **预期结果**:
  - 展开通知下拉面板
  - 通知按时间倒序排列
  - 未读通知有视觉区分（如浅蓝背景）
  - 每条通知显示：标题 + 内容摘要 + 时间
  - `data-testid`: `notif-dropdown`, `notif-item-{id}`, `notif-item-read`, `notif-item-unread`

### UI-143: Notif — 标记已读

- **优先级**: P0
- **前置条件**: 有未读通知
- **步骤**:
  1. 点击某条未读通知
- **预期结果**:
  - 通知标记为已读（高亮背景消除）
  - 未读计数减 1
  - 查看通知详情（如有链接则跳转）
  - `data-testid`: `notif-item-{id}`

### UI-144: Notif — 一键全部已读

- **优先级**: P1
- **前置条件**: 有多条未读通知
- **步骤**:
  1. 在通知面板顶部点击「全部已读」
- **预期结果**:
  - 所有通知标记为已读
  - 未读计数归零，红点消失
  - `data-testid`: `notif-mark-all-read`

### UI-145: Notif — 发送站内信（点对点）

- **优先级**: P0
- **前置条件**: 以 system_admin 登录，有另一个用户
- **步骤**:
  1. 进入通知/站内信发送界面
  2. 选择接收人「张三」
  3. 输入标题和内容
  4. 发送
- **预期结果**:
  - 接收人的铃铛图标出现未读红点
  - 发送方显示发送成功
  - `data-testid`: `notif-send-modal`, `notif-send-recipient`, `notif-send-subject`, `notif-send-body`, `notif-send-submit`

### UI-146: Notif — 群发站内信

- **优先级**: P1
- **前置条件**: 以 admin 登录
- **步骤**:
  1. 选择群发模式
  2. 选择多个接收人
  3. 发送
- **预期结果**:
  - 每个接收人都收到通知
  - 每日发送计数增加
  - 超过 50 条/天限制时提示错误
  - `data-testid`: `notif-group-send`, `notif-group-recipients`

### UI-147: Notif — 全站发送（仅 system_admin）

- **优先级**: P1
- **前置条件**: 以 system_admin 登录
- **步骤**:
  1. 选择全站发送模式
  2. 输入标题和内容
  3. 发送
- **预期结果**:
  - 所有用户可见该通知
  - 不产生 N 条独立数据（仅 1 条全局记录 + 视图聚合 — SPEC-013 §4）
  - `data-testid`: `notif-broadcast-btn`, `notif-broadcast-modal`

### UI-148: Notif — 通知 TTL 90 天自动清理

- **优先级**: P2
- **前置条件**: 有 90 天前的通知记录
- **步骤**:
  1. 检查 90 天前的通知
- **预期结果**:
  - 超过 90 天的通知自动清理（MongoDB TTL）
  - `data-testid`: N/A (后端逻辑)

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 通知列表/标记已读/发送 API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 未读/已读通知 mock 数据 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | Medium — 需要 mock 多用户通知状态同步 |
| 是否有无法实现的用例 | **待确认** — `UI-145` 点对点通知需要跨 test 验证接收方看到红点；`UI-148` TTL 清理为纯后端逻辑 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/notif.spec.ts` | NOTIF E2E 测试实现 | New |
| `tests/ui/fixtures/notif.fixture.ts` | Notif mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-141 ~ UI-143, UI-145）必须通过
- [ ] 所有 P1 用例（UI-144, UI-146, UI-147）必须通过
- [ ] 通知系统 UI 符合 SPEC-013 §4 规范

## 8. UI Test / E2E 验收规则

- [ ] **必须** mock 后端 Notification API
- [ ] **必须** 验证未读红点和计数的正确性
- [ ] **必须** 验证标记已读后红点消失
- [ ] **严禁** 依赖实时 WebSocket 推送（使用 mock API 响应模拟）

参考: `.agent/memory/E2E_TESTING.md`
