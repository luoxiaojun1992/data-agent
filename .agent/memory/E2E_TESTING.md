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
| UI-160 | Prompt — 增强不计 Token | ✅ 已实现 |
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

**合计**: 111 个真实用例 + 7 个手动测试用例

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

## 运行 E2E

```bash
cd tests && npx playwright test
```
