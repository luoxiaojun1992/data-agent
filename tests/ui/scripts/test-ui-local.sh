#!/bin/bash
# =============================================================================
# DataAgent 本地 UI E2E 统一入口
#
# 背景：历史上本地跑 Playwright E2E 每次都从头调研（装 Chrome / 切 agent-browser /
#       隧道端口乱 / 代理劫持 / 打错端口导致 404），浪费大量时间。
#       本脚本统一固化，一条命令跑完，不再重复调研。
#
# 统一决策（根因详见 .agent/skills/ui-e2e-local/SKILL.md）：
#   1. 浏览器 = 系统 Chrome（PW_CHANNEL=chrome）：playwright 自带 chromium 下载常失败
#   2. 代理   = 关闭（PW_NO_PROXY=1）：本地被注入沙箱代理(57022) + Clash(7897)，劫持浏览器请求
#   3. 隧道   = 单端口打 nginx 80：nginx 反代 / →frontend、/api/ →backend、/files/ →seaweedfs，
#              一条隧道覆盖全部；历史曾误打前端 3000 导致 /api 404
#   4. 端口   = 固定 18880：历史 18080/13000/8080/8888/3000 混乱
#
# 用法：
#   ./tests/ui/scripts/test-ui-local.sh [playwright 参数...]
#   例：
#     ./tests/ui/scripts/test-ui-local.sh i18n.spec.ts
#     ./tests/ui/scripts/test-ui-local.sh audit.spec.ts --grep "审计"
#     ./tests/ui/scripts/test-ui-local.sh          # 跑全部 spec（本地 WorkBuddy 下慎用，见 SKILL.md）
#
# 凭据（⛔ 禁止硬编码进本脚本 / git / skill，一律走环境变量）：
#   SSHPASS      服务器 root 密码（sshpass -e 读取）——必填
#   SSH_HOST     测试服务器地址——必填
#   E2E_USERNAME 预置测试账号（SPEC-084 后自注册已禁用，需预置）——必填
#   E2E_PASSWORD 预置测试账号密码——必填
#   TUNNEL_PORT  本地隧道端口（默认 18880）
# =============================================================================
set -euo pipefail

SSH_HOST="${SSH_HOST:-}"
SSH_USER="${SSH_USER:-root}"
TUNNEL_PORT="${TUNNEL_PORT:-18880}"
E2E_USERNAME="${E2E_USERNAME:-}"
E2E_PASSWORD="${E2E_PASSWORD:-}"

log() { echo "[ui-e2e-local] $*"; }

# --- 1. 依赖检查 ---
command -v sshpass >/dev/null 2>&1 || { log "错误：缺少 sshpass（brew install hudochenkov/sshpass/sshpass）"; exit 1; }
command -v curl >/dev/null 2>&1 || { log "错误：缺少 curl"; exit 1; }
[ -n "${SSHPASS:-}" ] || { log "错误：未设置 SSHPASS（服务器 root 密码）"; exit 1; }
[ -n "${SSH_HOST:-}" ] || { log "错误：未设置 SSH_HOST（测试服务器地址）"; exit 1; }
[ -n "${E2E_USERNAME:-}" ] || { log "错误：未设置 E2E_USERNAME（预置测试账号）"; exit 1; }
[ -n "${E2E_PASSWORD:-}" ] || { log "错误：未设置 E2E_PASSWORD（预置测试账号密码）"; exit 1; }

# --- 2. 隧道（复用已有，避免反复建立） ---
if lsof -nP -iTCP:"$TUNNEL_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  log "隧道端口 $TUNNEL_PORT 已监听，复用现有隧道"
else
  log "建立隧道 $TUNNEL_PORT -> $SSH_HOST:80 (nginx)"
  # -F /dev/null + UserKnownHostsFile=/dev/null + PubkeyAuthentication=no：
  #   避免 ssh 读取 ~/.ssh（config/id_rsa/known_hosts），
  #   在 WorkBuddy 沙箱下会触发 file-read 拦截警告并导致退出码非零。
  sshpass -e ssh -F /dev/null \
    -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
    -o PubkeyAuthentication=no -o PreferredAuthentications=password \
    -o LogLevel=ERROR \
    -o ConnectTimeout=20 -o ExitOnForwardFailure=yes \
    -f -N -L "$TUNNEL_PORT":localhost:80 "$SSH_USER@$SSH_HOST"
  sleep 1
fi

# --- 3. 连通性自检（打 nginx 80 而非前端 3000，否则 /api 404） ---
code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "http://127.0.0.1:$TUNNEL_PORT/" 2>/dev/null || echo "000")
if [ "$code" != "200" ]; then
  log "错误：隧道连通性检查失败（HTTP $code），请确认服务器可达、SSHPASS 正确"
  exit 1
fi
log "隧道连通 OK：http://127.0.0.1:$TUNNEL_PORT (HTTP 200)"

# --- 4. 统一环境变量 ---
export PW_CHANNEL=chrome
export PW_NO_PROXY=1
export UI_BASE_URL="http://127.0.0.1:$TUNNEL_PORT"
export API_BASE="http://127.0.0.1:$TUNNEL_PORT/api/v1"
export E2E_USERNAME E2E_PASSWORD

# --- 5. 跑测试 ---
cd "$(cd "$(dirname "$0")/.." && pwd)"   # 进入 tests/ui
log "运行：npx playwright test $*  (UI_BASE_URL=$UI_BASE_URL)"
npx playwright test "$@"
