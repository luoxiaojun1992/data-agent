# 前端列表页 UI 规范统一（分页组件 / 顶部主按钮 / 弹窗玻璃样式）

> **SPEC-078** | Status: 设计已定稿（待实现） | Phase: P15

## 1. 目标

统一 data-agent 前端所有列表页的 UI 规范，消除当前「分页组件多套实现、顶部主按钮多套配色、弹窗遮罩/面板多套样式」的割裂现状。具体三件事：

1. **API 管理列表页补分页**（当前只有裸数字页码、缺上一页/下一页与每页条数切换）。
2. **所有列表页分页统一样式**，收敛到唯一公共分页组件（以 `app/components/Pagination.tsx` 为准）。
3. **所有列表页顶部「新增/创建/上传」主按钮统一**（以用户管理「添加用户」按钮样式为准）。
4. **所有弹窗统一为玻璃透明样式**（遮罩 + 面板）。

**核心红线（晓军确认）**：
- ⛔ **禁止破坏 UI 布局**，禁止任何改动导致布局错乱。
- ⛔ 列表 item 上的按钮、其他功能性按钮**一律不碰**（第 5 条）。
- 本次为**纯前端样式/结构收敛**，不改变任何接口契约、不改后端、不改交互逻辑。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-035 UI 列表管理通用规范 | ✅ | 分页控件 data-testid / 交互约定（`{page}-pagination` 等） |
| SPEC-064 RBAC 角色权限管理系统 | ✅ | 各 admin 列表页已就绪 |
| SPEC-076 前端主题切换 + Light 主题 | ⛔ 不依赖 | 本 spec 仅针对当前主题，不考虑多主题/浅色主题适配（晓军确认） |
| SPEC-075 前端列表搜索/分页后端化 | ⚠️ 设计中 | 本 spec 只做**样式统一**，不改数据获取方式；分页数据源语义与 075 对齐（后端 page/page_size） |
| — | — | 无阻塞项 |

## 2. 背景（现状不足）

### 2.1 分页实现四套并存（严重割裂）

| 页面 | 实现方式 | 问题 |
|------|---------|------|
| `app/memory/page.tsx` | ✅ 公共组件 `components/Pagination.tsx`（`current/totalPages/onChange`） | 唯一用公共组件（Tailwind + 智能省略号） |
| `app/admin/rbac/page.tsx` | 页面内自定义 `Pagination`（`page/total/pageSize/onPage`） | 全量页码、无省略号，且与公共组件签名不一致 |
| `app/admin/users/page.tsx` | 手写 inline（上一页/下一页 + 1~5 页码 + page-size select） | 最完整但未复用组件 |
| `app/admin/models/page.tsx` | 手写 inline（上一页/下一页 ×2 处） | 无页码、无 page-size |
| `app/admin/skills/page.tsx` | 手写 inline（上一页/下一页） | 同上 |
| `app/admin/settings/page.tsx` | 手写 inline（上一页/下一页 + `x/y 页`） | 同上 |
| `app/admin/audit/page.tsx` | 手写 inline（上一页/下一页 + `第 x/y 页`） | 同上 |
| `app/admin/invites/page.tsx` | 手写 inline（上一页/下一页 ×2 处） | 同上 |
| `app/admin/users/[id]/rbac-roles/page.tsx` | 手写 inline（`‹ ›` + 页码数组） | 同上 |
| `app/admin/api-collections/page.tsx` | ⚠️ **裸数字页码**（无上一页/下一页、无 page-size） | **晓军指出：视为缺分页** |
| `app/knowledge/page.tsx` | 手写 inline（上一页/下一页 + `x/y 页`） | 同上 |
| `app/artifacts/page.tsx` | 手写 inline（`←上一页 / 下一页→`） | 同上 |
| `app/agent/page.tsx` | 手写 inline（`共 N 个 · 第 x/y 页`） | 同上 |
| `app/agent/tasks/[taskId]/page.tsx` | 手写 inline（同上风格） | 同上 |
| `app/chat/page.tsx` | 手写 inline（Tailwind class） | 同上 |

### 2.2 顶部主按钮三套配色

标准（`app/admin/users/page.tsx:258-273`）：
```tsx
background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
color: '#fff', border: 'none', borderRadius: '8px',
padding: '8px 20px', fontSize: '14px', fontWeight: 600,
```

| 页面 | 顶部主按钮样式 | 是否标准 |
|------|--------------|:---:|
| `admin/users`「添加用户」 | 渐变 `#5c7cfa→#7c3aed` 白字 `8px 20px` | ✅ 标准 |
| `admin/audit`「导出」 | 渐变 `8px 20px` | ✅ 一致 |
| `admin/rbac`「新建角色/权限」 | **纯色** `#5c7cfa` `8px 16px`（`btnPri`） | ⚠️ 非渐变 |
| `admin/rbac/roles/[id]/permissions`「添加权限」 | **纯色** `#5c7cfa` `8px 16px` | ⚠️ 非渐变 |
| `admin/users/[id]/rbac-roles`「添加角色」 | **纯色** `#5c7cfa` `8px 16px` | ⚠️ 非渐变 |
| `admin/models`「新增模型」 | `bg-[var(--accent)]`（纯 CSS 变量色） | ⚠️ 非渐变 |
| `admin/api-collections`「上传 OpenAPI」 | Tailwind `bg-[#B1E2FF] text-black`（亮蓝） | ⚠️ 非标准 |
| `admin/invites`「生成邀请」 | 渐变 `#B1E2FF→#9381FF` **黑字**（亮蓝系） | ⚠️ 非标准 |
| `knowledge`「上传文档」 | 渐变 `10px 20px`（padding 略不同） | ⚠️ 近似 |
| `admin/skills` | 无顶部新增按钮（仅「配置/保存」） | — 不涉及 |

### 2.3 弹窗遮罩/面板三套样式

| 页面 | 遮罩层 | 面板 | 玻璃透明? |
|------|--------|------|:---:|
| `admin/models` | `bg-black/60 backdrop-blur-sm` | `bg-[var(--bg-secondary)]` + `border-[var(--border-glass)]` + `rounded-2xl` | ✅ 标准玻璃 |
| `chat` / `agent` / `im/feishu` | `bg-black/50 backdrop-blur-sm` | 变量玻璃面板 | ✅ 接近 |
| `admin/users` | `rgba(0,0,0,0.6)` + `backdropFilter blur(4px)` | 硬编码 `#1a1a2e`（非变量、非玻璃） | ⚠️ 遮罩有 blur，面板非玻璃 |
| `admin/skills` | `rgba(0,0,0,0.6)`（无 blur） | boxShadow 面板 | ⚠️ 无 blur |
| `admin/rbac`（含 2 个 roles 子页） | `rgba(0,0,0,0.5)`（无 blur） | 内联面板 | ⚠️ 无 blur |
| `admin/audit` | `rgba(0,0,0,0.5)`（无 blur） | 内联面板 | ⚠️ 无 blur |
| `admin/api-collections` | `bg-black/50`（无 blur） | `bg-[var(--bg-primary)]` | ⚠️ 无 blur |
| `knowledge` | `rgba(0,0,0,0.5)`（无 blur） | 内联面板 | ⚠️ 无 blur |

## 3. 架构概述

纯前端收敛，不引入新依赖、不改后端。核心是「一份组件 + 一份样式常量 + 全部替换」：

```
┌─ 统一公共组件 ─────────────────────────────────────┐
│  app/components/Pagination.tsx   ← 唯一分页组件     │
│  app/components/ui.ts (新增)      ← 顶部主按钮/弹窗  │
│                                    玻璃样式常量       │
└──────────────────────┬─────────────────────────────┘
                       │ 各列表页 import 复用
┌─ 列表页 ────────────▼─────────────────────────────┐
│  users / rbac / models / skills / settings / audit │
│  invites / api-collections / knowledge / artifacts │
│  agent / agent-tasks / chat / memory / rbac 子页   │
└────────────────────────────────────────────────────┘
```

> 说明：本次只做样式统一，**不改变各页数据获取方式**（`page/page_size` 或 `skip/limit` 均保持原样），因此与 SPEC-075（分页后端化）互不冲突——075 管「数据从哪来、怎么过滤」，078 管「分页控件长什么样、怎么统一」。

## 4. 详细设计

### 4.1 唯一分页组件（`app/components/Pagination.tsx`）

**决策（晓军确认）：以现有 `app/components/Pagination.tsx` 为唯一标准分页组件**（不是某个 skill 定义的组件规范），并做兼容性扩展。

现状签名：
```tsx
Pagination({ current, totalPages, onChange, className })
```

改造后签名（向后兼容 + 对齐后端分页语义）：
```tsx
interface PaginationProps {
  // 语义 A（既有，保留）: 当前页 + 总页数
  current?: number;
  totalPages?: number;
  // 语义 B（新增，推荐）: 页码 + 总数 + 每页条数
  page?: number;
  total?: number;
  pageSize?: number;
  // 回调
  onChange: (page: number) => void;
  // 每页条数切换（晓军已确认：统一内嵌进分页组件）
  onPageSizeChange?: (size: number) => void;
  pageSizeOptions?: number[]; // 默认 [10, 20, 50, 100]
  className?: string;
}
```

- 优先识别语义 B（`total`/`pageSize` 存在时 `totalPages = Math.ceil(total/pageSize)`），否则用语义 A。
- 视觉标准（现有实现已满足，保留）：
  - `上一页 [1] ... [4] [5] [6] ... [20] 下一页`（智能省略号）。
  - active 页高亮 `bg-[#B1E2FF] text-black`（保留 Tailwind 现状，仅当前主题，不做多主题 token 化）。
  - `totalPages <= 1` 时返回 `null`（不渲染）。
- **每页条数切换下拉（晓军已确认：统一内嵌）**：分页组件统一附带「每页条数 10/20/50/100」下拉（SPEC-035 UI-167 要求 `{page}-page-size-select`）。当前仅 users 页有该下拉，其余页无——本次统一内嵌进 `Pagination.tsx`（`onPageSizeChange` + `pageSizeOptions`），保留各页 `data-testid` 前缀兼容（见 4.5）。切换条数后由各页自行 reset 到第 1 页并重拉数据。

### 4.2 顶部主按钮统一（新增 `app/components/ui.ts`）

统一导出 `primaryButtonStyle`（以 users「添加用户」为准）：
```tsx
export const primaryButtonStyle: React.CSSProperties = {
  background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
  color: '#fff', border: 'none', borderRadius: '8px',
  padding: '8px 20px', fontSize: '14px', fontWeight: 600,
  cursor: 'pointer',
};
```

替换清单：
- `admin/rbac`：`btnPri` 纯色 → `primaryButtonStyle`
- `admin/rbac/roles/[id]/permissions`：`btnPri` → `primaryButtonStyle`
- `admin/users/[id]/rbac-roles`：`btnPri` → `primaryButtonStyle`
- `admin/models`：`bg-[var(--accent)]` → 渐变（改用 `primaryButtonStyle` 或等价 Tailwind）
- `admin/api-collections`：`bg-[#B1E2FF] text-black` → `primaryButtonStyle`
- `admin/invites`：`#B1E2FF→#9381FF` 黑字 → `primaryButtonStyle`
- `knowledge`：`10px 20px` → `8px 20px`（`primaryButtonStyle`）

> 保留各按钮原有 `data-testid` 与 `onClick`/文案，只换样式。

### 4.3 弹窗玻璃透明统一（新增 `app/components/ui.ts`）

统一导出两常量：
```tsx
// 遮罩层
export const modalOverlayStyle: React.CSSProperties = {
  position: 'fixed', inset: 0, zIndex: 1000,
  background: 'rgba(0,0,0,0.6)',
  backdropFilter: 'blur(4px)',
  WebkitBackdropFilter: 'blur(4px)',
  display: 'flex', alignItems: 'center', justifyContent: 'center',
};
// 面板（玻璃，引用当前主题既有 CSS 变量，避免硬编码色值）
export const modalPanelStyle: React.CSSProperties = {
  background: 'var(--bg-secondary)',
  border: '1px solid var(--border-glass)',
  borderRadius: '16px',
  padding: '28px',
  maxWidth: '480px', width: '100%',
  boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
};
```

替换清单：
- `admin/users`：面板硬编码 `#1a1a2e` → `var(--bg-secondary)`（遮罩已达标）
- `admin/rbac` + 2 个 roles 子页 + `users/[id]/rbac-roles`：遮罩 `rgba(0,0,0,0.5)` 无 blur → `modalOverlayStyle`
- `admin/audit`：遮罩无 blur → `modalOverlayStyle`
- `admin/api-collections`：`bg-black/50` 无 blur → `modalOverlayStyle`；面板 `bg-[var(--bg-primary)]` → `var(--bg-secondary)`
- `knowledge`：遮罩无 blur → `modalOverlayStyle`
- `admin/skills`：遮罩补 blur
- `chat`/`agent`/`im/feishu`/`models`：已达标，仅核对统一（若已用 `backdrop-blur-sm` + 变量面板，可不改，避免破坏布局）

> 各弹窗尺寸差异（`max-width`、是否 `max-h + overflow-y-auto`）**保留各自原值**，仅统一「遮罩模糊 + 面板玻璃底色」两要素，禁止强行统一尺寸导致布局错乱。

### 4.4 主题范围（晓军已确认：仅当前主题）

- **本 spec 只针对当前主题落地，不考虑 SPEC-076 多主题 / 浅色主题**。
- 顶部主按钮渐变 `#5c7cfa→#7c3aed`、分页 active 高亮 `#B1E2FF` 等一律按当前主题现状固定，不引入新主题 token。
- 玻璃面板色/边框复用当前主题**既有** CSS 变量（`--bg-secondary` / `--border-glass`），避免硬编码十六进制色值（如 `#1a1a2e`）；但仅为复用现状变量，不承诺浅色主题表现。
- 多主题适配由 SPEC-076 后续单独处理，不在本 spec 范围内。

### 4.5 `data-testid` 兼容（对齐 SPEC-035）

SPEC-035 约定分页控件 testid 前缀为 `{page}-pagination*`、`{page}-page-size-select`。统一分页组件时**必须保留各页现有 `data-testid` 前缀**（如 `user-pagination-next`、`agent-page-prev`、`runs-page-next` 等），不得改名导致 UI E2E 用例失效。

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No（纯前端，数据获取方式不变） |
| 是否影响交互逻辑 | No（只换样式/复用组件，不改 handler） |
| 是否需要新增 Skill | No（分页组件/样式常量属前端组件，非 skill） |
| 性能影响 | 无（组件复用反而减少重复代码） |
| 风险 | 中——多处页面替换，需逐页截图回归防布局错乱 |
| 主题范围 | 仅当前主题；不涉及 SPEC-076 多主题适配 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `app/components/Pagination.tsx` | 扩展 props（语义 B 兼容） | Small |
| `app/components/ui.ts` | 新增：`primaryButtonStyle` / `modalOverlayStyle` / `modalPanelStyle` | New |
| `app/admin/api-collections/page.tsx` | 补标准分页 + 顶部按钮 + 弹窗玻璃 | Medium |
| `app/admin/users/page.tsx` | 分页复用组件 + 弹窗面板变量化 | Medium |
| `app/admin/rbac/page.tsx` | 分页复用组件 + 顶部按钮 + 弹窗玻璃 | Medium |
| `app/admin/rbac/roles/[id]/permissions/page.tsx` | 顶部按钮 + 弹窗玻璃 | Small |
| `app/admin/users/[id]/rbac-roles/page.tsx` | 分页 + 顶部按钮 + 弹窗玻璃 | Small |
| `app/admin/models/page.tsx` | 分页复用 + 顶部按钮 | Small |
| `app/admin/skills/page.tsx` | 分页复用 + 弹窗玻璃 | Small |
| `app/admin/settings/page.tsx` | 分页复用 | Small |
| `app/admin/audit/page.tsx` | 分页复用 + 弹窗玻璃 | Small |
| `app/admin/invites/page.tsx` | 分页复用 + 顶部按钮 | Small |
| `app/knowledge/page.tsx` | 分页复用 + 顶部按钮 + 弹窗玻璃 | Medium |
| `app/artifacts/page.tsx` | 分页复用 | Small |
| `app/agent/page.tsx` | 分页复用 | Small |
| `app/agent/tasks/[taskId]/page.tsx` | 分页复用 | Small |
| `app/chat/page.tsx` | 分页复用（弹窗已达标，核对） | Small |
| `app/memory/page.tsx` | 已用公共组件，核对新签名兼容 | Small |

## 7. 测试策略

1. **E2E 测试**：分页/弹窗/按钮属纯 UI，必须真实 E2E 回归。重点：
   - `tests/ui/api-collections.spec.ts`（或现有 API 用例）补「分页控件存在 + 翻页可用」断言。
   - 现有 users/rbac/models 分页用例（SPEC-023/024/025 相关 UI-xxx）回归通过，证明 data-testid 兼容未破坏。
2. **布局回归**：逐页截图对比（当前主题），确认无布局错乱（红线）。
3. **审计**：`.agent/skills/ui-test-audit` 审查用例质量。

## 8. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 修改 UI 组件时同步更新/保留 `data-testid` 属性（分页前缀 `{page}-pagination*` 不得变）
- [ ] **必须** API 管理页新增「分页控件」E2E 断言（存在 + 翻页）
- [ ] **必须** 所有被改页面的既有 E2E 用例回归通过
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9. 验证标准

1. **分页**：所有列表页（含 API 管理）分页控件视觉一致，均出自 `Pagination.tsx`；API 管理页具备「上一页 / 数字页码 / 下一页」。
2. **顶部主按钮**：所有列表页顶部「新增/创建/上传」按钮统一为渐变 `#5c7cfa→#7c3aed` 白字样式（与「添加用户」一致）。
3. **弹窗**：所有弹窗遮罩带 `backdrop blur`，面板底色统一为 `var(--bg-secondary)` + `var(--border-glass)`，无硬编码色。
4. **⛔ 布局红线**：当前主题逐页截图，无任何页面布局错乱、无元素溢出/重叠。
5. **⛔ 范围红线**：列表 item 上的按钮、其他功能性按钮（如分页内跳转、排序、checkbox）的**样式与文案未发生任何改动**。
6. **data-testid**：既有分页/弹窗 testid 前缀全部保留，UI E2E 全绿。

## 10. 已决策（晓军确认，2026-09-01）

1. **分页组件以「唯一公共组件 `app/components/Pagination.tsx`」为准**——不是某个 skill 定义的组件规范。
2. **每页条数切换下拉统一内嵌进分页组件**（默认 10/20/50/100），随组件一起复用，不再各页手写。
3. **本 spec 只考虑当前主题**，不考虑 SPEC-076 多主题 / 浅色主题；多主题适配由 SPEC-076 后续单独处理。
