#!/bin/bash
set -euo pipefail

echo "[entrypoint] Waiting for SonarQube to be fully ready..."
until curl -sf "http://sonarqube:9000/api/system/status" | grep -q '"status":"UP"'; do
  echo "[entrypoint] SonarQube not ready, waiting 5s..."
  sleep 5
done
echo "[entrypoint] SonarQube is UP"

SONAR_USER="${SONAR_USER:-admin}"
SONAR_PASSWORD="${SONAR_PASSWORD:-admin}"
TOKEN_NAME="scanner-$(date +%Y%m%d%H%M%S)"

echo "[entrypoint] Generating SonarQube API token: ${TOKEN_NAME}"
TOKEN_RESPONSE=$(curl -s -X POST "http://sonarqube:9000/api/user_tokens/generate" \
  -u "${SONAR_USER}:${SONAR_PASSWORD}" \
  -d "name=${TOKEN_NAME}" \
  -d "type=USER_TOKEN")

API_TOKEN=$(echo "$TOKEN_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null)

if [[ -z "$API_TOKEN" ]]; then
  echo "[entrypoint] ERROR: Failed to generate API token"
  echo "[entrypoint] Response: $TOKEN_RESPONSE"
  exit 1
fi
echo "[entrypoint] Token generated successfully"

echo "[entrypoint] Starting sonar-scanner..."
export SONAR_TOKEN="$API_TOKEN"
export SONAR_HOST_URL="${SONAR_HOST_URL:-http://sonarqube:9000}"

sonar-scanner \
  -Dsonar.token="$API_TOKEN" \
  -Dsonar.host.url="$SONAR_HOST_URL" \
  "$@"

SCAN_EXIT=$?
echo "[entrypoint] sonar-scanner exited with code: $SCAN_EXIT"

REPORT_HOST_DIR="/usr/src/scanner-report"
mkdir -p "$REPORT_HOST_DIR"
chmod 777 "$REPORT_HOST_DIR"

echo "[entrypoint] Parsing scan report..."
python3 /parse_report.py \
  --host "$SONAR_HOST_URL" \
  --token "$API_TOKEN" \
  --project data-agent \
  --output "$REPORT_HOST_DIR/sonar-issues.json"

echo "[entrypoint] Report written to $REPORT_HOST_DIR/sonar-issues.json"
echo "[entrypoint] Done. Scanner exit code: $SCAN_EXIT"

exit $SCAN_EXIT
