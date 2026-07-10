#!/bin/bash
# DataAgent UI E2E CI runner
# Keeps running after test failure so coverage/allure artifacts are still generated.
set +e

echo "[test-ui-ci] Running Playwright tests..."
npx playwright test -c playwright.config.ts
test_exit=$?
echo "[test-ui-ci] Tests completed with exit code: $test_exit"

echo "[test-ui-ci] Running coverage check..."
node scripts/check-ui-coverage.mjs
coverage_exit=$?
echo "[test-ui-ci] Coverage check completed with exit code: $coverage_exit"

# Exit with first non-zero (test failure takes priority)
if [ "$test_exit" -ne 0 ]; then
  echo "[test-ui-ci] Exiting with test exit code: $test_exit"
  exit $test_exit
fi
if [ "$coverage_exit" -ne 0 ]; then
  echo "[test-ui-ci] Exiting with coverage exit code: $coverage_exit"
  exit $coverage_exit
fi
echo "[test-ui-ci] All checks passed!"
exit 0
