# UI E2E 测试设计 — 响应式设计 (RESP)

> **SPEC-040** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 响应式设计的 E2E UI 测试用例规范。覆盖移动浏览器布局适配、平板布局适配和触摸友好交互。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §27
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-193 ~ UI-195（共 3 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-013 | ✅ | 管理后台（响应式设计规范） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 移动浏览器布局适配 | UI-193 | P1 |
| 平板布局适配 | UI-194 | P2 |
| 触摸友好交互 | UI-195 | P2 |

## 4. 测试用例

### UI-193: Resp — 移动浏览器布局适配

- **优先级**: P1
- **前置条件**: 在移动端浏览器（宽度 375px ~ 768px）中打开
- **步骤**:
  1. 访问登录页
  2. 登录后查看主界面
- **预期结果**:
  - 登录卡片宽度适配（≤ 屏幕宽度 - 40px）
  - 侧边栏变为 Hamburger 菜单或底部 Tab 栏
  - 表格水平滚动
  - KPI 卡片行变为单列或双列
  - 图表自适应容器宽度
  - `data-testid`: N/A (viewport-based)

### UI-194: Resp — 平板布局适配

- **优先级**: P2
- **前置条件**: 在 iPad/平板浏览器（宽度 768px ~ 1024px）中打开
- **步骤**:
  1. 访问主界面
- **预期结果**:
  - 侧边栏可能缩小或变为可折叠
  - 表格正常显示
  - KPI 卡片行 2 列或 4 列
  - `data-testid`: N/A

### UI-195: Resp — 触摸友好交互

- **优先级**: P2
- **前置条件**: 移动端浏览器
- **步骤**:
  1. 触摸点击按钮、链接、表格行
- **预期结果**:
  - 点击区域足够大（≥ 44px — Apple HIG）
  - 按钮间距足够防止误触
  - 长按不会触发意外的文本选择
  - `data-testid`: N/A

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 与各页面功能对应的 API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 各页面的 mock 数据 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | Medium — 需要使用 Playwright 的 `viewport` 设置和 `isMobile`/`hasTouch` 选项 |
| 是否有无法实现的用例 | **待确认** — `UI-195` 触摸友好交互：Playwright 支持 `page.touchscreen` 但仅限于 Chromium，且部分触摸行为（如长按）可能不支持 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/responsive.spec.ts` | RESP E2E 测试实现 | New |
| `tests/ui/fixtures/responsive.fixture.ts` | Responsive mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P1 用例（UI-193）必须通过
- [ ] 所有 P2 用例（UI-194, UI-195）建议通过
- [ ] 响应式设计 UI 100% 符合 PRD §F-16

## 8. UI Test / E2E 验收规则

- [ ] **必须** 使用多个 `viewport` 配置运行测试（375px, 768px, 1024px, 1440px）
- [ ] **必须** 验证在不同 viewport 下关键元素存在且可见
- [ ] **必须** 验证侧边栏在移动端的折叠行为
- [ ] **严禁** 仅测试桌面端 viewport

参考: `.agent/memory/E2E_TESTING.md`
