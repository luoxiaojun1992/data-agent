import fs from 'node:fs';
import path from 'node:path';

const resultsPath = process.argv[2] || 'artifacts/playwright-report/results.json';
const casesPathArg = process.argv[3] || 'coverage/cases.json';
const threshold = Number(process.argv[4] || process.env.UI_COVERAGE_THRESHOLD || 100);

const writeSummary = (summary) => {
  const dir = path.resolve('artifacts');
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, 'ui-coverage-summary.json'), JSON.stringify(summary, null, 2));
};

const fail = (message) => {
  const summary = { totalCases: 0, coveredCases: 0, uncoveredCases: [], coverage: 0, threshold, passed: false, error: message };
  writeSummary(summary);
  console.error(`[ui-coverage] FAILED: ${message}`);
  process.exit(1);
};

// Read results
let results;
try { results = JSON.parse(fs.readFileSync(resultsPath, 'utf8')); }
catch (e) { fail(`cannot read results file ${resultsPath}: ${e.message}`); }

// Read cases
let cases;
try { cases = JSON.parse(fs.readFileSync(casesPathArg, 'utf8')); }
catch (e) { fail(`cannot read cases file ${casesPathArg}: ${e.message}`); }

const requiredIds = cases.requiredCaseIds;
if (!Array.isArray(requiredIds) || requiredIds.length === 0) {
  fail('requiredCaseIds is missing or empty');
}

const covered = new Set();
const ranStatuses = new Set(['passed', 'failed', 'timedOut', 'interrupted']);

const walkSuites = (suite) => {
  for (const spec of suite.specs || []) {
    const title = String(spec.title || '');
    const match = title.match(/\[(UI-\d+)\]/);
    if (!match) continue;
    const testRan = (spec.tests || []).some(t =>
      (t.results || []).some(r => ranStatuses.has(r.status))
    );
    if (testRan) covered.add(match[1]);
  }
  for (const child of suite.suites || []) walkSuites(child);
};

for (const suite of results.suites || []) walkSuites(suite);

const total = requiredIds.length;
const coveredCount = requiredIds.filter(id => covered.has(id)).length;
const coverage = Number(((coveredCount / total) * 100).toFixed(2));

const summary = {
  totalCases: total,
  coveredCases: coveredCount,
  uncoveredCases: requiredIds.filter(id => !covered.has(id)),
  coverage,
  threshold,
  passed: coverage >= threshold,
};

writeSummary(summary);
console.log(`[ui-coverage] ${coveredCount}/${total} => ${coverage}% (threshold: ${threshold}%)`);

if (!summary.passed) {
  console.error(`[ui-coverage] FAILED: coverage below threshold; uncovered: ${summary.uncoveredCases.join(', ')}`);
  process.exit(1);
}
