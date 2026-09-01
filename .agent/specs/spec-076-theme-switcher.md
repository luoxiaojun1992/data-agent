# 前端主题切换 + 蓝白 Light 主题

> **SPEC-076** | Status: 设计已定稿（2026-09-01）

## 1. 目标

为前端提供主题切换能力：保留当前深色主题为默认，新增一套「蓝白配色」Light 主题，用户可在两种主题间切换。主题选择持久化到浏览器 `localStorage`，默认仍为当前深色主题。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| — | — | 无前置依赖，纯前端变更，可立即实现 |

## 2. 背景（现状）

前端当前**只有一套硬编码的深色主题**：

- 主题变量定义在 `app/globals.css` 的 `:root`（无 `data-theme`、无 `prefers-color-scheme`、无切换逻辑）
- 现有变量：`--bg-primary: #0a0a1a`（深蓝黑）、`--bg-secondary: #111133`、`--text-primary: #e8e8f0`、`--accent: #5c7cfa`（蓝紫）、`--glass-bg`、`--border-glass` 等
- 全站组件通过 `var(--accent)` / `var(--text-primary)` / `var(--text-secondary)` / `var(--bg-*)` 消费变量，**变量化程度高，主题化改造代价低**
- `body::before` 有 Aurora 背景效果（蓝紫 radial-gradient），属于当前深色主题视觉的一部分
- 无 ThemeProvider、无 `localStorage` 主题持久化、无切换入口

## 3. 架构概述

主题机制采用 **CSS 变量 + `data-theme` 属性**（最轻量、无需依赖）：

```
<html data-theme="dark|light">   ← 根元素属性切换
  ├─ :root / [data-theme="dark"]  当前深色主题（默认）
  └─ [data-theme="light"]         新增蓝白 Light 主题
        │
        ▼
  CSS 变量两套值（--bg-* / --text-* / --accent / --glass-* 等）
        │
        ▼
  所有组件无需改动，自动跟随变量切换
```

切换逻辑：

```
ThemeToggle 组件 → setTheme('light') 
  → document.documentElement.setAttribute('data-theme', 'light')
  → localStorage.setItem('data-agent-theme', 'light')
```

## 4. 详细设计

### 4.1 主题变量定义（两套）

| 变量 | 当前深色（默认，`:root`） | 蓝白 Light（`[data-theme="light"]`） |
|------|--------------------------|--------------------------------------|
| `--bg-primary` | `#0a0a1a` | `#f5f7fb` |
| `--bg-secondary` | `#111133` | `#eef2f9` |
| `--bg-card` | `rgba(15, 15, 45, 0.6)` | `rgba(255, 255, 255, 0.85)` |
| `--text-primary` | `#e8e8f0` | `#1a2233` |
| `--text-secondary` | `#a0a0c0` | `#5a6b85` |
| `--accent` | `#5c7cfa` | `#2563eb` |
| `--accent-glow` | `rgba(92, 124, 250, 0.3)` | `rgba(37, 99, 235, 0.18)` |
| `--border-glass` | `rgba(255, 255, 255, 0.08)` | `rgba(15, 23, 42, 0.08)` |
| `--glass-bg` | `rgba(255, 255, 255, 0.04)` | `rgba(255, 255, 255, 0.6)` |
| `--glass-hover` | `rgba(255, 255, 255, 0.08)` | `rgba(255, 255, 255, 0.9)` |

### 4.2 蓝白 Light 主题的 Aurora 背景

`body::before` 的深色 Aurora（蓝紫 radial-gradient）在 Light 主题下会显得过暗/突兀，需同步提供 Light 版：

- 深色（默认）：保持现有蓝紫 Aurora
- Light：改为浅蓝 `rgba(37, 99, 235, 0.05)` 等低透明度的蓝白渐变，或直接淡化为接近纯白

实现：将 Aurora 渐变色提取为 CSS 变量（如 `--aurora-1` / `--aurora-2` / `--aurora-3`），两套主题各给一份值。

### 4.3 主题状态管理与持久化

- 新建 `useTheme` hook（或 `ThemeContext`）：
  - state: `theme: 'dark' | 'light'`
  - 初始化：读 `localStorage['data-agent-theme']`，无则默认 `'dark'`
  - 切换：`setTheme` 同步写 `document.documentElement.dataset.theme` + `localStorage`
- localStorage key：`data-agent-theme`，值 `dark` | `light`
- **默认 `dark`**（当前主题，符合需求第 3 点）

### 4.4 SSR / 闪烁处理

项目为 Next.js App Router，`AppLayout`（`app/providers.tsx`）已是 `'use client'`，且已有「`!auth.hydrated` 时 return null」的防闪烁机制。主题初始化复用同思路：

- 在 hydration 阶段（`auth.hydrated` 前）不渲染，`useEffect` 中读 localStorage → 设置 `data-theme`
- 或更优：`app/layout.tsx` 的 `<head>` 注入 inline script，在首帧前同步读 localStorage 设置 `data-theme`，彻底避免主题闪烁（推荐）

### 4.5 ThemeToggle 组件与入口

- 新建 `app/components/ThemeToggle.tsx`：按钮切换 dark/light（图标 + 可选动画）
- 挂载点：顶栏（`providers.tsx` 中 `main` 顶部的 header 区域，与 `NotificationBell` 同级），或 `Sidebar` 底部
- `data-testid="theme-toggle"` 供 E2E 断言

### 4.6 既有硬编码色值的变量化收尾（本 spec 范围，重要）

按实施顺序（074 → 075 → 078 → 077 → **076**），076 落地时前端已存在两类**不消费 CSS 变量**的硬编码色值，本 spec 需一并变量化，否则 Light 主题下会显示异常：

| 来源 | 硬编码值 | 变量化建议 |
|------|---------|-----------|
| **SPEC-078 统一 UI**（本 spec 前置） | 顶部主按钮渐变 `#5c7cfa→#7c3aed`（白字）、弹窗面板曾用的 `#1a1a2e` 类硬编码 | 提取 `--btn-primary`（及 light 变体）、`--modal-bg`，两套主题各给值 |
| 历史遗留（各页面 inline style） | 各页内联 `style={{...}}` 中的色值（如分页 active `#5c7cfa`、toast `#34d399`/`#ef4444` 等） | 逐页排查，改为 `var(--*)` 引用（`--accent`、`--success`、`--danger`） |

> 红线：变量化收尾只改「颜色表达方式」，**禁止**改变视觉设计（色值换算为两套主题下的对应值，深色主题视觉保持原样）、**禁止**破坏布局（对齐 SPEC-078 红线）。

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No（纯前端） |
| 性能影响 | 无（CSS 变量切换，零额外请求） |
| 是否需要新增 Skill | No |
| 是否需要后端改动 | No |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/app/globals.css` | 新增 `[data-theme="light"]` 变量块 + Aurora 变量化 | Medium |
| `frontend/app/layout.tsx` | `<head>` 注入主题初始化 inline script（防闪烁） | Small |
| `frontend/app/components/ThemeToggle.tsx` | 主题切换按钮（新） | New |
| `frontend/app/providers.tsx` | 顶栏挂载 ThemeToggle | Small |
| `frontend/lib/theme.ts`（或 hook） | `useTheme` 状态 + localStorage 持久化 | New |

## 7. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 8. 验证标准

1. 首次访问默认深色主题（`data-theme` 缺省或 `dark`）。
2. 点击切换 → 全站切换到蓝白 Light 主题，所有 `var(--*)` 消费组件同步变色。
3. 再次切换回深色，样式恢复。
4. 刷新页面后主题保持（localStorage 持久化）。
5. 切换无白屏闪烁（inline script 首帧前设置 `data-theme`）。
6. 两套主题下 `--text-*` 与 `--bg-*` 对比度满足可读性（WCAG AA 建议）。
