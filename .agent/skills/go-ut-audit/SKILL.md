---
name: go-ut-audit
description: "Systematic audit of data-agent Go unit tests for quality: dead/fake tests, flaky patterns, assertion depth, mock abuse, spec compliance, and coverage gaps against RFC/PRD/spec docs. Triggers: 审查Go单元测试, 审查UT, audit go tests, review unit tests, 检查Go测试质量."
agent_created: true
---

# Go Unit Test Audit Skill — data-agent

Systematic audit of Go `*_test.go` files across four dimensions adapted from the UI test audit framework.

## Audit Scope

Run all four checks. Each produces findings categorized by severity:

| Severity | Meaning |
|----------|---------|
| 🔴 Critical | Dead test, `t.Skip()` violation, assertion-only-err-check (fake test) |
| 🟡 Important | Flaky pattern (`time.Sleep`), weak assertion, `TestNew*` boilerplate, `gomonkey` over-mocking |
| 🟢 Suggestion | Naming, style, `bson.M{}` dummy imports, `t.Parallel()` opportunities |

---

## Check 1: Dead Tests, Fake Tests, Unreasonable Compromises

### 1.1 Dead Tests — Zero Assertions / Erroneous Tests

**Command:**
```bash
# Find test files with dangerously low assertion counts
for f in internal/**/**_test.go skills/**/**_test.go; do
  name=$(basename "$f" .go)
  funcs=$(grep -c 'func Test' "$f")
  asserts=$(grep -c 't\.Error\|t\.Fatal\|assert\.\|require\.' "$f")
  ratio=$(echo "scale=2; $asserts / $funcs" | bc)
  if (( $(echo "$ratio < 1.0" | bc -l) )); then
    echo "LOW: $name — $funcs tests, $asserts assertions, ratio=$ratio"
  fi
done
```

Red flags:
- **Assertion-only-err-check**: `func TestXxx_Success(t *testing.T) { ... if err != nil { t.Fatal(...) } }` — only checks `err == nil`, never verifies actual behavior
- **`t.Skip()` in test body**: Skipped test means uncovered logic
  ```bash
  grep -rn 't\.Skip\|T\.Skip' internal/ skills/ | grep '_test\.go'
  ```
- **`t.Parallel()` missing**: All tests run serially, can mask race conditions
- **`assert.True(t, true)`**: Tautology (Go equivalent of `assert(true === true)`)

### 1.2 Fake Tests — Wrong Assertions / Mock Abuse

Patterns to flag:
- **`gomonkey.ApplyMethodReturn` for UpdateOne/InsertOne on Success path**: The mock returns `nil` error but the test never verifies **what** the update was. Replace with `ApplyMethodFunc`:
  ```bash
  grep -rn 'ApplyMethodReturn.*UpdateOne.*nil' internal/**/**_test.go
  ```
- **`gomonkey.ApplyMethodReturn` for handler service methods**: Handler tests that mock the entire service layer never verify parameter passing. Replace with `ApplyMethodFunc` to validate `req` fields:
  ```bash
  grep -rn 'ApplyMethodReturn.*svc.*Login' internal/api/handler/*_test.go
  ```
- **Name-behavior mismatch**: Test title says "token counting" but never checks token counts

### 1.3 Unreasonable Compromises

- **`time.Sleep(N)`** where synchronization primitives (channel, `sync.WaitGroup`) should be used:
  ```bash
  grep -rn 'time\.Sleep' internal/ skills/ | grep '_test\.go'
  ```
  **Exception**: `time.Sleep` is acceptable when testing timeout behavior (e.g., JWT expiry) — the test MUST fail if the sleep ends before the expected condition.
- **`bson.M{}` dummy imports**: `var _ = bson.M{}` at end of test files — indicates gomonkey workaround for unused imports
  ```bash
  grep -rn 'var _ = bson\.M{}' internal/ skills/ | grep '_test\.go'
  ```
- **`TestNew*` factory tests (43+ instances)**: Only checking `s != nil`. Improve by verifying field assignment:
  ```bash
  grep -rn 'func TestNew' internal/ skills/ | grep '_test\.go'
  ```

### 1.4 Transient State & Flaky Tests

- **Race conditions**: Run with `-race` flag; tests with `go func()` that don't verify completion
  ```bash
  grep -rn 'go func' internal/ skills/ | grep '_test\.go'
  ```
- **Cross-test state dependency**: Tests that depend on data from prior tests (e.g., relying on `beforeAll` to register a user)

---

## Check 2: Test Design Document Alignment

### Reference Documents

| Document | Path |
|----------|------|
| SPEC-045 (Go UT Coverage) | `.agent/specs/spec-045-go-service-ut.md` |
| SPEC-014 (Testing) | `.agent/specs/spec-014-testing.md` |
| CI UT Workflow | `.github/workflows/ut-workflow.yml` |
| Makefile test target | `Makefile` (search for `go test`) |

### Process

1. Read SPEC-045 and extract coverage targets (98% minimum, L1/L2/L3 tier requirements)
2. Run `go test -coverprofile=coverage.out ./internal/... ./skills/...` and analyze per-package coverage
3. Identify packages below 98% threshold
4. Categorize gaps per SPEC-045 tier:
   - **L1 (pure logic)**: Must have 100% coverage
   - **L2 (interface-dependent)**: Must have 100% coverage  
   - **L3 (integration-dependent)**: Must have 98% coverage

### SPEC-045 Test Tiers

| Tier | Characteristic | Target | Example Packages |
|:---:|------|:---:|------|
| L1 | No external deps, pure functions | **100%** | `logic/sql`, `logic/openapi`, `logic/report`, `logic/stats`, `config` |
| L2 | Depends on interfaces (mockable) | **100%** | `queue/`, service interfaces, handler business logic |
| L3 | Concrete deps (MongoDB/Redis/HTTP) | **98%** | `service/*`, `api/handler/*` |

---

## Check 3: Test Logic Issues

### Common Service Test Issues

1. **`UpdateOne` mock returns nil but test doesn't verify the update content**: The test passes even if the service writes wrong status/fields
2. **`InsertOne` mock returns nil but test doesn't verify inserted document**: Document field errors pass silently
3. **Date validation silently swallowed**: `time.Parse` errors are silently ignored in `buildDateFilter`-style functions — tests should verify error paths
4. **Error type assertion missing**: Tests check `err != nil` but never verify the error type/message

### Common Handler Test Issues

1. **`gomonkey.ApplyMethodReturn` hides parameter passing bugs**: If handler passes wrong fields to service but mock returns fixed response, test passes
2. **HTTP status code not verified for error paths**: Only checks `err != nil` but not status code (400 vs 401 vs 500)

### Verification Commands

```bash
# Find tests that only check err == nil without checking actual data
grep -A5 'func Test.*_Success' internal/**/**_test.go | grep 'if err != nil { t.Fatal' -A2 | grep -v '\.Status\|\.Title\|\.ID\|\.Name'

# Find gomonkey ApplyMethodReturn used on service methods in handler tests
grep -rn 'ApplyMethodReturn.*svc\.' internal/api/handler/*_test.go
```

---

## Check 4: Coverage vs RFC/PRD/Spec

### Reference Documents

| Document | Path |
|----------|------|
| PRD | `docs/PRD-企业数据分析Agent-MVP.md` |
| RFC | `docs/RFC-企业数据分析Agent-技术方案.md` |
| SPEC Index | `.agent/specs/INDEX.md` |
| SPEC-014 (Testing) | `.agent/specs/spec-014-testing.md` |
| SPEC-045 (UT Coverage) | `.agent/specs/spec-045-go-service-ut.md` |

### Process

1. Map each SPEC-0XX to its Go test file(s)
2. For each SPEC, verify:
   - All defined service/handler/logic tests implemented
   - No unintended gaps
   - Coverage meets tier requirements (L1=100%, L2=100%, L3=98%)
3. Run `go test -race -coverprofile=coverage.out ./internal/... ./skills/...` and verify gate

### Coverage Matrix Template

| SPEC | Module | Test File | Tier | Coverage Target | Current | Issues |
|:---:|--------|-----------|:---:|:---:|:---:|--------|
| SPEC-003 | Auth | `handler/auth_test.go`, `service/auth/auth_test.go` | L3 | 98% | — | — |
| SPEC-004 | Agent | `service/agent/agent_test.go` | L3 | 98% | — | — |
| SPEC-007 | Analysis Logic | `logic/stats/stats_test.go`, `logic/sql/validator_test.go` | L1 | 100% | — | `openapi/parser` missing |

---

## Output Template

```markdown
# Go Unit Test Audit Report

## Check 1: Dead/Fake/Flaky Tests

### Critical Findings
- [File:line] [Pattern] [Severity] [Description + Fix]

### Important Findings
- [File:line] [Pattern] [Severity] [Description + Fix]

### Pattern Summary
| Pattern | Count | Risk |
|---------|:---:|:---:|
| assertion-only-err-check (fake tests) | N | 🔴 |
| t.Skip() violations | N | 🔴 |
| TestNew* boilerplate | N | 🟡 |
| time.Sleep() | N | 🟡 |
| gomonkey ApplyMethodReturn on handlers | N | 🟡 |

## Check 2: SPEC-045 Alignment
| Tier | Packages Below Target | Gap |

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

## Core Principles

These are the non-negotiable rules to audit against:

1. **No `t.Skip()`** — Write proper test setup; if truly impossible (e.g., WebSocket Hijacker), document and track
2. **No assertion-only-err-check on Success path** — Every Success test must verify at least 2 behavioral assertions beyond `err == nil`
3. **No pure-return-value mocking without parameter verification** — Use `gomonkey.ApplyMethodFunc` instead of `ApplyMethodReturn` to verify service receives correct parameters from handler
4. **Strict error class verification** — Error tests must verify BOTH `err != nil` AND the error message/type
5. **Timeout = failure** — if `time.Sleep` is used, the test MUST fail when the sleep ends without condition met; never use sleep as a soft skip
6. **Deterministic only** — Only assert states that are guaranteed capturable
7. **L1 packages must have 100% coverage** — Pure logic packages (`logic/sql`, `logic/openapi`, `logic/report`, `logic/stats`, `config`) must have zero uncovered lines

## Verification

After implementing fixes:
- [ ] All `t.Skip()` removed or documented
- [ ] All Success tests have ≥2 behavioral assertions
- [ ] Zero `gomonkey.ApplyMethodReturn` on handler service methods (use `ApplyMethodFunc`)
- [ ] L1 coverage = 100%
- [ ] Overall coverage ≥ 98%
- [ ] `go test -race` passes with no data races
- [ ] CI passes (ut-workflow → sonar-check → ui-tests)
