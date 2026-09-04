# 会话空闲超时配置化（SESSION_IDLE_TIMEOUT）

> **SPEC-088** | Status: 设计中

## 1. 目标

修复「长期无活动的会话超时时间」与系统配置不一致的问题：当前前端 idle 超时是**硬编码 30 分钟**，与系统配置中的「会话超时 24 小时」毫无关联。本 spec 将其独立为一个**系统配置项 `SESSION_IDLE_TIMEOUT`**，由后端在登录时下发，前端 IdleTimer 读取该值替代硬编码，使空闲超时可在系统配置页统一管理。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| — | — | 无前置依赖，可立即开始（复用现有系统配置 seed 机制 + 登录响应链路） |

## 2. 背景（根因分析）

系统里存在**两套完全独立的「会话超时」机制**，语义不同、时间也不同：

### 2.1 系统配置 `SESSION_TIMEOUT`（默认 24 小时）= JWT 过期时间

- **定义**：`internal/service/config/service.go:32`
  ```go
  {Key: "SESSION_TIMEOUT", Description: "登录 Session 超时（小时）", Default: "24"},
  ```
- **作用点**：`internal/service/auth/service.go:124-131` 的 `Login` ——
  ```go
  expiration := s.jwtManager.GetExpiration() // default 24h
  if cfg, err := s.configCache.Get(ctx, "SESSION_TIMEOUT"); err == nil && cfg != nil && cfg.Value != "" {
      if h, err := strconv.ParseInt(cfg.Value, 10, 64); err == nil && h > 0 {
          expiration = time.Duration(h) * time.Hour
      }
  }
  token, _ := s.jwtManager.GenerateTokenWithExpiration(user.ID, user.Username, string(user.Role), expiration)
  ```
- **效果**：登录后 JWT token 在 24 小时内有效；过期后用户下一次 API 请求返回 401，前端 `apiFetch`（`lib/api.ts:145`）捕获 401 → `logout()` → 抛 `Session expired`。
- **性质**：**被动过期**——只有用户发起请求时才暴露过期。

### 2.2 前端 IdleTimer 硬编码 30 分钟 = 「长期无活动」主动检测

- **定义**：`frontend/app/components/IdleTimer.tsx:6`
  ```ts
  const DEFAULT_IDLE_TIMEOUT = 1800; // 30 minutes
  const DEFAULT_COUNTDOWN = 60;
  ```
- **作用点**：监听 `mousedown / mousemove / keydown / scroll / touchstart`，连续无操作 `DEFAULT_IDLE_TIMEOUT` 秒后弹出「会话即将过期」警告（`session-timeout-warning`），倒计时 60 秒后 `logout()` 并跳转 `/login?expired=true`。
- **读取优先级**（`getIdleTimeout`，`IdleTimer.tsx:17-26`）：`window.__IDLE_TIMEOUT__`（E2E）→ URL `?idle_timeout=`（E2E）→ 默认 1800。
- **性质**：**主动检测**，但**完全硬编码**，与系统配置 `SESSION_TIMEOUT` 无任何关联。

### 2.3 问题

用户观察到「长期无活动」约 30 分钟就触发登出，而系统配置页写的是「会话超时 24 小时」，两者对不上。根因是第 2.2 节的前端硬编码 30 分钟，它本应是**独立可配置项**。

## 3. 架构概述

```
┌─────────────────────────────────────────────────────────────┐
│ 系统配置（MongoDB system_configs，/admin/settings 管理）      │
│   SESSION_TIMEOUT        = 24  (小时) → JWT exp 过期时间      │
│   SESSION_IDLE_TIMEOUT   = 30  (分钟) → 前端 idle 主动登出    │  ← 新增
└──────────────────────┬──────────────────────────────────────┘
                       │ 登录时 configCache.Get
                       ▼
        auth.Service.Login → LoginResponse（新增 idle_timeout_minutes）
                       │
                       ▼
        前端 lib/api.ts login → localStorage.idleTimeoutMinutes
                       │
                       ▼
        IdleTimer.tsx getIdleTimeout 读取 → 替代硬编码 1800
```

## 4. API 设计

### 4.1 登录响应扩展（无新增 API）

`POST /api/v1/auth/login` 的响应体 `LoginResponse` 新增一个字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `idle_timeout_minutes` | `int64` | 会话空闲超时（分钟），由系统配置 `SESSION_IDLE_TIMEOUT` 决定，默认 30 |

示例响应：

```json
{
  "user_id": "xxx",
  "username": "xxx",
  "role": "admin",
  "access_token": "xxx",
  "token_type": "Bearer",
  "expires_in": 86400,
  "idle_timeout_minutes": 30,
  "need_change_pw": false
}
```

## 5. 详细设计

### 5.1 后端 — 新增系统配置项

`internal/service/config/service.go` 的 `SystemBuiltins()` 增加：

```go
{Key: "SESSION_IDLE_TIMEOUT", Description: "会话空闲超时（分钟，无操作自动登出）", Default: "30"},
```

> 加入后 `SeedBuiltins`（启动时幂等补齐）会自动将该 key 写入 DB 并出现在 `/admin/settings` 页面，**无需额外 seed 代码**（原始 seed 数据同步红线，同 SPEC-084/086/087）。

### 5.2 后端 — 登录响应携带 idle 超时

`internal/service/auth/service.go`：

1. `LoginResponse` 增加字段：
   ```go
   IdleTimeoutMinutes int64 `json:"idle_timeout_minutes"`
   ```
2. `Login` 内读取该配置（与 `SESSION_TIMEOUT` 同源，复用现有 `configCache`）：
   ```go
   idleMinutes := int64(30) // 默认 30 分钟
   if s.configCache != nil {
       if cfg, err := s.configCache.Get(ctx, "SESSION_IDLE_TIMEOUT"); err == nil && cfg != nil && cfg.Value != "" {
           if m, err := strconv.ParseInt(cfg.Value, 10, 64); err == nil && m > 0 {
               idleMinutes = m
           }
       }
   }
   ```
3. 填充响应：
   ```go
   return &LoginResponse{
       ...
       IdleTimeoutMinutes: idleMinutes,
   }, nil
   ```

### 5.3 前端 — 登录时持久化

`frontend/lib/api.ts` 的 `login`（`api.ts:86-114`）：

```ts
localStorage.setItem('idleTimeoutMinutes', String(data.idle_timeout_minutes ?? 30));
```

`logout` 中同步清理（可选，保持整洁）：

```ts
localStorage.removeItem('idleTimeoutMinutes');
```

### 5.4 前端 — IdleTimer 读取优先级调整

`frontend/app/components/IdleTimer.tsx` 的 `getIdleTimeout` 读取优先级改为：

1. `window.__IDLE_TIMEOUT__`（E2E 测试注入，最高优先级，保持不变）
2. URL 参数 `?idle_timeout=`（E2E，保持不变）
3. **`localStorage.idleTimeoutMinutes`（登录时后端下发）← 新增**
4. 默认 `1800` 秒（30 分钟兜底，保持现状）

实现示意：

```ts
const getIdleTimeout = useCallback(() => {
  if (typeof window !== 'undefined') {
    const w = window as any;
    if (w.__IDLE_TIMEOUT__) return w.__IDLE_TIMEOUT__;
    const params = new URLSearchParams(window.location.search);
    const e2e = params.get('idle_timeout');
    if (e2e) return parseInt(e2e, 10);
    const stored = localStorage.getItem('idleTimeoutMinutes');
    if (stored) {
      const m = parseInt(stored, 10);
      if (m > 0) return m * 60; // 分钟 → 秒
    }
  }
  return DEFAULT_IDLE_TIMEOUT;
}, []);
```

> 单位约定：系统配置用「分钟」（与 `SESSION_TIMEOUT` 的「小时」区分），前端 IdleTimer 内部统一转「秒」（`m * 60`）。`window.__IDLE_TIMEOUT__` / URL 参数仍为「秒」（E2E 现有契约不变，`session.spec.ts` 传 3 表示 3 秒）。

### 5.5 兼容性与边界

- **现有 E2E 不受影响**：`UI-177/178/179` 通过 `window.__IDLE_TIMEOUT__` 注入，优先级最高，行为不变。
- **旧浏览器 localStorage 无该 key**：走默认 1800 秒兜底，行为不变。
- **配置被误设为非法值（非数字 / ≤0）**：后端 `ParseInt` 失败或 `m <= 0` 时 fallback 默认 30 分钟；前端同理 fallback 1800 秒。
- **idle > session 的语义**：若管理员把 `SESSION_IDLE_TIMEOUT` 设得比 `SESSION_TIMEOUT` 还大，则 token 会先过期（401 被动登出），idle 警告可能永不触发——这是管理员配置责任，不强制校验，但 spec 在描述里注明「空闲超时应 ≤ 会话超时」。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `system_configs`） |
| 是否影响现有 API | 仅扩展 `/auth/login` 响应字段（向后兼容，新增字段不破坏旧客户端） |
| 性能影响 | 无（登录时多一次 `configCache.Get`，已有缓存机制） |
| 是否需要新增 Skill | No |
| 是否需新增路由 | No |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/config/service.go` | `SystemBuiltins()` 加 `SESSION_IDLE_TIMEOUT` | 1 行 |
| `internal/service/auth/service.go` | `LoginResponse` 加字段 + `Login` 读取配置 | 小 |
| `frontend/lib/api.ts` | `login` 持久化 / `logout` 清理 | 小 |
| `frontend/app/components/IdleTimer.tsx` | `getIdleTimeout` 加 localStorage 优先级 | 小 |

## 8. 测试策略

1. **Unit tests（Go）**：
   - `internal/service/config/service_test.go`：断言 `SystemBuiltins()` 包含 `SESSION_IDLE_TIMEOUT` 且默认 `"30"`。
   - `internal/service/auth/auth_test.go`：mock `configCache` 返回 `SESSION_IDLE_TIMEOUT="45"`，断言 `LoginResponse.IdleTimeoutMinutes == 45`；返回空/非法值时断言 fallback `30`。
2. **E2E（前端）**：`tests/ui/session.spec.ts` 现有 `UI-177/178/179` 已覆盖 idle 触发链路（`__IDLE_TIMEOUT__` 注入），保持不变；可增补一条验证「登录后 `localStorage.idleTimeoutMinutes` 被写入」。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 修改 UI 组件（IdleTimer）时确保 `data-testid` 不变（`session-timeout-warning` / `session-timeout-continue-btn` / `session-timeout-logout-btn`）
- [ ] **必须** 现有 `UI-177/178/179` 全部通过（回归）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L3 | `service/auth`（依赖 configCache/mongo） | ≥98% |

- [ ] **必须** 每个 Success 测试至少 2 个行为验证断言（验证 `IdleTimeoutMinutes` 实际值，非仅 `err == nil`）
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景

## 10. 验证标准

1. `/admin/settings` 页面出现新配置项 `SESSION_IDLE_TIMEOUT`（`settings-row-SESSION_IDLE_TIMEOUT`），默认值 `30`，描述「会话空闲超时（分钟，无操作自动登出）」。
2. 登录接口 `POST /api/v1/auth/login` 响应含 `idle_timeout_minutes`，值等于系统配置 `SESSION_IDLE_TIMEOUT`。
3. 管理员将 `SESSION_IDLE_TIMEOUT` 改为 `1` 后，重新登录（或刷新），前端在 1 分钟无操作时弹出「会话即将过期」警告（可用 `window.__IDLE_TIMEOUT__` 无法覆盖的 localStorage 路径做人工验证，或 E2E 注入）。
4. 未配置/非法值时，前端回退默认 30 分钟，行为与现状一致。
5. 现有 E2E `UI-177/178/179` 回归通过；`go test ./internal/service/auth/... ./internal/service/config/...` 通过，覆盖率 ≥98%。

## 待决策点（实现前请晓军拍板）

| # | 决策点 | 推荐 | 备选 |
|---|--------|------|------|
| D1 | `SESSION_IDLE_TIMEOUT` 默认值 | **30 分钟**（保持现状，仅配置化） | 1440 分钟（=24h，与 session 一致） |
| D2 | 单位 | **分钟** | 秒 / 小时 |
| D3 | idle 是否需校验 ≤ session | **不校验**（仅描述注明） | 强制 clamp |
