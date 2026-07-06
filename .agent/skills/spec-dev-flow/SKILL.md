---
name: spec-dev-flow
description: >
  SPEC 开发全流程编排技能。当用户要求开发某个 SPEC、实现某个功能规格、开始 SPEC 开发时触发。
  覆盖从确认 SPEC 编号到文档同步推送的完整工作流：
  确认编号 → 阅读 memory → 阅读 spec → 规划任务 → 实现 → review → code-lint →
  E2E 测试 → 推送分支 → 自动创建 PR → CI 验证（sonar + E2E）→ 自动合并 → doc-sync。
agent_created: true
---

# SPEC 开发全流程编排

## 概述

本 skill 将 SPEC 开发全流程标准化，确保每个 SPEC 的实现质量一致、文档同步完整。
执行时严格按步骤顺序推进，每步完成后确认再进入下一步。

## 前置条件

- 在 data-agent 项目根目录下执行
- 当前分支为 `main` 且代码已更新到最新
- 已加载 `code-lint`、`ci-verification`、`doc-sync` 三个子 skill

## 工作流

### Step 1 — 确认 SPEC 编号

1. 读取 `.agent/specs/INDEX.md`，解析索引表
2. 提取所有状态为 `设计中` 的 SPEC，按编号从小到大排序
3. 展示可选列表给用户选择
4. 用户输入纯数字自动补全为 `SPEC-XXX` 格式

### Step 2 — 前置依赖检查（强制）

1. 读取当前 spec 的「前置依赖检查」表格
2. 逐项确认每个前置 Spec 的状态是否为 ✅（已实现）
3. 任一项为 ❌ → **阻塞当前 spec，必须先完成前置 spec**
4. 全部 ✅ → 继续下一步

> **规则**: 前置 spec 未完成时禁止开始当前 spec 的开发。此检查不可跳过。

### Step 3 — 阅读 memory 文档

依次阅读以下文档：
1. `.agent/memory/INDEX.md`
2. `.agent/memory/MEMORY.md`
3. `.agent/memory/CONVENTIONS.md`
4. `.agent/memory/ARCHITECTURE.md`

### Step 4 — 阅读 SPEC 文档

- 查找路径：`.agent/specs/{spec-file}.md`
- 完整阅读，理解设计意图和验收标准
- 如有不明确之处，向用户澄清

### Step 4 — 规划任务列表

- 基于 spec 设计将实现工作分解为具体任务
- 使用 `TaskCreate` 创建结构化任务列表
- 展示给用户确认

### Step 5 — 实现 SPEC 设计

- 从 `main` 创建功能分支：`feat/SPEC-XXX-{简短描述}`
- 按任务顺序逐步实现
- 遵循 `.agent/memory/CONVENTIONS.md` 和 `REUSABLE_PATTERNS.md` 中的规范
- Go 代码必须遵循三层架构（Handler → Service → Repository）

### Step 6 — Review 代码

自查：
- 是否满足 spec 验收标准
- 是否有遗漏的边界情况
- 是否引入了不安全代码（SQL 注入、未授权访问）
- 是否有硬编码的敏感信息
- 幂等性是否满足项目约定
- SkillContext 是否正确注入

### Step 7 — code-lint 检查

- 加载 `code-lint` skill
- 执行 `golangci-lint run ./...` + `go vet ./...` + `govulncheck ./...`
- 如有 lint 错误，修复后重新检查

### Step 7b — E2E 测试编写（强制）

- 涉及前端 UI 变更时必须编写真实 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- 添加对应 `data-testid` 属性到组件
- 更新 `.agent/memory/E2E_TESTING.md` 测试矩阵（3 处：标题数字 + 表格行）
- 纯后端变更可跳过（仅保留占位用例）
- **运行** `cd tests && npx playwright test` 确认通过

### Step 8 — 推送分支

```bash
git add -A
git commit -m "feat(SPEC-XXX): {变更描述}"
git push origin feat/SPEC-XXX-{描述}
```

### Step 9 — 自动创建 PR（本项目特有，gm-dev-studio 为手动）

使用 GitHub API + PAT 自动创建 PR：

```bash
TOKEN=$(cat .github-pat)
BRANCH="feat/SPEC-XXX-{描述}"

# 创建 PR
PR_RESPONSE=$(curl -s -X POST https://api.github.com/repos/luoxiaojun1992/data-agent/pulls \
  -H "Authorization: token $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"title\": \"feat(SPEC-XXX): {变更描述}\",
    \"head\": \"$BRANCH\",
    \"base\": \"main\",
    \"body\": \"## 变更\n\n- xxx\n\n## PR 验收标准\n\n- [ ] sonar-check 通过\n- [ ] ui-tests 通过（含真实 E2E 用例）\n\n## 关联 Spec\n\n- SPEC-XXX: {标题}\"
  }")

PR_NUMBER=$(echo $PR_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin).get('number',''))")
echo "PR created: https://github.com/luoxiaojun1992/data-agent/pull/$PR_NUMBER"
```

### Step 10 — CI 验证 + 自动合并

> **PR 合并验收标准（必须同时满足）**: `sonar-check = success` **AND** `ui-tests = success`

**10.1 等待 CI 完成**

```bash
# 加载 ci-verification skill，运行 wait-for-ci.sh
bash scripts/wait-for-ci.sh feat/SPEC-XXX-{描述}
```

- 最多重试 10 次
- 禁止删除测试用例、禁止降低断言
- sonar-check 不通过 = CI 失败，必须修复

**10.2 验证通过后自动合并 PR**

```bash
# 确认 sonar-check 和 ui-tests 均为 success
CI_STATUS=$(curl -s -H "Authorization: token $TOKEN" \
  "https://api.github.com/repos/luoxiaojun1992/data-agent/commits/$BRANCH/check-runs" \
  | python3 -c "
import sys, json
data = json.load(sys.stdin)
statuses = {r['name']: r['conclusion'] for r in data.get('check_runs', [])}
sonar = statuses.get('sonar-check')
ui = statuses.get('ui-tests')
print(f'sonar-check: {sonar}, ui-tests: {ui}')
")

echo "$CI_STATUS"

# 仅当两者均为 success 时才合并
if echo "$CI_STATUS" | grep -q "sonar-check: success" && echo "$CI_STATUS" | grep -q "ui-tests: success"; then
  curl -s -X PUT "https://api.github.com/repos/luoxiaojun1992/data-agent/pulls/$PR_NUMBER/merge" \
    -H "Authorization: token $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"merge_method": "squash"}'
  echo "✅ PR #$PR_NUMBER merged (sonar-check ✅ + ui-tests ✅)"
else
  echo "❌ PR #$PR_NUMBER NOT merged — gate not satisfied"
  exit 1
fi
```

> **与 game-dev-studio 的区别**: 本项目 skill 有 PAT 权限，自动创建和合并 PR。不再需要等待人工操作。

### Step 11 — doc-sync 文档同步

CI 通过后：
- 加载 `doc-sync` skill
- 按区域 A → H 逐项检查
- 重点：README.md spec 状态、.agent/specs/INDEX.md 状态、ARCHITECTURE.zh-CN.md
- 提交文档变更
- 在 `.workbuddy/memory/YYYY-MM-DD.md` 中记录

## 编码规范引用

- **分支命名**：`feat/SPEC-XXX-xxx`
- **Commit**：`feat(SPEC-XXX): xxx`
- **禁止直接修改 main**
- **三层架构**：Handler → Service → Repository
- **幂等性**：创建 upsert，删除不返回 404
- **SkillContext**：自动注入，禁止外部传入 session_id

## 中断与恢复

- 记录当前步骤编号
- 恢复时从断点继续，先检查 Step 2 和 Step 3 是否有更新
- 如 main 有新提交，先 rebase
