---
name: engineering-lessons
description: >
  Hard-earned engineering lessons from DataAgent project. Trigger when making quality-related
  decisions, debugging CI failures, writing tests, handling errors, reviewing PRs, or whenever
  you're about to repeat a past mistake. Keywords: anti-pattern, lesson, mistake, quality,
  testing, error handling, review, PR, CI failure, 教训, 经验, 反模式, 质量.
agent_created: true
---

# Engineering Lessons Learned

This skill loads the non-negotiable lessons learned from real bugs and anti-patterns discovered in this project. Do NOT repeat these mistakes.

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
