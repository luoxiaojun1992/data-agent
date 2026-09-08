#!/usr/bin/env python3
"""
ScanCode license gate — rejects copyleft / viral licenses (SPEC-081 compliance).

Parses a ScanCode `--json-pp` output and fails if any detected license matches
the deny list. The project's license red line: no GPL / AGPL / LGPL / SSPL /
BUSL or other source-available / viral licenses (see MEMORY.md).

Supports both ScanCode output schemas:
  * New (v32+ / v33):  files[].license_detections[].matches[] with
    `license_expression` / `license_expression_spdx` / `score`.
  * Old (< v32):        files[].licenses[] with `key` / `spdx_license_key` / `score`.

Usage: python3 scancode-gate.py <scancode.json>
"""

import json
import sys

# License identifiers (ScanCode lowercase key or SPDX id) prefixes that are
# NOT allowed. Prefix match (case-insensitive) so variants like
# `GPL-3.0-only`, `GPL-3.0-or-later`, `gpl-3.0` are all covered.
DENY_PREFIXES = (
    "gpl-",          # GNU General Public License (all versions)
    "agpl-",         # GNU Affero General Public License
    "lgpl-",         # GNU Lesser General Public License
    "sspl-",         # Server Side Public License (MongoDB-style)
    "busl-",         # Business Source License
    "cc-by-nc-",     # Creative Commons NonCommercial (unusable in commercial product)
    "cc-by-sa-",     # Creative Commons ShareAlike (copyleft)
    "cc-by-nc-sa-",  # NonCommercial + ShareAlike
    "commons-clause",  # Commons Clause (source-available restriction)
)

# Low-confidence detections below this score are reported as warnings only,
# to avoid failing CI on ScanCode false positives while still surfacing them.
FAIL_SCORE = 60


def is_denied(key):
    low = key.lower()
    return any(low.startswith(p) for p in DENY_PREFIXES)


def iter_detections(file_info):
    """Yield (license_key, score) pairs from a file entry, across schema versions."""
    # New schema (v32+): license_detections[].matches[]
    for det in file_info.get("license_detections", []) or []:
        for m in (det.get("matches", []) or []):
            key = m.get("license_expression") or m.get("license_expression_spdx") or ""
            score = m.get("score", 0) or 0
            if key:
                yield key, score

    # Old schema (< v32): licenses[]
    for lic in file_info.get("licenses", []) or []:
        key = lic.get("key") or lic.get("spdx_license_key") or ""
        score = lic.get("score", 0) or 0
        if key:
            yield key, score


def main():
    if len(sys.argv) != 2:
        print("Usage: scancode-gate.py <scancode.json>", file=sys.stderr)
        sys.exit(2)

    path = sys.argv[1]
    with open(path, encoding="utf-8") as f:
        data = json.load(f)

    violations = []   # score >= FAIL_SCORE → hard fail
    warnings = []     # score < FAIL_SCORE → warn only

    for file_info in data.get("files", []):
        fpath = file_info.get("path", "?")
        for key, score in iter_detections(file_info):
            if not is_denied(key):
                continue
            entry = (fpath, key, score)
            if score >= FAIL_SCORE:
                violations.append(entry)
            else:
                warnings.append(entry)

    if warnings:
        print("=== ScanCode license gate: %d low-confidence copyleft hit(s) — manual review recommended ===" % len(warnings))
        for fpath, key, score in warnings:
            print("  [WARN] %s: %s score=%s" % (fpath, key, score))

    if violations:
        print("=== ScanCode license gate FAILED ===")
        print("发现 %d 处传染性 license（红线：禁 GPL/AGPL/LGPL/SSPL/BUSL 等）:" % len(violations))
        for fpath, key, score in violations:
            print("  [FAIL] %s: %s score=%s" % (fpath, key, score))
        sys.exit(1)

    print("=== ScanCode license gate PASSED ===")
    print("未发现传染性 license。")
    sys.exit(0)


if __name__ == "__main__":
    main()
