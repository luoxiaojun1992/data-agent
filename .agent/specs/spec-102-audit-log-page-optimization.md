# SPEC-102 审计日志页面功能优化（搜索分页统一 + action 描述 mapping + 参数校验 + 可见性过滤）

> **SPEC-102** | Status: 设计中（立项）
> 日期：2026-09-17

## 1. 目标

1. 审计日志页面的搜索、分页**样式与其他页面统一**（SPEC-075/078 规范）。
2. 搜索、分页**全部 DB 层筛选、DB 层分页**（参数统一 `q`/`path`/`status_class` + `page`/`page_size`）。
3. 后端建立 **hardcode mapping**：`API 路径（不带 query 参数）+ method` → 人类友好描述，展示层不再裸显示 `POST /api/v1/xxx`。
4. 完善后端 API 参数校验（分页参数、日期格式、导出参数非法值返回 400 而非 500/静默容错）。
5. **可见性过滤**：普通用户/普通管理员看「自己的 + 无 user 关联」的日志；系统管理员看全部。
6. 支持 **API 路径（不带 query 参数）模糊搜索**（D2 定稿）。
7. 支持 **HTTP 状态码类别筛选**（1xx~5xx 下拉，后端枚举校验 + 范围条件，D3 定稿）。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-075 前端列表搜索/分页后端化（q 参数 DB 层 $regex 模式） | ✅ | 本 spec 的搜索/分页参数对齐其规范 |
| SPEC-078 前端列表页 UI 规范（Pagination 组件 / 弹窗玻璃样式） | ✅ | 审计页样式对齐目标 |
| SPEC-084 API 权限整理（`audit:view` RBAC 补挂） | ✅ | `/api/v1/admin/audit/logs` + export 已挂 RBAC |
| 主角色三级体系（User.Role: user/admin/system_admin，ctx `role`） | ✅ | 可见性过滤的判定依据 |
| — | — | 无新增前置，可立即开始 |

## 2. 背景与动机（现状问题）

### 2.1 前端搜索/分页与其他页面不一致

- 分页参数旧式 `skip`/`limit`，其他页面已统一 `page`/`page_size`（SPEC-075）。
- 筛选栏是自定义「日期 + 邮箱 + 操作类型下拉」，其他页面统一为 `q` 搜索框。
- **操作类型下拉存在筛选 bug**：下拉枚举值（`chat:query`/`kb:upload`/`agent:task`…）与 DB 实际写入的 `Action` 值（middleware 写入 `"METHOD FullPath"`，如 `"POST /api/v1/chat"`）**完全不匹配**，选择任何下拉项都查不到记录。

### 2.2 操作类型展示不友好

`Action` 存的是 `"POST /api/v1/sessions"` 这类原始字符串，页面直接裸显示，人类难以理解；且前端下拉枚举与后端写入语义已脱节。

### 2.3 后端参数校验不完善

- `handler.ListAuditLogs` 用 `strconv.ParseInt` **忽略错误**（非法值静默为 0）。
- `start`/`end` 日期格式错误经 service 返回 → handler 统一 **500**（应 400）。
- `Export` 的 `format` 参数完全不校验（前端有 csv/json/xlsx 三选项，后端只实现 csv，选 json/xlsx 得到 csv 内容）。

### 2.4 无可见性过滤

`service.List` 无 viewer 概念——任何有 `audit:view` 权限的用户都能看到**全部**审计日志（含其他用户操作、IP、UA、请求体摘要）。需求：按主角色过滤（非 system_admin 只看自己的 + 无 user 关联的；system_admin 看全部）。导出接口同样需要过滤。

## 3. 现状关键事实（2026-09-17 调研实测）

| 项 | 事实 |
|---|---|
| 写入端 | `middleware/audit.go`：`Action = method + " " + c.FullPath()`（gin 路由模板，**天然不含 query、参数已泛化**）→ 正好可作为 mapping key，**无需新增 Method/Path 字段** |
| 数据模型 | `model.AuditLog{ID, Action, UserID, Resource, Details, IP, UserAgent, StatusCode, CreatedAt}`；collection `audit_logs` |
| 后端筛选 | service 层已 DB 层筛选（filterMap → repo.Count/List）；user 过滤 = email keyword → 查 users top10 → `$in` user_ids |
| 展示 | service `enrichUserEmails` 把 user_id 替换为 email |
| 路由/RBAC | `/api/v1/admin/audit/logs`（GET）+ `/export`（POST）；`audit:view` 权限（data_analyst/kb_admin/auditor/system_admin 持有） |
| 角色判定 | `ctx["role"] == "system_admin"`（`taskIdentity` 同模式）；主角色枚举 user/admin/system_admin |

## 5. 详细设计（方向）

### 5.1 搜索/分页参数统一（后端）

`GET /api/v1/admin/audit/logs` 参数改为：

| 参数 | 说明 | 校验（需求 4） |
|------|------|------|
| `q` | 操作人 **email 模糊搜索**（D1 定稿：`SearchByEmail(q, topN=10)` → userIDs → `$in`，见 §11） | 长度 ≤100，超长 400 |
| `path` | **API 路径模糊搜索**（D2 定稿：匹配 `action` 字段 `$regex`，见 §11） | 长度 ≤100，超长 400 |
| `status_class` | **HTTP 状态码类别筛选**（D3 定稿：枚举 1xx/2xx/3xx/4xx/5xx → 范围条件，见 §11） | 非法枚举 → 400 |
| `page` | 页码，从 1 起 | 非正整数/非数字 → 400（不再静默容错） |
| `page_size` | 每页条数 | 1~100，越界/非法 → 400 |
| `start` / `end` | 日期范围（YYYY-MM-DD，保留） | 格式非法 → **400**（现为 500） |

- `q` = **操作人 email 模糊搜索（D1 已定稿，见 §11）**：后端 `users.SearchByEmail(q, topN=10)` → userIDs → 审计日志过滤 `user_id $in userIDs`；查无匹配用户 → 返回空列表（total=0）。q **不做** action/details 的正则匹配。
- `path` = **API 路径模糊搜索（D2 已定稿）**：DB 层 `action: {$regex: QuoteMeta(path), $options: "i"}`。`Action` 值是 `"METHOD FullPath模板"`（不含 query 参数），如搜 `sessions` 命中 `POST /api/v1/sessions`、`DELETE /api/v1/sessions/:id` 等。
- `status_class` = **状态码类别（D3 已定稿）**：枚举 `1xx`/`2xx`/`3xx`/`4xx`/`5xx`，构造范围条件 `status_code: {$gte: X00, $lt: (X+1)00}`（如 4xx → `{$gte:400, $lt:500}`）；空 = 全部。
- 所有过滤条件（q/path/status_class/日期/可见性）**AND 叠加**。
- service 层统一转换 `page/page_size → skip/limit`（复用现有 `normalizeAuditLimit` 语义收紧到 1~100）。
- 前端筛选栏：`q` 搜索框（placeholder「按操作人邮箱搜索」）+ **`path` 搜索框（placeholder「按 API 路径搜索」）** + **状态码下拉（全部/1xx/2xx/3xx/4xx/5xx）** + 日期范围；移除「操作类型下拉」（旧枚举值 bug，见 §2.1）。

### 5.2 hardcode mapping（需求 3）

- 位置：`internal/service/audit/actions.go`（新文件），hardcode map：

```go
var actionDescriptions = map[string]string{
    "POST /api/v1/chat":            "Chat 对话",
    "POST /api/v1/sessions":        "创建会话",
    "DELETE /api/v1/sessions/:id":  "删除会话",
    "PUT /api/v1/users/:id":        "编辑用户",
    // ... 全量 CUD 路由
}
```

- 展示：service 层把每条 log 附 `action_desc`（`ListResult` 里 `Logs` 项加字段 `ActionDesc string`；命中 mapping 用描述，未命中回退原始 `Action`）。
- 前端「操作类型」列显示 `action_desc`。
- CSV 导出「操作类型」列同步用描述。
- mapping 覆盖全量 gin 路由注册的 CUD 路径（以 `routes.go` 为清单核对），立项不展开清单，实现时补全。

### 5.3 参数校验完善（需求 4）

- 分页参数非法 → 400（`{"error": "invalid page"}` 之类）。
- 日期格式非法 → 400（service 的 `buildDateFilter` 错误改为带类型错误透传 → handler 区分 400/500）。
- Export：`limit` 1~50000（已有）；`format` 枚举校验——**本期仅支持 csv**，非法值 400；前端导出弹窗移除 json/xlsx 选项（或置灰）。
- 导出与列表**共享同一套过滤 + 可见性**逻辑。

### 5.4 可见性过滤（需求 5）

- `service.List`/`Export` 增加 viewer 参数：`ViewerUserID string` + `IsSystemAdmin bool`。
- 过滤条件（与用户搜索等其他 filter **AND** 叠加）：
  - `IsSystemAdmin == true` → 不加可见性条件。
  - 否则 → `$or: [{user_id: ViewerUserID}, {user_id: ""}, {user_id: {$exists: false}}]`（自己的 + 无 user 关联的）。
- handler 经 `ctx["user_id"]` + `ctx["role"] == "system_admin"` 取 viewer（`taskIdentity` 同模式）。
- 前端无需改动（后端过滤，列表自然变少）。

### 5.5 前端样式统一（需求 1）

- 筛选栏：`q` 搜索框（复用其他列表页样式）+ 开始/结束日期两个 date input + 筛选/重置按钮。
- 分页：现有 `Pagination` 组件（已复用），参数切换为 `page`/`page_size`。
- 操作类型列显示 `action_desc`（pill 样式保留）。
- 导出弹窗：收敛为 csv 单选项（或直接去掉格式选择）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `audit_logs`；无新字段——Action 已是模板形式） |
| 是否影响现有 API | 影响：`/admin/audit/logs` 参数语义变更（skip/limit → q/page/page_size）+ 响应加 `action_desc`；export 加 format 校验 + 可见性 |
| 性能影响 | 正向：`q` 走 DB 层 $regex；可见性过滤走 `user_id` 条件（建议评估 `{user_id:1, created_at:-1}` 索引，实现阶段确认） |
| 是否需要新增 Skill | No |
| 数据迁移 | 无（不改存量数据） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/audit/actions.go`（新） | hardcode action→描述 mapping | Low |
| `internal/service/audit/service.go` | List/Export 加 viewer 可见性过滤 + q 搜索 + action_desc 映射 | High |
| `internal/api/handler/audit.go` | 参数解析校验（400 化）+ viewer 注入 + 响应透传 | Medium |
| `internal/api/middleware/audit.go` | 无改动（Action 已含 method+模板路径） | — |
| `frontend/app/admin/audit/page.tsx` | 搜索栏/分页参数/action_desc 展示/导出弹窗收敛 | High |
| `internal/infra/mongo/audit_repository.go` | 可能新增查询方法（q 正则 / 可见性条件透传，视 service 组装方式） | Low |

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 前端交互变更（搜索/分页/导出弹窗）同步更新对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5 Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | `actionDescriptions` 映射、参数解析/校验纯函数 |
| L2 | 依赖接口，可 mock | **100%** | audit service 过滤组装、可见性条件 |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `audit_repository`、`handler/audit.go` |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** 验证可见性过滤三分支：system_admin 无过滤；非 system_admin 命中 `$or [自己, 空, 不存在]`；与其他 filter AND 叠加
- [ ] **必须** 验证参数校验：非法 page/page_size/q 超长/日期格式 → 400（非 500 非静默）
- [ ] **必须** 验证 D1 q 搜索链路：email 模糊 → topN userIDs → `$in` 过滤；查无匹配用户 → 空列表 total=0；与日期/可见性条件 AND 叠加
- [ ] **必须** 验证 D2 path 搜索：`action` 字段 `$regex`（QuoteMeta + 忽略大小写）命中 `METHOD 模板路径`；path 超长 → 400
- [ ] **必须** 验证 D3 status_class：5 个枚举各构造正确范围（`1xx→[100,200)` … `5xx→[500,600)`）；非法枚举 → 400；空 = 不筛选
- [ ] **必须** 验证 `action_desc`：mapping 命中返回描述、未命中回退原始 Action；CSV 导出用描述
- [ ] **必须** 验证 export format 仅 csv，非法 400
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%；`go vet` 无警告

## 10. 验证标准

1. 搜索/分页与其他列表页一致：`q`（操作人 email）经 email 模糊 → userIDs → `$in` DB 层过滤；`page`/`page_size` DB 层分页，`explain` 无全表扫描（视索引评估）。
2. 操作类型列显示中文描述（mapping 命中），未覆盖路由显示原始 `METHOD path` 回退。
3. 非法参数（分页/日期/format/q 超长）全部返回 400。
4. 可见性：普通用户/普通管理员仅见「自己的 + user 关联为空」的日志；system_admin 见全部；导出与列表一致。
5. 前端筛选下拉 bug 消失（旧枚举值废弃），q 按 email 搜索命中正确（查无匹配 email → 空列表）。
6. path 模糊搜索命中正确（搜 `sessions` 命中该路径相关全部 CUD 记录）；status_class 各枚举筛选范围正确（如 4xx 只返回 400~499）。

## 11. 设计定稿记录

### D1 — q 搜索策略（2026-09-17 晓军拍板）

`q` 关键词 = **操作人 email 模糊搜索**，链路定稿：

1. `users.SearchByEmail(q, topN=10)`（现有方法，email 模糊匹配）→ userIDs；
2. `len(userIDs) == 0` → 直接返回空列表（`total=0`，现有行为保留）；
3. 审计日志查询条件加 `user_id $in userIDs`（与其他条件 AND 叠加）；
4. `q` 不做 action/action_desc/details 的正则匹配。

> 备注：这实质是现有 `UserID` 过滤参数（email keyword → top10 → `$in`）的语义平移——参数名 `user_id` 改为 `q`，逻辑不变。

### D2 — path 模糊搜索（2026-09-17 晓军拍板）

新增 `path` 查询参数，支持 API 路径（不带 query 参数）模糊搜索：

1. 搜索对象 = `action` 字段（值 = `"METHOD FullPath模板"`，天然不含 query 参数、参数已泛化）；
2. DB 层条件：`action: {$regex: regexp.QuoteMeta(path), $options: "i"}`；
3. 长度 ≤100，超长 400；空 = 不筛选；
4. 与其他过滤条件（q/status_class/日期/可见性）AND 叠加。

### D3 — status_class 状态码类别筛选（2026-09-17 晓军拍板）

新增 `status_class` 查询参数，前端下拉选择、后端校验枚举并构造范围：

1. 合法枚举：`1xx` / `2xx` / `3xx` / `4xx` / `5xx`；空 = 全部；
2. 后端校验：非法枚举 → **400**；
3. 构造范围条件：`status_code: {$gte: X*100, $lt: (X+1)*100}`（如 `4xx` → `{$gte: 400, $lt: 500}`，`1xx` → `{$gte: 100, $lt: 200}`）；
4. 前端下拉：全部 / 1xx / 2xx / 3xx / 4xx / 5xx（`data-testid="audit-status-select"`）；
5. 与其他过滤条件 AND 叠加。
