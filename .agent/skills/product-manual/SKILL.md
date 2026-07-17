---
name: product-manual
description: "Product user manual creation and update workflow for DataAgent. This skill covers the end-to-end process: enabling CI screenshot generation in Playwright, downloading real UI test screenshots from GitHub Actions artifacts, selecting representative screenshots for each feature page, writing/updating the user-facing manual Markdown, generating a pixel-perfect PDF via Chrome headless, and syncing to the docs/ directory in the repository. Triggers include: write product manual, update user manual, generate manual PDF, refresh manual screenshots, 产品手册, 使用手册."
agent_created: true
---

# Product Manual Skill — data-agent

Create and maintain the DataAgent product user manual (`docs/DataAgent-产品使用手册.md` + `.pdf`), illustrated with **real CI UI test screenshots** (not prototypes/mockups).

## Prerequisites

- Working directory: the root of the data-agent repo
- GitHub token: `.github-pat` in repo root
- Proxy (mandatory for downloading artifacts): `export https_proxy=http://127.0.0.1:7897 http_proxy=http://127.0.0.1:7897`
- Chrome: `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome` (for PDF generation)
- Python `markdown` library: `python3 -c "import markdown"`
- `md-to-pdf` skill installed in `~/.workbuddy/skills/md-to-pdf/`
- `gh` CLI (for `gh run download`)

## Workflow Overview

```
                          ┌─ First time or CI config changed ─┐
                          │  playwright.config.ts              │
                          │  screenshot: { mode: 'on',         │
                          │    fullPage: true }                │
                          │  .github/workflows/ui-tests.yml    │
                          │  (Upload Screenshots step)         │
                          └──────────────┬────────────────────┘
                                         │
                                         ▼
                      git push → CI triggers → 212 PNG screenshots
                                         │
                                         ▼
                   ┌─ Step 1: Download screenshots artifact ─┐
                   │  gh run download <run-id> -n screenshots │
                   │  or: curl -L -H "Authorization: Bearer"  │
                   │  .../artifacts/<id>/zip                  │
                   └─────────────────┬───────────────────────┘
                                     │
                                     ▼
                   ┌─ Step 2: Select & rename screenshots ───┐
                   │  Pick 1 best PNG per feature page        │
                   │  Name: 01-login, 02-chat-workspace, etc. │
                   └─────────────────┬───────────────────────┘
                                     │
                                     ▼
                   ┌─ Step 3: Write/update manual Markdown ──┐
                   │  User perspective, no code/sensitive info│
                   │  Reference: ![title](manual-screenshots/ │
                   └─────────────────┬───────────────────────┘
                                     │
                                     ▼
                   ┌─ Step 4: Generate PDF ──────────────────┐
                   │  python3 scripts/md2pdf.py outputs/      │
                   │  --chrome "/Applications/Google Chrome"  │
                   └─────────────────┬───────────────────────┘
                                     │
                                     ▼
                   ┌─ Step 5: Sync to repo ──────────────────┐
                   │  cp manuals + screenshots → docs/        │
                   │  git add → commit → push                 │
                   └─────────────────────────────────────────┘
```

## Step 0: Enable CI Screenshot Generation (one-time setup)

The Playwright config must generate screenshots on **all** tests (not just failure).

**`tests/ui/playwright.config.ts`:**
```typescript
screenshot: { mode: 'on', fullPage: true },
```

> `fullPage: true` captures the entire scrollable page — essential for product screenshots.

**`.github/workflows/ui-tests.yml`** — add an artifact upload step:
```yaml
- name: Upload Screenshots
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: screenshots
    path: tests/ui/artifacts/test-results/**/*.png
    retention-days: 7
    if-no-files-found: warn
```

> Playwright auto-screenshots go to `outputDir` (configured as `artifacts/test-results`).

## Step 1: Download Screenshots from CI

After CI passes, get the latest run ID:

```bash
cd data-agent
PAT=$(cat .github-pat)
RUN_ID=$(curl -s -H "Authorization: Bearer $PAT" \
  "https://api.github.com/repos/luoxiaojun1992/data-agent/actions/runs?per_page=3" \
  | python3 -c "import json,sys; r=json.load(sys.stdin)['workflow_runs']; print(r[0]['id'])" 2>/dev/null)
```

Then download the artifact:

```bash
# MUST use proxy — without it download is ~100 KB/s and times out
export https_proxy=http://127.0.0.1:7897 http_proxy=http://127.0.0.1:7897
cd data-agent
PAT=$(cat .github-pat)
GH_TOKEN=$PAT gh auth login --with-token < <(echo "$PAT") 2>/dev/null
GH_TOKEN=$PAT gh run download $RUN_ID -n screenshots -D /path/to/outputs/screenshots-ci/
```

**Or via curl (no gh auth needed):**
```bash
export https_proxy=http://127.0.0.1:7897 http_proxy=http://127.0.0.1:7897
ARTIFACT_ID=8402483559  # from API
curl -L -H "Authorization: Bearer $PAT" --max-time 600 \
  -o screenshots-ci.zip \
  "https://api.github.com/repos/luoxiaojun1992/data-agent/actions/artifacts/$ARTIFACT_ID/zip"
unzip screenshots-ci.zip -d screenshots-ci/
```

The artifact typically contains ~212 PNG files organized by test spec directory.

## Step 2: Select Representative Screenshots

Each test spec directory contains screenshots named like:
```
chat-CHAT-—-Complete-UI-020-Chat-—-quick-prompt-chips-chromium/test-finished-1.png
```

Key selection criteria:
- **Prefer "page rendering" tests** over interaction/edge-case tests
- **Pick the cleanest screenshot** (no error toasts, no empty states if populated version exists)
- **One screenshot per feature page** for the core manual; add detail screenshots for rich features

### Manual screenshot mapping (27 total for v1.1)

| # | File | Source CI test | Feature page |
|---|------|---------------|-------------|
| 01 | login | auth-brand-elements-rendering | 登录页 |
| 02 | chat-workspace | chat-quick-prompt-chips | Chat 对话主页 |
| 03 | agent-workspace | agent-page-header-and-empty-state | Agent 任务列表 |
| 04 | task-detail | agent-task-detail-expand | 任务详情 |
| 05 | dashboard | dashboard-stats-cards | 仪表盘主页 |
| 06 | user-mgmt | user-用户管理页渲染 | 用户管理 |
| 07 | permission | role-权限管理页渲染 | 权限管理 |
| 08 | model-config | model-模型配置页渲染 | 模型配置 |
| 09 | task-mgmt | task-任务管理页渲染 | 管理员任务管理 |
| 10 | knowledge-base | kb-知识库管理页渲染 | 知识库 |
| 11 | audit-log | audit-审计日志页渲染 | 审计日志 |
| 12 | api-review | api-API转换审核页渲染 | API 转换审核 |
| 13 | chat-table | chat-data-table-rendering | Chat 数据表格 |
| 14 | chat-chart | chat-chart-rendering | Chat 图表内嵌 |
| 15 | chat-sql | chat-SQL-code-block-rendering | Chat SQL 代码块 |
| 16 | chat-session | chat-session-panel-opens | 会话历史面板 |
| 17 | agent-filters | agent-task-filters | Agent 状态筛选 |
| 18 | agent-create | agent-create-task-modal | 新建任务弹窗 |
| 19 | agent-download | agent-batch-download-ZIP | 批量下载产出物 |
| 20 | kb-upload | kb-上传单个文档 | 知识库上传 |
| 21 | kb-search | kb-搜索知识库文档 | 知识库搜索 |
| 22 | dashboard-token | dashboard-Token-consumption-chart | Token 消耗图表 |
| 23 | dashboard-roi | dashboard-Token-ROI-KPIs | AI Agent ROI |
| 24 | notif-bell | notif-铃铛图标与未读数 | 铃铛通知图标 |
| 25 | notif-list | notif-点击展开通知列表 | 通知下拉列表 |
| 26 | chat-enhance | prompt-点击增强按钮(有输入) | 增强提示词 |
| 27 | api-approve | api-批准API转换 | 批准 API |

### Shell snippet for batch copying

```bash
cd outputs/screenshots-ci && \
DEST=../manual-screenshots && \
cp auth-*brand-elements*/test-finished-1.png $DEST/01-login.png && \
cp chat-*quick-prompt*/test-finished-1.png $DEST/02-chat-workspace.png && \
# ... etc for all 27
```

## Step 3: Write the Manual

### Content principles

1. **User perspective only** — describe what users see and do, never how it's implemented
2. **No sensitive info** — no API keys, tokens, internal URLs, Vault mechanisms
3. **Only implemented features** — match the current SPEC completion status
4. **Illustrate with CI screenshots** — every major section should have a real screenshot
5. **Chinese primary language** — the product and users are Chinese-speaking

### Manual structure

```markdown
# DataAgent 企业数据分析平台 — 产品使用手册
## 目录
## 1. 产品概览 (what, who, navigation, roles)
## 2. 登录与界面导览
## 3. Chat 对话：即时数据查询
## 4. Agent 任务：批量分析
## 5. Hermes 探索：自由对话模式
## 6. 知识库：让 AI 引用你的资料
## 7. 文档
## 8. 仪表盘：数据看板
## 9. 管理后台（管理员专用）
## 10. 飞书机器人：IM 渠道使用
## 11. 常见问题 (FAQ by topic)
## 附录 A：术语表
## 附录 B：联系与支持
## 附录 C：截图来源说明
```

### Image paths in Markdown

All screenshots must use **relative paths** so the PDF generator can inline them:

```markdown
![登录页](manual-screenshots/01-login.png)
```

The `manual-screenshots/` directory must be co-located with the `.md` file.

## Step 4: Generate PDF

Use the `md-to-pdf` skill from `~/.workbuddy/skills/md-to-pdf/`:

```bash
python3 ~/.workbuddy/skills/md-to-pdf/scripts/md2pdf.py outputs/ \
  --chrome "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
```

> The script auto-discovers all `.md` files in the directory and converts each.
> Local images are automatically inlined as base64 — no external dependencies in the PDF.

**Workaround for exit code 137:** The Chrome process may get killed despite successful PDF output. Check the file exists and has reasonable size after conversion:
```bash
ls -lh outputs/DataAgent-产品使用手册.pdf
mdls -name kMDItemNumberOfPages outputs/DataAgent-产品使用手册.pdf
```

## Step 5: Sync to Repository

Copy outputs to the repo's `docs/` directory and push:

```bash
cp outputs/DataAgent-产品使用手册.md data-agent/docs/
cp outputs/DataAgent-产品使用手册.pdf data-agent/docs/
mkdir -p data-agent/docs/manual-screenshots
cp outputs/manual-screenshots/*.png data-agent/docs/manual-screenshots/
cd data-agent
git add docs/DataAgent-产品使用手册.* docs/manual-screenshots/
git commit -m "docs: update product manual with CI screenshots"
git push origin main
```

## Common Pitfalls

| Pitfall | Solution |
|---------|----------|
| Download times out (72MB artifact) | Must use proxy. Without proxy: ~100 KB/s, times out. With proxy: ~9 seconds |
| `gh run download` "not a git repository" | Must run from inside the data-agent repo directory |
| `unzip` garbled Chinese filenames | macOS unzip uses CP437→UTF-8, garbled but file contents OK |
| Playwright screenshot only JPEG | Auto-screenshots are always PNG. `quality` option not available; use `fullPage` for full capture |
| PDF generation exit code 137 | Chrome process killed by macOS after completion. Check output file exists and is valid |
| Prototype screenshots vs real UI | **Always verify with real CI screenshots.** Prototype mockups often diverge from actual implementation (v1.0 used prototypes and had wrong navigation structure) |

## After Manual Update

- The CI now produces fresh screenshots on every push to main
- Screenshots expire after 7 days (GitHub artifact retention)
- Manual PDF is regenerated from Markdown + inline images — no external image file dependencies in the final PDF
