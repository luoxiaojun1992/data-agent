#!/usr/bin/env python3
"""
ScanCode license gate — rejects copyleft / viral licenses (SPEC-081 compliance).

Parses a ScanCode `--json-pp` output and fails if any detected license matches
the deny list. The project's license red line: no GPL / AGPL / LGPL / SSPL /
BUSL or other source-available / viral licenses (see MEMORY.md).

Usage: python3 scancode-gate.py <scancode.json>
"""

import json
import sys

# License identifiers (ScanCode `key` / SPDX `spdx_license_key`) prefixes that
# are NOT allowed. Prefix match (case-insensitive) so variants like
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
        for lic in file_info.get("licenses", []):
            key = lic.get("spdx_license_key") or lic.get("key") or ""
            if not key:
                continue
            low = key.lower()
            if any(low.startswith(p) for p in DENY_PREFIXES):
                score = lic.get("score", 0)
                name = lic.get("name") or lic.get("short_name") or key
                entry = (fpath, key, name, score)
                if score >= FAIL_SCORE:
                    violations.append(entry)
                else:
                    warnings.append(entry)

    if warnings:
        print(f"=== ScanCode license gate: {len(warnings)} low-confidence copyleft hit(s) — manual review recommended ===")
        for fpath, key, name, score in warnings:
            print(f"  [WARN] {fpath}: {key} ({name}) score={score}")

    if violations:
        print("=== ScanCode license gate FAILED ===")
        print(f"发现 {len(violations)} 处传染性 license（红线：禁 GPL/AGPL/LGPL/SSPL/BUSL 等）:")
        for fpath, key, name, score in violations:
            print(f"  [FAIL] {fpath}: {key} ({name}) score={score}")
        sys.exit(1)

    print("=== ScanCode license gate PASSED ===")
    print("未发现传染性 license。")
    sys.exit(0)


if __name__ == "__main__":
    main()
