---
name: engineering-lessons
description: >
  Hard-earned engineering lessons from DataAgent project. Trigger when making quality-related
  decisions, debugging CI failures, writing tests, handling errors, reviewing PRs, or whenever
  you're about to repeat a past mistake. Also triggers after completing non-trivial work to
  self-audit and update .agent/ documentation. Keywords: anti-pattern, lesson, mistake, quality,
  testing, error handling, review, PR, CI failure, coverage, sonar, gitignore, lesson-learned,
  教训, 经验, 反模式, 质量, 覆盖, 记录教训.
agent_created: true
---

# Engineering Lessons Learned

This skill loads the non-negotiable lessons learned from real bugs and anti-patterns discovered in this project. Do NOT repeat these mistakes.

**Active duty**: After completing any non-trivial engineering task (spec, bug fix, quality improvement, CI fix), self-audit against this skill's lessons. If a new lesson is discovered, update this SKILL.md AND the corresponding memory file in `.agent/memory/`.

---

## Lesson 1: Fix the Root Cause, Never Patch the Symptom

**Anti-pattern**: Test fails → weaken the test instead of fixing the bug.

**Real example 2026-07-16**:
- Agent UI-052 test expected `agent-task-title-*` to appear after task creation
- Test failed in CI repeatedly (30s timeout)
- Initial response: remove the assertion, keep only `agent-page-header` check
- **Correct response**: debug why `loadTasks()` doesn't render the task row → fix the frontend `createTask` flow

**Rule**: When a test or validation fails, the only acceptable responses are: (a) fix the underlying bug, or (b) delete the entire test/validation if the behavior under test doesn't exist. Never weaken assertions to accommodate bugs.

**Checklist when test fails repeatedly**:
```
1. Is this test asserting real behavior that should work? → YES → fix the bug
2. Is this testing something that doesn't exist in the codebase? → YES → delete the test
3. Is this a timing/async issue? → YES → add proper waitFor, not longer timeouts
4. Is this a CI-only failure? → YES → download CI logs, search for backend errors
```

---

## Lesson 2: Conditional Code That Hides Failures Is Worse Than No Code

**Anti-pattern**: Wrap assertions in conditions so they never fail.

```typescript
// ❌ NEVER do this — silently passes when things are broken
if (await button.isVisible().catch(() => false)) {
  await button.click();
}

// ❌ NEVER do this — swallows assertion failures
await expect(el).not.toBeVisible({ timeout: 3000 }).catch(() => {});

// ❌ NEVER do this — hides API errors from operators
try { await fetchData(); } catch { /* ignore */ }

// ✅ Always let failures surface
await expect(button).toBeVisible({ timeout: 10000 });
await button.click();
```

**Why**: When `loadTasks()` in `agent/page.tsx` had `catch { /* ignore */ }`, the task list silently showed empty. CI tests timed out after 30s with no clue why. Fix took hours instead of minutes. A single `console.error` would have saved hours.

**Rule**: Every `catch` block must either:
- Log the error (`console.error`, `log.Printf`, etc.)
- Re-throw after logging
- Show user-visible error feedback

---

## Lesson 3: Debug Systematically — Check Logs Before Changing Code

**Anti-pattern**: Test times out at 20s → increase timeout to 30s → still fails → remove assertion.

**Correct workflow**:
```
Test fails
  ↓
1. Download CI logs → grep for backend 4xx/5xx
2. Search frontend code for catch { /* ignore */ } patterns
3. Search for mismatch between test selectors and actual DOM
4. Only after understanding root cause: fix code OR delete test
```

**Real example 2026-07-16**: Agent/task tests failing with 30s timeout. Instead of checking backend logs (which would have shown the `loadTasks` API call succeeding but React not re-rendering), we tried: 20s timeout → 30s timeout → page.reload → remove assertion. Hours wasted.

**Tooling**: Use `.agent/skills/ci-verification/scripts/` for CI log analysis:
```bash
bash get-logs.sh <run-id> --failed-only
grep -E 'data-agent.*[45][0-9]{2}' ci-logs-<id>/*.log
```

---

## Lesson 4: Document Every Lesson Immediately

**Anti-pattern**: Fix the bug, move on, forget why it happened. Repeat next month.

**Correct workflow**: After fixing any non-trivial bug:
1. Add to `.agent/memory/CONVENTIONS.md` under "已纠正的错误" table
2. If it's a systemic issue, add a red line (红线)
3. If it affects E2E testing, update `.agent/memory/E2E_TESTING.md`
4. If it's a reusable pattern, update this skill

**Why**: The `catch { /* ignore */ }` anti-pattern appeared in at least 3 different files. After the first fix, we documented it. After the third fix, we made it a red line. Each iteration was faster.

---

## Lesson 5: State Change Must Be Verified End-to-End

**Anti-pattern**: Test named "cancel task" only verifies `agent-page-header` is visible.

**Correct pattern**: Every test named "do X" must verify the before-state, the action, and the after-state.

```
Before state → verify preconditions
      ↓
Action → perform the operation
      ↓
After state → verify the change happened
```

**Example** (cancel task):
```typescript
// 1. Before: task row is visible (createTask → loadTasks)
await expect(row).toBeVisible({ timeout: 10000 });

// 2. Action: click cancel
await cancelBtn.click();

// 3. After: row is gone (cancelTask → loadTasks)
await expect(row).not.toBeVisible({ timeout: 10000 });
```

**Rule**: If any of the three steps can't be verified deterministically in CI, don't write the test. Search for the root cause — often a missing `loadTasks()` call or a silenced error — and fix that instead.

---

## Lesson 6: One PR, One Purpose

**Anti-pattern**: Fix 15 different issues across 20 files in one PR.

**When it's OK**: When all changes serve the same purpose (e.g., "fix test quality"). This PR changed 19 files but all served the same goal.

**When it's NOT OK**: Mixing feature work with test fixes, mixing refactoring with bug fixes. Separate PRs for separate concerns.

---

## Lesson 7: Trust CI, Not Instinct

**Anti-pattern**: "This change is trivial, it'll pass" → push without waiting for CI.

**Rule**: Every push must be verified by CI. If CI fails:
1. Check if the failure is a real bug exposed by the change
2. If yes: fix the bug
3. If no: investigate why CI is flaky (is it a real flake or a hidden bug?)

**No exception**: CI is the single source of truth for whether a change is safe to merge.

---

## Lesson 8: Never Downgrade Quality Gates — Fix the Issues

**Anti-pattern**: Sonar reports 24 CRITICAL issues → modify the gate script to exclude CODE_SMELL from blocking → pretend it's fixed. CI still red, credibility lost.

**Real example 2026-07-17 (SPEC-045)**:
- Sonar quality gate blocked on 24 CRITICAL CODE_SMELL
- First wrong response: edited `tests/ci/sonar-quality-gate.py` to skip CODE_SMELL
- User feedback: "有issue就修复，而不应该降级绕过"
- **Correct response**: refactored main() 357→<15 complexity, extracted 17 duplicate literals as constants, fixed all 24 issues

**Rule**: **Quality gates are hard constraints.** Every BLOCKER, CRITICAL, VULNERABILITY must be fixed in the actual code. Never modify the gate script, never lower thresholds, never exclude categories to make numbers look good.

---

## Lesson 9: CI vs Local Discrepancy Means a Real Problem

**Anti-pattern**: Local coverage 100%, CI 99.3% → declare "toolchain precision difference" 3 times → all wrong.

**Real example 2026-07-17 (SPEC-045)**:
- CI showed 3 functions at 0% (hermes service.go), local showed 100%
- Three wrong theories in sequence: cross-package counting → Go version difference → gomonkey+Linux/race failure
- **Actual root cause**: `.gitignore` line `hermes` matched `internal/service/hermes/` directory — the test file was never committed to CI
- Discovery method: downloaded CI coverage artifact → compared per-function coverage → found exact 0% lines → `git check-ignore` revealed the gitignore bug

**Rule**: When CI and local disagree:
```
1. Download CI artifact → go tool cover -func → find exact gap
2. NEVER guess. Evidence first, then theory.
3. Say "I haven't found the root cause yet" instead of "it must be X"
```

---

## Lesson 10: Verify Locally Before Push — Every Time

**Anti-pattern**: Write code → push → wait for CI → fail → fix → push → fail → ... (loop 6+ times).

**Real example 2026-07-17 (SPEC-045)**: 
- Pushed 3 times with UT gate strategy changes (per-function → total aggregate → back)
- Pushed 2 rounds of Sonar refactoring without verifying all 24 issues were resolved
- Each CI cycle takes ~8 minutes. 6 cycles = 48 minutes wasted.

**Correct workflow**:
```bash
# Before every push:
go test -race -gcflags=all=-l -coverprofile=/tmp/ut-check.out \
  -coverpkg=./internal/api/...,./internal/config/...,./internal/domain/...,./internal/logic/...,./internal/service/...,./skills/... \
  ./internal/... ./skills/...
go tool cover -func=/tmp/ut-check.out | grep total    # must be >= 98%
golangci-lint run ./...                                # must be 0 errors
```
CI is for CONFIRMATION, not for DISCOVERY. Discover issues locally first.

---

## Lesson 11: .gitignore Patterns Match Subdirectories

**Anti-pattern**: `hermes` in .gitignore → silently excludes `internal/service/hermes/` from version control.

**Rule**: 
- `hermes` → matches hermes **anywhere** in the tree (file or directory)
- `/hermes` → matches only in the **repository root**
- Always use `/dirname` for root-level patterns unless you intend to match subdirectories

**Checklist when files are mysteriously missing from CI**:
```bash
git check-ignore -v path/to/file   # shows which pattern caused the ignore
```

---

## Lesson 12: gomonkey + Linux + race = Fail Silently

**Anti-pattern**: Use gomonkey (runtime function patching) to mock `http.Client.Do` in tests, then run with `-race`.

**Why**: gomonkey modifies function prologues at runtime. The Go race detector on Linux blocks unauthorized memory writes. Tests compile and appear to run, but the patches silently don't apply → functions show 0% coverage.

**Correct alternatives** (in priority order):
1. **httptest.NewServer** — spin up a real HTTP server for integration-like tests
2. **Interface mock** — change production code to accept an interface instead of concrete `*http.Client`
3. **gomonkey** — only as last resort, never with `-race`

**Example** (hermes test rewrite):
```go
// ❌ gomonkey — fails on Linux + race
patches := gomonkey.ApplyMethodReturn(s.client, "Do", resp, nil)

// ✅ httptest.Server — works everywhere
srv := httptest.NewServer(http.HandlerFunc(handler))
s := NewService(srv.URL) // uses real http.Client.Do
```

---

## Lesson 13: Go Cover Doesn't Count Anonymous Inline Functions

**Anti-pattern**: Inline anonymous function inside `log.Printf` argument → Go cover shows 0%.

```go
// ❌ Go cover misses the return statement
log.Printf("first_msg=%q", func() string {
    if len(msgs) > 0 { return msgs[0].Content }
    return ""
}())

// ✅ Extract to variable — cover counts properly
var firstMsg string
if len(msgs) > 0 { firstMsg = msgs[0].Content }
log.Printf("first_msg=%q", firstMsg)
```

---

## Lesson 14: Cognitive Complexity in main() Is Real

**Anti-pattern**: 1200-line `main()` with 40+ inline gin handlers → Sonar cognitive complexity 357.

**Real example 2026-07-17**: `cmd/server/main.go` main() was a monolith instantiating MongoDB, Redis, Qdrant, Vault, SeaweedFS, building every service, wiring dependencies, registering all routes inline. Each inline `func(c *gin.Context)` adds to complexity counter.

**Fix pattern**:
```go
// main() — 7 lines
func main() {
    cfg, logger, mongoClient, deps := initServer()
    defer cleanup(logger, mongoClient, &deps)
    router := buildRouter(cfg)
    registerAllRoutes(router, &deps, logger)
    startServer(router, cfg, logger)
}
```
Extract: `initServer()` → `buildRouter()` → 22 `setupXxxRoutes()` functions → all inline handlers as named functions.

---

## Documentation Protocol

After completing non-trivial work, auto-check and update:

| What happened | Update file |
|--------------|-------------|
| New lesson / anti-pattern discovered | `.agent/skills/engineering-lessons/SKILL.md` |
| Bug fix with root cause | `.agent/memory/CONVENTIONS.md` (已纠正的错误) |
| Engineering decision | `.agent/memory/MEMORY.md` (按日期追加) |
| Red line violation | `.agent/memory/CONVENTIONS.md` (红线) + User `.agent/memory/MEMORY.md` |
| E2E test pattern | `.agent/memory/E2E_TESTING.md` |
| Reusable code pattern | `.agent/memory/REUSABLE_PATTERNS.md` |
| Lessons overview | `.agent/memory/LESSONS_LEARNED.md` (session summary) |

**Rule**: Documentation updates are part of "done". A task is NOT complete until the corresponding memory file is updated.

---

## Decision Flowchart

When encountering a quality issue (failing test, bug, CI failure):

```
Issue found
    │
    ├── Is this a real bug in the system?
    │   YES → Fix the root cause. Do not weaken any validation.
    │
    ├── Is this a false positive in the test/validation?
    │   YES → Delete the test/validation entirely. Do not conditionalize it.
    │
    ├── Is the root cause unclear?
    │   YES → Check logs. Add logging. Re-run CI. DO NOT guess.
    │
    └── After fixing: document. Update CONVENTIONS.md, E2E_TESTING.md, or this skill.
```

---

## Related Resources

- `.agent/memory/CONVENTIONS.md` — coding conventions and red lines
- `.agent/memory/E2E_TESTING.md` — E2E testing iron laws (6 rules)
- `.agent/skills/ui-test-audit/SKILL.md` — 4-axis test quality audit workflow
- `.agent/skills/ci-verification/SKILL.md` — CI polling and log analysis workflow
