# 邀请注册系统设计

> **SPEC-044** | Status: 设计中 | Date: 2026-07-15 | Phase: P9

## 1. 目标

移除自由注册功能，改为邀请制注册。管理员生成含加密临时 token 的邀请链接，被邀请人通过链接完成注册。未携带有效 token 的 URL 直接拒绝访问。

## 2. 架构决策

### ADR-044-01: Token 方案 — HMAC-SHA256 自签名 vs 第三方库

| 选项 | 优点 | 缺点 |
|------|------|------|
| **A. 自实现** (`crypto/hmac` + `crypto/sha256`) | 零依赖，50行代码，完全控制 | 需测试覆盖 |
| B. `urlsigner` 库 | 开箱即用，expiry 内置 | 额外依赖，签名逻辑黑盒，API 不直观 |
| C. `sa-token-go` | 完整认证框架 | 重型框架，远超过我们需要的功能 |

**决策**: **A. 自实现**。Go 标准库提供 `crypto/hmac`、`crypto/sha256`、`encoding/base64`，完全满足需求。关键实现在 50 行以内，无需引入外部依赖。

**备选触发条件**: 若未来需要邀请码批量导入、邀请统计、多级邀请等复杂场景，再评估是否需要独立 invite service 或引入专用库。

### ADR-044-02: 注册流程 — 两步 vs 一步

| 选项 | 流程 | 优点 | 缺点 |
|------|------|------|------|
| A. 两步 | ① 点在链接 → ② 填信息完成注册 | token 在 URL 中，分享方��� | 两次请求 |
| **B. 一步** | 点在链接时直接创建账户 | 零摩擦 | 无法收集额外信息（姓名、部门等） |

**决策**: **A. 两步完成注册**。第一步点击链接时校验 token 有效性，展示注册表单（需填写密码、姓名等必填信息）；第二步提交表单完成注册。token 作为表单页的隐藏参数传递或存储在前端 sessionStorage。

## 3. 前置依赖

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | Infrastructure & Auth（用户模型、JWT、MongoDB） |
| SPEC-004 | ✅ | Agent Core Engine（安全审计日志记录邀请创建事件） |
| SPEC-023 | ✅ | UI E2E — User Mgmt（管理后台用户管理） |

## 4. 功能设计

### 4.1 邀请生成

**端点**: `POST /api/v1/auth/invites`

**权限**: `system_admin`, `admin`

**角色邀请权限矩阵**:

| 邀请人角色 | 可邀请 `user` | 可邀请 `admin` |
|:---|:---:|:---:|
| `system_admin` | ✅ | ✅ |
| `admin` | ✅ | ❌ |
| `user` | ❌ | ❌ |

**规则**:
- `system_admin` 可邀请任意角色（包括 `admin`），邀请 `admin` 时需额外确认
- `admin` 只能邀请 `user` 角色，若指定 `role: "admin"` → `403 Cannot invite role higher than your own`
- 两个角色都不能邀请 `system_admin` — 该角色仅通过系统初始化创建

**请求体**:
```json
{
    "email": "newuser@company.com",     // 可选，预填邮箱
    "role": "user",                      // 默认 "user"，system_admin 可设 "admin"
    "expire_hours": 24                   // 默认 24h，可选 1-168（7天）
}
```

**响应体**:
```json
{
    "invite_id": "inv_a1b2c3d4",
    "invite_url": "https://data-agent.example.com/register?token=ZXhhbXBsZS10b2tlbg==",
    "expires_at": "2026-07-16T18:00:00Z"
}
```

**邀请链接域名配置**:

`invite_url` 的 base URL 从系统配置读取，支持运行时变更：

| 来源 | 优先级 | 说明 |
|------|:---:|------|
| 环境变量 `INVITE_BASE_URL` | 高 | Docker/K8s 部署时注入 |
| 系统配置集合 `sys_config` | 中 | 管理后台「系统配置」页修改，热更新 |
| 默认值 `http://localhost:3000` | 低 | 开发环境回退值 |

读取逻辑：
```go
func GetInviteBaseURL() string {
    // 1. 环境变量
    if u := os.Getenv("INVITE_BASE_URL"); u != "" {
        return strings.TrimRight(u, "/")
    }
    // 2. 系统配置（热更新，通过 UpdateRules 机制）
    if cfg, _ := sysConfigRepo.Get("invite_base_url"); cfg != nil {
        return strings.TrimRight(cfg.Value, "/")
    }
    // 3. 默认值
    return "http://localhost:3000"
}
```

拼接示例：
```go
inviteURL := fmt.Sprintf("%s/register?token=%s", GetInviteBaseURL(), token)
```

**Token 生成算法** (Go stdlib):
```go
// payload = invite_id + ":" + expire_unix + ":" + email + ":" + role
// token = base64URL(payload + "." + HMAC-SHA256(secret, payload))
func GenerateInviteToken(inviteID string, expireAt time.Time, email, role string, secret []byte) string {
    payload := fmt.Sprintf("%s:%d:%s:%s", inviteID, expireAt.Unix(), email, role)
    mac := hmac.New(sha256.New, secret)
    mac.Write([]byte(payload))
    sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
    return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}
```

**安全特性**:
- 签名密钥从 Vault 或环境变量读取，不硬编码
- Token 在 MongoDB 中持久化为 `Invite` 文档（含 `status: "pending" | "used" | "expired"`）
- 验证时先查 HMAC 签名，再查 DB 状态（防止重放）
- Token 在 URL 中使用 `base64.RawURLEncoding`（URL-safe，无填充）

**签名密钥管理**:

| 来源 | 优先级 | 说明 |
|------|:---:|------|
| 环境变量 `INVITE_HMAC_SECRET` | 高 | Docker/K8s 部署时注入，优先使用 |
| Vault `secret/data/invite` | 中 | 运行时从 Vault 读取，system_admin 可通过管理后台修改 |
| 错误 | — | 两者都没有 → 服务启动失败，Fatal log |

特权操作限制：
- **仅 `system_admin`** 可在管理后台修改 `INVITE_HMAC_SECRET`（写入 Vault）
- `admin` 可查看邀请列表、生成/撤销邀请，但**不能**修改签名密钥
- 密钥修改后，旧 token 不受影响（验证时使用签发时的密钥版本，见密钥轮换策略）

**密钥轮换策略**:
```
密钥版本化存储格式（Vault）:
{
  "current": "v2_secret_xxx",
  "previous": "v1_secret_yyy"     // 保留上一版，旧 token 仍可验证
}
```
- 修改密钥时，旧密钥保留为 `previous`，新密钥写入 `current`
- 验证 token 时：先用 `current` 验，失败再用 `previous` 验
- `previous` 失效窗口 = 最长邀请有效期（168h = 7天），超期后可废弃

**启动检查**:
```go
func LoadInviteHMACSecret(vaultClient *vault.Client) ([]byte, error) {
    // 1. 环境变量优先
    if s := os.Getenv("INVITE_HMAC_SECRET"); s != "" {
        return []byte(s), nil
    }
    // 2. Vault 兜底
    if vaultClient != nil {
        secret, err := vaultClient.Read("secret/data/invite")
        if err == nil && secret != nil {
            return []byte(secret.Data["hmac_secret"].(string)), nil
        }
    }
    // 3. 都没有 → fatal
    return nil, fmt.Errorf("INVITE_HMAC_SECRET not set in env or Vault")
}
```

### 4.2 邀请链接验证

**端点**: `GET /api/v1/auth/register?token=xxx`

**逻辑**:
1. 解码 token，提取 `payload` 和 `signature`
2. 用服务器密钥验证 HMAC-SHA256 签名
3. 签名不匹配 → `401 Invalid or tampered invite token`
4. 签名有效，解析 payload 中的 `expire_unix` → 已过期 → `410 Invite link has expired`
5. 查询 MongoDB `invites` 集合 → 不存在 → `404 Invite not found`
6. `status == "used"` → `409 This invite has already been used`
7. 验证通过，返回 `200` + 预填信息（`email`, `role`）

**规则**: 直接访问 `/register`（无 token 参数）→ `400 Missing invite token`，不渲染注册页。

### 4.3 完成注册

**端点**: `POST /api/v1/auth/complete-registration`

**请求体**:
```json
{
    "token": "ZXhhbXBsZS10b2tlbg==",
    "username": "newuser",
    "password": "SecurePass123!",
    "display_name": "张三"          // 新增字段：显示名
}
```

**逻辑**:
1. 重新验证 token（同上，防中间人替换）
2. 校验 `username` 唯一性（复用现有 `Register` 逻辑）
3. 校验密码强度（复用现有密码强度校验）
4. 创建 User，`role` 使用 token payload 中指定的角色
5. 更新 Invite 状态 → `"used"`
6. 返回 `201` + User 信息（不含 token）

### 4.4 移除自由注册

- 删除或禁用 `POST /api/v1/auth/register`
- 前端移除登录页的「注册」入口
- 前端新增 `/register?token=xxx` 页面

## 5. 数据模型

### Invite 集合 (MongoDB)

```go
type Invite struct {
    ID          primitive.ObjectID `bson:"_id,omitempty"    json:"id"`
    InviteID    string             `bson:"invite_id"        json:"invite_id"`     // "inv_" + uuid
    Email       string             `bson:"email,omitempty"  json:"email,omitempty"`
    Role        string             `bson:"role"             json:"role"`          // "user" | "admin"
    Status      string             `bson:"status"           json:"status"`        // "pending" | "used" | "expired"
    TokenHash   string             `bson:"token_hash"       json:"-"`            // SHA256(token payload) 用于快速查询
    CreatedBy   string             `bson:"created_by"       json:"created_by"`   // 创建者 user_id
    CreatedAt   time.Time          `bson:"created_at"       json:"created_at"`
    ExpiresAt   time.Time          `bson:"expires_at"       json:"expires_at"`
    UsedAt      *time.Time         `bson:"used_at,omitempty" json:"used_at,omitempty"`
    UsedBy      string             `bson:"used_by,omitempty" json:"used_by,omitempty"`
}
```

**索引**:
- `invite_id` — 唯一索引
- `token_hash` — 快速验证查询
- `status` + `expires_at` — 清理过期邀请的 TTL 索引

## 6. 前端路由设计

| 路由 | 权限 | 描述 |
|------|:---:|------|
| `/admin/invites` | admin | 邀请管理页：生成邀请、查看列表、撤销邀请 |
| `/register` | 公开 | 仅含 `?token=xxx` 时可访问，否则 403 |
| `/register/complete` | 公开 | 提交注册表单，需 valid token |
| `/login` | 公开 | 移除「注册」入口 |

### 邀请管理页功能

- 表单：邮箱（可选）、角色、有效期（下拉：24h/48h/7d/30d）
- 「生成邀请链接」按钮 → 显示链接 + 复制按钮
- 邀请列表：邮箱、角色、状态（pending/used/expired）、创建时间、过期时间
- 「撤销」按钮：将 pending 状态的邀请标记为 expired
- **仅 system_admin 可见**: 签名密钥配置区（查看 Vault 中的密钥版本，输入新密钥滚动更新）

## 7. API 汇总

| Method | Endpoint | Auth | 描述 |
|--------|----------|:---:|------|
| `POST` | `/api/v1/auth/invites` | JWT (admin) | 生成邀请 |
| `GET` | `/api/v1/auth/invites` | JWT (admin) | 列出邀请 |
| `DELETE` | `/api/v1/auth/invites/:id` | JWT (admin) | 撤销邀请 |
| `PUT` | `/api/v1/auth/invites/hmac-secret` | JWT (system_admin) | 滚动更新签名密钥 |
| `GET` | `/api/v1/auth/register?token=xxx` | 公开 | 验证 token，返回预填信息 |
| `POST` | `/api/v1/auth/complete-registration` | 公开 | 完成注册 |
| `DELETE` | `/api/v1/auth/register` | 公开 | 禁用端点（返回 410 Gone） |
| `POST` | `/api/v1/auth/login` | 公开 | 保持不变 |

## 8. 安全考量

| 风险 | 缓解措施 |
|------|----------|
| Token 泄露 | HMAC 签名防篡改；短有效期（默认 24h）；一次性使用（used 后拒绝） |
| 暴力破解 token | 签名验证 + DB 查询；Rate limit 5次/IP/分钟 |
| 重放攻击 | `status: "used"` 后立即拒绝；used 状态不可逆 |
| 密钥泄露 | 密钥存 Vault/环境变量；支持轮换（多密钥验证，新邀请用新密钥签） |
| 注册绕过 | `/register` 无 token 直接返回 400；前端路由守卫 |
| 权限越权 | `admin` 邀请 `admin` → 403；`user` 访问邀请 API → 403；禁止邀请 `system_admin` |

## 9. 用户模型变更

```go
type User struct {
    // ... 现有字段保持不变 ...
    DisplayName  string  `bson:"display_name,omitempty" json:"display_name,omitempty"`  // 新增：显示名（邀请注册时填写）
    InvitedBy    string  `bson:"invited_by,omitempty"   json:"invited_by,omitempty"`    // 新增：邀请人 user_id
    InviteID     string  `bson:"invite_id,omitempty"    json:"-"`                        // 新增：关联的邀请 ID（审计用）
}
```

## 10. 审计日志

邀请相关操作记录到现有审计日志系统：

| 事件 | 操作类型 | 详情 |
|------|----------|------|
| 生成邀请 | `invite_created` | `{created_by, invite_id, email, role, expires_at}` |
| 撤销邀请 | `invite_revoked` | `{revoked_by, invite_id}` |
| 完成注册 | `invite_claimed` | `{invite_id, user_id, username}` |
| Token 验证失败 | `invite_verify_failed` | `{reason: "expired" | "used" | "invalid_sig" | "not_found"}` |

## 11. 覆盖的 SPEC-039（RBAC E2E 测试范围）

本 spec 的邀请管理页为 SPEC-039 提供测试素材：
- 非 admin 无法访问 `/admin/invites` → 403
- admin 可生成、查看、撤销邀请
- 被邀请人只能使用自己的 token 注册

## 12. 前端组件树

```
/register (公开，需 token)
  └─ TokenValidator (解析 URL 参数，验证 token)
     ├─ [loading] Token 验证中...
     ├─ [error] 无效/过期/已使用 token → 显示错误 + 跳转登录
     └─ [success] RegistrationForm
        ├─ email (预填，只读)
        ├─ username (必填)
        ├─ display_name (必填)
        ├─ password (必填，强度校验)
        └─ [提交] → 注册成功 → 跳转 /chat

/admin/invites (admin)
  ├─ InviteForm (生成邀请)
  │  ├─ email (可选)
  │  ├─ role (下拉)
  │  ├─ expire (下拉)
  │  └─ [生成链接] → 显示 URL + 复制按钮
  └─ InviteList (邀请列表)
     ├─ 表格列：邮箱、角色、状态、创建时间、过期时间
     └─ [撤销] 按钮（仅 pending 状态显示）
```

## 13. 实现范围

| 层级 | 文件 | 新增/修改 |
|------|------|:---:|
| 数据模型 | `internal/domain/model/invite.go` | 新增 |
| Repository | `internal/infra/mongo/invite_repository.go` | 新增 |
| Service | `internal/service/auth/invite.go` | 新增 |
| Handler | `internal/api/handler/auth.go` | 修改 + 新增 |
| 路由 | `cmd/server/main.go` | 修改 |
| 前端页面 | `frontend/app/register/page.tsx` | 修改 |
| 前端页面 | `frontend/app/admin/invites/page.tsx` | 新增 |
| 前端页面 | `frontend/app/login/page.tsx` | 修改（移除注册入口） |
| E2E 测试 | `tests/ui/invite.spec.ts` | 新增 |

## 14. 与现有系统的兼容性

- **JWT 流程不变**: `complete-registration` 成功后调用 `Login` 逻辑生成 JWT，返回给前端
- **密码策略复用**: 使用现有 `middleware.HashPassword` / `CheckPassword`
- **角色体系不变**: 使用现有 `model.RoleUser` / `model.RoleAdmin`
- **已有用户不受影响**: 仅影响新用户注册流程

## 15. 测试策略

| 场景 | 预期 |
|------|------|
| `/register` 无 token | 400 + 不渲染注册表单 |
| `/register` 无效 token（篡改签名） | 401 Invalid token |
| `/register` 过期 token | 410 Expired |
| `/register` 已使用 token | 409 Already used |
| `/register` 有效 token | 200 + 渲染预填表单 |
| 注册使用有效 token | 201 + 返回 JWT + token 标记为 used |
| 注册使用已使用 token | 409 |
| 非 admin 生成邀请 | 403 |
| admin 生成邀请 | 201 + 返回 invite_url |
| admin 尝试邀请 admin 角色 | 403 Cannot invite role higher than your own |
| system_admin 邀请 admin 角色 | 201 + role=admin 的 invite_url |
| system_admin 尝试邀请 system_admin | 400 Cannot invite system_admin role |
| admin 撤销邀请 | 200 + status 变为 expired |
| 同一 email 可多发邀请（不同的 token） | 每个独立有效 |
