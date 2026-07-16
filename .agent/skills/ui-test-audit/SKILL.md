---
name: ui-test-audit
description: "Systematically audit data-agent UI E2E Playwright tests for quality: dead/fake tests, flaky patterns, spec compliance, and coverage gaps against RFC/PRD/spec docs. Triggers: 审查UI测试, audit ui tests, 检查测试质量, review e2e tests."
agent_created: true
---

# UI Test Audit Skill — data-agent

Systematic audit of Playwright UI E2E tests across four dimensions.

## Audit Scope

Run all four checks. Each produces findings categorized by severity:

| Severity | Meaning |
|----------|---------|
| 🔴 Critical | Dead test, fake test, `page.route()` violation, `test.skip()` violation |
| 🟡 Important | Flaky pattern, weak assertion, transient state check, missing test case |
| 🟢 Suggestion | Naming, style, timing, optimization |

---

## Check 1: Dead Tests, Fake Tests, Unreasonable Compromises

### 1.1 Dead Tests — Zero Assertions

```bash
# Find tests with no assertions
grep -rn 'test(' tests/ui/*.spec.ts | while read line; do
    # Check if test body contains any 'expect('
done
```

Red flags:
- `expect(true).toBe(true)` — tautology
- No `expect()` calls in test body
- Empty for-loop (never executes tests)

### 1.2 Fake Tests — Wrong Assertions

Patterns to flag:
- **`page.route()`**: Bypasses Handler→Service→Repository. Must use mockllm seed + real SSE.
  ```bash
  grep -rn 'page.route' tests/ui/*.spec.ts
  ```
- **`test.skip()`**: Data-driven skip. Must pre-create data in `beforeAll`.
  ```bash
  grep -rn 'test.skip()' tests/ui/*.spec.ts
  ```
- **`.toBeTruthy()` on Playwright Locators**: Locator objects are always truthy. Must use `.toBeAttached()` or `.toHaveCount(N)`.
  ```bash
  grep -rn '\.toBeTruthy()' tests/ui/*.spec.ts
  ```
- **`.catch(() => {})` on assertions**: Swallows assertion failures. Only acceptable in `afterAll` cleanup.
  ```bash
  grep -rn '\.not.toBeVisible.*\.catch' tests/ui/*.spec.ts
  grep -rn '\.catch(() => {})' tests/ui/*.spec.ts | grep -v 'afterAll\|catch.*delete\|catch.*clearMocks'
  ```
- **Wrong testid**: Assertion on testid that doesn't match the test's described behavior.
- **Name-behavior mismatch**: Test title says "token counting" but never checks token counts.
- **API test masquerading as UI**: Uses `request` fixture only, never `page` browser interaction.

### 1.3 Unreasonable Compromises

- **`waitForTimeout(N)` where `waitForSelector`/`waitForResponse` should be used**: Fixed waits that could be replaced by condition-based waits
  ```bash
  grep -rn 'waitForTimeout' tests/ui/*.spec.ts | wc -l
  ```
- **Coordinate-based clicking**: `{ position: { x: N, y: M } }` — fragile to layout changes
- **`button:has-text()` instead of `data-testid`**
  ```bash
  grep -rn 'has-text' tests/ui/*.spec.ts
  ```
- **Dialog listener after click**: Race condition
  ```bash
  grep -rn 'page.once.*dialog' tests/ui/*.spec.ts -A1 | grep -B1 'click()'
  ```

### 1.4 Transient State & Flaky Tests

- **Loading indicator assertions**: Loading states appear briefly — hard to capture reliably
- **SSE stream timing**: AI message assertions depend on mockllm chunk delay
- **Real-time badge / animation checks**: Pulse/presence depends on backend state
- **`.not.toBeVisible()` on async content**: Element may appear while assertion is evaluating
- **Index-based selectors**: `[data-testid="chat-msg-ai-1"]` — fragile if message ordering changes
- **`test.describe.serial`**: Tests share mutable state; reordering breaks tests
  ```bash
  grep -rn 'describe.serial' tests/ui/*.spec.ts
  ```
- **Cross-test state dependency**: Comments like "from previous test" or "leftover from"
  ```bash
  grep -rn 'previous test\|from prior\|leftover' tests/ui/*.spec.ts
  ```

---

## Check 2: Test Design Document Alignment

### Reference Documents

| Document | Path |
|----------|------|
| UI Test Case Design | `docs/DataAgent-UI测试用例文档.md` |
| E2E Testing Standards | `.agent/memory/E2E_TESTING.md` |

### Process

1. Read the test design document and extract all `UI-XXX` case IDs with their expected behavior
2. Search for each `UI-XXX` in test files:
   ```bash
   grep -rn 'UI-[0-9]\{3\}' tests/ui/*.spec.ts | sort
   ```
3. Compare: designer's expected list vs. implemented list
4. Categorize gaps:
   - **Missing**: In design doc, no implementation
   - **Reasonable skip**: Manual test (e.g., Feishu client, drag-and-drop)
   - **Implementation mismatch**: Test exists but tests different behavior than spec says

### Known Reasonable Skips (data-agent specific)

| Case IDs | Reason |
|----------|--------|
| UI-163~166 | Feishu client-side features → manual |
| UI-173 | Drag-and-drop upload → Playwright limitation → manual |
| Backend-only (F-03-1, F-09, F-14) | Cannot test via UI → needs integration tests |

---

## Check 3: Test Logic & System Functionality

For each failing pattern from Check 1, cross-reference with the frontend source:

1. **testid exists in frontend code?**
   ```bash
   grep -rn "data-testid" frontend/app/ | grep "<expected-testid>"
   ```
2. **Page route works?** Verify URL pattern matches frontend routing
3. **Data flow correct?** API → handler → service → repository → response
4. **Conditional assertions masking real bugs?**
   - If element may not exist due to backend state, the test should either:
     - Pre-create data to guarantee element existence, OR
     - Not test that element at all
   - **Never**: `if (visible) { test } else { pass silently }`

### Common Logic Issues

- Asserting wrong field's error element (`pwd-old-error` for new-password validation)
- Asserting nav-admin hidden when page structure always shows it
- Testing page-header visibility as proxy for task creation success
- Filter tests that click filter button but never verify results changed

---

## Check 4: Coverage vs RFC/PRD/Spec

### Reference Documents

| Document | Path |
|----------|------|
| PRD | `docs/PRD-企业数据分析Agent-MVP.md` |
| RFC | `docs/RFC-企业数据分析Agent-技术方案.md` |
| Architecture | `docs/ARCHITECTURE.zh-CN.md` |
| Specs | `.agent/specs/spec-017-ui-auth.md` through `spec-042-ui-e2e-scenarios.md` |
| Spec Index | `.agent/specs/INDEX.md` |

### Process

1. Map each SPEC-0XX to its test file(s)
2. For each SPEC, verify:
   - All defined UI test cases implemented
   - No unintended gaps
   - Feature coverage matches PRD functional requirements
3. Check the coverage matrix from the test design document §30

### Coverage Matrix Template

| SPEC | Module | Test File | Cases Defined | Cases Implemented | Coverage | Issues |
|:---:|--------|-----------|:---:|:---:|:---:|--------|
| SPEC-017 | AUTH | auth.spec.ts | 10 | 10 | 100% | — |

---

## Output Template

```markdown
# UI Test Audit Report

## Check 1: Dead/Fake/Flaky Tests

### Critical Findings
- [File:line] [Pattern] [Severity] [Description + Fix]

### Important Findings
- [File:line] [Pattern] [Severity] [Description + Fix]

### Pattern Summary
| Pattern | Count | Risk |
|---------|:---:|:---:|
| page.route() violations | N | 🔴 |
| test.skip() violations | N | 🔴 |
| .toBeTruthy() on Locator | N | 🔴 |
| .catch(()⇒{}) on assertions | N | 🔴 |
| waitForTimeout() | N | 🟡 |
| Conditional assertions | N | 🟡 |
| describe.serial | N | 🟡 |

## Check 2: Design Document Alignment

### Missing Test Cases
| Case ID | Module | Priority | Notes |

### Implementation Mismatches
| Case ID | Spec says | Code does | Severity |

## Check 3: Test Logic Issues
| File | Test | Issue | Severity |

## Check 4: SPEC Coverage
| SPEC | Module | Coverage | Gap |

## Fix Priority
| Priority | Issue | File | Effort |
|:---:|-------|------|:---:|
| P0 | ... | ... | ... |
| P1 | ... | ... | ... |
```

---

## Core Principles (from E2E_TESTING.md)

These are the non-negotiable rules to audit against:

1. **No `test.skip()`** — Pre-create data via API in `beforeAll`
2. **No `page.route()`** — Real backend + mockllm only
3. **No workarounds** — No debug-only endpoints or shortcuts
4. **Strict assertions** — Sensitive data checks must verify BOTH `toContain(masked)` AND `not.toContain(original)`
5. **Timeout = failure** — Never conditionally pass; if an element doesn't appear in time, fail with clear error
6. **Deterministic only** — Only assert states that are guaranteed capturable; don't test async-dependent transient states
7. **Conditional skip = meaningless** — If a test passes regardless of element state, it tests nothing

## Verification

After implementing fixes:
- [ ] All `page.route()` removed
- [ ] All `test.skip()` removed  
- [ ] Zero `.catch(() => {})` on assertions
- [ ] Zero `if (visible) { test }` patterns
- [ ] Design doc gaps documented
- [ ] CI passes (sonar-check → ui-tests)
