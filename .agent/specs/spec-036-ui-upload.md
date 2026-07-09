# UI E2E 测试设计 — 批量文件上传 (UPLOAD)

> **SPEC-036** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 批量文件上传功能的 E2E UI 测试用例规范。**E2E 自动化通过真实 fixture 文件上传到后端进行测试**，验证文件上传的完整链路（前端 → API → 存储）。拖拽上传 (UI-173) 标记为人工测试。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §23
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-172 ~ UI-176（共 5 个用例，其中 1 个为人工测试）

> **说明**: 本 Spec 覆盖跨知识库管理 (KB) 和 API 转换审核 (API) 的通用上传交互规范。

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-006 | ✅ | 知识库系统（文件上传存储） |
| SPEC-013 | ✅ | 管理后台（上传交互通用规范） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-028 | ✅ | KB E2E |
| SPEC-030 | ✅ | API E2E |

> **环境要求**: 测试时后端服务必须运行（`docker compose up` 或本地启动），SeaweedFS 可用。

## 3. 测试范围

| 子功能 | 用例 | 优先级 | 测试方式 |
|--------|------|:------:|------|
| 文件多选 | UI-172 | P0 | 🤖 E2E（真实上传） |
| 拖拽上传 | UI-173 | P1 | 👤 **人工测试** |
| 独立进度条 | UI-174 | P0 | 🤖 E2E（真实上传） |
| 取消单个文件上传 | UI-175 | P1 | 🤖 E2E（真实上传） |
| 上传不阻塞 UI | UI-176 | P1 | 🤖 E2E（真实上传） |

> **说明**: UI-173 拖拽上传涉及复杂的 DOM 事件模拟（dragenter/dragover/drop + DataTransfer + File），Playwright 可以模拟但可靠性有限。拖拽功能标记为人工测试，在正式发布前手动验证。

## 4. 文件上传测试策略

> **核心原则**: 真实上传到后端，验证完整链路。不 mock 上传端点。

| 层 | 方式 | 说明 |
|----|------|------|
| 文件选择 | 真实 fixture 文件 | `page.setInputFiles()` 使用 `tests/ui/fixtures/files/` 下的小文件（如 1KB `.txt`, `.pdf`, `.json`） |
| 上传请求 | **真实调用后端** | 不拦截上传端点，文件真实上传到后端 → SeaweedFS |
| 验证方式 | 后端响应 + UI 状态 | 检查 HTTP 200 响应、文件列表中状态变更（进度条、✅ 完成图标） |

> **Playwright 模式**:
> ```typescript
> // 不做 route mock，让请求真实到达后端
> // 使用 fixture 小文件触发 input
> await page.setInputFiles('[data-testid="kb-upload-file-input"]', [
>   'tests/ui/fixtures/files/test-1.pdf',
>   'tests/ui/fixtures/files/test-2.txt',
>   'tests/ui/fixtures/files/test-3.json',
> ]);
> // 等待所有文件上传完成（进度条消失或出现 ✅ 图标）
> await expect(page.locator('[data-testid="kb-upload-progress-0"]')).not.toBeVisible({ timeout: 30000 });
> // 验证文件列表中出现 3 个文件
> await expect(page.locator('[data-testid^="kb-file-item-"]')).toHaveCount(3);
> ```

## 5. 测试用例

### UI-172: Upload — 文件多选

- **优先级**: P0
- **前置条件**: 知识库管理页或 API 转换审核页，后端服务运行中
- **步骤**:
  1. 点击上传按钮
  2. 在文件选择框中 Ctrl/Cmd + 点击选择 3 个 fixture 文件
- **预期结果**:
  - 文件选择框支持多选
  - 选中的文件出现在文件列表中
  - 文件真实上传到后端
  - `data-testid`: `{page}-upload-file-input`

### UI-173: Upload — 拖拽上传（人工测试）

- **优先级**: P1
- **前置条件**: 上传区域可见
- **步骤**:
  1. 将 2 个文件拖拽到上传区域
- **预期结果**:
  - 拖入时上传区域高亮（虚线变实线 + 颜色变化）
  - 释放后 2 个文件加入上传队列
  - `data-testid`: `{page}-drop-zone`
- **测试方式**: 👤 人工在浏览器中验证

### UI-174: Upload — 独立进度条

- **优先级**: P0
- **前置条件**: 批量上传 3 个文件，后端运行中
- **步骤**:
  1. 触发批量上传
  2. 在上传过程中检查进度显示
  3. 等待上传完成
- **预期结果**:
  - 每个文件有**独立**的进度条
  - 进度条显示百分比
  - 上传完成显示 ✅ 图标
  - 后端返回成功响应
  - `data-testid`: `{page}-upload-progress-{index}`

### UI-175: Upload — 取消单个文件上传

- **优先级**: P1
- **前置条件**: 正在批量上传（可使用稍大文件确保上传有足够时间）
- **步骤**:
  1. 点击某个上传中文件的「取消」按钮
- **预期结果**:
  - 该文件上传取消，进度条消失
  - 其他文件继续上传并最终完成
  - `data-testid`: `{page}-upload-cancel-{index}`

### UI-176: Upload — 上传不阻塞 UI

- **优先级**: P1
- **前置条件**: 正在批量上传
- **步骤**:
  1. 在上传过程中操作其他 UI 元素（如切换筛选、翻页）
- **预期结果**:
  - UI 可正常操作
  - 上传在后台继续
  - `data-testid`: N/A

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | No — 真实上传到后端，不 mock 上传端点 |
| 是否依赖后端服务 | **Yes — 需要 Docker 后端运行**（API 服务 + SeaweedFS） |
| 是否需要特殊测试数据 | Yes — 小尺寸 fixture 文件（`tests/ui/fixtures/files/`，如 1KB ~ 100KB） |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 — 使用小文件不影响性能 |
| 实现复杂度 | Medium — 需要后端服务运行；进度条验证需要合适的文件大小以捕获上传中状态；取消上传测试需要控制文件大小确保有足够时间点击取消 |
| 是否有无法实现的用例 | 无 — `UI-173` 拖拽上传标记为人工测试；其余用例通过真实上传验证 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/upload.spec.ts` | UPLOAD E2E 测试实现 | New |
| `tests/ui/fixtures/files/test-1.txt` | 测试用 fixture 文件 | New |
| `tests/ui/fixtures/files/test-2.pdf` | 测试用 fixture 文件 | New |
| `tests/ui/fixtures/files/test-3.json` | 测试用 fixture 文件 | New |

## 8. 验证标准

- [ ] 所有 P0 用例（UI-172, UI-174）必须通过（真实后端上传）
- [ ] 所有 P1 E2E 用例（UI-175, UI-176）必须通过
- [ ] 👤 人工测试：UI-173 拖拽上传在发布前人工验证
- [ ] 上传交互 UI 100% 符合 PRD §F-22

## 9. UI Test / E2E 验收规则

- [ ] **必须** 使用小尺寸 fixture 文件触发 `<input type="file">`（`tests/ui/fixtures/files/`）
- [ ] **必须** 真实调用后端上传端点，不 mock（后端必须运行）
- [ ] **必须** 验证上传完成后的文件列表状态（✅ 图标、文件出现在列表中）
- [ ] **必须** 测试取消上传后其他文件继续
- [ ] 👤 UI-173 拖拽上传标记为人工测试
- [ ] **严禁** mock 上传端点

参考: `.agent/memory/E2E_TESTING.md`
