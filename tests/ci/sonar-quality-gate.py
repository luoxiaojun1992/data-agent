#!/usr/bin/env python3
"""
SonarQube Quality Gate — checks JSON report and fails on blockers/criticals/vulns.

Usage: python3 sonar-quality-gate.py scanner-report/sonar-issues.json
"""

import json
import sys


def main():
    if len(sys.argv) < 2:
        print("Usage: sonar-quality-gate.py <sonar-issues.json>")
        sys.exit(1)

    report_path = sys.argv[1]
    with open(report_path) as f:
        data = json.load(f)

    summary = data.get("summaryBySeverity", {})
    open_total = data.get("openIssues", 0)

    blocker = summary.get("BLOCKER", {})
    blocker_total = sum(blocker.values())
    critical = summary.get("CRITICAL", {})
    critical_total = sum(critical.values())

    vuln_total = sum(summary.get(s, {}).get("VULNERABILITY", 0) for s in summary)
    bug_total = sum(summary.get(s, {}).get("BUG", 0) for s in summary)
    hotspot_total = sum(summary.get(s, {}).get("SECURITY_HOTSPOT", 0) for s in summary)
    smell_total = sum(summary.get(s, {}).get("CODE_SMELL", 0) for s in summary)

    print(f"Total open issues: {open_total}")
    print(f"BLOCKER: {blocker_total}, CRITICAL: {critical_total}")
    print(f"VULNERABILITY: {vuln_total}, BUG: {bug_total}, HOTSPOT: {hotspot_total}, SMELL: {smell_total}")

    failed = False

    # Count only BUG/VULNERABILITY/SECURITY_HOTSPOT — CODE_SMELL is informational
    blocker_bugs = blocker.get("BUG", 0) + blocker.get("VULNERABILITY", 0) + blocker.get("SECURITY_HOTSPOT", 0)
    critical_bugs = critical.get("BUG", 0) + critical.get("VULNERABILITY", 0) + critical.get("SECURITY_HOTSPOT", 0)

    if blocker_bugs > 0 or critical_bugs > 0:
        print(f"FAIL: {blocker_bugs} blocker + {critical_bugs} critical BUG/VULN/HOTSPOT")
        for issue in data.get("openIssuesList", []):
            if issue.get("severity") in ("BLOCKER", "CRITICAL") and issue.get("type") != "CODE_SMELL":
                print(f"  {issue['severity']} {issue['type']}: {issue['message'][:120]}")
        failed = True

    if vuln_total > 0:
        print(f"FAIL: {vuln_total} vulnerability(s)")
        for issue in data.get("openIssuesList", []):
            if issue.get("type") == "VULNERABILITY":
                print(f"  [{issue['severity']}] {issue['message'][:120]}")
        failed = True

    # MAJOR+ bugs
    major_bugs = (
        summary.get("MAJOR", {}).get("BUG", 0)
        + summary.get("BLOCKER", {}).get("BUG", 0)
        + summary.get("CRITICAL", {}).get("BUG", 0)
    )
    if major_bugs > 0:
        print(f"FAIL: {major_bugs} MAJOR+ bug(s)")
        failed = True

    if failed:
        sys.exit(1)

    print("PASS: Quality gate passed")


if __name__ == "__main__":
    main()
