---
name: ui-e2e-local
description: "在本地机器跑 data-agent 前端 Playwright E2E 测试的统一方案（ssh 隧道 + 系统 Chrome + 关代理）。触发词：本地跑 E2E / 本地跑 UI 测试 / 本地 playwright / 隧道测前端 / ui e2e local / 验证前端功能。禁止再从头调研装浏览器、切 agent-browser、纠结隧道端口。"
agent_created: true
---

# UI E2E Local — 本地跑前端测试统一方案

> 目标：本地验证前端功能时，**一条命令跑完 Playwright E2E**，不再每次从头调研装 Chrome、切 agent-browser、纠结隧道端口、踩代理劫持。

## 一句话方案

用统一脚本 `tests/ui/scripts/test-ui-local.sh`：

```bash
export SSHPASS='<服务器 root 密码>'          # 仅存本地，禁止进 git / 本 skill
export SSH_HOST='<测试服务器地址>'
export E2E_USERNAME='<预置测试账号>'
export E2E_PASSWORD='<预置测试账号密码>'
./tests/ui/scripts/test-ui-local.sh i18n.spec.ts
./tests/ui/scripts/test-ui-local.sh audit.spec.ts --grep "审计"
```

脚本自动完成：复用/建立 ssh 隧道 → 连通性自检 → 注入统一 env → 跑 playwright。

## 四个统一决策（含历史踩坑根因，勿再改回）

| 维度 | 统一值 | 为什么（历史坑） |
|---|---|---|
| 浏览器 | 系统 Chrome `PW_CHANNEL=chrome` | playwright 自带 chromium 下载常失败（CDN 直连/代理都拉不动 chromium_headless_shell）；agent-browser / executablePath / 独立脚本 4 种方案历史轮换导致混乱 |
| 代理 | 关闭 `PW_NO_PROXY=1` | 本地 shell 被注入 `HTTP_PROXY/HTTPS_PROXY=127.0.0.1:57022`（WorkBuddy 沙箱 sandbox-cli）+ 系统 `127.0.0.1:7897`（Clash），浏览器继承后 API 请求被劫持 →「服务离线」+ 登录失败；curl 直连不受影响 |
| 隧道 | 单端口打 **nginx 80** | nginx 反代 `/`→frontend:3000、`/api/`→data-agent:8080、`/files/`→seaweedfs，一条隧道覆盖全部；历史曾误打前端 3000 → `/api/v1/health` 404 |
| 端口 | 固定 `18880` | 历史 18080/13000/8080/8888/3000 混乱，且 3000/8080 常与本机已有隧道冲突 |

## 为什么必须走 ssh 隧道

本地直连公网 IP 会被防火墙拦截，curl 拿到的是假的「Web Filter Block Override」拦截页，非真实应用。必须 `ssh -L <本地端口>:localhost:80` 到服务器 nginx。

## 统一环境变量（脚本已自动注入）

```
PW_CHANNEL=chrome                          # 复用系统 Chrome，不下载 playwright 自带浏览器
PW_NO_PROXY=1                              # 注入 --no-proxy-server，绕开沙箱/Clash 代理劫持
UI_BASE_URL=http://127.0.0.1:18880         # 前端同源（nginx）
API_BASE=http://127.0.0.1:18880/api/v1     # 测试内 request fixture 用，走 nginx /api/ 反代
E2E_USERNAME / E2E_PASSWORD                # 预置测试账号（必填 env，见「凭据约定」）
```

## 凭据约定（红线）

- **服务器 root 密码**：走 `SSHPASS` 环境变量（脚本用 `sshpass -e` 读取），**禁止**写进脚本 / 本 skill / git。
- **测试账号**（`E2E_USERNAME` / `E2E_PASSWORD`）：一次性账号，已预置在测试服务器 MongoDB（`users` 集合，bcrypt `$2a$10`）。账号名与密码仅存本地，**禁止**写进脚本 / 本 skill / git。
- 其他敏感凭据（admin 密码、PAT、token）一律不进 git；PAT 读 `.github-pat`（已 gitignore）。

## 预置测试用户（SPEC-084 后自注册已禁用）

SPEC-084 起 `/auth/register` 已移除（改邀请制），本地 E2E 依赖预置账号登录：

```bash
# 服务器上，重置预置测试用户（一次性账号，账号名/密码仅存本地）
docker exec -i data-agent-mongodb-1 mongosh data_agent --quiet <<'EOF'
db.users.updateOne(
  { username: "<预置测试账号>" },
  { $set: { password_hash: "<bcrypt $2a$10 哈希>", role: "admin", status: "enabled", password_changed: true, rbac_role_count: 0 } }
)
EOF
```

> 注：bcrypt 哈希可用 `htpasswd -bnBC 10 "" "<密码>"` 生成后把 `$2y$` 前缀改成 `$2a$`（Go bcrypt 原生支持 $2a$）。哈希本身非明文，但也不要进 git。

## 手动跑（不用脚本时）

```bash
cd tests/ui
SSHPASS='<密码>' sshpass -e ssh -o StrictHostKeyChecking=no -f -N -L 18880:localhost:80 root@<测试服务器地址>
PW_CHANNEL=chrome PW_NO_PROXY=1 UI_BASE_URL=http://127.0.0.1:18880 API_BASE=http://127.0.0.1:18880/api/v1 \
  npx playwright test i18n.spec.ts
```

## 已知坑与边界

1. **跑全部 spec 慎用**：本地 WorkBuddy 沙箱下 `npx playwright test`（不指定文件）会扫描整个 testDir，读到含硬编码凭据的旧 spec（auth/invite 等）会触发 fs-shim 敏感保护 →「No tests found」。本地只跑指定 spec 文件；全量回归交给 CI（docker 环境无此问题）。
2. **隧道复用**：脚本 `lsof` 检测 `18880` 已监听则复用，不重复建。若想强制重建，先 `kill` 掉旧 ssh 进程再跑。
3. **断言锚点避开 RBAC 过滤**：侧边栏 `nav-*` 项受 RBAC 权限控制，预置测试用户无 `sidebar:*` 权限时不渲染。断言优先用 `page-title` / `dashboard-stat-*` / `language-toggle` 等不受权限影响的 testid。
4. **CI 行为不变**：`PW_CHANNEL` / `PW_NO_PROXY` / `API_BASE` / `E2E_USERNAME` 均为可选 env，CI 不设时走 docker 网络默认（Playwright 内置 chromium + `data-agent:8080`），零影响。

## 相关文件

| 文件 | 作用 |
|---|---|
| `tests/ui/scripts/test-ui-local.sh` | 统一本地运行入口（进 git，无敏感信息） |
| `tests/ui/playwright.config.ts` | 已支持 `PW_CHANNEL` / `PW_NO_PROXY` / `UI_BASE_URL` |
| `tests/ui/i18n.spec.ts` | 参考实现：`API_BASE` / `E2E_USERNAME` / `E2E_PASSWORD` env 约定 |
| `.github/workflows/ui-tests.yml` | CI 全量回归（docker compose ui-test，勿在本地复刻） |
