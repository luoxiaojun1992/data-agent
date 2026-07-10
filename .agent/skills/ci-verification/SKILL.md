---
name: ci-verification
description: "GitHub Actions CI verification and bug-fix cycle for data-agent. This skill should be used when verifying Go code changes through CI, polling CI run status, downloading failure logs, or executing the push-wait-fix-retry loop. Triggers include: check CI, verify PR, CI status, wait for CI, download CI logs, fix CI failure, CI failed."
agent_created: true
---

# CI Verification Skill — data-agent

Verify code changes through GitHub Actions CI. **PR merge gate: sonar-check + ui-tests must both pass.** Auto-creates and auto-merges PRs via PAT — unlike game-dev-studio where PR creation is manual.

## Prerequisites

- Working directory: the root of the data-agent repo
- GitHub token (priority order):
  1. `GITHUB_TOKEN` environment variable
  2. `.github-pat` file in repo root (auto-read by scripts)
  3. `gh auth token` (fallback, requires `gh` CLI)

## Workflow Overview

```
Code change → git push → CI triggers automatically
                              │
                              ▼
                     sonar-check (15 min)
                              │
                              ▼
                      ui-tests (20 min)
                              │
                    ┌─────────┴──────────┐
                    ▼                    ▼
                 All pass             Failure
                    │                    │
                    ▼                    ▼
                  Done             Analyze logs
                                      │
                                      ▼
                                  Fix code
                                      │
                                      ▼
                                  git push ──→ (back to CI trigger)
```

## Tool Scripts

All scripts live under `scripts/` in the skill directory.

> **Script 来源**: 以下脚本为 CI verification 通用脚本，核心 GitHub API 逻辑项目无关。
> 脚本路径均为相对于本 skill 安装目录的相对路径。

### wait-for-ci.sh — Poll Until Complete

Poll CI check runs until all required jobs complete (or timeout). Uses GitHub Checks API directly — no `gh` CLI required.

```bash
# Default: 120s interval, 1h timeout
bash scripts/wait-for-ci.sh feat/my-branch

# Custom interval and timeout
bash scripts/wait-for-ci.sh feat/my-branch --interval 60 --timeout 1800
```

- Exit codes: 0=all pass, 1=failure, 124=timeout

**🚨 MANDATORY: Foreground execution only**

`wait-for-ci.sh` MUST be executed in the **foreground**. The agent must block and wait synchronously for the script to complete.

```bash
# ✅ Correct
bash scripts/wait-for-ci.sh

# ❌ Forbidden — NEVER run in background
bash scripts/wait-for-ci.sh &   # forbidden
```

### check-ci.sh — Quick Status Check

```bash
bash scripts/check-ci.sh              # current branch
bash scripts/check-ci.sh feat/my-branch  # specific branch
# Exit codes: 0=all pass, 1=failure, 2=still running
```

### get-logs.sh — Download Failure Logs

```bash
bash scripts/get-logs.sh 12345678              # all job logs
bash scripts/get-logs.sh 12345678 --failed-only  # only failed
bash scripts/get-logs.sh 12345678 --dir ./my-logs
```

### get-videos.sh — Download Branch-Related UI Test Videos

After CI passes, download the UI test recordings for tests modified on the current branch.

```bash
bash scripts/get-videos.sh <run-id>

# Custom output directory
bash scripts/get-videos.sh 12345678 --dir ./my-videos
```

**How it works:**
1. Downloads the `allure-results` artifact from the CI run
2. Detects which UI tests are new/changed on this branch (`git diff origin/main`, auto-discovers all `*.spec.ts` files in `tests/ui/`)
3. Extracts only the matching `video.webm` files
4. Names them `UI-XXX-video.webm` for easy identification

**Prerequisites:**
- `gh` CLI authenticated (uses `GH_TOKEN` or `gh auth login`)
- Working directory: data-agent repo root
- `origin/main` must be fetchable for diff comparison

## Debug-Fix-Retry Cycle

When CI fails, max 10 retries:

1. Get the failing run ID: `bash scripts/check-ci.sh`
2. Download failure logs: `bash scripts/get-logs.sh <run-id> --failed-only`
3. Analyze the failure — identify root cause
4. Fix the code — **no workarounds, no relaxing assertions**
5. Push and re-verify: `git push && bash scripts/wait-for-ci.sh`

**🚫 Red-line rules:**
- NEVER delete a test case to make CI pass
- NEVER downgrade a test assertion (e.g. skip, fixme, relax checks)
- NEVER modify business logic to work around a test failure

### Step 6: 假性成功排查 — 检查日志中的隐藏错误

**CI "passed" ≠ 真的没问题。** Mock chains、error recovery middleware、HTTP 200 响应都可能掩盖真实的业务错误，导致测试通过但底层服务实际返回了异常。CI 通过后必须执行以下检查：

#### 6.1 下载完整日志

```bash
# 下载 ui-tests job 的完整日志（不仅 failed-only）
bash scripts/get-logs.sh <run-id> --dir ./ci-logs-full
```

#### 6.2 检查 Agent Service HTTP 错误码

data-agent 的 gin.Recovery() 中间件会捕获 panic 并返回 HTTP 500，但测试可能没有验证响应体内容。检查 data-agent 容器日志中的 >=400 状态码：

```bash
# 检查 data-agent 服务的 HTTP 错误响应（排除 /health 健康检查）
cat ./ci-logs-full/*.log \
  | grep -E "status.*[45][0-9]{2}" \
  | grep -v "/health"
```

当 data-agent 出现以下状态码时，CI 视为**假性成功**，必须修复：

| 状态码 | 常见假性成功场景 |
|--------|----------------|
| 500 | gin.Recovery() 捕获 panic，返回 500 但测试未验证响应体 |
| 404 | Handler 路由未注册或资源不存在，测试用了错误的 API 路径 |
| 422 | 请求参数校验失败，但测试 mock 了错误的请求体 |
| 503 | 下游服务（MongoDB/Qdrant/Redis）不可达，被熔断器降级 |

#### 6.3 检查基础设施服务错误

逐个检查 docker compose 中各基础设施服务的错误日志：

```bash
# MongoDB: 连接错误、认证失败、超时
cat ./ci-logs-full/*.log \
  | grep -iE "(mongo|mongodb)" \
  | grep -iE "(error|fail|timeout|refused)"

# Qdrant: collection 未加载、搜索超时、连接断开
cat ./ci-logs-full/*.log \
  | grep -iE "(qdrant)" \
  | grep -iE "(error|fail|timeout|not found|not loaded)"

# Redis: 连接池耗尽、命令失败
cat ./ci-logs-full/*.log \
  | grep -iE "(redis)" \
  | grep -iE "(error|fail|timeout|refused)"

# SeaweedFS: 上传/下载失败、master 不可达
cat ./ci-logs-full/*.log \
  | grep -iE "(seaweedfs|filer|weed)" \
  | grep -iE "(error|fail|timeout|refused)"
```

| 服务 | 常见假性成功 |
|------|-------------|
| MongoDB | 连接超时被自动重试掩盖，集合不存在返回空结果 |
| Qdrant | Collection 未加载，搜索返回空但无报错 |
| Redis | 连接池耗尽时 fallback 到直接 DB 查询，性能降级但不报错 |
| SeaweedFS | 上传失败但 fileID 为空，后续操作静默跳过 |
| SonarQube | 扫描超时或项目未创建，CI 步骤 marked as warning |

#### 6.4 检查应用层错误日志

HTTP 200 不意味着业务逻辑正确。还需检查 Go 应用的错误级别日志输出：

```bash
# 搜索 Go 错误级别日志（排除误匹配）
cat ./ci-logs-full/*.log \
  | grep -iE "\b(error|panic|fatal|fail|exception)\b" \
  | grep -v "error message above" \
  | grep -v "expected.*error" \
  | grep -v "/health"
```

常见掩藏在正常响应中的 Go 错误模式：
- `[ERROR]` — 业务错误被 log.Error() 记录但返回了降级结果
- `panic: ...` — gin.Recovery() 恢复的 panic，HTTP 层面返回 500 但测试可能未断言
- `context deadline exceeded` — 超时被 context 取消，返回部分结果
- `connection refused` — 下游服务不可达，降级返回空数据
- `driver:.*connection.*error` — MongoDB driver 连接错误被自动重试吞噬

> **处理原则**：发现任何隐藏错误，即使 CI 显示 "success"，也必须定位根因并修复。假性成功 = 实际失败。

## After CI Passes

- Download branch-related UI test videos:
  ```bash
  bash scripts/get-videos.sh <run-id>
  ```
- Mark the task as complete
- Update daily memory log with the fix summary
- If the root cause was a non-obvious pattern, update `.agent/memory/CONVENTIONS.md`

## Full Automation (One Command)

```bash
git push
bash scripts/wait-for-ci.sh   # FOREGROUND — blocks until CI completes
```

If the above fails, proceed to the debug-fix-retry cycle.
