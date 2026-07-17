# 主分支 UI 截图审查与布局修复

> **SPEC-047** | Status: 设计中 | Date: 2026-07-17 | Phase: P11

## 1. 目标

系统审查 main 分支 UI E2E 测试产物（`docs/manual-screenshots/01-27-*.png`），识别前端页面中存在的**布局错乱、元素重叠、内容溢出、动画卡帧、路径错配**等视觉/交互 bug，逐项修复并新增防回归测试。

> **核心原则**: **截图是真相**。所有声称「功能正常」的页面必须在 CI 截图中可被肉眼验证。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-017~042 | ✅ | UI E2E 测试套件（已实现） |
| SPEC-018 | ✅ | LAYOUT 布局测试（已实现） |
| SPEC-019 | ✅ | CHAT 测试（已实现，但未覆盖 session 面板布局） |
| SPEC-022 | ✅ | Dashboard UI（已实现但 Dashboard 全 0 数据 — 由 SPEC-046 解决） |
| SPEC-028 | ✅ | KB UI（已实现但 KB 永远"已上传" — 由 SPEC-046 解决） |
| SPEC-040 | ✅ | 响应式设计（已实现） |

## 2. 背景

### 2.1 审查范围

**27 张主分支 UI 截图**（`docs/manual-screenshots/01-login.png` ~ `27-api-approve.png`）由 GitHub Actions Run #29568729640（2026-07-17）Playwright 全量测试生成。

经逐张肉眼审查，识别出 **7 类 bug**，严重程度分级如下：

| 严重度 | 数量 | 说明 |
|:------:|:----:|------|
| 🔴 P0 阻塞 | 3 | 功能完全不可用、严重重叠 |
| 🟠 P1 重要 | 4 | 视觉错乱、动画卡帧、路径错配 |
| 🟡 P2 优化 | 2 | 表格列宽、按钮密度 |
| **合计** | **9** | — |

### 2.2 Bug 详细清单

#### 🔴 P0-1: Chat Session 面板严重重叠（16-chat-session.png）

**现象**:
- 主聊天区与"历史会话"侧栏完全重叠
- 文本元素互相穿插：Session ID 列表（140a2237 / 4d40247d / ...）与"Chat 对话"主标题、新建会话表单、输入框混叠
- "提示词 / 增强"按钮**文字竖向排列**（每个字独占一行）
- "输入 / 发送"文字**竖向排列**
- 整页布局呈现"上下挤压 + 左右堆叠"的混乱状态

**根因**（`frontend/app/chat/page.tsx:397-440`）:
```jsx
<div className="flex h-[calc(100vh-64px)] animate-fade-in">
  {/* Main chat area */}
  <div className="flex-1 flex flex-col min-w-0">...</div>

  {/* Session Panel — BUG: 缺少 width + flex-shrink-0 */}
  {showSessions && (
    <div className="border-t border-[var(--border-glass)]" data-testid="session-sidebar">
      ...
    </div>
  )}
</div>
```

**问题**:
1. `border-t` 暗示是垂直堆叠元素，但父容器是 `flex`（水平布局）
2. session 面板没有固定 `width` 也没有 `flex-shrink-0`，所以它会**压缩为最小宽度**
3. 所有内联元素在没有明确宽度的情况下变成 `min-content`，中文/英文文本变成竖向
4. `max-h-48` (session-list) 与 `min-w-0` 缺失导致文字溢出

**修复**:
```jsx
{/* Session Panel — FIXED */}
{showSessions && (
  <div className="w-72 flex-shrink-0 border-l border-[var(--border-glass)] h-full overflow-y-auto bg-[var(--bg-secondary)]/30" data-testid="session-sidebar">
    <div className="p-3">
      ...
    </div>
  </div>
)}
```

#### 🔴 P0-2: 审计日志加载失败（11-audit-log.png, 24-notif-bell.png, 25-notif-list.png）

**现象**:
- 页面顶部出现红色 **"加载失败"** 错误条幅
- 但下方仍显示"暂无审计日志"空状态
- 三个截图均出现此问题：审计页 + 通知铃铛 + 通知列表

**根因**:
- `frontend/app/admin/audit/page.tsx` 的 audit 数据拉取在异常分支显示 `加载失败`，但失败原因被 `try/catch` 吞掉
- 实际错误可能是后端 audit 端点 5xx（user/role 上下文缺失？鉴权？）
- 通知页面（24/25）实际渲染的是 audit 页面 → **说明通知 API 失败导致降级到 audit 兜底？或路由配置错误？**

**待调查**:
1. 后端 `/api/v1/audit` 端点的实际错误响应
2. 前端 audit 页面如何"fallback"到 audit 页面（如果确实发生了）
3. 通知页面为什么没渲染自己的组件

**修复**（先做错误暴露，再做后端修复）:
```typescript
// 前端：显示具体错误而非通用"加载失败"
catch (err) {
  setError({
    message: err.message,
    status: err.status,
    endpoint: '/api/v1/audit',
  });
}
```

#### 🔴 P0-3: 通知页面路径错误（24-notif-bell.png, 25-notif-list.png）

**现象**:
- 24 截图（测试名 "Notif — 铃铛图标"）实际渲染的是**审计日志页**
- 25 截图（测试名 "Notif — 展开通知列表"）实际渲染的是**审计日志页 + 通知下拉面板**（叠加状态）
- 通知页面应渲染独立组件，但截图说明它重定向/降级到 audit 页面

**根因（推测）**:
- `frontend/app/notifications/page.tsx`（或类似路由）可能未实现，导致 Next.js 渲染最近的 404 fallback 到 audit
- 或用户无通知权限，组件被替换为 audit
- 需查 Next.js 路由表 + 通知组件实际路径

**修复**:
- 实施第一步：定位通知页面实际路由
- 实施第二步：如果通知功能未实现则开发（建议拆出独立 spec）
- 实施第三步：通知测试用 admin 账号（确保有通知权限）

#### 🟠 P1-1: 页面 fade-in 动画卡帧（03-agent-workspace.png, 09-task-mgmt.png, 17-agent-filters.png）

**现象**:
- 页面标题、副标题、"可用技能"区域、状态标签全部呈**半透明**状态
- 仅"新建任务"按钮、列表项、空状态文字保持正常不透明
- 像是截图时正处在 `animate-fade-in` 动画中间帧（0.5s 内）

**根因**（`frontend/app/globals.css:80-92`）:
```css
@keyframes fadeIn {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}
.animate-fade-in {
  animation: fadeIn 0.5s ease-out;
}
```

`animation-fill-mode` 默认为 `none`，动画结束后属性回归原始值（即无 `from` 设置的状态）— 但因是 opacity 0→1，关键帧结束后应是 1，所以这里可能是**截图时机问题**（CI 在 0.5s 内截图）。

**修复方案**（双管齐下）:
1. **CSS 加 `forwards`**: 确保动画结束状态保持
   ```css
   .animate-fade-in {
     animation: fadeIn 0.5s ease-out forwards;
   }
   ```
2. **Playwright 全局等待动画完成**: 在 `playwright.config.ts` 增加 `page.addStyleTag({ content: '*, *::before, *::after { animation-duration: 0s !important; }' })` 注入到 `use: {}` 钩子

**推荐方案 1**（最小侵入、保持视觉体验）。方案 2 会让所有 E2E 截图"无动画"，可能掩盖其他动画 bug。

#### 🟠 P1-2: API 审核页操作按钮（12-api-review.png vs 27-api-approve.png）

**现象**:
- 12-api-review 截图显示 CRM API 卡片，状态为 `待审核`，右侧仅 `待审核` / `等待审核` 标签，无操作按钮
- 27-api-approve 截图（测试名"API — 批准 API 转换"）状态也是 `待审核`，但右侧有 `批准` / `驳回` 按钮

**推断**:
- 两个截图状态相同（都"待审核"），但 27 有操作按钮 → **可能是 27 截图的卡片实际是 `pending` 状态（可操作）而 12 是 `reviewing` 状态（待复核）**
- 状态枚举命名不一致：`pending` / `reviewing` / `approved` / `rejected`？需要查后端定义

**修复**:
1. 统一状态枚举命名（前端 + 后端）
2. 状态 `pending` 才显示「批准 / 驳回」按钮
3. 增加 `data-testid="api-status-{status}"` 便于测试断言

#### 🟠 P1-3: KB 搜索测试无效果（21-kb-search.png）

**现象**:
- 21-kb-search.png 与 10-knowledge-base.png **二进制完全相同**（MD5 一致）
- 测试名"KB — 搜索"应触发搜索输入过滤，但截图无任何变化

**根因**:
- `tests/ui/kb.spec.ts:139-162` UI-121 测试：搜索"销售"后断言 `afterCount >= 1`（永远为 true）— **断言无效**
- 测试只检查元素数量，不验证内容过滤

**修复**:
- 强化测试断言（合并到 SPEC-046 的 UI-207）：搜索"销售"应只返回 1 个文档（`销售数据汇总`），不包含 `Q2 财务报告` / `客户合同模板`
- 该问题将由 SPEC-046 解决

#### 🟠 P1-4: Agent Download 测试无效（19-agent-download.png）

**现象**:
- 19-agent-download.png 与 04-task-detail.png **二进制完全相同**（MD5 一致）
- 测试名"Agent — 下载"应触发下载流程，但截图无任何操作

**根因**:
- `tests/ui/agent-extras.spec.ts` 中"下载"测试可能仅点击展开按钮，未真正下载
- 或下载流被 mock 跳过

**修复**:
- 强化测试：点击下载按钮 → 监听 `page.on('download')` → 验证文件名
- 由 SPEC-046 强化 agent.spec.ts 解决

#### 🟡 P2-1: Dashboard 图表扁平化（05-dashboard.png, 22-dashboard-token.png, 23-dashboard-roi.png）

**现象**:
- 6 个图表（柱状图）显示为**扁平线段**，柱体高度 ≈ 1-2px
- 即使有数据，柱体过矮无法辨识
- 成功率趋势 Mon-Sun 显示 7 个**等高满绿块**（应为高低变化）

**根因**（`frontend/app/page.tsx:24-40`）:
```jsx
function SimpleBar({ data }) {
  const max = Math.max(...data.map(d => d.value), 1);
  return (
    <div className="flex items-end gap-2" style={{ height: '100px' }}>
      {data.map((d, i) => (
        <div key={i} className="...">
          <div className="w-full rounded-t" style={{
            height: `${Math.max(8, (d.value / max) * 80)}px`,  // ← BUG
            ...
            opacity: 0.7,  // ← 0.7 透明度
          }} />
```

**问题**:
- `Math.max(8, ...)` 最小 8px 是合理的，但 `(d.value / max) * 80` 在数据全 0 时 max=1，柱子都是 `Math.max(8, 0) = 8px`，看不出高低
- `opacity: 0.7` 让深色背景上几乎不可见
- 没有数据时（0/0）显示固定 8px 矮柱，看上去像「有数据但很矮」

**修复**:
1. 真实数据由 SPEC-046 解决
2. CSS 优化：增加 min-height 检测、移除 0.7 透明度
3. 空数据占位：明确显示「暂无数据」而非渲染 8px 矮柱

```jsx
function SimpleBar({ data, emptyText = '暂无数据' }) {
  const hasData = data.some(d => d.value > 0);
  if (!hasData) {
    return (
      <div className="flex items-center justify-center h-[100px] text-xs text-[var(--text-secondary)]">
        📊 {emptyText}
      </div>
    );
  }
  const max = Math.max(...data.map(d => d.value), 1);
  return (
    <div className="flex items-end gap-2" style={{ height: '100px' }}>
      {data.map((d, i) => (
        <div key={i} className="flex-1 flex flex-col items-center gap-1">
          <div className="w-full rounded-t" style={{
            height: `${Math.max(8, (d.value / max) * 80)}px`,
            backgroundColor: d.color || 'var(--accent)',
            opacity: 1,  // 不再降透明
            minHeight: '8px',  // 占位高度
          }} />
          <span className="text-[9px] text-[var(--text-secondary)]">{d.label}</span>
        </div>
      ))}
    </div>
  );
}
```

#### 🟡 P2-2: 用户管理操作列拥挤（06-user-mgmt.png）

**现象**:
- 用户表格操作列（查看/编辑/启用/删除）按钮**过小且密集**
- 8-10 个按钮挤在一行，字号约 10px，几乎不可读

**根因**（`frontend/app/admin/users/page.tsx`）:
- 操作列未限制按钮尺寸
- 桌面端视口 1280px 时 8 列均分，操作列宽不足

**修复**:
- 操作列使用 Icon-only 按钮 + `tooltip` 替代文字按钮
- 或将操作列改为下拉菜单（... 按钮）
- 最小宽度 32px、间距 4px

```jsx
<td className="px-2 py-2">
  <div className="flex items-center justify-end gap-1 min-w-[120px]">
    <button className="w-7 h-7 rounded hover:bg-white/10" title="查看">👁</button>
    <button className="w-7 h-7 rounded hover:bg-white/10" title="编辑">✏️</button>
    <button className="w-7 h-7 rounded hover:bg-white/10" title="启用/停用">{user.active ? '⏸' : '▶'}</button>
    <button className="w-7 h-7 rounded hover:bg-red-400/10 text-red-400" title="删除">🗑</button>
  </div>
</td>
```

## 3. 架构概述

### 3.1 Bug 分布拓扑

```
┌────────────────────────────────────────────────────────────┐
│                    截图审查覆盖                              │
│                                                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │  Layout Bugs │  │  Data Bugs   │  │  Routing Bugs│         │
│  │              │  │              │  │              │         │
│  │ • Chat 面板  │  │ • Dashboard  │  │ • 通知页 404 │         │
│  │   重叠 P0-1  │  │   全 0 数据  │  │   → audit    │         │
│  │ • fade 卡帧  │  │ • KB 永远    │  │   P0-3       │         │
│  │   P1-1       │  │   "已上传"   │  │              │         │
│  │ • 表格列拥挤 │  │ • 图表扁平   │  │              │         │
│  │   P2-2       │  │   P2-1       │  │              │         │
│  └──────────────┘  └──────────────┘  └──────────────┘         │
│         │                  │                  │               │
│         ▼                  ▼                  ▼               │
│  ┌────────────────────────────────────────────────────┐       │
│  │  Spec-047 修复 + 截图回归                          │       │
│  │  Spec-046 真实数据                                  │       │
│  └────────────────────────────────────────────────────┘       │
└────────────────────────────────────────────────────────────┘
```

### 3.2 修复优先级矩阵

| Bug | 严重度 | 修复成本 | 依赖 | 优先级 |
|:---:|:------:|:--------:|------|:------:|
| P0-1 Chat 面板重叠 | 🔴 | 低（CSS） | 无 | 1 |
| P0-2 审计加载失败 | 🔴 | 中 | 后端查错 | 2 |
| P0-3 通知路径错误 | 🔴 | 高 | 通知功能开发 | 3 |
| P1-1 fade 卡帧 | 🟠 | 低 | 无 | 1 |
| P1-2 API 状态枚举 | 🟠 | 中 | 后端 | 4 |
| P1-3 KB 搜索无效 | 🟠 | 低 | SPEC-046 | 5 |
| P1-4 Agent 下载无效 | 🟠 | 低 | SPEC-046 | 5 |
| P2-1 Dashboard 图表 | 🟡 | 低 | SPEC-046 | 6 |
| P2-2 表格列拥挤 | 🟡 | 低 | 无 | 7 |

## 4. API/接口设计

### 4.1 新增测试用端点（建议）

| Method | Path | Description | 用途 |
|--------|------|-------------|------|
| GET | `/api/v1/audit/debug` | 调试模式审计日志（含错误） | 暴露后端真实错误 |
| GET | `/api/v1/knowledge/docs/:id/status` | 单文档完整索引状态 | 便于测试断言 |

### 4.2 现有端点修改

| 端点 | 变更 |
|------|------|
| `GET /api/v1/audit` | 错误时返回结构化错误（含 request_id） |
| `GET /api/v1/notifications` | 确认返回正确（404 时显式说明） |

## 5. 详细设计

### 5.1 P0-1 修复（Chat Session 面板）

#### 5.1.1 修改文件

`frontend/app/chat/page.tsx:397-440`

#### 5.1.2 修改内容

```diff
   {/* Session Panel */}
   {showSessions && (
-    <div className="border-t border-[var(--border-glass)]" data-testid="session-sidebar">
+    <div
+      className="w-72 flex-shrink-0 border-l border-[var(--border-glass)] h-full overflow-y-auto bg-[var(--bg-secondary)]/30"
+      data-testid="session-sidebar"
+    >
       <div className="p-3">
```

#### 5.1.3 验证

- UI-019 (chat.spec.ts) 强化：打开 session 面板后断言：
  1. `session-sidebar` 元素 `boundingBox().width >= 200px`
  2. `session-sidebar` 与 `chat-messages` 元素 `boundingBox()` 不重叠
  3. 输入框 `chat-input` 的 `boundingBox().y` 在 session-sidebar 之外

### 5.2 P0-2 修复（审计加载失败）

#### 5.2.1 调查步骤

1. 查后端日志：`docker logs data-agent 2>&1 | grep -i audit`
2. 查 `/api/v1/audit` 返回：`curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/audit`
3. 区分：401 未鉴权 / 500 服务错误 / 200 业务异常

#### 5.2.2 前端修复（先做错误暴露）

`frontend/app/admin/audit/page.tsx`:
```diff
- catch { setError('加载失败'); }
+ catch (err: any) {
+   setError({
+     message: err.message || '未知错误',
+     status: err.status,
+     endpoint: '/api/v1/audit',
+   });
+ }
```

#### 5.2.3 后端修复（视调查结论）

- 如果 401：检查 RBAC 中 audit 权限配置
- 如果 500：修复 service 层错误
- 如果 MongoDB 查询异常：增加 try/catch + 详细日志

### 5.3 P0-3 修复（通知路径）

#### 5.3.1 调查步骤

1. 查通知页面实际路由：`find frontend/app -name "page.tsx" | xargs grep -l "notif"`
2. 查 Sidebar 是否显示通知入口（应有"🔔 通知"导航）
3. 复现：用 admin 账号访问 `/notifications` 路径，看是否 404

#### 5.3.2 修复方案

**方案 A（推荐）**: 通知功能未实现 → 实施 SPEC-031 的子集（铃铛 + 列表 + 标记已读）
**方案 B**: 通知功能已存在但路径错误 → 修复 Sidebar 链接和路由
**方案 C**: 截图实际是 audit 测试截图被命名错 → 修正截图命名 + 加强测试断言

实际修复方向需先完成调查步骤决定。本 spec 暂定**方案 A**（若需独立完整功能开发，应另开 SPEC-048）。

### 5.4 P1-1 修复（fade 动画）

#### 5.4.1 修改文件

`frontend/app/globals.css:90-92`

#### 5.4.2 修改内容

```diff
 .animate-fade-in {
-  animation: fadeIn 0.5s ease-out;
+  animation: fadeIn 0.5s ease-out forwards;
 }
```

#### 5.4.3 验证

- 在 E2E 测试中，等待动画完成（用 `await page.waitForFunction(() => !document.querySelector('.animate-fade-in') || getComputedStyle(...).opacity === '1')`）
- 或在 `playwright.config.ts` 添加 `page.addInitScript` 注入动画加速 CSS

### 5.5 P1-2 修复（API 审核状态）

#### 5.5.1 后端状态枚举

查 `internal/domain/api_review/` 或类似定义：
- 应统一为：`pending` / `reviewing` / `approved` / `rejected`
- 每个状态对应不同的按钮显示

#### 5.5.2 前端修复

`frontend/app/admin/api-review/page.tsx`:
```diff
- {status === '待审核' && (
+ {status === 'pending' && (
   <>
     <button data-testid="api-approve-btn">批准</button>
     <button data-testid="api-reject-btn">驳回</button>
   </>
 )}
```

### 5.6 P1-3 / P1-4 修复（KB 搜索 / Agent 下载）

由 SPEC-046 强化测试解决，本 spec 仅记录问题归属。

### 5.7 P2-1 修复（Dashboard 图表）

见 §2.2 P2-1。

#### 5.7.1 修改文件

`frontend/app/page.tsx:24-40`（SimpleBar 组件）

#### 5.7.2 新增数据占位

```jsx
function SimpleBar({ data, emptyText = '暂无数据' }: ...) {
  const hasData = data.some(d => d.value > 0);
  if (!hasData) {
    return <EmptyChart text={emptyText} />;
  }
  ...
}
```

### 5.8 P2-2 修复（用户表格列）

见 §2.2 P2-2。

#### 5.8.1 修改文件

`frontend/app/admin/users/page.tsx`（操作列）

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | 部分（audit 错误响应结构调整） |
| 性能影响 | 无（仅 CSS / 前端逻辑调整） |
| 是否需要新增 Skill | No |
| 是否需要新增后端端点 | 可选（debug 端点） |
| 是否影响现有测试 | No（强化断言不破坏现有用例） |
| 修复工作量 | 9 个 bug，~3-4 天 |
| 涉及文件 | 7 个 frontend + 2 个 backend |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/app/chat/page.tsx` | Chat Session 面板布局 | Edit（~10 行） |
| `frontend/app/admin/audit/page.tsx` | 错误显示 | Edit（~15 行） |
| `frontend/app/admin/api-review/page.tsx` | 状态按钮显示 | Edit（~10 行） |
| `frontend/app/admin/users/page.tsx` | 操作列优化 | Edit（~30 行） |
| `frontend/app/page.tsx` | Dashboard 图表空态 | Edit（~20 行） |
| `frontend/app/globals.css` | fade 动画 | Edit（1 行） |
| `frontend/app/components/Sidebar.tsx` | 通知入口（可能新增） | Edit（视调查结果） |
| `tests/ui/chat.spec.ts` | UI-019 强化 | Edit（~30 行） |
| `tests/ui/audit.spec.ts` | UI-125 强化 | Edit（~30 行） |
| `tests/ui/dashboard.spec.ts` | UI-064 强化（非空验证） | Edit（~20 行） |
| `tests/ui/api-review.spec.ts` | UI-137/138 强化 | Edit（~40 行） |
| `tests/ui/users.spec.ts` | UI-080 强化（操作列可点） | Edit（~20 行） |
| `docs/manual-screenshots/01-27-*.png` | 重新生成 | Refresh（CI 跑一遍） |
| `docs/DataAgent-UI测试用例文档.md` | 同步修复点 | Edit |
| `.agent/specs/INDEX.md` | 新增 SPEC-047 | Edit |

## 8. 测试策略

### 8.1 截图回归

- 修复每个 bug 后，**必须重新跑全量 E2E 测试**
- 手动对比新旧截图（diff 工具：`compare` 或 `git diff --no-index`）
- 截图保存到 `docs/manual-screenshots/manual-screenshots-{PR#}/` 子目录便于对比

### 8.2 视觉断言

新增 `tests/ui/visual-regression.spec.ts`（可选）：
```typescript
test('[UI-V01] Chat Session 面板不重叠', async ({ page }) => {
  await loginAsUser(page);
  await page.goto('/chat');
  await page.locator('[data-testid="chat-session-btn"]').click();
  await page.waitForSelector('[data-testid="session-sidebar"]');
  
  const sidebar = page.locator('[data-testid="session-sidebar"]');
  const chatArea = page.locator('[data-testid="chat-messages"]');
  
  const sb = await sidebar.boundingBox();
  const ca = await chatArea.boundingBox();
  
  expect(sb).not.toBeNull();
  expect(ca).not.toBeNull();
  // 不重叠
  expect(sb!.x).toBeGreaterThanOrEqual(ca!.x + ca!.width);
});
```

### 8.3 布局断言覆盖

| Bug | 断言 testid | 断言内容 |
|-----|------------|----------|
| P0-1 | `session-sidebar` | width ≥ 200px，与 chat-messages 不重叠 |
| P0-2 | `audit-error-banner` | 错误信息含具体 status / message / endpoint |
| P0-3 | 通知路由 | admin 访问 `/notifications` 不重定向到 audit |
| P1-1 | page-title | `getComputedStyle().opacity === '1'` |
| P1-2 | `api-status-*` | 状态值符合枚举，pending 时显示 approve/reject 按钮 |
| P2-1 | `chart-call-trend` | 数据全 0 时显示「暂无数据」而非扁平柱 |
| P2-2 | user-row-actions | 操作按钮 width ≥ 24px，可点击 |

## 9. UI Test / E2E 验收规则

- [x] **必须** 新增前端交互功能时同步编写对应 E2E 用例
- [x] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [x] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [x] **必须** 修复 UI bug 后新增视觉回归测试（避免再次回归）
- [x] **必须** 修复后截图与原 bug 截图**人工对比确认**
- [x] **严禁** 只改 CSS 样式不增加 data-testid（无法测试）
- [x] **严禁** 用 `page.evaluate` 强制设置 opacity 绕过测试（掩盖问题）

## 9.5. Go Unit Test 验收规则

不适用（不涉及 Go 代码变更，除非 P0-2 后端 audit 错误响应格式调整需要更新 service UT）。

## 10. 验证标准

### 10.1 修复完成度

| 指标 | 当前 | 目标 |
|------|------|------|
| 截图回归问题数 | 9 | 0 |
| 视觉断言用例数 | 0 | ≥ 7（覆盖 P0/P1） |
| fade 动画截图卡帧率 | ~30% | 0% |
| Chat Session 重叠问题 | 100% | 0% |
| 审计页错误显示 | "加载失败" 笼统 | 含 status + endpoint |
| Dashboard 空数据占位 | 扁平柱 | "暂无数据" |

### 10.2 截图对比

修复后必须重新生成全部 27 张截图，与原图逐张对比。差异容忍度：

| 区域 | 容忍度 |
|------|--------|
| 登录页 | 0 像素差 |
| 列表页/详情页 | 仅允许文字内容变化（如时间戳） |
| Dashboard | 数据值变化但布局不变 |
| 通知页 | 完全不同（旧 = audit 降级，新 = 真实通知） |

### 10.3 自动化对比脚本（可选）

```bash
# scripts/visual-diff.sh
# 对比新旧截图，超过阈值则报警
for old in docs/manual-screenshots/manual-screenshots-{PR#}/*.png; do
  new="docs/manual-screenshots/manual-screenshots-{main}/$(basename $old)"
  diff=$(compare -metric AE -fuzz 5% "$old" "$new" /dev/null 2>&1)
  if [ "$diff" -gt 1000 ]; then
    echo "VISUAL DIFF: $old vs $new = $diff pixels"
  fi
done
```

## 11. 实施步骤建议

1. **第 1 天**：P0-1 Chat Session 面板（最低成本、最高可见）
2. **第 2 天**：P1-1 fade 动画 + P2-1 Dashboard 图表占位
3. **第 3 天**：P2-2 用户表格列优化
4. **第 4 天**：P0-2 审计页 + P1-2 API 状态枚举
5. **第 5 天**：P0-3 通知路径调查 + 决策（可能另开 SPEC）
6. **第 6 天**：新增视觉回归测试 + CI 集成
7. **第 7 天**：截图全量重生成 + 文档同步 + PR 提交

每步均需 CI 通过才能进入下一步。**严禁绕过 CI 强制合并**（沿用项目红线规则）。

## 12. 与 SPEC-046 的关系

| 维度 | SPEC-046 | SPEC-047 |
|------|----------|----------|
| 焦点 | 真实数据/行为 | 视觉/布局 |
| 范围 | 后端 + 前端 + mockllm | 主要前端 CSS + 布局 |
| 触发条件 | 截图显示 0 数据 | 截图显示重叠/错乱 |
| 修复点 | KB 索引链路、工具调用链、Dashboard 数据 | CSS、flex 布局、动画、错误显示 |
| 依赖 | 无 | 依赖 SPEC-046 提供真实数据后再次验证图表 |
| 优先级 | P0（与 047 并行） | P0（与 046 并行） |

**两个 spec 必须同步完成**，因为：
- SPEC-046 完成后 Dashboard 才有真实数据 → 才能用 SPEC-047 的 P2-1 验证「非空数据时图表高度合理」
- SPEC-047 完成后 SPEC-046 的截图回归测试才不会被「布局错乱」假性失败
