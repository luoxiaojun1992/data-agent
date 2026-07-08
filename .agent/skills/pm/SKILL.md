---
name: pm
description: >
  项目管理全流程技能。从需求收集 → spec 设计 → spec 开发 → PR 合并，全流程编排。
  触发词：PM、项目经理、需求开发、开始需求、需求规划、实现全部spec、开发spec。
agent_created: true
---

# PM — 项目管理全流程

## 🚫 核心红线（禁止事项）

以下规则为硬性约束，不可违反：

1. **一个 Spec = 一个分支 = 一个 PR。** 禁止将多个 spec 的代码合并到同一个分支或同一个 PR 中。每个 spec 必须独立分支、独立 PR、独立合并。
2. **CI 未完全通过，禁止合并 PR。** 合并前提：`sonar-check = success` **AND** `ui-tests = success`。任一未通过均不得合并，必须先修复再重跑 CI。
3. **禁止合并开发。** 不在一个分支上同时实现多个 spec 的功能。拆分 spec 的目的就是隔离变更、独立验证。

> 违反以上任何一条，PR 不得合并，必须拆分后重新提交。

## 概述

本 skill 编排从需求到合并的完整开发流程，覆盖三个阶段：
1. **需求收集** — 用户描述需求 → spec-writer 编写设计文档
2. **实施规划** — 确认 scope → 编排 spec 实现顺序
3. **迭代实现** — spec-dev-flow 逐个实现 → 创建 PR → CI 通过 → 合并 → 下一个

## 前置条件

- 在 data-agent 项目根目录下执行
- GitHub PAT 包含 `repo` 和 `pull` scope（创建 + 合并 PR 权限）
- 已安装 `spec-writer`、`spec-dev-flow`、`ci-verification`、`doc-sync` 子 skill

## 工作流

---

### Phase 1: 需求收集 → Spec 设计

#### Step 1.1 — 询问需求

主动向用户提问，收集需求信息：

```
请描述你的需求，我会整理为设计文档。可以从以下角度说明：
- 要解决什么问题？
- 涉及哪些功能模块？
- 有什么约束条件？
- 期望的结果是什么？
```

**注意**：如果用户说"没有需求"或"先跳过"，直接跳到 Phase 2。

#### Step 1.2 — 判断拆分策略

收集到需求后，判断是否需要拆分为多个 spec：

| 条件 | 策略 |
|------|------|
| 功能单一、影响范围小 | 1 个 spec |
| 涉及多个独立模块 | 按模块拆分 |
| 有前置依赖关系 | 按依赖链拆分，注明依赖顺序 |
| 包含前后端 + 基础设施 | 按技术层拆分 |

将拆分方案呈现给用户确认：

```
我建议拆分为以下 N 个 spec：
1. SPEC-XXX: {标题} — {简要说明}
2. SPEC-XXX: {标题} — {简要说明}  ← 依赖 SPEC-XXX
3. SPEC-XXX: {标题} — {简要说明}

确认后我开始编写 spec 设计文档。
```

#### Step 1.3 — 调用 spec-writer 编写 spec

- 加载 `spec-writer` skill
- 为每个 spec 创建设计文档（`.agent/specs/<name>.md`）
- 更新 `.agent/specs/INDEX.md`，状态设为 `设计中`
- 提交 spec 文档

```bash
git add .agent/specs/ && git commit -m "docs: add SPEC-XXX, SPEC-XXX design docs"
git push
```

---

### Phase 2: 实施规划 → 确认 Scope

#### Step 2.1 — 列出未实现 spec

读取 `.agent/specs/INDEX.md`，提取状态为 `设计中` 的 spec，同时读取每个 spec 的「前置依赖检查」表格：

```
当前有 N 个未实现的 spec：

| 编号 | 标题 | 前置依赖 | 是否阻塞 |
|------|------|---------|:---:|
| SPEC-002 | CI/CD | — | ✅ 可开始 |
| SPEC-003 | Infra | SPEC-002 | ❌ 等待 SPEC-002 |
| SPEC-004 | Agent Core | SPEC-003 | ❌ 等待 SPEC-003 |
```

#### Step 2.2 — 确认 Scope

向用户确认实现范围：

```
请确认要实现的 spec 范围：
- 输入 "全部" → 实现以上全部 N 个 spec
- 输入编号列表 → 如 "SPEC-002, SPEC-003" 只实现指定的
- 输入 "跳过" → 不实现，结束流程

如果没有前置 spec（Phase 1 未执行），则只列出已有未实现 spec。
```

**默认策略**：用户未明确说明 scope 时，默认实现全部。

#### Step 2.3 — 编排实现顺序

按以下优先级确定顺序：
1. **依赖优先**：有依赖的 spec 必须先实现它所依赖的
2. **编号优先**：无依赖冲突时，按编号从小到大

输出实现计划：

```
实现顺序（共 N 个）：
1. SPEC-002: xxx — 分支 feat/SPEC-002-xxx
2. SPEC-004: xxx — 分支 feat/SPEC-004-xxx
3. SPEC-003: xxx — 分支 feat/SPEC-003-xxx（依赖 SPEC-002 已完成）

确认后开始？
```

---

### Phase 3: 迭代实现 → PR → 合并

#### Step 3.1 — 切换到 main 并更新

```bash
git checkout main && git pull origin main
```

#### Step 3.2 — 实现当前 spec

> **红线确认**: 每次只实现 **一个** spec。分支上只包含当前 spec 的代码变更，严禁混入其他 spec 的代码。

对每个 spec（按 Phase 2 确定的顺序），执行以下子流程：

**3.2.1 加载 spec-dev-flow**

加载 `spec-dev-flow` skill，告知当前 spec 编号。

**3.2.2 执行 spec-dev-flow 的 Step 1-8**

spec-dev-flow 会自动完成：
- 确认编号 → 阅读文档 → 规划任务 → 实现 → review → lint → 推送分支

> **注意**：spec-dev-flow 的 Step 9（等待人工建 PR）在本 skill 中由 Step 3.2.3 接管。

**3.2.3 创建 PR**

分支推送后，使用 GitHub API 创建 PR：

```bash
# 从 .github-pat 读取 token
TOKEN=$(cat .github-pat)

# 创建 PR
curl -X POST https://api.github.com/repos/luoxiaojun1992/data-agent/pulls \
  -H "Authorization: token $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "{SPEC-XXX}: {描述}",
    "head": "feat/SPEC-XXX-xxx",
    "base": "main",
    "body": "## 变更\n\n- xxx\n\n## 关联 Spec\n\n- SPEC-XXX: {标题}\n\n## Checklist\n\n- [ ] golangci-lint 通过\n- [ ] go vet 通过\n- [ ] E2E 占位用例通过"
  }'
```

告知用户 PR 链接。

**3.2.4 CI 验证**

加载 `ci-verification` skill：
- 等待 CI 完成（`wait-for-ci.sh`）
- 如有失败 → 修复 → push → 重新等待
- 最多 10 次重试

**3.2.5 合并 PR**

> **合并前置条件（红线）**: `sonar-check = success` **AND** `ui-tests = success`。任一不满足，**绝对禁止合并**。

CI 全部通过后，合并 PR：

```bash
# 使用 GitHub API 合并 PR
curl -X PUT "https://api.github.com/repos/luoxiaojun1992/data-agent/pulls/{PR_NUMBER}/merge" \
  -H "Authorization: token $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"merge_method": "squash"}'
```

删除远程分支（可选）：
```bash
git push origin --delete feat/SPEC-XXX-xxx
```

**3.2.6 更新 spec 状态**

```bash
# 将 SPEC-XXX 状态从"设计中"更新为"已实现"
# 更新 .agent/specs/XXX.md 文档头部
# 更新 .agent/specs/INDEX.md

git add .agent/specs/ && git commit -m "docs: mark SPEC-XXX as implemented"
git push
```

**3.2.7 Doc Sync**

加载 `doc-sync` skill，执行全量文档同步。

#### Step 3.3 — 下一个 spec

返回 Step 3.1，为下一个 spec 创建新分支，重复流程。

#### Step 3.4 — 全部完成

所有 spec 实现完毕：

```
🎯 全部 N 个 spec 已实现并合并到 main

| Spec | PR | 状态 |
|------|----|:---:|
| SPEC-002 | #N | ✅ 已合并 |
| SPEC-004 | #N+1 | ✅ 已合并 |
| SPEC-003 | #N+2 | ✅ 已合并 |

文档已同步，可以开始下一轮需求。
```

## 分支命名

| Spec | 分支 |
|------|------|
| SPEC-XXX | `feat/SPEC-XXX-{kebab-description}` |

## PR 模板

```
## 变更

- {change 1}
- {change 2}

## 关联 Spec

- SPEC-XXX: {标题}

## Checklist

- [ ] golangci-lint 通过
- [ ] go vet 通过
- [ ] govulncheck 通过
- [ ] E2E 占位用例通过
```

## 错误处理

| 场景 | 处理 |
|------|------|
| CI 超过 10 次重试仍失败 | 暂停，告知用户排查 |
| PR 合并冲突 | 手动 rebase main 解决冲突后重新 push |
| PAT 权限不足 | 提示用户检查 PAT scope（需要 repo + pull） |
| spec-dev-flow 中断 | 记录当前 spec 和步骤，恢复时从断点继续 |

## 子 Skill 引用

| Skill | 使用时机 |
|-------|---------|
| `spec-writer` | Phase 1 — 编写 spec 设计文档 |
| `spec-dev-flow` | Phase 3 — 单个 spec 的实现流程 |
| `ci-verification` | Phase 3 — CI 验证修复 |
| `doc-sync` | Phase 3 — 文档同步 |
