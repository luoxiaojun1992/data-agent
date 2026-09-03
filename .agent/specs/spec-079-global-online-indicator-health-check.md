# 全局在线指示灯 + 后端健康检查 API

> **SPEC-079** | Status: 设计中（暂不实现）

## 1. 目标

统一在所有页面（含登录页 / 注册页）右上角挂一个「在线指示灯」，指示后端服务与依赖组件的实时健康状态；并配套一个后端健康检查 API 供前端轮询。同时消除登录页「登录已过期提醒」与「用户名/密码错误提示」两个 toast 之间、以及它们与在线指示灯之间的位置重叠。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 后端依赖组件（MongoDB/Redis/Vault）已在 main 初始化，健康检查需复用其连接 |
| SPEC-048 / SPEC-062 | ✅ | ADK 引擎与多模型 Runtime 已就绪，健康检查不依赖模型，无阻塞 |
| SPEC-070 | ✅ | ArcadeDB 图数据库已接入（`infra/arcadedb`），健康检查需探活 |
| SPEC-076 | ✅/📐 | 主题切换（暂不实现）——指示灯配色需兼容当前深色主题，落地时对色值做变量化 |
| SPEC-078 | ✅ | 弹窗/UI 视觉规范已统一，指示灯沿用 `var(--*)` CSS 变量体系 |
| — | — | 无硬阻塞依赖，可独立开发 |

> 开发前逐项确认为 ✅；任一项 ❌ 则阻塞开发。

## 2. 背景（现状不足）

### 2.1 前端指示灯是「假在线」

当前有两处硬编码的绿色脉冲圆点，与后端真实状态**完全脱钩**：

| 位置 | 文件 | testid | 实现 |
|------|------|--------|------|
| 首页/Dashboard | `frontend/app/page.tsx` | `dashboard-realtime-badge` | 硬编码 `bg-emerald-400 animate-pulse` + 「数据实时更新」 |
| Chat 页 | `frontend/app/chat/page.tsx` | `chat-online-badge` / `chat-online-dot` | 硬编码 `bg-emerald-400 animate-pulse` + 「在线」 |

问题：后端哪怕宕机、MongoDB 断连，指示灯依然是绿色——误导用户。

### 2.2 后端 `/health` 太单薄

`internal/api/handler/routes.go:74` 已注册 `GET /health`（无认证），但 `health.go` 的 `HealthCheck` 只返回：

```json
{ "status": "ok", "time": "2026-09-02T12:00:00Z" }
```

不 ping 任何依赖，无法反映 MongoDB / Redis / Vault / Qdrant / ArcadeDB / SeaweedFS / Presidio / MySQL 的真实可用性。

### 2.3 登录页 toast 重叠

`frontend/app/login/page.tsx` 两个 toast 都定位在 `fixed top-4 right-4 z-50`：

- `login-session-expired-toast`（「登录已过期，请重新登录」）
- `login-error-toast`（「邮箱或密码错误」）

当用户「session 过期被重定向到登录页」再「输错密码」时，两个 toast 在同一位置**互相覆盖**。新增在线指示灯后，若指示灯也在右上角，三者将进一步重叠。

## 3. 架构概述

```
                    ┌─────────────────────────────────────────────┐
 浏览器(所有页面)     │  OnlineIndicator (client, fixed 右上角, z-60) │
                    └───────────────┬─────────────────────────────┘
                                    │ GET /api/v1/health (轮询 15s, 3s 超时)
                                    ▼
                    ┌─────────────────────────────────────────────┐
                    │  nginx  (已代理 /api/v1 → Go 后端 :8080)        │
                    └───────────────┬─────────────────────────────┘
                                    ▼
                    ┌─────────────────────────────────────────────┐
                    │  handler.HealthCheck → monitor.HealthService  │
                    │   并发 Probe: mongo/redis/vault/qdrant/       │
                    │   seaweedfs/mysql (+ 条件: arcadedb/presidio)  │
                    │   (各自 2s 超时)                               │
                    └─────────────────────────────────────────────┘
```

对比表：

| 维度 | 现状 | 目标 |
|------|------|------|
| 指示灯挂载 | 2 处硬编码、各自为政 | 1 个全局组件挂 RootLayout，全页面（含登录/注册）统一 |
| 指示灯状态来源 | 硬编码绿色 | 后端 `/api/v1/health` 真实探测 |
| 指示灯状态 | 仅「在线」 | 三态：绿(ok) / 黄(degraded) / 红(down) |
| 健康检查 API | `/health` 只返回 ok | 返回逐依赖 up/down + latency + 进程信息 |
| 登录页 toast | 两 toast 同位置覆盖 | 堆叠容器 + 与指示灯上下错开 |

## 4. API 设计

### 4.1 增强健康检查端点（保持 `/health` 兼容 + 新增 `/api/v1/health`）

| Method | Path | Auth | Description |
|--------|------|:----:|-------------|
| GET | `/health` | 无 | 保留，供 nginx / 基础设施探活；与下述 handler 复用 |
| GET | `/api/v1/health` | 无 | 新增，供前端轮询（复用 `NEXT_PUBLIC_API_URL` 前缀，nginx 已代理 `/api/v1`） |

> 两个路径指向同一 handler（`HealthCheck`），在 `routes.go` public routes 区并列注册；`/api/v1/health` 必须注册在 `api.Use(JWT.AuthMiddleware())` 之前，保证未登录（登录页）也能访问。

### 4.2 响应结构

```json
{
  "status": "ok",
  "time": "2026-09-02T12:00:00Z",
  "version": "v1.5.0",
  "uptime_seconds": 86400,
  "dependencies": {
    "mongodb":    { "status": "up",    "latency_ms": 3 },
    "redis":      { "status": "up",    "latency_ms": 2 },
    "vault":      { "status": "up",    "latency_ms": 12 },
    "qdrant":     { "status": "up",    "latency_ms": 8 },
    "arcadedb":   { "status": "up",    "latency_ms": 15 },
    "seaweedfs":  { "status": "down",  "latency_ms": 0, "error": "connection refused" },
    "mysql":      { "status": "up",    "latency_ms": 5 },
    "presidio":   { "status": "up",    "latency_ms": 18 }
  }
}
```

> 依赖清单与逐项探活方式、探活语义见 §5.1.1。`status:"skipped"` 表示该依赖按当前配置未接入/未启用，跳过探测且不参与 degraded 判定。

**状态语义**（HTTP 恒 200，语义在 `status` 字段）：

| `status` | 含义 | 前端指示灯 |
|----------|------|-----------|
| `ok` | 进程存活 + 全部「必探」依赖 up（skipped 不影响判定） | 🟢 绿 |
| `degraded` | 进程存活 + 至少一个「必探」依赖 down | 🟡 黄 |
| （请求失败/超时） | 后端不可达 | 🔴 红 |

**依赖项 `status` 语义**：

| 依赖项 `status` | 含义 |
|----------------|------|
| `up` | 探活通过 |
| `down` | 探活失败，`error` 字段给出脱敏后的原因 |
| `skipped` | 该依赖未配置/未启用，跳过探测，不参与整体判定 |

> 设计取舍：不用 503 表达 degraded，因为前端 `fetch` 对非 2xx 会走 catch，反而丢失依赖明细；用 200 + `status` 字段承载语义，前端一套逻辑通吃。基础设施探活若需要非 200，可另加 `?strict=1` 返回 503（可选，暂不实现）。

### 4.3 Go 数据模型

```go
// internal/service/monitor/health.go
type DependencyStatus struct {
    Status    string `json:"status"`               // "up" | "down"
    LatencyMS int64  `json:"latency_ms,omitempty"`
    Error     string `json:"error,omitempty"`
}

type HealthResponse struct {
    Status       string                     `json:"status"` // "ok" | "degraded"
    Time         string                     `json:"time"`
    Version      string                     `json:"version,omitempty"`
    UptimeSec    int64                      `json:"uptime_seconds"`
    Dependencies map[string]DependencyStatus `json:"dependencies"`
}
```

## 5. 详细设计

### 5.1 后端：HealthService（service 层，零 infra 依赖）

#### 5.1.1 探针清单：与 docker-compose healthcheck 对齐

健康探针的**清单与探测端点必须与 `docker-compose.yml` 中 `data-agent` 服务的 `depends_on`（`condition: service_healthy`）逐项对齐**，否则会出现「compose 认为健康、后端探针却报 down（或反之）」的割裂。完整映射：

| # | 组件 | compose healthcheck 探测 | 后端连接方式 | 后端探活端点（对齐 compose） | 探活语义 |
|---|------|------------------------|-------------|---------------------------|---------|
| 1 | mongodb | `mongosh --eval "db.runCommand('ping').ok"` | 常驻 `mongoClient`（硬依赖） | `c.client.Ping(ctx, nil)`（需补 `Ping` wrapper） | **必探** |
| 2 | redis | `redis-cli ping` | 常驻 `redisClient`（连不上→cache/queue 降级） | `c.Client().Ping(ctx).Err()`（需补 `Ping` wrapper） | **必探** |
| 3 | qdrant | `GET /readyz` → HTTP 200 | 常驻 `qdrantClient` | `GET /readyz`（需新增 `Health`，**对齐 compose 的 `/readyz`**） | **必探** |
| 4 | vault | `vault status`（即 `/v1/sys/health`） | 常驻 `vaultClient`（连不上→加密降级） | `IsAvailable(ctx)`（读 `/v1/sys/health`，已有） | **必探** |
| 5 | arcadedb | `wget http://localhost:2480/` | 常驻 `graphRepo`（`ARCADE_URI` 为空则无） | `GET http://<arcade>:2480/`（需新增 `Ping`，**对齐 compose 的 HTTP 2480**） | **条件探**（配了 `ARCADE_URI` 才探） |
| 6 | seaweedfs | `curl -f http://localhost:9333/cluster/status` | 常驻 `swClient` | `GET /cluster/status`（需新增 `Health`，**对齐 compose 的 `/cluster/status`**） | **必探** |
| 7 | presidio | `curl -f http://localhost:3000/health`（analyzer / anonymizer 各一） | 常驻 `PIIRedactor`（HTTP client，按需调用） | `GET /health` ×2（需新增 `Health`，**对齐 compose 的 `/health`**） | **条件探**（受 `pii_redaction_enabled` 开关控制） |
| 8 | mysql | `mysqladmin ping -h localhost` | **health-check 专用全局单例 client**（读 `MYSQL_DSN`，仅探活；业务连接仍由 sql_executor skill config 自带 DSN 按需建立） | `sqlDB.Ping(ctx)`（新增单例 client + `Ping`） | **必探** |
| — | ollama | `ollama list` | **按需**（embedding baseURL 由 admin UI 配置，可能指向外部 MaaS） | — | **不探**（见 §5.1.1.2） |
| — | mockllm | `curl -f http://localhost:8082/health` | 仅开发桩（生产走 MaaS deepseek-v4-pro） | — | **不探** |

#### 5.1.1.2 三档探活语义（避免误报）

| 档位 | 依赖 | 判定规则 |
|------|------|---------|
| **必探** | mongodb / redis / qdrant / vault / seaweedfs / mysql | 任一 `down` → 整体 `degraded`（黄灯） |
| **条件探** | arcadedb / presidio | 按配置启用才注入 Probe；未启用→`skipped`，不影响整体判定 |
| **不探** | ollama / mockllm | 不注入 Probe，不出现在 `dependencies` 中 |

**关键决策说明（防止误报）**：

1. **mysql 用 health-check 专用单例 client 探活，业务连接仍按需**。为把 MySQL 纳入健康检查，新增一个**全局 MySQL 单例 client（`*sql.DB`，连接串来自 compose 已注入的 `MYSQL_DSN` 环境变量），仅用于 health check 的 `Ping` 探活，不承载任何业务查询、不建连接池**。sql_executor 的业务连接仍走 skill config 自带的 DSN 按需 `gorm.Open`，两者互不干扰、无冗余配置。因此 mysql 落「必探」：MySQL 挂掉 → 整体指示灯黄（degraded）。
2. **presidio 受开关控制**。`pii_redaction_enabled=false` 时 PII 脱敏已关闭，presidio 不可达不影响业务，应 `skipped` 而非 `down`。
3. **ollama/embedding 不探**。embedding baseURL 由 admin 模型配置动态决定（可能是本地 Ollama，也可能是外部 MaaS embedding API），不是固定 docker 依赖，纳入固定探针会误报。
4. **mockllm 不探**。它是 `MOCK_` 前缀的开发桩，生产环境 LLM 走 MaaS `deepseek-v4-pro`，健康检查不应绑定开发桩。

> ✅ **落地需补齐（本 spec 实施项）**：当前后端不读 `MYSQL_DSN`、也无全局 MySQL 连接。落地本 spec 时需：**新增 health-check 专用全局 MySQL 单例 client**（`sql.Open("mysql", cfg.MYSQL_DSN)`，仅用于 `Ping`，不建连接池、不承载业务）。`depends_on: mysql: service_healthy` 与 `MYSQL_DSN` 环境变量**均保留**（后者是健康探活的连接源）。业务连接（sql_executor）仍用 skill config 自带 DSN，不经过该单例。

遵循 DDD 铁律——`service` 层不 import `mongo-driver` 等 infra 具体类型，改用**闭包 Probe** 注入：

```go
// internal/service/monitor/health.go
type Probe struct {
    Name  string
    Check func(ctx context.Context) error // 探活闭包，由 wire.go 注入
}

type HealthService struct {
    probes    []Probe
    version   string
    startTime time.Time
}

func NewHealthService(version string, probes ...Probe) *HealthService

// Check 并发探测所有依赖（每个 2s 超时），聚合结果。
func (s *HealthService) Check(ctx context.Context) HealthResponse
```

`wire.go` 组装 Probe 切片（闭包把各 infra client 的探活方法包装成统一签名）：

```go
probes := []monitor.Probe{
    {Name: "mongodb",   Check: mongoClient.Ping},   // 需补 Ping wrapper（内部 c.client.Ping）
    {Name: "redis",     Check: redisClient.Ping},   // 需补 Ping wrapper（内部 c.Client().Ping）
    {Name: "vault",     Check: vaultProbe(vaultClient)}, // 包装 IsAvailable(bool)→error
    {Name: "qdrant",    Check: qdrantClient.Health},    // 需新增，GET /readyz（对齐 compose）
    {Name: "seaweedfs", Check: seaweedClient.Health},   // 需新增，GET /cluster/status（对齐 compose）
    {Name: "mysql",     Check: mysqlHealthClient.Ping}, // health-check 专用单例 client（读 MYSQL_DSN）
}

// 条件探：按配置启用时才追加（未启用 → skipped，不参与 degraded 判定）
if deps.graphRepo != nil { // ARCADE_URI 已配置
    probes = append(probes, monitor.Probe{Name: "arcadedb", Check: arcadeClient.Ping}) // GET :2480/
}
if deps.piiEnabled != nil && deps.piiEnabled() { // pii_redaction_enabled=true
    probes = append(probes, monitor.Probe{Name: "presidio", Check: deps.piiRedactor.Health}) // GET /health ×2
}
```

**需新增/补全的探活方法**（暂不实现，仅登记；探测端点对齐 compose healthcheck）：

| 组件 | 现状 | 需新增/补全 |
|------|------|-------------|
| `infra/mongo` | `Client` 仅暴露 `DB()`，内部 `c.client`（mongo.Client 有 `Ping`） | `Ping(ctx) error` wrapper（`c.client.Ping(ctx, nil)`），对齐 compose `mongosh ping` |
| `infra/redis` | `Client` 仅暴露 `Client()`（底层 go-redis 有 `Ping`） | `Ping(ctx) error` wrapper（`c.Client().Ping(ctx).Err()`），对齐 compose `redis-cli ping` |
| `infra/qdrant` | 仅 HTTP client | `Health(ctx) error`（**GET `/readyz`**，对齐 compose 的 `/readyz`，非 `/healthz`） |
| `infra/arcadedb` | Bolt driver | `Ping(ctx) error`（**HTTP `GET http://<arcade>:2480/`**，对齐 compose 的 `wget :2480/`） |
| `infra/seaweedfs` | HTTP client | `Health(ctx) error`（**GET master `/cluster/status`**，对齐 compose，非 `/status`） |
| `service/pii` | `PIIRedactor` 持有 HTTP client | `Health(ctx) error`（GET analyzer/anonymizer **`/health`** ×2，对齐 compose） |
| `infra/mysql`（新） | 当前无 mysql client | 新增 health-check 专用全局单例 client（`sql.Open("mysql", cfg.MYSQL_DSN)`）+ `Ping(ctx)`；仅探活，不承载业务 |
| `infra/vault` | `IsAvailable(ctx) bool`（已有） | `vaultProbe` 薄包装 `bool→error`（闭包内 `if !ok { return err }`） |

`handler.HealthCheck` 改为接收 `*monitor.HealthService`（经 `RouteDeps` 注入），调用 `Check(ctx)` 返回 JSON。

### 5.2 前端：OnlineIndicator 全局组件

**新建 `frontend/app/components/OnlineIndicator.tsx`**（`'use client'`）：

- 状态机：`ok`（绿）/ `degraded`（黄）/ `down`（红，fetch 失败或超时）。
- 轮询：首次挂载立即请求，之后 `setInterval` 15s 一次；用 `AbortController` 设 3s 超时。
- 请求：原生 `fetch('/api/v1/health')`（相对路径走 nginx 代理，复用 `/api/v1` 前缀）。
- 视觉：圆点（`w-2 h-2 rounded-full`）+ 文案（绿=「在线」/ 黄=「服务降级」/ 红=「服务离线」），沿用 `var(--*)` 配色；hover 出 tooltip 泡泡框展示各依赖探活结果（详见 §5.2.1）。
- 容器：`fixed top-4 right-4 z-[60]`，`data-testid="global-online-indicator"`，圆点 `data-testid="global-online-dot"`。

**挂载点：`frontend/app/layout.tsx`（RootLayout）** 的 `<body>` 内、`{children}` 之前：

```tsx
import OnlineIndicator from './components/OnlineIndicator';
// ...
<body className="antialiased">
  <OnlineIndicator />
  {children}
</body>
```

> 选 RootLayout 而非 AppLayout 的原因：`providers.tsx` 的 `AppLayout` 有 `if (!auth.hydrated) return null` 与 `if (!auth.token) return <>{children}</>` 的提前返回，且登录/注册页根本不走 AppLayout。RootLayout 是唯一的「所有页面必经」出口，一处改动全覆盖。

#### 5.2.1 Hover Tooltip：依赖探活泡泡框

用户鼠标 hover 到指示灯上时，弹出一个泡泡框（tooltip），逐项展示各依赖服务的实时探活结果——**只显示「在线 / 离线」即可，不显示 latency_ms / error 明细**。

**触发与交互**：

- `onMouseEnter` / `onMouseLeave`（辅以 `onFocus` / `onBlur` 保证键盘可达）切换显隐；移出立即收起，不设延时。
- 落地用 React state 控制显隐（而非纯 `group-hover`），便于 E2E 对 `data-testid` 的显隐断言。

**内容（只显示在线/离线，不显示 latency）**：

| 依赖项 `status` | 文案 | 状态色 |
|----------------|------|--------|
| `up` | 在线 | 🟢 `emerald-400` |
| `down` | 离线 | 🔴 `red-400` |
| `skipped` | 未启用 | ⚪ `gray-400` |

- 逐行列出 `dependencies` 中每一项：`<服务名> — <在线/离线/未启用>`，每行一个状态圆点 + 状态文案。
- 排序：必探项在前、条件探项次之（与 §5.1.1 表序一致），保证阅读稳定。
- **红灯（后端不可达）时**：`dependencies` 为空，tooltip 显示单行「后端服务不可达」，不渲染空列表。

**定位与样式**：

- 指示灯外层套 `relative` 容器；tooltip 为 `absolute top-full right-0 mt-2`——**向下展开、右缘对齐指示灯**（右上角固定布局下，向右展开会溢出视口，故右对齐 + 向下展开）。
- 玻璃面板样式，沿用 SPEC-078 弹窗视觉规范：`var(--bg-secondary)` + `var(--border-glass)` + `rounded-xl` + `px-4 py-3` + `shadow` + `backdrop-blur`。
- 宽度 `w-max min-w-[180px]`，层级 `z-[70]`（高于指示灯 `z-[60]`，确保泡泡框浮于其上）。
- 每行 `flex items-center justify-between gap-3 text-xs`：服务名 `text-(--text-secondary)`，状态文案按上表着色（颜色做主题变量化，兼容 SPEC-076 深色主题）。

**testid**：

- 泡泡框：`data-testid="global-online-tooltip"`
- 每行依赖项：`data-testid="tooltip-dep-<name>"`（如 `tooltip-dep-mongodb`）

**示例结构（伪代码）**：

```tsx
<div className="relative" onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)}>
  {/* 指示灯圆点 + 文案 */}
  {open && (
    <div data-testid="global-online-tooltip"
         className="absolute top-full right-0 mt-2 z-[70] w-max min-w-[180px]
                    rounded-xl border border-(--border-glass) bg-(--bg-secondary)
                    px-4 py-3 shadow backdrop-blur">
      {deps.length === 0 ? (
        <span className="text-xs">后端服务不可达</span>
      ) : (
        deps.map(d => (
          <div key={d.name} data-testid={`tooltip-dep-${d.name}`}
               className="flex items-center justify-between gap-3 text-xs">
            <span className="text-(--text-secondary)">{d.name}</span>
            <span className={statusColor(d.status)}>
              {d.status === 'up' ? '在线' : d.status === 'down' ? '离线' : '未启用'}
            </span>
          </div>
        ))
      )}
    </div>
  )}
</div>
```

### 5.3 登录页 toast 布局治理（解决重叠）

现状（`login/page.tsx`）两个 toast 均 `fixed top-4 right-4 z-50`，互相覆盖且会与指示灯重叠。调整为：

| 元素 | 定位（调整后） | 说明 |
|------|---------------|------|
| 在线指示灯 | `fixed top-4 right-4 z-[60]` | 全局组件，占右上角最高位 |
| 登录页 toast 堆叠容器 | `fixed top-14 right-4 z-50 flex flex-col gap-2 items-end` | 指示灯正下方（`top-14`≈56px，避开指示灯约 40px 高度） |
| `login-session-expired-toast` | 容器内第一项 | 「登录已过期，请重新登录」 |
| `login-error-toast` | 容器内第二项 | 「邮箱或密码错误」 |

要点：

1. 两个 toast 移入同一个堆叠容器，`gap-2` 纵向排列，**彼此不再覆盖**。
2. 容器整体下移到 `top-14`，避开 `top-4` 的指示灯，**不与指示灯重叠**。
3. `emailError` / `passwordError` 是表单内联错误（input 下方），不属 toast，无需调整。
4. 其余页面右上角的 `NotificationBell`（AppLayout header 内 inline，非 fixed）、`IdleTimer` 均在文档流内，与 fixed 指示灯天然不冲突；如未来出现新的 `fixed top-* right-*` 提示，统一约定「指示灯 z-60 最高 + 其余 toast 下移堆叠」。

### 5.4 数据流

```
页面加载 → OnlineIndicator mount → fetch /api/v1/health (3s 超时)
   ├─ 200 & status=ok        → 绿灯「在线」
   ├─ 200 & status=degraded  → 黄灯「服务降级」(tooltip 显示 down 依赖)
   └─ 网络错误/超时/非 200    → 红灯「服务离线」
        ↓
   每 15s 重复（组件卸载时 clearInterval）
```

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（健康检查无状态，不落库） |
| 是否影响现有 API | 增强 `/health` 返回结构（新增字段，向后兼容）；新增 `/api/v1/health`（无认证） |
| 是否需新增 Skill | No |
| 性能影响 | 极低：每次探测并发 + 2s 超时，前端 15s 一轮；健康检查本身无 DB 写 |
| 探针清单对齐 | 探针清单/端点与 `docker-compose.yml` `depends_on` 的 healthcheck 逐项对齐（§5.1.1）；mysql 经 health-check 专用单例 client 落「必探」 |
| 分层合规 | `service/monitor` 用闭包 Probe，零 infra import；handler 仅调用 service |
| 安全 | `/api/v1/health` 无认证但只暴露「up/down + latency + 版本」，无敏感数据（不返回连接串/账号/密钥）；`error` 字段仅返回连接错误文案，需脱敏处理 |
| 兼容性 | 保留 `/health` 路径，nginx 现有探活不中断 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/monitor/health.go` | New：HealthService + Probe + 数据模型 | New |
| `internal/service/monitor/health_test.go` | New：HealthService 单测 | New |
| `internal/api/handler/health.go` | 改：HealthCheck 注入 HealthService，返回逐依赖状态 | Modify |
| `internal/api/handler/routes.go` | 改：public 区新增 `GET /api/v1/health` | Small |
| `internal/api/handler/routes_test.go` / `health_test.go` | 改：断言新路由 + 新响应结构 | Modify |
| `internal/infra/mongo/client.go` | 新增 `Ping(ctx) error` wrapper | Small |
| `internal/infra/redis/client.go` | 新增 `Ping(ctx) error` wrapper | Small |
| `internal/infra/qdrant/client.go` | 新增 `Health(ctx)`（GET `/readyz`） | Small |
| `internal/infra/arcadedb/graph_store.go` | 新增 `Ping(ctx)`（GET `:2480/`） | Small |
| `internal/infra/seaweedfs/client.go` | 新增 `Health(ctx)`（GET `/cluster/status`） | Small |
| `internal/service/pii/redactor.go` | 新增 `Health(ctx)`（GET analyzer/anonymizer `/health`） | Small |
| `internal/infra/vault/client.go` | 探活闭包 `vaultProbe`（包装 `IsAvailable(bool)→error`） | Small |
| `internal/infra/mysql/client.go` | New：health-check 专用全局单例 client + `Ping(ctx)`（仅探活） | New |
| `internal/wire.go`（或 main.go 依赖组装处） | 组装 Probe 切片 + 条件探逻辑 + 注入 HealthService | Modify |
| `frontend/app/components/OnlineIndicator.tsx` | New：全局在线指示灯组件 | New |
| `frontend/app/layout.tsx` | 改：RootLayout body 挂载 OnlineIndicator | Small |
| `frontend/app/login/page.tsx` | 改：两个 toast 移入堆叠容器、下移 top-14 | Modify |
| `frontend/app/page.tsx` | 改：移除/替换 dashboard 硬编码 `dashboard-realtime-badge` | Small |
| `frontend/app/chat/page.tsx` | 改：移除/替换 chat 硬编码 `chat-online-badge` | Small |

## 8. 测试策略

1. **Unit tests（Go）**：`monitor/health_test.go` 覆盖——全 up→ok、部分 down→degraded、probe 超时、并发探测不阻塞；handler `HealthCheck` 用 mock HealthService 断言响应结构与状态码恒 200。
2. **Integration tests（条件）**：`go test -tags=integration` 下对真实依赖打点（可选）。
3. **E2E tests（前端，条件）**：用例编号 `UI-XXX`，见 §9。
4. **审计**：`.agent/skills/go-ut-audit` 审查 UT 质量，覆盖率对齐 SPEC-045（L1 100% / L3 98%）。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增 `OnlineIndicator` 时同步编写 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 指示灯加 `data-testid="global-online-indicator"` / `global-online-dot`
- [ ] **必须** tooltip 泡泡框加 `data-testid="global-online-tooltip"`，每行依赖项加 `data-testid="tooltip-dep-<name>"`
- [ ] **必须** hover 指示灯断言 tooltip 弹出，逐项断言依赖「在线/离线/未启用」文案与 `tooltip-dep-*` 一一对应
- [ ] **必须** 红灯（后端不可达）时 tooltip 显示「后端服务不可达」而非空列表
- [ ] **必须** 登录页 toast 堆叠容器加 `data-testid="login-toast-stack"`
- [ ] **必须** 覆盖三类页面断言指示灯存在：登录页、Dashboard、Chat（及至少一个 admin 页）
- [ ] **必须** mock 健康检查 API 返回 ok/degraded/超时 三态，断言指示灯颜色/文案切换
- [ ] **必须** 断言登录页「session-expired + error 同时出现」时两个 toast 纵向堆叠、互不遮挡，且不与指示灯重叠
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体 | **100%** | `monitor/health.go` 的聚合逻辑（无外部依赖，注入 mock Probe） |
| L2 | 依赖接口，可 mock | **100%** | handler 依赖 `HealthService` 接口 |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `infra/*` 探活方法（用 mock server / miniredis） |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少 **2 个行为验证断言**（除 `err == nil` 外验证实际值/状态/副作用）
- [ ] **必须** `Check` 测试验证「全 up→`status:"ok"`」「任一 down→`status:"degraded"`」两种聚合结果
- [ ] **必须** 超时 probe 场景断言该依赖 `status:"down"` 且不阻塞其他依赖探测
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil`

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

参考: `.agent/specs/spec-045-go-service-ut.md`、`.agent/skills/go-ut-audit/SKILL.md`

## 10. 验证标准

| # | 标准 | 可度量 |
|---|------|--------|
| 1 | 后端 `/api/v1/health` 无认证可访问 | `curl` 未带 token 返回 200 |
| 2 | 全依赖 up 时返回 `status:ok`，任一依赖 down 时返回 `status:degraded` | 停掉某依赖（如 Redis）后 curl 断言 |
| 3 | 响应含逐依赖 `up/down` + `latency_ms` | JSON 字段齐全 |
| 4 | 登录页/Dashboard/Chat/admin 页右上角均出现指示灯 | E2E 截图 + testid 断言 |
| 5 | 三态正确：ok=绿 / degraded=黄 / 后端不可达=红 | mock 三态断言颜色/文案 |
| 6 | 登录页两个 toast 同时出现时不重叠，且不与指示灯重叠 | E2E 断言 bounding box 不相交 |
| 7 | `/health` 旧路径仍 200（向后兼容） | curl `/health` 200 |
| 8 | 覆盖率 ≥ 98%，CI 全绿 | `ut-workflow.yml` + `ui-tests.yml` |
| 9 | 探针清单/端点与 `docker-compose.yml` `depends_on` 的 healthcheck 逐项对齐 | 逐项比对 §5.1.1 映射表：必探 6 项（含 mysql）+ 条件探 2 项（arcadedb/presidio）+ 排除 2 项（ollama/mockllm） |
| 10 | hover 指示灯弹出 tooltip，逐依赖显示在线/离线（不显示 latency） | E2E hover 断言 `tooltip-dep-*` 状态文案；红灯时显示「后端服务不可达」 |
