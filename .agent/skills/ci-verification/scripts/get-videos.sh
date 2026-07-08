#!/bin/bash
# Download UI test videos for tests modified on the current branch.
# Usage: ./get-videos.sh <run-id> [--dir OUTPUT_DIR]
#   run-id     The GitHub Actions workflow run ID
#   --dir      Output directory (default: ./ui-test-videos-<run-id>)
#
# Workflow:
#   1. Downloads the allure-results artifact from the given CI run
#   2. Detects which UI tests are new/changed on this branch (git diff vs main)
#   3. Extracts only the relevant video.webm files
#   4. Names them UI-XXX-video.webm for easy identification
#
# Prerequisites:
#   - gh CLI installed and authenticated (GITHUB_TOKEN or gh auth login)
#   - Artifact name must be "allure-results"
#   - Working directory: data-agent repo root

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: get-videos.sh <run-id> [--dir OUTPUT_DIR]"
  exit 1
fi

RUN_ID="$1"
shift

OUTPUT_DIR="./ui-test-videos-${RUN_ID}"

while [ $# -gt 0 ]; do
  case "$1" in
    --dir) OUTPUT_DIR="$2"; shift 2 ;;
    *) echo "Unknown flag: $1"; exit 1 ;;
  esac
done

REPO="${GITHUB_REPO:-luoxiaojun1992/data-agent}"
ARTIFACT_NAME="allure-results"
TEMP_DIR="/tmp/ci-videos-$$"

echo "🎬 Downloading UI test videos for run #$RUN_ID"
echo "   Repo: $REPO"
echo "   Output: $OUTPUT_DIR"

# ── Step 0: Detect branch-related test IDs ──
echo ""
echo "📋 Detecting branch-specific UI tests..."

# Get current branch name
BRANCH=$(git rev-parse --abbrev-ref HEAD)

# Find UI-XXX test IDs added or modified on this branch vs main.
# Auto-discovers all *.spec.ts files in tests/ui/ (project-specific structure).
SPEC_FILES=$(find tests/ui -name "*.spec.ts" -type f 2>/dev/null | sort)

if [ -z "$SPEC_FILES" ]; then
  echo "   ⚠️  No spec files found in tests/ui/"
  SPEC_FILES=""
fi

# Get the set of test IDs on main (base) — aggregate across all spec files
MAIN_TESTS=""
for f in $SPEC_FILES; do
  T=$(git show "origin/main:$f" 2>/dev/null | grep -oE "test\('\[UI-[0-9]+\]" | grep -oE 'UI-[0-9]+' | sort -u || echo "")
  MAIN_TESTS=$(echo -e "${MAIN_TESTS}\n${T}" | sort -u | sed '/^$/d')
done
if [ -z "$MAIN_TESTS" ]; then
  echo "   ⚠️  Could not read spec files from origin/main, using all tests"
fi

# Get the set of test IDs on current branch — aggregate across all spec files
BRANCH_TESTS=""
for f in $SPEC_FILES; do
  T=$(grep -oE "test\('\[UI-[0-9]+\]" "$f" | grep -oE 'UI-[0-9]+' | sort -u)
  BRANCH_TESTS=$(echo -e "${BRANCH_TESTS}\n${T}" | sort -u | sed '/^$/d')
done

# Find new tests (not in main)
if [ -n "$MAIN_TESTS" ]; then
  NEW_TESTS=$(comm -13 <(echo "$MAIN_TESTS") <(echo "$BRANCH_TESTS"))
else
  NEW_TESTS="$BRANCH_TESTS"
fi

# Also find tests whose test block content changed (git diff, all spec files)
CHANGED_TESTS=""
for f in $SPEC_FILES; do
  T=$(git diff "origin/main" -- "$f" 2>/dev/null | grep -oE "test\('\[UI-[0-9]+\]" | grep -oE 'UI-[0-9]+' | sort -u || echo "")
  CHANGED_TESTS=$(echo -e "${CHANGED_TESTS}\n${T}" | sort -u | sed '/^$/d')
done

# Merge: new tests + changed tests
TARGET_TESTS=$(echo -e "${NEW_TESTS}\n${CHANGED_TESTS}" | sort -u | sed '/^$/d')

if [ -z "$TARGET_TESTS" ]; then
  echo "   ⚠️  No branch-specific tests detected. Downloading ALL videos."
  TARGET_TESTS="$BRANCH_TESTS"
fi

echo "   Branch: $BRANCH"
echo "   Target tests: $(echo "$TARGET_TESTS" | tr '\n' ' ')"

# ── Step 1: Download the allure-results artifact ──
echo ""
echo "📥 Downloading artifact '$ARTIFACT_NAME' from run #$RUN_ID..."

mkdir -p "$TEMP_DIR"

# Try gh CLI first
DOWNLOAD_OK=false
if gh run download "$RUN_ID" -n "$ARTIFACT_NAME" -D "$TEMP_DIR" --repo "$REPO" 2>/dev/null; then
  DOWNLOAD_OK=true
else
  # Fallback: curl via GitHub API
  ARTIFACT_ID=$(gh api "repos/${REPO}/actions/runs/${RUN_ID}/artifacts" --jq ".artifacts[] | select(.name == \"${ARTIFACT_NAME}\") | .id" 2>/dev/null)
  if [ -n "$ARTIFACT_ID" ]; then
    gh api "repos/${REPO}/actions/artifacts/${ARTIFACT_ID}/zip" > "$TEMP_DIR/${ARTIFACT_NAME}.zip" 2>/dev/null && DOWNLOAD_OK=true
  fi
fi

if [ "$DOWNLOAD_OK" != "true" ]; then
  echo "ERROR: Failed to download artifact" >&2
  rm -rf "$TEMP_DIR"
  exit 1
fi

echo "   ✓ Downloaded"

# ── Step 2: Extract the artifact ──
echo ""
echo "📦 Extracting artifact..."

if [ -f "$TEMP_DIR/${ARTIFACT_NAME}.zip" ]; then
  # Downloaded as zip via gh api
  unzip -q -o "$TEMP_DIR/${ARTIFACT_NAME}.zip" -d "$TEMP_DIR" 2>/dev/null || true
else
  # gh run download may have extracted it already, or placed a zip inside
  ZIP_FILE=$(find "$TEMP_DIR" -name "*.zip" -type f 2>/dev/null | head -1)
  if [ -n "$ZIP_FILE" ]; then
    unzip -q -o "$ZIP_FILE" -d "$TEMP_DIR" 2>/dev/null || true
  fi
fi

echo "   ✓ Extracted"

# ── Step 3: Find and copy matching videos ──
echo ""
echo "🔍 Finding matching videos..."

mkdir -p "$OUTPUT_DIR"
VIDEO_COUNT=0

for TEST_ID in $TARGET_TESTS; do
  # Video files follow Playwright naming: spec-file--UI-XXX-...--chromium/video.webm
  # Search case-insensitively for the test ID in the directory path
  VIDEO_FILE=$(find "$TEMP_DIR" -path "*${TEST_ID}*" -name "video.webm" -type f 2>/dev/null | head -1)

  if [ -n "$VIDEO_FILE" ]; then
    OUT_NAME="${TEST_ID}-video.webm"
    cp "$VIDEO_FILE" "${OUTPUT_DIR}/${OUT_NAME}"
    size=$(ls -lh "${OUTPUT_DIR}/${OUT_NAME}" | awk '{print $5}')
    echo "   ✅ ${OUT_NAME} (${size})"
    VIDEO_COUNT=$((VIDEO_COUNT + 1))
  else
    echo "   ⚠️  ${TEST_ID}: video not found in artifact"
  fi
done

# ── Cleanup ──
rm -rf "$TEMP_DIR"

echo ""
echo "📊 Downloaded $VIDEO_COUNT video(s) to $OUTPUT_DIR/"
