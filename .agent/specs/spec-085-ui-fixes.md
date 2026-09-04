# 前端 UI 缺陷修复（分页/弹窗/子角色显示/视觉一致性）

> **SPEC-085** | Status: 📐 设计已定稿 | Phase: P15

## 1. 目标

修复测试服务器 `120.26.179.218` 上 8 个页面截图暴露的前端 UI 缺陷，按晓军指示「没有额外说明的就是缺少分页，有说明的以说明为准」分类处理：

1. **缺分页**：8 个 URL 接入/补齐分页控制
2. **RBAC 子角色显示**：系统管理员为父时子角色列表为空
3. **弹窗位置错乱**：admin/users 添加用户弹窗顶到顶部 / 掉到下面
4. **弹窗输入框边框不明显**：rbac/rbac-roles 弹窗输入框在深色背景下看不清
5. **弹窗样式不统一**：透明度、边框、padding 不一致，统一以「新建分析任务」弹窗（`agent/page.tsx`）的玻璃样式为基准
6. **重复项**：admin/invites 出现两次（同一页面）

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-035 UI 列表管理通用规范 | ✅ | 分页 testid 前缀约定 |
| SPEC-041 弹窗视觉统一红线 | ✅ | 红线 #41 已在 CONVENTIONS.md（弹窗必须用 `modalOverlayStyle`/`modalPanelStyle`） |
| SPEC-042 视觉/样式回归运行时验证红线 | ✅ | 红线 #42：必须 `getComputedStyle` 验证 |
| SPEC-049 多模型配置 | ✅ | 依赖发 |
| SPEC-064 RBAC 角色权限管理系统 | ✅ | rbac 角色和子角色数据来源 |
| SPEC-075 前端列表搜索/分页后端化 | ✅ | 分页数据后端化 |
| SPEC-078 前端列表页 UI 规范统一 | ✅ 已实现（部分） | 本 spec 补充 SPEC-078 未覆盖的页 |

## 2. 背景（现状）

### 2.1 缺分页（8 个 URL 调研结果）

| URL | 当前实现 | 是否真缺 | 备注 |
|-----|---------|:---:|------|
| `/agent` | ✅ `Pagination` 已接入（page.tsx:251） | 否 | 已接，可能是数据少时 `totalPages<=1` 不渲染 |
| `/agent/tasks/[taskId]` | ✅ `Pagination` 已接入（page.tsx:229） | 否 | 同上 |
| `/artifacts` | ✅ `Pagination` 已接入条件渲染（page.tsx:151） | 否 | 同上 |
| `/im/feishu` | ❌ **硬编码 `page=1&page_size=50`**（page.tsx:77） | **是** | 真正缺分页，需接入 |
| `/admin/models` | ✅ `Pagination` 已接入（page.tsx:520/632） | 否 | 已接 |
| `/admin/users/{id}/rbac-roles` | ✅ `Pagination` 已接入（page.tsx:75） | 否 | 已接 |
| `/admin/rbac` | ✅ `Pagination` 已接入（page.tsx:125/160） | 否 | 已接（roles/permissions 双 tab） |
| `/admin/invites` | ✅ `Pagination` 已接入（page.tsx:285） | 否 | 重复项，#16 = #9 同页面 |

**结论**：8 个 URL 中仅 `/im/feishu` 真正缺分页。其余 7 个 URL 已接入 `Pagination` 公共组件，**晓军看到"缺分页"很可能是因为数据量少时分页组件按现有逻辑（`totalPages<=1`）自动隐藏**（Pagination.tsx:38）。

### 2.2 RBAC 系统管理员子角色显示（图 1）

页面 `/admin/rbac`，标题「RBAC 管理 — 系统管理员 的子角色」，但**列表区空白**。代码（page.tsx:42）：

```tsx
const q = new URLSearchParams({ page: String(rolePage), page_size: String(PAGE_SIZE) });
if (parentFilter) q.set('parent_id', parentFilter);
```

API `GET /admin/rbac/roles?parent_id=system_admin` 查询 RBAC 角色表。

**根本原因**：RBAC 角色层级（L0/L1/L2）和业务主角色（system_admin/admin/user）是**两套独立体系**：
- 业务主角色是 user 表的 `User.Role` 字段
- RBAC 角色是 `rbac_roles` 集合，`parent_id` 指向**其他 RBAC 角色**的 ID，不是业务主角色名

当前页面通过 `setParentFilter(r.id)` 设的 parentFilter 是 RBAC 角色 ID（如 `role-abc-123`），不是 `system_admin`。当 URL 直接带 `?parent_id=system_admin` 时，API 查不到 → 返回空。

但截图标题明确显示「— 系统管理员 的子角色」，说明前端硬编码或误传 `system_admin`。需查 UI E2E 是否真的进入这个状态，或前端要"显示业务主角色对应的 RBAC 子角色"映射。

### 2.3 弹窗位置错乱（图 2 / 图 3）

URL `/admin/users`，弹窗「添加用户」：
- 图 2：弹窗顶到视口顶部
- 图 3：弹窗底部超出视口

代码（users/page.tsx:711 ModalOverlay）：

```tsx
<div style={{
  position: 'fixed',
  top: 0, left: 0, right: 0, bottom: 0,
  background: 'rgba(0,0,0,0.6)',
  backdropFilter: 'blur(4px)',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  zIndex: 1000,
}}>
```

代码看起来正确居中。但实际渲染出现错乱，可能原因：
1. **AppLayout (providers.tsx) 内的某个祖先元素含 `transform`/`filter`/`will-change`**，导致 fixed 子元素以该祖先为定位参考（CSS spec 行为）。
2. **modal 高度超过视口高度**：modalStyle `padding: 28px` + 4 个 input + 按钮区可能超过 800px 视口高度。`max-height + overflow-y-auto` 缺失。
3. **flex 内容溢出后 align-items: center 失效**：内容超出 flex 容器高度后，center 行为可能向上对齐到 viewport top，导致"顶到顶部"。

需运行时 `getComputedStyle` 验证（红线 #42）。

### 2.4 弹窗输入框边框不明显（图 4/5/6）

| 页面 | inputStyle 边框 |
 |  |  |
| admin/users 添加用户 | `rgba(255,255,255,0.1)`（line 680） |
| admin/rbac 新建角色/权限 | `var(--border)`（line 183） |
| admin/users/{id}/rbac-roles 添加角色 | `var(--border)`（line 110） |
| admin/models 编辑/新增 | `rgba(255,255,255,0.1)`（line 814） |

`var(--border)` 在深色背景下值较暗（透明度低），看不清边框。`rgba(255,255,255,0.1)` 透明度更亮。

**现状不一致**（红线 #41 已定义弹窗必须统一）。

### 2.5 弹窗样式不统一（玻璃 vs 不玻璃）

晓军描述「有的透明度高像玻璃，有的透明度低偏蓝色。应该以玻璃弹窗为准」。晓军进一步明确：**玻璃基准 = 「新建分析任务」弹窗（`agent/page.tsx`）**。

「新建分析任务」弹窗的关键实现（agent/page.tsx:258-260）：

```tsx
<div className="fixed inset-0 z-50 flex items-center justify-center">
  <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={...} />
  <div className="relative glass p-6 rounded-2xl max-w-lg w-full mx-4">
```

| 页面 | 遮罩 | 面板背景 | 边框 | 圆角 | 一致性 |
|------|------|------|------|------|:---:|
| **agent（新建分析任务）** | `bg-black/50`(=rgba(0,0,0,0.5)) + `backdrop-blur-sm`(=4px) | **`.glass`** = `var(--glass-bg)`(rgba(255,255,255,0.04)) + `backdrop-filter: blur(20px)` | `var(--border-glass)` | `16px` | 🎯 **基准** |
| admin/users | `rgba(0,0,0,0.6)` + `blur(4px)` | `var(--bg-secondary)`(#111133 不透明) | `var(--border-glass)` | `16px` | ⚠️ 面板不透明（偏蓝），遮罩 0.6 偏暗 |
| admin/rbac | `modalOverlayStyle`（ui.ts） | `var(--bg-secondary)` + `var(--border-glass)` | 同 | `12px` ⚠️ | ⚠️ 面板不透明 + 圆角 12px |
| admin/users/{id}/rbac-roles | `modalOverlayStyle` | `var(--bg-secondary)` + `var(--border-glass)` | 同 | `12px` ⚠️ | ⚠️ 面板不透明 + 圆角 12px |
| im/feishu | `bg-black/60 backdrop-blur-sm`（Tailwind） | `var(--bg-secondary)` | `var(--border-glass)` | `16px` | ⚠️ 面板不透明 |
| knowledge | `rgba(0,0,0,0.5)` 无 blur | 内联面板 | 无 | 无 | ❌ 不玻璃 |

**核心问题**：
- **面板背景不透明是最大问题**：`modalPanelStyle` 用 `var(--bg-secondary)`（#111133 实心蓝黑），而基准 `.glass` 是 `var(--glass-bg)`（半透明白 0.04）+ `backdrop-filter: blur(20px)`。前者造成「透明度低偏蓝色」。
- admin/users 的 ModalOverlay 是**手写 inline**，没用 ui.ts 常量
- admin/rbac 子页弹窗用了 ui.ts 但圆角 12px（不一致）
- 输入框边框 `var(--border)` 太暗
- 同一项目两套弹窗实现并存

### 2.6 重复项

晓军列了 16 条，#9 和 #16 都是 `/admin/invites`（同一页面）。视为同一 bug。

## 3. 架构概述

纯前端样式/结构收敛，不改后端，不改接口契约。涉及：

```
┌─ 共享组件 / 常量 ──────────────────────────────┐
│  components/ui.ts          ← 已有，常量补充      │
│    + modalInputStyle                             │
│    + modalPanelBorderStyle                       │
│    + modalPanelPaddingStyle                      │
│  components/Pagination.tsx ← 已有，按需调整      │
└──────────────────────┬─────────────────────────┘
                       │ 各页面 import 复用
┌─ 页面（按修复范围）─────────────────────────────┐
│  admin/users                                 中 │
│    - ModalOverlay 改用 modalOverlayStyle 常量  │
│    - Modal 改用 modalPanelStyle + max-height   │
│  admin/rbac + 子页 + rbac-roles               中 │
│    - inputStyle 统一                           │
│    - modalPanel 圆角 16px                     │
│  im/feishu                                    小 │
│    - 接入 Pagination 公共组件                  │
│  agent / agent-tasks / artifacts / models     小 │
│    - 已是 Pagination，校验运行时显示条件        │
│  admin/invites                                小 │
│    - 已是 Pagination，校验运行时显示条件        │
└────────────────────────────────────────────────┘
```

## 4. 详细设计

### 4.1 `/im/feishu` 接入分页（真实缺分页）

修改 `frontend/app/im/feishu/page.tsx`：

```tsx
// 当前（line 74-83）
const loadConfigs = useCallback(async () => {
  setLoading(true);
  try {
    const res = await apiFetch('/im/feishu/configs?page=1&page_size=50');
    // ...
}, [apiFetch]);

// 修改为
const [page, setPage] = useState(1);
const PAGE_SIZE = 10;
const [total, setTotal] = useState(0);

const loadConfigs = useCallback(async () => {
  setLoading(true);
  try {
    const res = await apiFetch(`/im/feishu/configs?page=${page}&page_size=${PAGE_SIZE}`);
    // ...
    setTotal(data.total || 0);
  } catch (e) { /* ... */ }
  setLoading(false);
}, [apiFetch, page]);

useEffect(() => {
  if (!auth.hydrated || !auth.token) return;
  loadConfigs();
}, [auth.hydrated, auth.token, loadConfigs]);

// 在列表下方添加
<Pagination
  page={page}
  total={total}
  pageSize={PAGE_SIZE}
  onChange={setPage}
  testIdPrefix="feishu-configs"
/>
```

### 4.2 已接入分页的 7 个 URL：调整显示条件

晓军"缺分页"语义可能是：数据少时分页组件不显示（Pagination `totalPages <= 1` 时返回 null）。两种处理：

**选项 A（推荐）**：保留现状，当数据量足够时分页自动显示
**选项 B**：每页条数默认 10，分页始终显示（包括 1 条数据时也显示，用于 UI 一致性）

晓军 2026-09-04 修正：分页**只要有数据就显示**，禁止「数据少时隐藏」（原 SPEC-078 §4.1 的 `totalPages <= 1` 返回 null 规则作废）。本 spec 按此修正 Pagination.tsx（commit 3e86531），校验现状：
- 列表为空时分页不渲染（合理）
- 列表有数据且 totalPages >= 2 时分页渲染

不修改 Pagination 默认行为，但**在 E2E 测试中加「当数据 ≥2 页时分页可见」断言**（防回归）。

### 4.3 弹窗统一规范（按红线 #41）

**新增 `components/ui.ts` 常量**（在已有 3 个基础上加）：

```ts
// 弹窗面板 input 样式（解决 #4 输入框不明显）
export const modalInputStyle: React.CSSProperties = {
  width: '100%',
  padding: '8px 12px',
  fontSize: '14px',
  background: 'rgba(255,255,255,0.06)',
  border: '1px solid rgba(255,255,255,0.15)',  // 比 0.1 更亮，可见边框
  borderRadius: '8px',
  color: 'var(--text-primary)',
  outline: 'none',
  boxSizing: 'border-box',
};

// 弹窗面板 label 样式
export const modalLabelStyle: React.CSSProperties = {
  display: 'block',
  fontSize: '13px',
  color: 'var(--text-secondary)',
  marginBottom: '4px',
};

// 弹窗面板 select 样式（继承 modalInputStyle）
export const modalSelectStyle: React.CSSProperties = { ...modalInputStyle };

// 弹窗取消按钮
export const modalCancelBtnStyle: React.CSSProperties = {
  padding: '8px 16px',
  background: 'transparent',
  border: '1px solid rgba(255,255,255,0.15)',
  borderRadius: '8px',
  color: 'var(--text-secondary)',
  fontSize: '14px',
  cursor: 'pointer',
};
```

**统一后的弹窗规范**（以「新建分析任务」弹窗为基准）：

| 元素 | 样式 |
|------|------|
| 遮罩 | `modalOverlayStyle`：`rgba(0,0,0,0.5)`（= `bg-black/50`，由 0.6 改）+ `blur(4px)` + flex center |
| 面板 | `modalPanelStyle`：**`.glass` 玻璃等效** —— `var(--glass-bg)`(rgba(255,255,255,0.04)) + `backdrop-filter: blur(20px)`（由 `var(--bg-secondary)` 实心色改）+ `var(--border-glass)` + `16px` 圆角 + `24px` padding（由 28px 改）+ `maxWidth: 512px`（由 480px 改） |
| 输入框 | `modalInputStyle`（新增）`rgba(255,255,255,0.15)` 边框 |
| Label | `modalLabelStyle`（新增） |
| 主按钮 | `primaryButtonStyle`（已有） |
| 取消按钮 | `modalCancelBtnStyle`（新增） |

**同步修改 `ui.ts` 现有常量**（关键——否则全项目弹窗仍是实心色）：

```ts
// 弹窗遮罩层：透明度对齐 bg-black/50
export const modalOverlayStyle: React.CSSProperties = {
  position: 'fixed',
  inset: 0,
  zIndex: 1000,
  background: 'rgba(0,0,0,0.5)',   // 0.6 → 0.5（对齐新建分析任务弹窗 bg-black/50）
  backdropFilter: 'blur(4px)',
  WebkitBackdropFilter: 'blur(4px)',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
};

// 弹窗面板：对齐 .glass 玻璃透明（var(--glass-bg) + blur(20px)）
export const modalPanelStyle: React.CSSProperties = {
  background: 'var(--glass-bg)',          // var(--bg-secondary) → 玻璃半透明
  backdropFilter: 'blur(20px)',           // 新增：.glass 的 backdrop-filter
  WebkitBackdropFilter: 'blur(20px)',     // 新增：Safari 兼容
  border: '1px solid var(--border-glass)',
  borderRadius: '16px',
  padding: '24px',                        // 28px → 24px（p-6）
  maxWidth: '512px',                      // 480px → 512px（max-w-lg）
  width: '100%',
  boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
};
```

**面板 max-height 强制约束**（防图 2/3 弹窗超出视口）：

```tsx
<ModalPanel style={{ ...modalPanelStyle, maxHeight: '85vh', overflowY: 'auto' }}>
```

替换清单：
- `admin/users`：ModalOverlay 改用 `modalOverlayStyle`，inputStyle 改 `modalInputStyle`，modalStyle 改 `modalPanelStyle + maxHeight: '85vh'`
- `admin/rbac`：inStyle 改 `modalInputStyle`，mContent 改 `modalPanelStyle`（border 圆角 12px → 16px）
- `admin/rbac/roles/[id]/permissions`：input 同 admin/rbac
- `admin/users/[id]/rbac-roles`：inp 改 `modalInputStyle`，mc 改 `modalPanelStyle`
- `im/feishu`：弹窗面板 inline 样式 → `modalPanelStyle`
- `knowledge`：弹窗改 `modalOverlayStyle` + `modalPanelStyle`（非玻璃的也要统一）

### 4.4 RBAC 系统管理员子角色显示（图 1）

**根因**：URL `/admin/rbac` 直接带 `parentFilter='system_admin'` 时，RBAC API `?parent_id=system_admin` 查不到数据（system_admin 是业务主角色，不是 RBAC 角色 ID）。

**两种语义**：
- **A（晓军原始意图）**：业务主角色 system_admin 对应的 RBAC 角色子角色 —— 需前端映射"主角色 → 默认 RBAC 角色"
- **B（当前实现）**：RBAC 角色层级下的子角色 —— parentFilter 应该是 RBAC 角色 ID

**决策**：走 A 路径。`parentFilter='system_admin'` 时：
- 前端查 `/admin/rbac/roles?q=system_admin&limit=10`，找到名为 system_admin 的 RBAC 角色 ID
- 用该 ID 替换 parentFilter，再查子角色

修改 `admin/rbac/page.tsx:42`：

```tsx
const fetchRoles = () => {
  if (parentFilter) {
    // 业务主角色名 → RBAC 角色 ID 映射
    resolveRoleID(parentFilter).then(id => {
      const q = new URLSearchParams({ page: String(rolePage), page_size: String(PAGE_SIZE) });
      q.set('parent_id', id);
      return apiFetch(`/admin/rbac/roles?${q}`);
    }).then(r => r.json()).then(/* ... */);
  } else {
    const q = new URLSearchParams({ page: String(rolePage), page_size: String(PAGE_SIZE) });
    return apiFetch(`/admin/rbac/roles?${q}`).then(r => r.json()).then(/* ... */);
  }
};

const resolveRoleID = async (parentID: string) => {
  // parentID 已是 UUID 直接返回，否则按 name 查询
  if (/^[0-9a-f]{8}-/.test(parentID)) return parentID;
  const res = await apiFetch(`/admin/rbac/roles?q=${encodeURIComponent(parentID)}&limit=1`);
  const data = await res.json();
  return data.roles?.[0]?.id || parentID;
};
```

同时后端补一个便捷端点（备选）：
- `GET /admin/rbac/roles/by-name?name=system_admin` → 返回 `{id, display_name, level, ...}`
- 或扩展现有 `GET /admin/rbac/roles?q=system_admin` 支持 name 搜索（q 已支持）

**优先前端映射方案**（不改后端）。如不行再加端点。

### 4.5 弹窗位置错乱修复

针对图 2/3 admin/users ModalOverlay 错位：

1. **改用 `modalOverlayStyle`**：保证全项目弹窗遮罩统一（但 ModalOverlay 已经是 inline 等价样式，所以这不是根因）
2. **加 `max-height: 85vh; overflow-y: auto` 到 modalPanel**：内容超出视口时弹窗可滚动，不会顶到顶部/掉到下面
3. **运行时验证（红线 #42）**：开发完成后用 `getComputedStyle(modalPanel).maxHeight + getBoundingClientRect()` 验证 modal 在 viewport 中央
4. **检查 AppLayout 内的祖先元素**：确认无 `transform`/`filter`/`will-change` 导致 fixed 失效

```tsx
// admin/users/page.tsx ModalPanel 修复
const modalStyleFixed: React.CSSProperties = {
  ...modalPanelStyle,
  maxHeight: '85vh',
  overflowY: 'auto',
};

// 使用
<ModalOverlay onClose={...}>
  <div style={modalStyleFixed} onClick={(e) => e.stopPropagation()}>
    {/* 内容 */}
  </div>
</ModalOverlay>
```

弹窗内容（form 等）需 `marginBottom` 配合 `overflow-y: auto`，确保滚动体验。

### 4.6 弹窗 input 边框统一

按 §4.3 新增 `modalInputStyle`（边框 `rgba(255,255,255,0.15)`）。替换所有弹窗内 inline inputStyle。

涉及文件（grep `inStyle:.*var(--border)` 和 `inputStyle:.*var(--border)`）：
- `admin/rbac/page.tsx`
- `admin/rbac/roles/[id]/permissions/page.tsx`
- `admin/users/[id]/rbac-roles/page.tsx`

涉及文件（用 `rgba(255,255,255,0.1)` 边框的 inline input）：
- `admin/users/page.tsx` (inputStyle)
- `admin/models/page.tsx` (inputStyle)
- `im/feishu/page.tsx` (line 223)
- `agent/page.tsx` (input 内联)

**统一目标**：所有弹窗内 input 用 `modalInputStyle`（边框 `rgba(255,255,255,0.15)`），可见性比当前更清晰。

### 4.7 范围红线

- ⛔ 不改后端
- ⛔ 不改业务逻辑（弹窗内 onClick handler / 表单提交逻辑保留）
- ⛔ 不破坏现有 `data-testid`（防 UI E2E 回归失败）
- ⛔ 不改 `.glass` 类**定义**（`.glass` 已正确，是弹窗面板玻璃基准；改的是各页面让弹窗面板**采用** `.glass` 等效样式，而非改 `.glass` 本身）
- ⛔ 弹窗面板不得再用 `var(--bg-secondary)` 实心色（必须 `.glass` 玻璃等效）
- ⛔ SPEC-076 多主题不涉及（沿用 SPEC-078「仅当前主题」原则）

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No（纯前端，admin/rbac/roles q 已支持 name 搜索） |
| 是否影响现有交互 | No（仅样式/位置统一） |
| 是否需要新增 Skill | No |
| 性能影响 | 无 |
| 风险 | 低-中：弹窗统一可能改变视觉，需逐页回归；RBAC 父角色映射需运行时验证 |
| 主题范围 | 仅当前主题（红线 #41 / SPEC-078） |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/app/components/ui.ts` | **改** `modalOverlayStyle`(0.6→0.5) + `modalPanelStyle`(实心→玻璃) + 新增 `modalInputStyle`/`modalLabelStyle`/`modalSelectStyle`/`modalCancelBtnStyle` | Small |
| `frontend/app/components/Pagination.tsx` | 不改 | — |
| `frontend/app/im/feishu/page.tsx` | 接入 Pagination + 弹窗统一 | Small |
| `frontend/app/admin/users/page.tsx` | ModalOverlay 改用常量 + modalPanelStyle + maxHeight + inputStyle 统一 | Medium |
| `frontend/app/admin/rbac/page.tsx` | inputStyle/mPanel 统一 + parentFilter 业务主角色映射 | Medium |
| `frontend/app/admin/users/[id]/rbac-roles/page.tsx` | inputStyle/mPanel 统一 | Small |
| `frontend/app/admin/rbac/roles/[id]/permissions/page.tsx` | inputStyle 统一 | Small |
| `frontend/app/admin/models/page.tsx` | 弹窗内 input 样式统一 | Small |
| `frontend/app/agent/page.tsx` | 弹窗内 input 样式统一 | Small |
| `frontend/app/knowledge/page.tsx` | 弹窗玻璃化（遮罩加 blur） | Small |

## 7. 测试策略

1. **E2E 测试**（必须，按红线 #42）：
   - `tests/ui/admin-users.spec.ts`：弹窗位置 `getBoundingClientRect()` 验证居中 + `getComputedStyle(modal).maxHeight === '85vh'`
   - `tests/ui/admin-rbac.spec.ts`：弹窗输入框 `getComputedStyle(input).borderColor` 验证包含 `rgba(255,255,255,0.15)`
   - `tests/ui/im-feishu.spec.ts`（新建）：分页控件存在 + 翻页可用
   - `tests/ui/online-indicator.md`（SPEC-079 已有）：补"分页按钮不被弹窗遮挡"断言
2. **运行时验证脚本**（开发辅助）：
   - `getComputedStyle` 列出所有 .modal-* 元素的 `position / zIndex / backdropFilter / border / maxHeight`
   - 截图对比（dark 主题下）
3. **回归审计**：所有现存 E2E 用例（SPEC-023/024/025/027/028/031）必须通过

## 8. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 修改 UI 组件时同步保留 `data-testid`（红线）
- [ ] **必须** 新增 E2E：`im-feishu` 分页存在/可用；`admin-users` 弹窗居中且 maxHeight
- [ ] **必须** 所有被改页面的既有 E2E 用例回归通过
- [ ] **必须** 运行时验证（红线 #42）：用 `getComputedStyle` 验证弹窗定位、边框、maxHeight
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 弹窗面板用 `var(--bg-secondary)` 实心色（必须 `.glass` 玻璃等效：`var(--glass-bg)` + `backdrop-filter: blur(20px)`）
- [ ] **严禁** 弹窗 inputStyle 用 `var(--border)`（必须 `rgba(255,255,255,0.15)`）

参考: `.agent/memory/E2E_TESTING.md`

## 9. 验证标准

1. **分页**：8 个 URL 列表在 totalPages >= 2 时显示分页控制（来自 Pagination 公共组件）。`/im/feishu` 真实接入分页。
2. **RBAC 系统管理员子角色**：`/admin/rbac?parent_id=system_admin` 加载子角色列表（非空）。业务主角色 → RBAC 角色 ID 映射正确。
3. **弹窗位置**：admin/users 等页面弹窗 `getBoundingClientRect()` 居中可视区（top > 0, bottom < viewport.height - headerHeight）。
4. **弹窗 maxHeight**：所有弹窗面板 `getComputedStyle().maxHeight === '85vh'`。
5. **弹窗 input 边框**：所有弹窗 input `getComputedStyle().borderColor` 包含 `rgba(255,255,255,0.15)`（或等价亮色）。
6. **弹窗样式统一**：所有列表/管理类弹窗遮罩 `rgba(0,0,0,0.5)`+blur(4px)、面板 `.glass` 玻璃等效（`var(--glass-bg)` + `backdrop-filter: blur(20px)`）+`var(--border-glass)`+16px 圆角+24px padding。
7. **data-testid**：既有 E2E testid 全部保留。
8. **CI 全绿**：sonar-check + ui-tests + ut-workflow 通过。

## 10. 已决策（待晓军确认）

1. **8 个 URL 分页**：仅 `/im/feishu` 真实缺分页（已接入），其余 7 个已接。分页规则按晓军 2026-09-04 修正：**只要有数据就显示**（不再因数据少隐藏）。
2. **RBAC 父角色映射走前端方案**：不改后端，前端 q=system_admin 查 RBAC 角色 ID 再用 parent_id 查子角色。
3. **弹窗玻璃规范以「新建分析任务」弹窗（`agent/page.tsx`）为基准**：遮罩 `rgba(0,0,0,0.5)`+blur(4px)、面板 `.glass` 玻璃等效（`var(--glass-bg)` + `backdrop-filter: blur(20px)`）+`var(--border-glass)`+16px 圆角+24px padding，推广到 admin/users / admin/rbac + 子页 + rbac-roles + im/feishu + knowledge。
4. **弹窗 input 边框统一为 `rgba(255,255,255,0.15)`**：比当前 `0.1` 更亮（晓军已确认"输入框很不明显"需修复）。
5. **不改 SPEC-076 多主题**（沿用 SPEC-078 §4.4）。