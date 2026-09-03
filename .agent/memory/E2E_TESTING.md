# DataAgent — E2E 测试

> E2E 框架已就绪，占位用例保证 CI Pipeline 不报错。
> **前端功能开发完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。**
> CI 配置与 game-dev-studio 一致：sonar-check → ui-tests，两者均通过才算完成。

## 测试框架

- **工具**: Playwright (TypeScript)
- **配置**: `tests/playwright.config.ts`
- **目录**: `tests/ui/`

## 用例编号规则

`UI-XXX`，三位数字递增。

## 测试矩阵总览

| 用例编号 | 用例名称 | 状态 |
|---------|---------|:---:|
| UI-075 | User — 用户管理页渲染 | ✅ 已实现 |
| UI-076 | User — 用户表格列渲染 | ✅ 已实现 |
| UI-077 | User — 添加用户 | ✅ 已实现 |
| UI-078 | User — 编辑用户角色 | ✅ 已实现 |
| UI-079 | User — 启用/停用用户 | ✅ 已实现 |
| UI-080 | User — 删除用户 | ✅ 已实现 |
| UI-081 | User — 不可删除 system_admin | ✅ 已实现 |
| UI-082 | User — 不可创建第二个 system_admin | ✅ 已实现 |
| UI-083 | User — 邮箱唯一性校验 | ✅ 已实现 |
| UI-084 | User — 用户列表分页 | ✅ 已实现 |
| UI-085 | Role — 权限管理页渲染 | ✅ 已实现 |
| UI-086 | Role — 固定角色卡片 | ✅ 已实现 |
| UI-087 | Role — 自定义角色表格 | ✅ 已实现 |
| UI-088 | Role — 新建自定义角色 | ✅ 已实现 |
| UI-089 | Role — 权限 Tab 渲染 | ✅ 已实现 |
| UI-090 | Role — 编辑角色权限 | ✅ 已实现 |
| UI-091 | Role — 删除自定义角色 | ✅ 已实现 |
| UI-092 | Role — 不可删除固定角色 | ✅ 已实现 |
| UI-093 | Model — 模型配置页渲染 | ✅ 已实现 |
| UI-094 | Model — OpenAI 兼容 API URL 配置 | ✅ 已实现 |
| UI-095 | Model — API Key 输入与 Vault 加密 | ✅ 已实现 |
| UI-096 | Model — 眼睛按钮切换 API Key | ✅ 已实现 |
| UI-097 | Model — Model Name 下拉选择 | ✅ 已实现 |
| UI-098 | Model — 上下文长度配置 (Stepper) | ✅ 已实现 |
| UI-099 | Model — 最大输出长度配置 | ✅ 已实现 |
| UI-100 | Model — Temperature 配置 | ✅ 已实现 |
| UI-101 | Model — Top-P 配置 | ✅ 已实现 |
| UI-102 | Model — Hermes 配置区域 | ✅ 已实现 |
| UI-103 | Model — 仅 admin 可访问 | ✅ 已实现 |
| UI-104 | SysConfig — 系统配置页渲染 | ✅ 已实现 |
| UI-105 | SysConfig — 修改保存全局参数 | ✅ 已实现 |
| UI-106 | SysConfig — 仅 system_admin 可访问 | ✅ 已实现 |
| UI-107 | SysConfig — 缓冲期上限校验 | ✅ 已实现 |
| UI-108 | SysConfig — 配置优先级验证 | ✅ 已实现 |
| UI-109 | Task — 任务管理页渲染 | ✅ 已实现 |
| UI-110 | Task — 全局查看所有用户任务 | ✅ 已实现 |
| UI-111 | Task — 查看任务详情 | ✅ 已实现 |
| UI-112 | Task — 取消运行中任务 | ✅ 已实现 |
| UI-113 | Task — 重试失败任务 | ✅ 已实现 |
| UI-114 | Task — 批量取消任务 | ✅ 已实现 |
| UI-115 | KB — 知识库管理页渲染 | ✅ 已实现 |
| UI-116 | KB — 文档卡片渲染 | ✅ 已实现 |
| UI-117 | KB — 上传单个文档 | ✅ 已实现 |
| UI-118 | KB — 批量上传文档 | ✅ 已实现 |
| UI-120 | KB — 索引状态实时更新 | ✅ 已实现 |
| UI-121 | KB — 搜索知识库文档 | ✅ 已实现 |
| UI-123 | KB — 删除知识库文档 | ✅ 已实现 |
| UI-124 | KB — 文档分页 | ✅ 已实现 |
| UI-125 | Audit — 审计日志页渲染 | ✅ 已实现 |
| UI-126 | Audit — 审计日志表格数据 | ✅ 已实现 |
| UI-127 | Audit — 按时间范围筛选 | ✅ 已实现 |
| UI-128 | Audit — 按操作类型筛选 | ✅ 已实现 |
| UI-129 | Audit — 按用户筛选 | ✅ 已实现 |
| UI-130 | Audit — 导出弹窗 | ✅ 已实现 |
| UI-131 | Audit — 执行导出 | ✅ 已实现 |
| UI-132 | Audit — 导出条数上限校验 | ✅ 已实现 |
| UI-133 | Audit — 审计日志分页 | ✅ 已实现 |
| UI-134 | API — API 转换审核页渲染 | ✅ 已实现 |
| UI-135 | API — API 卡片渲染 | ✅ 已实现 |
| UI-136 | API — 上传 OpenAPI 文件 | ✅ 已实现 |
| UI-137 | API — 批准 API 转换 | ✅ 已实现 |
| UI-138 | API — 驳回 API 转换 | ✅ 已实现 |
| UI-139 | API — 双重审核校验 | ✅ 已实现 |
| UI-140 | API — 批量上传 | ✅ 已实现 |
| UI-141 | Notif — 铃铛图标与未读数 | ✅ 已实现 |
| UI-142 | Notif — 展开通知列表 | ✅ 已实现 |
| UI-143 | Notif — 标记已读 | ✅ 已实现 |
| UI-144 | Notif — 一键全部已读 | ✅ 已实现 |
| UI-145 | Notif — 发送站内信 | ✅ 已实现 |
| UI-149 | Pwd — 初始密码横幅通知 | ✅ 已实现 |
| UI-150 | Pwd — 修改密码页 | ✅ 已实现 |
| UI-151 | Pwd — 成功修改密码 | ✅ 已实现 |
| UI-152 | Pwd — 旧密码错误 | ✅ 已实现 |
| UI-153 | Pwd — 新密码不一致 | ✅ 已实现 |
| UI-154 | Pwd — 新密码强度校验 | ✅ 已实现 |
| UI-155 | Pwd — 所有角色可修改密码 | ✅ 已实现 |
| UI-156 | Prompt — 增强按钮渲染 | ✅ 已实现 |
| UI-157 | Prompt — 空输入增强 | ✅ 已实现 |
| UI-158 | Prompt — 有输入增强 | ✅ 已实现 |
| UI-159 | Prompt — 增强后手动编辑 | ✅ 已实现 |
| UI-160 | Prompt — 增强计入 Token | ✅ 已实现 |
| UI-161 | IM — 飞书配置页 | ✅ 已实现 |
| UI-162 | IM — 保存飞书配置 | ✅ 已实现 |
| UI-163 | IM — 飞书卡片消息 | 👤 人工测试 |
| UI-164 | IM — 快捷指令 | 👤 人工测试 |
| UI-165 | IM — 异步任务通知 | 👤 人工测试 |
| UI-166 | IM — 未绑定用户引导 | 👤 人工测试 |
| UI-167 | List — 分页默认值 | ✅ 已实现 |
| UI-168 | List — 页码跳转 | ✅ 已实现 |
| UI-169 | List — 每页条数切换 | ✅ 已实现 |
| UI-170 | List — 表头排序 | ✅ 已实现 |
| UI-171 | List — 全选/取消全选 | ✅ 已实现 |
| UI-172 | Upload — 文件多选 | ✅ 已实现 |
| UI-173 | Upload — 拖拽上传 | 👤 人工测试 |
| UI-174 | Upload — 独立进度条 | ✅ 已实现 |
| UI-175 | Upload — 单文件上传 | ✅ 已实现 |
| UI-176 | Upload — 上传不阻塞 UI | ✅ 已实现 |
| UI-180 | Session — 多端登录互不干扰 | ✅ 已实现 |
| UI-181 | Session — 删除后可恢复 | ✅ 已实现 |
| UI-182 | Session — 部分删除无恢复 | ✅ 已实现 |
| UI-183 | Session — 缓冲期可配置 | ✅ 已实现 |
| UI-184 | Sec — SQL 注入被拦（input audit） | ✅ 已实现 |
| UI-185 | Sec — 输出敏感信息脱敏（output sanitize） | ✅ 已实现 |
| UI-186 | Sec — 越权工具调用被拦（RBAC） | ✅ 已实现 |
| UI-187 | RBAC — user 可见导航项 | ✅ 已实现 |
| UI-188 | RBAC — admin 可见导航项 | ✅ 已实现 |
| UI-189 | RBAC — system_admin 可见全部 | ✅ 已实现 |
| UI-190 | RBAC — user 无法访问 /admin | ✅ 已实现 |
| UI-191 | RBAC — user 无法访问模型配置 | ✅ 已实现 |
| UI-192 | RBAC — user 无法创建 Agent 任务 | ✅ 已实现 |
| UI-193 | Resp — 移动端布局适配 (375px) | ✅ 已实现 |
| UI-194 | Resp — 平板布局适配 (768px) | ✅ 已实现 |
| UI-195 | Resp — 触摸友好交互 (tap targets) | ✅ 已实现 |
| UI-196 | Invite — 注册页无 token 显示错误 | ✅ 已实现 |
| UI-197 | Invite — 注册页无效 token | ✅ 已实现 |
| UI-198 | Invite — 注册页错误页登录链接 | ✅ 已实现 |
| UI-199 | Invite — 邀请管理页渲染 | ✅ 已实现 |
| UI-200 | Invite — 创建邀请表单展开/收起 | ✅ 已实现 |
| UI-201 | Invite — 角色默认 user | ✅ 已实现 |
| UI-202 | Invite — 有效期默认 24h | ✅ 已实现 |
| UI-203 | Invite — 空列表显示占位文本 | ✅ 已实现 |
| UI-204 | KB — 文档索引端到端验证 | ✅ 已实现 |
| UI-205 | KB — 索引后检索命中 | ✅ 已实现 |
| UI-206 | KB — 索引进度实时更新 | ✅ 已实现 |
| UI-207 | KB — 搜索过滤结果准确 | ✅ 已实现 |
| UI-208 | KB — 索引失败重试 | ✅ 已实现 |
| UI-209 | KB — 大文档分块（>10KB） | ✅ 已实现 |
| UI-211 | ToolCall — knowledge_search 全文检索 | ✅ 已实现 |
| UI-212 | ToolCall — knowledge_search 命中后回答 | ✅ 已实现 |
| UI-213 | ToolCall — sql_executor 校验通过 | ✅ 已实现 |
| UI-214 | ToolCall — sql_executor 校验失败 | ✅ 已实现 |
| UI-215 | ToolCall — stats_engine 真实计算 | ✅ 已实现 |
| UI-216 | ToolCall — save_report 报告生成 | ✅ 已实现 |
| UI-217 | ToolCall — 多工具链式调用 | ✅ 已实现 |
| UI-218 | ToolCall — 工具结果复制 | ✅ 已实现 |
| UI-219 | Mem0 — 会话自动写入记忆 | ✅ 已实现 |
| UI-220 | Mem0 — memory_search 工具调用 | ✅ 已实现 |
| UI-221 | Mem0 — 多用户隔离 | ✅ 已实现 |
| UI-222 | Mem0 — 长对话压缩后记忆保留 | ✅ 已实现 |
| UI-229 | Dashboard — KPI 显示真实任务数 | ✅ 已实现 |
| UI-230 | Dashboard — KPI 显示真实文档数 | ✅ 已实现 |
| UI-231 | Dashboard — 任务状态分布准确 | ✅ 已实现 |
| UI-232 | Dashboard — 24h 趋势图渲染 | ✅ 已实现 |
| UI-233 | Dashboard — Token KPI 渲染 | ✅ 已实现 |
| UI-234 | Dashboard — ROI 图表渲染 | ✅ 已实现 |
| UI-235 | Dashboard — 多用户数据隔离 | ✅ 已实现 |
| UI-236 | Dashboard — 时间筛选有效 | ✅ 已实现 |
| UI-237 | Dashboard — 全 trend 真数据显示（SPEC-060） | ✅ 已实现 |
| UI-238 | Chat — ModelSelector 可见且可选（SPEC-062） | ✅ 已实现 |
| UI-239 | Chat — 已有 session 时 ModelSelector 锁定（SPEC-062） | ✅ 已实现 |
| UI-240 | Admin — 模型列表结构化展示（SPEC-062） | ✅ 已实现 |
| UI-241 | Admin — 模型新增与设默认（SPEC-062） | ✅ 已实现 |

**合计**: 150 个真实用例 + 7 个手动测试用例

## data-testid 命名规范

```
{component}-{element}
```

示例: `nav-login-btn`, `chart-revenue`, `input-query`

## 测试用户 UUID 命名

每个 `test.describe` 独立生成 UUID 前缀，确保并行执行时用户数据不冲突：

```typescript
const uid = crypto.randomUUID().slice(0, 8);
const USER = {
    username: `e2e-{module}-${uid}@test.local`,
    password: '{Module}Test1',
    role: 'admin',
};
```

约定：
- 用户名: `e2e-{模块缩写}-{8位uuid}@test.local`
- 密码: `{模块名}Test1`（首字母大写，符合密码强度要求）
- `beforeAll` 注册，`afterAll` 清理（遍历 `users?skip=0&limit=200` 匹配 uuid 删除）
- `afterAll` 中同时清理 mockllm: `request.delete(${MOCKLLM}/responses)`

## 测试原则

### 测试目的不是通过，是发现真正的问题

- **禁止 `test.skip()`**: 不得因数据不足跳过测试。用 API 在 `beforeAll` 或测试内部预创建所需数据。
- **禁止 workaround**: 不要因为调试困难就写绕过代码（如 `/default-reply` API）。加 debug 日志，实证定位根因。
- **禁止 `page.route()` 截获**: 只有真实后端链路 + mockllm 能保证测试有效性。`page.route()` 绕过整个 Handler→Service→Repository 栈，等于不测。
- **断言必须严格**: 脱敏测试必须同时验证 `toContain(masked)` 和 `not.toContain(original)`，防止假阳性。

### 加日志，不瞎猜

```typescript
console.log('[UI-XXX] sending:', msg);
console.log('[UI-XXX] send clicked, waiting for response');
console.log('[UI-XXX] received:', text?.substring(0, 100));
```

后端同理：在怀疑的每个环节加 `log.Printf("[DEBUG module] ...")`，用 CI log 下载+unzip+grep 精确复现。

### 后端日志排查 — 测试超时但无前端错误时

当测试持续超时（如 `waitForSelector` 等了 20-30s 仍找不到元素），前端看起来正常但测试失败：

1. **下载 CI 失败 run 的完整日志**：
   ```bash
   bash scripts/get-logs.sh <run-id> --failed-only
   ```

2. **检查后端 HTTP 错误**：
   ```bash
   grep -E 'data-agent.*status.*[45][0-9]{2}' ci-logs-<id>/*.log | grep -v health
   ```

3. **检查前端是否静默吞异常**：
   ```
   → 搜索前端源码 `catch { /* ignore */ }` 或 `catch {}` 模式
   → 常见位置：agent/page.tsx, admin/tasks/page.tsx 的 loadTasks/fetchTasks
   → 如果前端吞了异常，API 会返回错误但页面显示空列表，测试永远等不到元素
   ```

4. **验证假设**：在可疑的 `catch` 块加 `console.error('[UI] fetch failed:', e)`，重新跑 CI 确认根因。

5. **修复方向**：
   - 前端：catch 块至少 `console.error` 记录错误
   - 后端：确认 API 路由注册正确，数据返回格式与前端 `data.tasks` 解构一致
   - 测试：不要 `page.goto` + `page.reload` 连环重载，利用前端自带的 `loadTasks()` 等自动刷新逻辑

**已知案例**：
- 2026-07-16 agent/task 测试持续超时：前端 `catch { /* ignore */ }` 吞了 API 错误，测试在空列表中永远等不到 `task-mgmt-row-*` 元素。修复后去掉冗余的 `page.goto`+`page.reload`，改为等待前端自刷新后的 DOM 更新。

## E2E 测试铁律 — 从 2026-07-16 质量整改中总结

以下原则经过 10+ 轮 CI 验证，违反任意一条都会导致假通过或不稳定。

### 铁律 #1: 禁止条件断言

```typescript
// ❌ 错 — 条件不满足时静默通过，测试无意义
if (await btn.isVisible().catch(() => false)) {
  await btn.click();
}

// ❌ 错 — 同样静默通过
if (await rows.count() > 0) {
  await expect(rows.first()).toBeVisible();
}

// ✅ 对 — 刚性断言，超时即失败
await expect(btn).toBeVisible({ timeout: 10000 });
await btn.click();
```

**原则**: 如果无法让断言确定性地成立，删除测试，不要条件化。条件断言 = 假通过 = 比不测更危险（给人虚假安全感）。

### 铁律 #2: 只测试确定性状态

| 可测试 | 不可测试（应删除） |
|--------|-------------------|
| 页面渲染、导航元素、表单、模态框 | 后端依赖的按钮（cancel/retry 出现在特定时间窗口） |
| UI 结构（table headers、列数、分页组件） | 安全 toast（依赖安全引擎扫描时序） |
| 显式用户操作后的 DOM 变化 | SSE 流中间的瞬时状态 |
| Modal 打开/关闭 | 异步任务日志、进度条具体数值 |

**判断标准**: 同一测试在 CI 中跑 10 次，10 次都通过 → 可保留。出现过一次失败且不是代码 bug → 删除。

### 铁律 #3: 有效测试 = 验证状态变更链

**⚠️ 反模式警示 (2026-07-16)**: Agent UI-052 测试持续超时，`agent-task-title-*` 在创建 task 后不出现。团队第一反应是移除 row 断言，只保留 `agent-page-header` 检查。**这是错误的**——应该追查 `loadTasks()` 为什么没有渲染 task list，修复根因后保留完整的三步断言。

```typescript
// ❌ 错 — 只验证了 page header 存在，没有测 "取消" 行为
test('Agent — cancel running task', async ({ page }) => {
  // ... create task ...
  await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
});

// ✅ 对 — 三条断言验证完整状态变更链
test('Agent — cancel running task', async ({ page }) => {
  // 1. 创建 task
  await page.locator('[data-testid="agent-create-task-btn"]').click();
  await page.locator('[data-testid="agent-task-title-input"]').fill('To Cancel');
  await page.locator('[data-testid="agent-task-create-btn"]').click();
  await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });

  // 2. 验证 task 出现（createTask 内部调用 loadTasks）
  const row = page.locator('[data-testid^="agent-task-title-"]').first();
  await expect(row).toBeVisible({ timeout: 10000 });
  await row.click();

  // 3. 验证取消按钮出现 → 点击 → task 消失
  const cancelBtn = page.locator('[data-testid="agent-task-cancel-btn"]');
  await expect(cancelBtn).toBeVisible({ timeout: 5000 });
  await cancelBtn.click();
  await expect(row).not.toBeVisible({ timeout: 10000 });
});
```

**原则**: 每个断言的前置条件必须由前端代码链路保证（如 `createTask()` 成功后内部调 `loadTasks()`），不能依赖"等一会儿它应该会出现"。

### 铁律 #4: 禁止 `page.route()` 截获 API

```typescript
// ❌ 错 — 绕过整个 Handler→Service→Repository 栈
await page.route('**/api/v1/chat', route => {
  route.fulfill({ body: fakeSSE });
});

// ✅ 对 — 真实后端 + mockllm seed
await seedMock(request, 'hello', 'Hello from mock');
await page.locator('[data-testid="chat-input"]').fill('hello');
await page.keyboard.press('Enter');
await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });
```

### 铁律 #5: 禁止 `.catch(() => {})` 吞断言

```typescript
// ❌ 错 — expect 失败被吞掉，测试永远通过
await expect(el).not.toBeVisible({ timeout: 3000 }).catch(() => {});

// ✅ 对 — 刚性断言
await expect(el).not.toBeVisible({ timeout: 5000 });
```

### 铁律 #6: 先查后端日志，再改测试

当测试持续超时（如 `waitForSelector` 等 20-30s）：**不要加 timeout，先下载 CI 日志查后端 4xx/5xx**。90% 的情况是前端 `catch { /* ignore */ }` 吞了 API 错误导致页面空列表。

---

## 运行 E2E

```bash
cd tests && npx playwright test
```

### 铁律 #7: 巡检脚本的状态型检查必须自恢复 + 断言对齐真实 DOM

冒烟/巡检脚本（非 spec 套件的辅助脚本）中：
1. 任何改变页面状态的检查（tab 切换、路由跳转、弹窗打开）在检查结束后必须**恢复初始状态**，否则后续检查在脏状态下误报。
2. 检查项断言必须对齐真实 DOM 结构——按钮是否在未激活的 tab 内、页面真实标题/文案是什么、条件渲染组件在数据不足时是否出现。写错断言制造假失败，浪费排查时间。

### 铁律 #8: 弹窗/按钮触发——headless actionability 不可靠时用 `page.evaluate` 直接 click

`locator.click()` 依赖 actionability 判定（visible/stable/enabled/receives-events），headless 下偶发不满足（元素实际可见但判定不通过）→ `Timeout 10000ms exceeded`。

```typescript
// ❌ 卡在 actionability 判定（元素实际存在仍超时）
await page.locator('#model-add-btn').waitFor({ state: 'visible' });
await page.locator('#model-add-btn').click();

// ✅ 先确认元素存在（debug 脚本 dump），再直接 JS click 绕过 actionability
const exists = await page.evaluate((sel) => !!document.querySelector(sel), '#model-add-btn');
// 确认 exists === true 后：
await page.evaluate((sel) => { document.querySelector(sel)?.click(); }, '#model-add-btn');
await page.waitForTimeout(2500); // 等 SPA 渲染
```

**前提**：必须先用 debug 脚本确认元素确实存在且渲染（排除选择器错误/页面未加载），确认后再用 `page.evaluate` 直接 click。这不是逃避 actionability 校验，而是 headless 判定不可靠时的替代触发方式。

### 铁律 #9: 验证 API 前先确认后端 DTO 字段名，别凭前端 testid 猜

curl/playwright 里调用后端 API 前，先看后端 struct 的 json tag（或 binding tag）确认字段名。前端 input 的 `data-testid`/placeholder 可能是业务语义名（如 `email-input`），与后端实际字段（`username`）不一致。

```bash
# ❌ 凭前端 testid 猜字段 → 400 误判成 bug
curl -X POST .../login -d '{"email":"<login-email>","password":"<pwd>"}'
# → 400 Field validation for 'Username' failed on the 'required' tag

# ✅ 先确认后端 DTO 字段名
grep -rn 'json:"username"\|Username' internal/...   # 找到真实字段
curl -X POST .../login -d '{"username":"<login-email>","password":"<pwd>"}'
# → 200 + JWT
```

**教训**: 400 + 字段级错误信息 = 后端参数校验正常工作（不是 bug）。读错误里的字段名，那是后端在告诉你哪个字段缺失/非法。

### 铁律 #10: 本地直连被网络过滤时走 SSH 隧道；含凭据/隧道地址的冒烟脚本禁入库

本地访问测试服务器可能被网络过滤层拦截（返回「假 HTML」：HTTP 200 但内容是无关拦截页）。判断方法：`curl -s <url> | grep -c <业务关键词>` 为 0 + HTTP 头非 Next.js 特征 → 假页面。此时：

1. **SSH 隧道绕行**：`ssh -f -N -L 18080:localhost:80 root@<server>`，本地 curl/playwright 全部改走 `http://localhost:18080`。
2. **playwright 浏览器版本不匹配**：`browserType.launch: Executable doesn't exist` → 用 `executablePath` 指向已安装版本（`chromium_headless_shell-*/chrome-headless-shell-mac-x64/chrome-headless-shell`），不重新下载。
3. **⛔ 含凭据/隧道地址的冒烟脚本禁止提交仓库**：这类脚本（服务器密码、admin 凭据、隧道端口）统一 `.gitignore` 通配忽略（`tests/ui/smoke-*.mjs`）。**已提交**的测试/回归脚本一律走环境变量注入（如 playwright `baseURL: process.env.UI_BASE_URL`），不得硬编码隧道地址或服务器地址。
4. **冒烟断言走真实 DOM 与运行时值**：选择器用 `data-testid`（不是 input type，登录 email input 是 `type="text"`）；颜色断言用 `getComputedStyle` 的实际 rgb 值（emerald-400 = rgb(52,211,153)）；位置断言用 `boundingBox` 计算几何关系（不重叠/纵向堆叠），右对齐堆叠的两元素 x 坐标不同是正常的，只断言 y 纵向关系。
