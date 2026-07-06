# Phase 5 — 管理后台与 Hermes 探索模式

> **SPEC-009** | Status: 设计中 | 依赖: SPEC-004（管理后台依赖 Agent API），SPEC-006（Hermes 独立）

## 目标

实现管理后台全量前端页面（React/Next.js）+ Hermes 自由探索模式独立服务。

## 背景

Roadmap Phase 5 (Week 9-10) + Week 10 附加，P5-01 ~ P5-20，总计 ~64h。

## 详细设计

### 1. 管理后台前端架构
- Next.js 14 项目初始化
- Admin Layout + 菜单路由（侧边栏 7 项导航）
- UI 框架：深色玻璃极光风格（Apple Vision OS + 玻璃卡片）

### 2. 管理后台页面（12 页）

| 页面 | 对应功能 |
|------|---------|
| 登录页 | JWT 认证 |
| 数据看板 Dashboard | KPI显示 + Token/ROI/调用量图表 |
| 用户管理 | CRUD + 角色分配 + 启停 |
| 权限管理 | 角色定义 + 权限映射 |
| 模型配置 | LLM 连接配置 + 参数调整 |
| 任务管理 | 全局任务监控 + 取消/重试 |
| 知识库管理 | 上传 + 列表 + 搜索 + 索引状态 |
| 审计日志 | 筛选 + 导出 |
| API 转换审核 | OpenAPI 导入 + 审批 |
| 消息渲染组件 | 工具调用卡片/SQL块/表格/图表 |
| 增强提示词 | AI Suggest 按钮 + 下拉面板 |
| 响应式适配 | 移动浏览器兼容 |

### 3. Hermes 自由探索模式
- Hermes Service（Go 轻量转发层）
- 请求直接转发 Hermes（不经 Agent Service）
- SSE 流式透传
- MongoDB `hermes_sessions` 集合（上下文快照）

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（hermes_sessions） |
| 是否影响现有 API | Yes（Dashboard/stats/token/output/roi API） |
| 性能影响 | 前端图表渲染 ≤ 1s |
| 是否需要新增 Skill | No |
| 是否需要 E2E 测试 | Yes（全部 12 页面需要 UI 用例） |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `frontend/` | React/Next.js 前端 | 新建 |
| `cmd/hermes/main.go` | Hermes 服务入口 | 新建 |
| `internal/service/admin/` | 管理后台 API | 新建 |

## UI Test / E2E 验收规则

> 管理后台 12 页面必须编写真实 E2E 用例。

- [ ] 登录页 → Dashboard → 用户管理 → 权限管理 核心流程
- [ ] 知识库上传→搜索→状态检查
- [ ] 审计日志筛选→导出
- [ ] 模型配置增删改查
- [ ] `data-testid` 属性覆盖全部交互元素

## 验证标准

1. 全部 12 页面可正常访问，数据与后端 API 一致
2. Dashboard KPI 卡片 + 图表实时更新
3. 知识库上传文档后索引状态正确展示
4. Hermes 探索模式独立可用，SSE 流式返回
5. E2E 用例覆盖核心页面，CI 通过
