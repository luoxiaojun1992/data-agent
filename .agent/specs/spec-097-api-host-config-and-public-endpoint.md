# API Host 系统配置 + 公开查询接口 + 前端动态获取

> **SPEC-097** | Status: 📐 立项（暂不实现、暂不深化）

## 1. 目标

在系统配置（sysconfig）中新增一个「API host」配置项，并提供一个**无需 RBAC 鉴权**的公开接口返回该值；前端运行时动态获取该 API host，**获取失败或未配置时 fallback 到当前前端 host**（`window.location.origin`），从而把「后端 API 地址」从构建时硬编码（`NEXT_PUBLIC_API_URL`）解耦为运行时可配置。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-079（/health 无鉴权公开接口模式） | ✅ 已实现 | 公开路由区块已存在，health 即无鉴权范式 |
| 系统配置（sysconfig）基础设施 | ✅ 已存在 | `SystemBuiltins()` + `config.Service` + admin 设置页 |
| — | — | 无其他阻塞依赖 |

## 2. 背景（现状）

| 现状 | 位置 | 问题 |
|------|------|------|
| 前端 API 地址硬编码在构建时 | `frontend/lib/api.ts:48`、`frontend/app/chat/page.tsx:16,263,282,592` | `process.env.NEXT_PUBLIC_API_URL \|\| 'http://localhost:8080/api/v1'`，改地址需重新构建前端镜像 |
| 系统配置已有成熟机制 | `internal/service/config/service.go:23` `SystemBuiltins()`（启动 seed 到 MongoDB `system_configs`） | 缺「API host」这一项 |
| 公开接口范式已存在 | `internal/api/handler/routes.go:76-84`（Public routes 区块，`GET /api/v1/health` 无鉴权） | 可直接套用 |
| 前端 fallback 目标已明确 | 浏览器 `window.location.origin` | 同源部署时即 nginx 对外地址 |

## 4. API 设计

| Method | Path | Description | 鉴权 |
|--------|------|-------------|:---:|
| GET | `/api/v1/api-host` | 返回系统配置中的 API host（未配置返回空串） | **无**（Public routes 区块） |

响应示例：

```json
{ "api_host": "https://api.example.com/api/v1" }
// 未配置：
{ "api_host": "" }
```

## 5. 详细设计

### 数据流

```
admin 在系统设置页配置 API_HOST 值
  → config.Service.Upsert(key="API_HOST", value, desc) → MongoDB system_configs
前端启动/需要时：
  fetch('/api/v1/api-host')  (相对路径，走 nginx 同源代理，必定可达)
    ├─ 200 且 api_host 非空 → 使用该值
    └─ 失败 / 空串       → fallback 到 window.location.origin
```

### 后端改动

1. **配置项**：`internal/service/config/service.go` 的 `SystemBuiltins()` 增加一项
   `{Key: "API_HOST", Description: "对外 API 基地址（前端运行时获取后端地址，空则回退到前端同源）", Default: ""}`
2. **Handler**：新增 `internal/api/handler/config.go` 或独立 `api_host.go`，读 `config.Service`（或 `SysConfigRepository`）取 `API_HOST` 值返回。
3. **路由**：`routes.go` Public routes 区块新增 `router.GET("/api/v1/api-host", h.GetAPIHost)`（**不挂 AuthMiddleware、不挂 RequirePermission**）。

### 前端改动

1. 新增 `frontend/lib/api-host.ts`：`getApiHost()` 封装上述 fetch + fallback 逻辑。
2. （待定稿 D3）是否重构 `lib/api.ts` 的 `API_BASE` 为运行时动态获取，还是仅新增独立函数供特定场景使用。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `system_configs`） |
| 是否影响现有 API | No（纯新增一个公开 GET 接口） |
| 性能影响 | 极小（一次 key 查询，可命中 sysconfig Cache-Aside 缓存） |
| 是否需要新增 Skill | No |
| 安全 | 该接口仅暴露一个非敏感地址字符串，无鉴权可接受（等价于 /health 暴露部署信息） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/config/service.go` | `SystemBuiltins()` 增加 API_HOST 项 | Small |
| `internal/api/handler/config.go`（或新 `api_host.go`） | 新增 GetAPIHost handler | Small |
| `internal/api/handler/routes.go` | Public routes 注册 `/api/v1/api-host` | Small |
| `cmd/server/wire.go` | 注入依赖（如 handler 需要 config.Service） | Small |
| `frontend/lib/api-host.ts` | 新增 `getApiHost()` 封装 | New |
| `frontend/lib/api.ts`（待定稿 D3） | 可能重构 `API_BASE` | Small |

## 8. 测试策略

1. **Unit tests**（Go）：handler `GetAPIHost` —— 成功返回配置值 / 未配置返回空串 / 服务错误。覆盖率按 SPEC-045（L3 handler ≥98%）。
2. **E2E**（条件，前端涉及时）：`/api/v1/api-host` 无需登录即可 200；前端 fallback 逻辑。

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

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** |
| L2 | 依赖接口，可 mock | **100%** |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** |
| Overall | 全量 | ≥98% |

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc` 验证 handler→service 参数传递
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景

参考: `.agent/specs/spec-045-go-service-ut.md`

## 10. 验证标准

1. 系统设置页（admin/settings）出现「API Host」配置项，可编辑保存。
2. `GET /api/v1/api-host` 无需登录返回 200；未配置时返回 `{"api_host":""}`。
3. 配置非空值后接口返回该值。
4. 前端 `getApiHost()`：接口可达且非空 → 用配置值；不可达/空 → fallback `window.location.origin`。

## 11. 待定稿决策点

| # | 决策点 | 选项 | 备注 |
|---|--------|------|------|
| D1 | 配置项 key 命名 | `API_HOST`（大驼峰，同 `INVITE_BASE_URL`）vs `api_host`（小写蛇形，同 `pii_redaction_enabled`） | 现有两套命名混用 |
| D2 | `api_host` 返回值语义 | 完整 base URL（含 `/api/v1`）vs 纯 origin（不含路径） | 影响前端如何拼接 |
| D3 | 前端消费方式 | 重构 `lib/api.ts` 的 `API_BASE` 为运行时动态 vs 仅新增 `getApiHost()` 独立函数 | 核心分歧：是否动现有请求链路 |
| D4 | fallback 值优先级 | `window.location.origin` vs 现有 `NEXT_PUBLIC_API_URL` | 用户已明确「当前前端 host」，倾向 origin |
