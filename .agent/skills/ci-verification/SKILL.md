---
name: ci-verification
description: "GitHub Actions CI verification and bug-fix cycle for data-agent. This skill should be used when verifying Go code changes through CI, polling CI run status, downloading failure logs, or executing the push-wait-fix-retry loop. Triggers include: check CI, verify PR, CI status, wait for CI, download CI logs, fix CI failure, CI failed."
agent_created: true
---

# CI Verification Skill — data-agent

Verify code changes through GitHub Actions CI and execute the debug-fix-retry cycle when tests fail.

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
                     go-lint + go-test (15 min)
                              │
                              ▼
                      integration-tests (20 min)
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

> **Script 来源**: 以下脚本从 game-dev-studio 的 CI verification skill 移植，核心 GitHub API 逻辑通用。
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

## After CI Passes

- Mark the task as complete
- Update daily memory log with the fix summary
- If the root cause was a non-obvious pattern, update `.agent/memory/CONVENTIONS.md`

## Full Automation (One Command)

```bash
git push
bash scripts/wait-for-ci.sh   # FOREGROUND — blocks until CI completes
```

If the above fails, proceed to the debug-fix-retry cycle.
