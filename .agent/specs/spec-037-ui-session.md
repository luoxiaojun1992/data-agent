# UI E2E 测试设计 — Session 管理 (SESSION)

> **SPEC-037** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent Session 管理模块的 E2E UI 测试用例规范。超时测试通过修改系统配置缩短空闲超时（如设为 10 秒），不等待实际 30 分钟。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §24
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-177 ~ UI-183（共 7 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-004 | ✅ | Agent 核心引擎（Session 管理逻辑） |
| SPEC-005 | ✅ | Artifact 存储（Session 恢复的临时文件） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |
| SPEC-019 | ✅ | CHAT（会话历史） |
| SPEC-026 | ✅ | SYSCONFIG（超时配置项） |

## 3. 测试范围

| 子功能 | 用例 | 优先级 | 测试策略 |
|--------|------|:------:|------|
| 30 分钟无操作超时提示 | UI-177 | P0 | 配置缩短超时 |
| 超时后自动登出 | UI-178 | P0 | 配置缩短超时 |
| 点击继续使用续期 | UI-179 | P1 | 配置缩短超时 |
| 多端登录互不干扰 | UI-180 | P1 | 2 个 browser context |
| 整体删除后 24 小时内可恢复 | UI-181 | P0 | 标准 E2E |
| 删除部分上下文历史不可恢复 | UI-182 | P0 | 标准 E2E |
| 恢复缓冲期可配置性 | UI-183 | P1 | 标准 E2E |

## 4. 超时测试策略

> **核心原则**: 不等待实际 30 分钟，不使用 API mock 伪造超时状态。通过修改系统配置将空闲超时缩短到可测试范围（如 10 秒）。

### 配置方式

| 方式 | 配置项 | 测试用值 | 说明 |
|------|--------|:------:|------|
| 环境变量 | `SESSION_IDLE_TIMEOUT_SECONDS` | `10` | 启动前端前设置 |
| 系统配置页 | 系统配置 → Session 超时 | `10` | 通过 UI 修改（如果有前端配置项） |

### 测试流程

```
beforeAll:
  1. 设置 SESSION_IDLE_TIMEOUT_SECONDS=10（或通过系统配置 API）
  2. 重新加载页面使配置生效

UI-177 (超时提示):
  1. 登录
  2. 停止所有操作（不移动鼠标/键盘）
  3. 等待 ~10 秒 → 弹出超时警告
  4. 验证倒计时 60 秒

UI-178 (自动登出):
  1. 超时警告弹出后不操作
  2. 等待倒计时结束 (~60 秒)
  3. 验证自动跳转登录页

UI-179 (续期):
  1. 超时警告弹出后点击「继续使用」
  2. 验证警告消失，Session 续期
```

## 5. 测试用例

### UI-177: Session — 无操作超时提示

- **优先级**: P0
- **前置条件**: 已登录，`SESSION_IDLE_TIMEOUT_SECONDS` 设为 10 秒
- **步骤**:
  1. 登录后停止所有操作
  2. 等待 10 秒空闲
- **预期结果**:
  - 弹出超时提示：「您已 10 秒未操作，会话即将过期。点击继续使用。」
  - 有「继续使用」按钮
  - 倒计时 60 秒
  - `data-testid`: `session-timeout-warning`, `session-timeout-continue-btn`

### UI-178: Session — 超时后自动登出

- **优先级**: P0
- **前置条件**: 超时提示已弹出，不操作
- **步骤**:
  1. 等待倒计时结束
- **预期结果**:
  - 自动登出
  - 跳转到登录页
  - 显示提示：「会话已过期，请重新登录」
  - `data-testid`: `login-session-expired-toast`

### UI-179: Session — 点击继续使用续期

- **优先级**: P1
- **前置条件**: 超时提示已弹出
- **步骤**:
  1. 点击「继续使用」
- **预期结果**:
  - 超时提示消失
  - 会话续期（重新计时）
  - `data-testid`: `session-timeout-continue-btn`

### UI-180: Session — 多端登录互不干扰

- **优先级**: P1
- **前置条件**: 同一用户在 2 个独立的 browser context 中登录
- **步骤**:
  1. 在 context A 中发送 Chat 消息
  2. 在 context B 中查看会话列表
- **预期结果**:
  - 各自有独立的 Session
  - context B 看不到 context A 的进行中会话
  - 各自的临时工作区文件互不干扰
  - `data-testid`: N/A

### UI-181: Session — 整体删除后 24 小时内可恢复

- **优先级**: P0
- **前置条件**: 用户有历史会话
- **步骤**:
  1. 在会话历史侧边栏中删除一个完整会话（整体删除）
  2. 检查恢复入口
  3. 点击恢复
- **预期结果**:
  - 整体删除的会话在缓冲期内可恢复
  - 提供「恢复已删除会话」入口
  - 恢复后会话历史、上下文中全部消息完整还原
  - `data-testid`: `session-recovery-banner`, `session-recovery-restore-btn`
- **相关 PRD**: PRD §F-07, SPEC-005 §4

### UI-182: Session — 删除部分上下文历史不可恢复

- **优先级**: P0
- **前置条件**: 用户当前会话中有多条历史消息
- **步骤**:
  1. 在当前会话中删除其中几条消息（非整体删除会话）
  2. 尝试找回被删除的消息
- **预期结果**:
  - 部分上下文历史删除后**不可恢复**
  - 无恢复入口
  - 仅整体删除的会话（删除整个 session）才支持恢复
  - `data-testid`: `session-history`, `session-item`

### UI-183: Session — 恢复缓冲期可配置性

- **优先级**: P1
- **前置条件**: 系统管理员登录
- **步骤**:
  1. 进入系统配置页
  2. 检查「Session 恢复缓冲期」配置项
  3. 修改缓冲期为 168 小时（1 周）
  4. 保存配置
  5. 验证环境变量 `SESSION_RECOVERY_HOURS` 也被设置
- **预期结果**:
  - 缓冲期默认值 24 小时
  - 支持修改（最小 1 小时，最大 168 小时 / 1 周）
  - **配置优先级**：后台配置 > 环境变量 `SESSION_RECOVERY_HOURS` > 默认值 24h
  - 超过 168 小时（1 周）时输入框拒绝并提示「缓冲期最长 1 周」
  - 修改后新删除的会话按新缓冲期计算
  - `data-testid`: `sysconfig-session-recovery-hours`, `sysconfig-session-recovery-save`
- **说明**: 具体恢复逻辑和触发方式在实现测试功能时根据实际 API 细化

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | No — 不使用 API mock 伪造超时状态 |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 多 Session 数据、已删除 Session 数据 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| **是否需要修改系统配置** | Yes — `SESSION_IDLE_TIMEOUT_SECONDS` 需可在测试中设为短值 |
| 性能影响 | UI-177/178 等待 ~70 秒（10s idle + 60s countdown） |
| 实现复杂度 | Medium — 超时等待可接受（~70s）；双 context 测试需要 Playwright 支持 |
| 是否有无法实现的用例 | **需系统支持** — `SESSION_IDLE_TIMEOUT_SECONDS` 配置项必须存在或可添加；多端登录需要后端支持同一用户多 Session |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/session.spec.ts` | SESSION E2E 测试实现 | New |
| `tests/ui/fixtures/session.fixture.ts` | Session mock 数据 fixture | New |

## 8. 验证标准

- [ ] 所有 P0 用例（UI-177, UI-178, UI-181, UI-182）必须通过
- [ ] 所有 P1 用例（UI-179, UI-180, UI-183）必须通过
- [ ] Session 超时测试通过配置缩短时间，无 API mock
- [ ] Session 管理 UI 100% 符合 PRD §F-07 和 SPEC-005 §4

## 9. UI Test / E2E 验收规则

- [ ] **必须** 通过环境变量 `SESSION_IDLE_TIMEOUT_SECONDS` 或系统配置 API 缩短空闲超时
- [ ] **必须** 测试超时警告弹出、倒计时、自动登出、续期四个环节
- [ ] **必须** 验证整体删除后恢复入口的存在
- [ ] **必须** 验证部分删除后无恢复入口
- [ ] **必须** 使用独立 browser context 测试多端登录
- [ ] **严禁** 使用 `page.clock` 或 API mock 伪造超时状态
- [ ] **严禁** 实际等待 30 分钟

参考: `.agent/memory/E2E_TESTING.md`
